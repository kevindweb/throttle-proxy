package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/kevindweb/throttle-proxy/proxymw"
	"github.com/kevindweb/throttle-proxy/proxyutil"
)

func fullConfigRoundTripper(ctx context.Context) (*proxymw.RoundTripperEntry, error) {
	cfg, err := proxyutil.ParseProxyConfigFile(os.Getenv("CONFIG_FILE"))
	if err != nil {
		return nil, err
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	mw := proxymw.NewRoundTripperFromConfig(cfg, http.DefaultTransport)
	mw.Init(ctx)
	return mw, err
}

func main() {
	ctx := context.Background()
	rt, err := fullConfigRoundTripper(ctx)
	if err != nil {
		log.Fatal(err)
	}

	c := &http.Client{
		Timeout:   time.Minute,
		Transport: rt,
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://x.com", http.NoBody)
	if err != nil {
		log.Fatal(fmt.Errorf("failed to make request: %w", err))
	}

	resp, err := c.Do(request) // nolint:bodyclose // ignore body close
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Status", resp.StatusCode)
}
