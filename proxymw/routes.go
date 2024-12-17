package proxymw

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
)

type routes struct {
	upstream *url.URL
	handler  http.Handler

	mux http.Handler
}

func NewRoutes(
	ctx context.Context, cfg Config, proxyPaths, passthroughPaths []string, upstreamURL string,
) (http.Handler, error) {
	upstream, err := parseUpstream(upstreamURL)
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(upstream)
	proxy.ErrorLog = log.Default()

	r := &routes{
		upstream: upstream,
		handler:  proxy,
	}

	mw, err := NewServeFromConfig(cfg, r.passthrough)
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

	for _, path := range proxyPaths {
		mux.Handle(path, mw.Proxy())
	}

	if len(passthroughPaths) == 0 {
		mux.Handle("/", http.HandlerFunc(r.passthrough))
	} else {
		for _, path := range passthroughPaths {
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
