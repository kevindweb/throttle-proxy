package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/kevindweb/throttle-proxy/proxymw"
)

func fullConfigRoundTripper(ctx context.Context) (*proxymw.RoundTripperEntry, error) {
	cfg, err := parseConfigFile(os.Getenv("CONFIG_FILE"))
	if err != nil {
		return nil, err
	}

	mw, err := proxymw.NewRoundTripperFromConfig(cfg, http.DefaultTransport)
	if err != nil {
		return nil, err
	}

	mw.Init(ctx)
	return mw, err
}

func parseConfigFile(configFile string) (proxymw.Config, error) {
	// nolint:gosec // accept configuration file as input
	file, err := os.Open(configFile)
	if err != nil {
		return proxymw.Config{}, fmt.Errorf("error opening config file: %v", err)
	}
	defer file.Close()

	var cfg proxymw.Config
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&cfg); err != nil {
		return proxymw.Config{}, fmt.Errorf("error decoding YAML: %v", err)
	}

	return cfg, nil
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

	resp, err := c.Do(request)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Status", resp.StatusCode)
}
