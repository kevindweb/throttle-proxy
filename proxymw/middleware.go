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

// ProxyClient defines the interface for middleware components
type ProxyClient interface {
	Init(context.Context)
	ServeHTTP(http.ResponseWriter, *http.Request) error
}

// Config holds all middleware configuration options
type Config struct {
	BackpressureConfig
	EnableJitter     bool
	JitterDelay      time.Duration
	EnableObserver   bool
	ObserverRegistry *prometheus.Registry
}

// APIErrorResponse represents the standard error response format
type APIErrorResponse struct {
	Status    string `json:"status"`
	ErrorType string `json:"errorType"`
	Error     string `json:"error"`
}

const (
	ErrorTypeProxyQuery = "query-proxy"
	ContentTypeJSON     = "application/json; charset=utf-8"
)

// Validate ensures all enabled features have proper configuration
func (c Config) Validate() error {
	var errs []error

	if c.EnableBackpressure {
		if err := c.BackpressureConfig.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("backpressure config: %w", err))
		}
	}

	if c.EnableJitter && c.JitterDelay == 0 {
		errs = append(errs, ErrJitterDelayRequired)
	}

	if c.EnableObserver && c.ObserverRegistry == nil {
		errs = append(errs, ErrRegistryRequired)
	}

	return errors.Join(errs...)
}

// Entry represents the entry point of the middleware chain
type Entry struct {
	client ProxyClient
}

// NewFromConfig constructs a middleware chain based on configuration.
// The middleware chain is constructed in the following order:
// 1. HTTP Request wrapping (Entry)
// 2. Metrics collection (Observer)
// 3. Request spreading (Jitter)
// 4. Load management (Backpressure)
// 5. Final handler (Exit)
func NewFromConfig(cfg Config, next http.HandlerFunc) (*Entry, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	var querier ProxyClient = &Exit{next: next}

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

	return &Entry{client: querier}, nil
}

// Proxy returns an http.Handler that processes requests through the middleware chain
func (e *Entry) Proxy() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := e.client.ServeHTTP(w, r); err != nil {
			e.handleError(w, err)
		}
	})
}

// handleError processes errors from the middleware chain and returns appropriate responses
func (e *Entry) handleError(w http.ResponseWriter, err error) {
	var blocked *RequestBlockedError
	if errors.As(err, &blocked) {
		writeAPIError(w, blocked.Error(), http.StatusTooManyRequests)
		return
	}
	writeAPIError(w, fmt.Sprintf("proxy error: %v", err), http.StatusInternalServerError)
}

// Init initializes the middleware chain
func (e *Entry) Init(ctx context.Context) {
	e.client.Init(ctx)
}

// Exit represents the final handler in the middleware chain
type Exit struct {
	next http.HandlerFunc
}

func (e *Exit) Init(_ context.Context) {}

func (e *Exit) ServeHTTP(w http.ResponseWriter, r *http.Request) error {
	e.next.ServeHTTP(w, r)
	return nil
}

// writeAPIError writes a standardized error response
func writeAPIError(w http.ResponseWriter, errorMessage string, code int) {
	w.Header().Set("Content-Type", ContentTypeJSON)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)

	response := APIErrorResponse{
		Status:    "error",
		ErrorType: ErrorTypeProxyQuery,
		Error:     errorMessage,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("error: Failed to encode error response: %v", err)
	}
}
