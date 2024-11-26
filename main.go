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
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"gopkg.in/yaml.v3"

	"github.com/kevindweb/throttle-proxy/proxymw"
)

type routes struct {
	upstream *url.URL
	handler  http.Handler

	mux http.Handler

	logger *log.Logger
}

func NewRoutes(
	ctx context.Context, cfg proxymw.Config, passthroughs []string, upstream *url.URL,
) (*routes, error) {
	proxy := httputil.NewSingleHostReverseProxy(upstream)

	r := &routes{
		upstream: upstream,
		handler:  proxy,
		logger:   log.Default(),
	}

	mux := http.NewServeMux()

	mw, err := proxymw.NewServeFromConfig(cfg, r.passthrough)
	if err != nil {
		return nil, fmt.Errorf("failed to create middleware from config: %v", err)
	}

	mw.Init(ctx)

	mux.Handle("/api/v1/query", mw.Proxy())
	mux.Handle("/api/v1/query_range", mw.Proxy())
	mux.Handle("/federate", http.HandlerFunc(r.passthrough))
	mux.Handle("/api/v1/alerts", http.HandlerFunc(r.passthrough))
	mux.Handle("/api/v1/rules", http.HandlerFunc(r.passthrough))
	mux.Handle("/api/v1/series", http.HandlerFunc(r.passthrough))
	mux.Handle("/api/v1/query_exemplars", http.HandlerFunc(r.passthrough))
	mux.Handle("/healthz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}))

	// Register optional passthrough paths.
	for _, path := range passthroughs {
		mux.Handle(path, http.HandlerFunc(r.passthrough))
	}

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

type Config struct {
	// InsecureListenAddresss is the address the proxy HTTP server should listen on
	InsecureListenAddress string `yaml:"insecure_listen_addr"`
	// Upstream is the upstream URL to proxy to
	Upstream               string         `yaml:"upstream"`
	UnsafePassthroughPaths []string       `yaml:"unsafe_passthrough_paths"`
	ProxyConfig            proxymw.Config `yaml:"proxymw_config"`
}

type StringSlice []string

func (s *StringSlice) String() string {
	return strings.Join(*s, ",")
}

func (s *StringSlice) Set(value string) error {
	*s = append(*s, value)
	return nil
}

type Float64Slice []float64

func (f *Float64Slice) String() string {
	values := make([]string, len(*f))
	for i, v := range *f {
		values[i] = fmt.Sprintf("%g", v)
	}
	return strings.Join(values, ",")
}

func (f *Float64Slice) Set(value string) error {
	v, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return err
	}
	*f = append(*f, v)
	return nil
}

func parseConfigs() (Config, error) {
	var (
		insecureListenAddress           string
		upstream                        string
		unsafePassthroughPaths          string
		enableBackpressure              bool
		backpressureMonitoringURL       string
		backpressureQueries             StringSlice
		backpressureWarnThresholds      Float64Slice
		backpressureEmergencyThresholds Float64Slice
		congestionWindowMin             int
		congestionWindowMax             int
		enableJitter                    bool
		jitterDelay                     time.Duration
		enableObserver                  bool
		configFile                      string
	)

	flagset := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	flagset.StringVar(&configFile, "config-file", "", "Config file to initialize the proxy")
	flagset.StringVar(
		&insecureListenAddress, "insecure-listen-address", "",
		"The address the proxy HTTP server should listen on.",
	)
	flagset.StringVar(&upstream, "upstream", "", "The upstream URL to proxy to.")
	flagset.BoolVar(&enableJitter, "enable-jitter", false, "Use the jitter middleware")
	flagset.DurationVar(
		&jitterDelay, "jitter-delay", 0,
		"Random jitter to apply when enabled",
	)
	flagset.BoolVar(
		&enableBackpressure, "enable-bp", false,
		"Use the additive increase multiplicative decrease middleware using backpressure metrics",
	)
	flagset.IntVar(
		&congestionWindowMin, "bp-min-window", 0,
		"Min concurrent queries to passthrough regardless of spikes in backpressure.",
	)
	flagset.IntVar(
		&congestionWindowMax, "bp-max-window", 0,
		"Max concurrent queries to passthrough regardless of backpressure health.",
	)
	flagset.StringVar(
		&backpressureMonitoringURL, "bp-monitoring-url", "",
		"The address on which to read backpressure metrics with PromQL queries.",
	)
	flagset.Var(
		&backpressureQueries, "bp-query",
		"PromQL that signifies an increase in downstream failure",
	)
	flagset.Var(
		&backpressureWarnThresholds, "bp-warn",
		"Threshold that defines when the system should start backing off",
	)
	flagset.Var(
		&backpressureEmergencyThresholds, "bp-emergency",
		"Threshold that defines when the system should apply maximum throttling",
	)
	flagset.BoolVar(
		&enableObserver, "enable-observer", false,
		"Collect middleware latency and error metrics",
	)
	flagset.StringVar(&unsafePassthroughPaths, "unsafe-passthrough-paths", "",
		"Comma delimited allow list of exact HTTP paths that should be allowed to hit "+
			"the upstream URL without any enforcement.")
	if err := flagset.Parse(os.Args[1:]); err != nil {
		return Config{}, err
	}

	if configFile != "" {
		return parseConfigFile(configFile)
	}

	for _, path := range strings.Split(unsafePassthroughPaths, ",") {
		u, err := url.Parse(fmt.Sprintf("http://example.com%v", path))
		if err != nil {
			return Config{}, fmt.Errorf(
				"path %q is not a valid URI path, got %v", path, unsafePassthroughPaths,
			)
		}
		if u.Path != path {
			return Config{}, fmt.Errorf(
				"path %q is not a valid URI path, got %v", path, unsafePassthroughPaths,
			)
		}
		if u.Path == "" || u.Path == "/" {
			return Config{}, fmt.Errorf(
				"path %q is not allowed, got %v", u.Path, unsafePassthroughPaths,
			)
		}
	}

	n := len(backpressureQueries)
	queries := make([]proxymw.BackpressureQuery, n)
	if len(backpressureWarnThresholds) != n {
		return Config{}, fmt.Errorf("expected %d warn thresholds for %d backpressure queries", n, n)
	}
	for i, query := range backpressureQueries {
		queries[i] = proxymw.BackpressureQuery{
			Query:              query,
			WarningThreshold:   backpressureWarnThresholds[i],
			EmergencyThreshold: backpressureEmergencyThresholds[i],
		}
	}

	return Config{
		InsecureListenAddress:  insecureListenAddress,
		Upstream:               upstream,
		UnsafePassthroughPaths: strings.Split(unsafePassthroughPaths, ","),
		ProxyConfig: proxymw.Config{
			EnableJitter:   enableJitter,
			JitterDelay:    jitterDelay,
			EnableObserver: enableObserver,
			BackpressureConfig: proxymw.BackpressureConfig{
				EnableBackpressure:        enableBackpressure,
				BackpressureMonitoringURL: backpressureMonitoringURL,
				BackpressureQueries:       queries,
				CongestionWindowMin:       congestionWindowMin,
				CongestionWindowMax:       congestionWindowMax,
			},
		},
	}, nil
}

func parseConfigFile(configFile string) (Config, error) {
	// nolint:gosec // accept configuration file as input
	file, err := os.Open(configFile)
	if err != nil {
		return Config{}, fmt.Errorf("error opening config file: %v", err)
	}
	defer file.Close()

	var cfg Config
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("error decoding YAML: %v", err)
	}

	return cfg, nil
}

