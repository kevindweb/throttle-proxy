// Package proxyhttp provides an HTTP mux handler for serving requests using httputil.ReverseProxy
package proxyhttp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/kevindweb/throttle-proxy/proxymw"
	"github.com/kevindweb/throttle-proxy/proxyutil"
)

// routes holds the configuration and handlers for the proxy server
type routes struct {
	upstream *url.URL
	handler  http.Handler
	mux      http.Handler
}

// NewRoutes creates a new HTTP handler for proxying requests based on the provided configuration
func NewRoutes(ctx context.Context, cfg proxyutil.Config) (http.Handler, error) {
	upstream, err := parseUpstream(cfg.Upstream)
	if err != nil {
		return nil, fmt.Errorf("failed to parse upstream URL: %w", err)
	}

	if err := cfg.ProxyConfig.Validate(); err != nil {
		return nil, fmt.Errorf("failed to validate middleware config: %w", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(upstream)
	proxy.ErrorLog = log.Default()

	r := &routes{
		upstream: upstream,
		handler:  proxy,
	}

	mw := proxymw.NewServeFromConfig(cfg.ProxyConfig, r.passthrough)
	mw.Init(ctx)

	mux := http.NewServeMux()

	mux.Handle("/healthz", http.HandlerFunc(handleHealthCheck))

	for _, path := range cfg.ProxyPaths {
		mux.Handle(path, mw.Proxy())
	}

	registerPassthroughPaths(mux, cfg.PassthroughPaths, r.passthrough)

	r.mux = mux
	return r, nil
}

// handleHealthCheck responds to health check requests
func handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	if err := json.NewEncoder(w).Encode(map[string]bool{"ok": true}); err != nil {
		log.Printf("error writing healthz endpoint: %v", err)
	}
}

// registerPassthroughPaths configures routes that should bypass the proxy middleware
func registerPassthroughPaths(mux *http.ServeMux, paths []string, handler http.HandlerFunc) {
	if len(paths) == 0 {
		mux.Handle("/", handler)
		return
	}

	for _, path := range paths {
		mux.Handle(path, handler)
	}
}

// parseUpstream validates and parses the upstream URL
func parseUpstream(upstream string) (*url.URL, error) {
	upstreamURL, err := url.Parse(upstream)
	if err != nil {
		return nil, fmt.Errorf("failed to parse upstream URL: %w", err)
	}

	if upstreamURL.Scheme != "http" && upstreamURL.Scheme != "https" {
		return nil, fmt.Errorf(
			"invalid scheme for upstream URL %q, only 'http' and 'https' are supported",
			upstream,
		)
	}

	return upstreamURL, nil
}

// ServeHTTP implements the http.Handler interface
func (r *routes) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mux.ServeHTTP(w, req)
}

// passthrough forwards requests directly to the upstream server without middleware
func (r *routes) passthrough(w http.ResponseWriter, req *http.Request) {
	r.handler.ServeHTTP(w, req)
}
