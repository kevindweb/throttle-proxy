package proxymw

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type ProxyClient interface {
	Init(context.Context)
	ServeHTTP(http.ResponseWriter, *http.Request) error
}

type Config struct {
	BackpressureConfig

	EnableJitter bool
	JitterDelay  time.Duration

	EnableObserver   bool
	ObserverRegistry *prometheus.Registry
}

func (c Config) Validate() error {
	if c.EnableBackpressure {
		if err := c.BackpressureConfig.Validate(); err != nil {
			return err
		}
	}

	if c.EnableJitter && c.JitterDelay == 0 {
		return ErrJitterDelayRequired
	}

	if c.EnableObserver && c.ObserverRegistry == nil {
		return ErrRegistryRequired
	}
	return nil
}

// NewFromConfig reads the middleware config to inject related proxies.
// Proxies are wrapped from last to first (when enabled) to
//
// 1. Wrap *http.Request into the ProxyClient interface
//
// 2. Collect metrics on the internal proxies
//
// 3. Wait for some jitter to spread requests
//
// 4. Apply backpressure using signals from a Prometheus/Thanos server
//
// 5. Unwrap into the next http.HandlerFunc (or a passthrough http.ReverseProxy)
func NewFromConfig(cfg Config, next http.HandlerFunc) (*Entry, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	var querier ProxyClient = &Exit{next}

	if cfg.EnableBackpressure {
		querier = NewBackpressure(
			querier,
			cfg.CongestionWindowMin,
			cfg.CongestionWindowMax,
			cfg.BackpressureQueries,
			cfg.BackpressureMonitoringURL,
		)
	}

	if cfg.EnableJitter {
		querier = NewJitterer(querier, cfg.JitterDelay)
	}

	if cfg.EnableObserver {
		querier = NewObserver(querier, cfg.ObserverRegistry)
	}

	return &Entry{querier}, nil
}

type Entry struct {
	client ProxyClient
}

func (e *Entry) Proxy() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := e.client.ServeHTTP(w, r)
		if err == nil {
			return
		}

		var blocked *RequestBlockedError
		if !errors.As(err, &blocked) {
			prometheusAPIError(w, fmt.Sprintf("request blocked: %v", err), http.StatusTooManyRequests)
		} else {
			prometheusAPIError(w, fmt.Sprintf("proxy error: %v", err), http.StatusInternalServerError)
		}
	})
}

func (e *Entry) Init(ctx context.Context) {
	e.client.Init(ctx)
}

type Exit struct {
	next http.HandlerFunc
}

func (e *Exit) Init(_ context.Context) {}

func (e *Exit) ServeHTTP(w http.ResponseWriter, r *http.Request) error {
	e.next.ServeHTTP(w, r)
	return nil
}

func prometheusAPIError(w http.ResponseWriter, errorMessage string, code int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)

	res := map[string]string{
		"status":    "error",
		"errorType": "prom-query-proxy",
		"error":     errorMessage,
	}

	if err := json.NewEncoder(w).Encode(res); err != nil {
		log.Printf("error: Failed to encode json: %v", err)
	}
}