func main() {
	cfg, err := parseConfigs()
	if err != nil {
		log.Fatalf("Failed to parse config file: %v", err)
	}

	upstreamURL, err := url.Parse(cfg.Upstream)
	if err != nil {
		log.Fatalf("Failed to build parse upstream URL: %v", err)
	}

	if upstreamURL.Scheme != "http" && upstreamURL.Scheme != "https" {
		log.Fatalf("Invalid scheme for upstream URL %q, only 'http' and 'https' are supported", cfg.Upstream)
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	ctx, cancel := context.WithCancel(context.Background())
	routes, err := NewRoutes(ctx, cfg.ProxyConfig, cfg.UnsafePassthroughPaths, upstreamURL)
	if err != nil {
		log.Fatalf("Failed to create proxymw Routes: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", routes)

	l, err := net.Listen("tcp", cfg.InsecureListenAddress)
	if err != nil {
		log.Fatalf("Failed to listen on insecure address: %v", err)
	}

	srv := &http.Server{
		Handler:      mux,
		WriteTimeout: time.Second,
		ReadTimeout:  time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		fmt.Printf("Listening on %s\n", l.Addr().String())
		if err := srv.Serve(l); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Could not start server: %s\n", err)
		}
	}()

	<-stop
	cancel()
	fmt.Println("\nShutting down server...")

	if err := srv.Shutdown(ctx); err != nil {
		fmt.Printf("Server forced to shut down: %s\n", err)
	} else {
		fmt.Println("Server gracefully stopped")
	}
}
