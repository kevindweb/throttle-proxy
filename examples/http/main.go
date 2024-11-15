package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/efficientgo/core/merrors"
	"github.com/metalmatze/signal/server/signalhttp"
	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"

	"github.com/kevindweb/throttle-proxy/proxymw"
)

type routes struct {
	upstream *url.URL
	handler  http.Handler

	mux http.Handler

	logger *log.Logger
}

type options struct {
	registerer prometheus.Registerer
}

type Option interface {
	apply(*options)
}

type optionFunc func(*options)

func (f optionFunc) apply(o *options) {
	f(o)
}

// mux abstracts away the behavior we expect from the http.ServeMux type in this package.
type mux interface {
	http.Handler
	Handle(string, http.Handler)
}

// strictMux is a mux that wraps standard HTTP handler with safer handler that allows safe user provided handler registrations.
type strictMux struct {
	mux
	seen map[string]struct{}
}

func newStrictMux(m mux) *strictMux {
	return &strictMux{
		m,
		map[string]struct{}{},
	}

}

// Handle is like HTTP mux handle but it does not allow to register paths that are shared with previously registered paths.
// It also makes sure the trailing / is registered too.
func (s *strictMux) Handle(pattern string, handler http.Handler) error {
	sanitized := pattern
	for next := strings.TrimSuffix(sanitized, "/"); next != sanitized; sanitized = next {
	}

	if _, ok := s.seen[sanitized]; ok {
		return fmt.Errorf("pattern %q was already registered", sanitized)
	}

	for p := range s.seen {
		if strings.HasPrefix(sanitized+"/", p+"/") {
			return fmt.Errorf("pattern %q is registered, cannot register path %q that shares it", p, sanitized)
		}
	}

	s.mux.Handle(sanitized, handler)
	s.mux.Handle(sanitized+"/", handler)
	s.seen[sanitized] = struct{}{}

	return nil
}

// instrumentedMux wraps a mux and instruments it.
type instrumentedMux struct {
	mux
	i signalhttp.HandlerInstrumenter
}

func newInstrumentedMux(m mux, r prometheus.Registerer) *instrumentedMux {
	return &instrumentedMux{
		m,
		signalhttp.NewHandlerInstrumenter(r, []string{"handler"}),
	}
}

// Handle implements the mux interface.
func (i *instrumentedMux) Handle(pattern string, handler http.Handler) {
	i.mux.Handle(pattern, i.i.NewHandler(prometheus.Labels{"handler": pattern}, handler))
}

func NewRoutes(cfg proxymw.Config, upstream *url.URL) (*routes, error) {
	proxy := httputil.NewSingleHostReverseProxy(upstream)

	r := &routes{
		upstream: upstream,
		handler:  proxy,
		logger:   log.Default(),
	}

	reg := prometheus.NewRegistry()
	mux := newStrictMux(newInstrumentedMux(http.NewServeMux(), reg))

	mw, err := proxymw.NewFromConfig(cfg, r.passthrough)
	if err != nil {
		log.Fatalf("failed to create middleware from config: %v", err)
	}

	errs := merrors.New(
		mux.Handle("/api/v1/query", mw.Proxy()),
		mux.Handle("/api/v1/query_range", mw.Proxy()),
	)

	errs.Add(
		mux.Handle("/healthz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		})),
	)

	if err := errs.Err(); err != nil {
		return nil, err
	}

	r.mux = mux
	proxy.ErrorHandler = r.errorHandler
	proxy.ErrorLog = log.Default()

	return r, nil
}

func (r *routes) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mux.ServeHTTP(w, req)
}

func (r *routes) errorHandler(rw http.ResponseWriter, _ *http.Request, err error) {
	r.logger.Printf("http: proxy error: %v", err)
	rw.WriteHeader(http.StatusBadGateway)
}

func (r *routes) passthrough(w http.ResponseWriter, req *http.Request) {
	r.handler.ServeHTTP(w, req)
}

