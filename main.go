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
	"gopkg.in/yaml.v3"

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
	passthrough := Passthrough{proxy}

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
	mux.Handle("/federate", passthrough)
	mux.Handle("/api/v1/alerts", passthrough)
	mux.Handle("/api/v1/rules", passthrough)
	mux.Handle("/api/v1/series", passthrough)
	mux.Handle("/api/v1/query_exemplars", passthrough)
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

type Passthrough struct {
	proxy *httputil.ReverseProxy
}

func (p Passthrough) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	p.proxy.ServeHTTP(w, req)
}

type Config struct {
	// InsecureListenAddresss is the address the proxy HTTP server should listen on
	InsecureListenAddress string `yaml:"insecure_listen_addr"`
	// Upstream is the upstream URL to proxy to
	Upstream    string         `yaml:"upstream"`
	ProxyConfig proxymw.Config `yaml:"proxymw_config"`
}

func parseConfigFile() (Config, error) {
	configFile := ""
	flagset := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	flagset.StringVar(&configFile, "config-file", "", "Config file to initialize the proxy")

	if err := flagset.Parse(os.Args[1:]); err != nil {
		return Config{}, err
	}

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
	cfg, err := parseConfigFile()
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
	routes, err := NewRoutes(ctx, cfg.ProxyConfig, upstreamURL)
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
