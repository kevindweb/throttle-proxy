package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/kevindweb/throttle-proxy/proxymw"
)

func FullConfigRoundTripper() (*proxymw.RoundTripperEntry, error) {
	u := os.Getenv("PROXY_URL")
	if u == "" {
		return nil, fmt.Errorf("empty PROXY_URL")
	}
	query := os.Getenv("PROXY_QUERY")
	if query == "" {
		return nil, fmt.Errorf("empty PROXY_QUERY")
	}
	warn, err := getEnvInt("PROXY_WARN")
	if err != nil {
		return nil, err
	}
	emer, err := getEnvInt("PROXY_EMERGENCY")
	if err != nil {
		return nil, err
	}
	cwndmin, err := getEnvInt("CWIND_MIN")
	if err != nil {
		return nil, err
	}
	cwndmax, err := getEnvInt("CWIND_MAX")
	if err != nil {
		return nil, err
	}

	delay := os.Getenv("JITTER_PROXY_DELAY")
	jitterDelay, err := time.ParseDuration(delay)
	if err != nil {
		return nil, err
	}

	cfg := proxymw.Config{
		BackpressureConfig: proxymw.BackpressureConfig{
			EnableBackpressure:        true,
			BackpressureMonitoringURL: u,
			BackpressureQueries: []proxymw.BackpressureQuery{
				{
					Query:              query,
					WarningThreshold:   float64(warn),
					EmergencyThreshold: float64(emer),
				},
			},
			CongestionWindowMin: cwndmin,
			CongestionWindowMax: cwndmax,
		},

		EnableJitter: true,
		JitterDelay:  jitterDelay,

		EnableObserver: true,
	}

	mw, err := proxymw.NewRoundTripperFromConfig(cfg, http.DefaultTransport)
	if err != nil {
		return nil, err
	}

	mw.Init(context.Background())
	return mw, err
}

func getEnvInt(s string) (int, error) {
	a := os.Getenv(s)
	i, err := strconv.Atoi(a)
	if err != nil {
		return 0, err
	}
	return i, nil
}

func main() {
	rt, err := FullConfigRoundTripper()
	if err != nil {
		log.Fatal(err)
	}

	c := &http.Client{
		Timeout:   time.Minute,
		Transport: rt,
	}

	ctx := context.Background()
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		"https://google.com",
		http.NoBody,
	)
	if err != nil {
		log.Fatal(fmt.Errorf("failed to make request: %w", err))
	}

	resp, err := c.Do(request)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Status", resp.StatusCode)
}
