// Package proxyhttp generates an http mux handler to servce requests using *httputil.ReverseProxy
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

type routes struct {
	upstream *url.URL
	handler  http.Handler

	mux http.Handler
}

func NewRoutes(ctx context.Context, cfg proxyutil.Config) (http.Handler, error) {
	upstream, err := parseUpstream(cfg.Upstream)
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(upstream)
	proxy.ErrorLog = log.Default()

	r := &routes{
		upstream: upstream,
		handler:  proxy,
	}

	mw, err := proxymw.NewServeFromConfig(cfg.ProxyConfig, r.passthrough)
	if err != nil {
		return nil, fmt.Errorf("failed to create middleware from config: %v", err)
	}

	mw.Init(ctx)

	mux := http.NewServeMux()
	mux.Handle("/healthz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]bool{"ok": true}); err != nil {
			log.Printf("error writing healthz endpoint: %v", err)
		}
	}))

	for _, path := range cfg.ProxyPaths {
		mux.Handle(path, mw.Proxy())
	}

	if len(cfg.PassthroughPaths) == 0 {
		mux.Handle("/", http.HandlerFunc(r.passthrough))
	} else {
		for _, path := range cfg.PassthroughPaths {
			mux.Handle(path, http.HandlerFunc(r.passthrough))
		}
	}

	r.mux = mux
	return r, nil
}

func parseUpstream(upstream string) (*url.URL, error) {
	upstreamURL, err := url.Parse(upstream)
	if err != nil {
		return nil, fmt.Errorf("failed to parse upstream URL: %v", err)
	}

	if upstreamURL.Scheme != "http" && upstreamURL.Scheme != "https" {
		return nil, fmt.Errorf(
			"invalid scheme for upstream URL %q, only 'http' and 'https' are supported",
			upstream,
		)
	}

	return upstreamURL, nil
}

func (r *routes) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mux.ServeHTTP(w, req)
}

func (r *routes) passthrough(w http.ResponseWriter, req *http.Request) {
	r.handler.ServeHTTP(w, req)
}
