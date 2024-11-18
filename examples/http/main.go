package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

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

func NewRoutes(ctx context.Context, cfg proxymw.Config, upstream *url.URL) (*routes, error) {
	proxy := httputil.NewSingleHostReverseProxy(upstream)

	r := &routes{
		upstream: upstream,
		handler:  proxy,
		logger:   log.Default(),
	}

	mux := http.NewServeMux()

	mw, err := proxymw.NewFromConfig(cfg, r.passthrough)
	if err != nil {
		log.Fatalf("failed to create middleware from config: %v", err)
	}

	mw.Init(ctx)

	mux.Handle("/api/v1/query", mw.Proxy())
	mux.Handle("/api/v1/query_range", mw.Proxy())
	mux.Handle("/healthz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}))

	r.mux = mux
	proxy.ErrorLog = log.Default()

	return r, nil
}

func (r *routes) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mux.ServeHTTP(w, req)
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
	flagset.StringVar(&insecureListenAddress, "insecure-listen-address", "", "The address the proxy HTTP server should listen on.")
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
		BackpressureConfig: proxymw.BackpressureConfig{
			EnableBackpressure:        enableBackpressure,
			BackpressureMonitoringURL: backpressureMonitoringURL,
			BackpressureQueries: []proxymw.BackpressureQuery{
				{
					Query:              "vector(82)",
					WarningThreshold:   80,
					EmergencyThreshold: 100,
				},
			},
			CongestionWindowMin: congestionWindowMin,
			CongestionWindowMax: congestionWindowMax,
		},

		EnableJitter: enableJitter,
		JitterDelay:  jitterDelay,

		EnableObserver:   enableObserver,
		ObserverRegistry: reg,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run the insecure HTTP server.
	routes, err := NewRoutes(ctx, cfg, upstreamURL)
	if err != nil {
		log.Fatalf("Failed to create proxymw Routes: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", routes)

	l, err := net.Listen("tcp", insecureListenAddress)
	if err != nil {
		log.Fatalf("Failed to listen on insecure address: %v", err)
	}

	srv := &http.Server{
		Handler:      mux,
		WriteTimeout: time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// Run the server in a goroutine
	go func() {
		if err := srv.Serve(l); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Could not start server: %s\n", err)
		}
	}()

	<-stop
	fmt.Println("\nShutting down server...")

	if err := srv.Shutdown(ctx); err != nil {
		fmt.Printf("Server forced to shut down: %s\n", err)
	} else {
		fmt.Println("Server gracefully stopped")
	}
}
