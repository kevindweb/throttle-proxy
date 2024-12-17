package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/syumai/workers"

	"github.com/kevindweb/throttle-proxy/proxyutil"
	"github.com/kevindweb/throttle-proxy/proxyutil/proxyhttp"
)

func main() {
	cfg, err := proxyutil.ParseConfigEnvironment()
	if err != nil {
		log.Fatalf("Failed to parse flags: %v", err)
	}

	ctx := context.Background()
	mux, err := setupServer(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}

	workers.Serve(mux)
}

func setupServer(ctx context.Context, cfg proxyutil.Config) (*http.ServeMux, error) {
	if cfg.ReadTimeout != "" {
		readTimeout, err := time.ParseDuration(cfg.ReadTimeout)
		if err != nil {
			return nil, fmt.Errorf("error parsing read timeout: %v", err)
		}

		if cfg.ProxyConfig.ClientTimeout == 0 {
			cfg.ProxyConfig.ClientTimeout = readTimeout
		}
	}

	routes, err := proxyhttp.NewRoutes(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create proxymw Routes: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", routes)
	return mux, nil
}