func main() {
	var (
		insecureListenAddress string
		upstream              string

		enableBackpressure        bool
		backpressureMonitoringURL string
		backpressureQueries       string
		congestionWindowMin       int
		congestionWindowMax       int

		enableJitter bool
		jitterDelay  time.Duration

		enableObserver bool
	)

	flagset := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	flagset.StringVar(&insecureListenAddress, "insecure-listen-address", "", "The address the prom-label-proxy HTTP server should listen on.")
	flagset.StringVar(&upstream, "upstream", "", "The upstream URL to proxy to.")
	flagset.BoolVar(&enableJitter, "enable-jitter", false, "Use the jitter middleware")
	flagset.DurationVar(&jitterDelay, "jitter-delay", time.Second, "Random jitter to apply when enabled")
	flagset.BoolVar(&enableBackpressure, "enable-backpressure", false, "Use the additive increase multiplicative decrease middleware using backpressure metrics")
	flagset.IntVar(&congestionWindowMin, "backpressure-min-window", 0, "Min concurrent queries to passthrough regardless of spikes in backpressure.")
	flagset.IntVar(&congestionWindowMax, "backpressure-max-window", 0, "Max concurrent queries to passthrough regardless of backpressure health.")
	flagset.StringVar(&backpressureMonitoringURL, "backpressure-monitoring-url", "", "The address on which to read backpressure metrics with PromQL queries.")
	flagset.StringVar(&backpressureQueries, "backpressure-queries", "", "Newline separated allow list of queries that signifiy increase in downstream failure. Will be used to reduce congestion window. "+
		"Queries should be in the form of `sum(rate(throughput[5m])) > 100tbps` where an empty result means no backpressure is occuring")
	flagset.BoolVar(&enableObserver, "enable-observer", false, "Collect middleware latency and error metrics")

	//nolint: errcheck // Parse() will exit on error.
	flagset.Parse(os.Args[1:])

	upstreamURL, err := url.Parse(upstream)
	if err != nil {
		log.Fatalf("Failed to build parse upstream URL: %v", err)
	}

	if upstreamURL.Scheme != "http" && upstreamURL.Scheme != "https" {
		log.Fatalf("Invalid scheme for upstream URL %q, only 'http' and 'https' are supported", upstream)
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	cfg := proxymw.Config{
		EnableBackpressure:        enableBackpressure,
		BackpressureMonitoringURL: backpressureMonitoringURL,
		BackpressureQueries:       strings.Split(backpressureQueries, "\n"),
		CongestionWindowMin:       congestionWindowMin,
		CongestionWindowMax:       congestionWindowMax,

		EnableJitter: enableJitter,
		JitterDelay:  jitterDelay,

		EnableObserver:   enableObserver,
		ObserverRegistry: reg,
	}

	var g run.Group

	{
		// Run the insecure HTTP server.
		routes, err := NewRoutes(cfg, upstreamURL)
		if err != nil {
			log.Fatalf("Failed to create proxymw Routes: %v", err)
		}

		mux := http.NewServeMux()
		mux.Handle("/", routes)

		l, err := net.Listen("tcp", insecureListenAddress)
		if err != nil {
			log.Fatalf("Failed to listen on insecure address: %v", err)
		}

		srv := &http.Server{Handler: mux}

		g.Add(func() error {
			log.Printf("Listening insecurely on %v", l.Addr())
			if err := srv.Serve(l); err != nil && err != http.ErrServerClosed {
				log.Printf("Server stopped with %v", err)
				return err
			}
			return nil
		}, func(error) {
			srv.Close()
		})
	}

	g.Add(run.SignalHandler(context.Background(), syscall.SIGINT, syscall.SIGTERM))

	if err := g.Run(); err != nil {
		if !errors.As(err, &run.SignalError{}) {
			log.Printf("Server stopped with %v", err)
			os.Exit(1)
		}
		log.Print("Caught signal; exiting gracefully...")
	}
}
