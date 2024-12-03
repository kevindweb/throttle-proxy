// Package proxymw holds interfaces and configuration to safeguard backend services from dynamic load
package proxymw

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"
)

// ProxyClient defines the interface for middleware components
type ProxyClient interface {
	Init(context.Context)
	Next(Request) error
}

type Request interface {
	Request() *http.Request
}

type Response interface {
	Response() *http.Response
	SetResponse(*http.Response)
}

type ResponseWriter interface {
	ResponseWriter() http.ResponseWriter
}

type RequestResponseWrapper struct {
	req *http.Request
	res *http.Response
	w   http.ResponseWriter
}

func (c *RequestResponseWrapper) Request() *http.Request {
	return c.req
}

func (c *RequestResponseWrapper) Response() *http.Response {
	return c.res
}

func (c *RequestResponseWrapper) SetResponse(res *http.Response) {
	c.res = res
}

func (c *RequestResponseWrapper) ResponseWriter() http.ResponseWriter {
	return c.w
}

// Config holds all middleware configuration options
type Config struct {
	BackpressureConfig `yaml:"backpressure_config"`
	EnableJitter       bool          `yaml:"enable_jitter"`
	JitterDelay        time.Duration `yaml:"jitter_delay"`
	EnableObserver     bool          `yaml:"enable_observer"`
	EnableLatency      bool          `yaml:"enable_latency"`
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

	return errors.Join(errs...)
}

// ServeEntry represents the entry point of the middleware chain
type ServeEntry struct {
	client ProxyClient
}

// NewServeFromConfig constructs a middleware chain based on configuration.
// The middleware chain is constructed in the following order:
// 1. HTTP Request wrapping (Entry)
// 2. Metrics collection (Observer)
// 3. Request spreading (Jitter)
// 4. Adaptive rate limiting (Backpressure)
// 5. Throttling by request latency (LatencyTracker)
// 6. Final handler (Exit)
func NewServeFromConfig(cfg Config, next http.HandlerFunc) (*ServeEntry, error) {
	var client ProxyClient = &ServeExit{next: next}

	var err error
	client, err = NewFromConfig(cfg, client)
	if err != nil {
		return nil, err
	}

	return &ServeEntry{client: client}, nil
}

func NewFromConfig(cfg Config, client ProxyClient) (ProxyClient, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	if cfg.EnableLatency {
		client = NewLatencyTracker(client, cfg.CongestionWindowMin, cfg.CongestionWindowMax)
	}

	if cfg.EnableBackpressure {
		client = NewBackpressure(
			client,
			cfg.CongestionWindowMin,
			cfg.CongestionWindowMax,
			cfg.BackpressureQueries,
			cfg.BackpressureMonitoringURL,
		)
	}

	if cfg.EnableJitter {
		client = NewJitterer(client, cfg.JitterDelay)
	}

	if cfg.EnableObserver {
		client = NewObserver(client)
	}

	return client, nil
}

// Proxy returns an http.Handler that processes requests through the middleware chain
func (se *ServeEntry) Proxy() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rr := &RequestResponseWrapper{
			w:   w,
			req: r,
		}
		err := se.client.Next(rr)
		if err == nil {
			return
		}

		var blocked *RequestBlockedError
		if errors.As(err, &blocked) {
			writeAPIError(w, blocked.Error(), http.StatusTooManyRequests)
			return
		}
		writeAPIError(w, fmt.Sprintf("proxy error: %v", err), http.StatusInternalServerError)
	})
}

// Init initializes the middleware chain
func (se *ServeEntry) Init(ctx context.Context) {
	se.client.Init(ctx)
}

// ServeExit represents the final handler in the middleware chain for http.HandlerFunc
type ServeExit struct {
	next http.HandlerFunc
}

func (se *ServeExit) Init(_ context.Context) {}

func (se *ServeExit) Next(rr Request) error {
	rrw, ok := rr.(ResponseWriter)
	if !ok {
		return fmt.Errorf("request is of type %T not RequestResponseWriter", rr)
	}

	w := rrw.ResponseWriter()
	if w == nil {
		return ErrNilResponseWriter
	}

	r := rr.Request()
	if r == nil {
		return ErrNilRequest
	}

	se.next.ServeHTTP(w, r)
	return nil
}

type RoundTripperEntry struct {
	client ProxyClient
}

func NewRoundTripperFromConfig(cfg Config, rt http.RoundTripper) (*RoundTripperEntry, error) {
	var client ProxyClient = &RoundTripperExit{transport: rt}

	var err error
	client, err = NewFromConfig(cfg, client)
	if err != nil {
		return nil, err
	}

	return &RoundTripperEntry{client: client}, nil
}

func (rte *RoundTripperEntry) RoundTrip(req *http.Request) (*http.Response, error) {
	rr := &RequestResponseWrapper{
		req: req,
	}

	err := rte.client.Next(rr)
	if err != nil {
		return nil, err
	}

	res := rr.Response()
	if res == nil {
		return nil, ErrNilResponse
	}

	return res, nil
}

func (rte *RoundTripperEntry) Init(ctx context.Context) {
	rte.client.Init(ctx)
}

// RoundTripperExit represents the final handler in the middleware chain for http.RoundTripper
type RoundTripperExit struct {
	transport http.RoundTripper
}

func (rte *RoundTripperExit) Init(_ context.Context) {}

func (rte *RoundTripperExit) Next(r Request) error {
	rr, ok := r.(Response)
	if !ok {
		return fmt.Errorf("request is of type %T not RequestResponseWriter", rr)
	}

	req := r.Request()
	if req == nil {
		return ErrNilRequest
	}

	res, err := rte.transport.RoundTrip(req) // nolint:bodyclose // passthrough
	rr.SetResponse(res)
	return err
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
