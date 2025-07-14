// Package proxymw holds interfaces and configuration to safeguard backend services from dynamic load
package proxymw

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// ProxyClient defines the interface for middleware components in the chain.
// Each middleware component must implement Init for setup and Next for request processing.
type ProxyClient interface {
	// Init initializes the middleware component with a context.
	// It should be called before the middleware starts processing requests.
	Init(context.Context)

	// Next processes the incoming request through the middleware chain.
	// It returns an error if the request cannot be processed.
	Next(Request) error
}

// Request represents an HTTP request in the middleware chain.
// It provides access to the underlying http.Request.
type Request interface {
	Request() *http.Request
}

// Response represents an HTTP response in the middleware chain.
// It provides access to and modification of the underlying http.Response.
type Response interface {
	Response() *http.Response
	SetResponse(*http.Response)
}

// ResponseWriter represents the HTTP response writer in the middleware chain.
// It provides access to the underlying http.ResponseWriter.
type ResponseWriter interface {
	ResponseWriter() http.ResponseWriter
}

var (
	_ Request           = &RequestResponseWrapper{}
	_ Response          = &RequestResponseWrapper{}
	_ ResponseWriter    = &RequestResponseWrapper{}
	_ ProxyClient       = &ServeExit{}
	_ ProxyClient       = &RoundTripperExit{}
	_ http.Handler      = &ServeEntry{}
	_ http.RoundTripper = &RoundTripperEntry{}
)

// RequestResponseWrapper implements Request, Response, and ResponseWriter interfaces
// to wrap HTTP request/response pairs as they flow through the middleware chain.
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
	BlockerConfig      `yaml:"blocker_config"`
	EnableJitter       bool          `yaml:"enable_jitter"`
	JitterDelay        time.Duration `yaml:"jitter_delay"`
	EnableObserver     bool          `yaml:"enable_observer"`
	ClientTimeout      time.Duration `yaml:"client_timeout"`
	EnableCriticality  bool          `yaml:"enable_criticality"`
}

// APIErrorResponse represents the standard error response format
type APIErrorResponse struct {
	Status    string `json:"status"`
	ErrorType string `json:"errorType"`
	Error     string `json:"error"`
}

// Validate ensures all enabled features have proper configuration
func (c Config) Validate() error {
	var errs []error

	if c.EnableBackpressure {
		if err := c.BackpressureConfig.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("backpressure config: %w", err))
		}
	}

	if c.EnableBlocker {
		if err := c.BlockerConfig.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("blocker config: %w", err))
		}
	}

	if c.EnableJitter && c.JitterDelay == 0 {
		errs = append(errs, ErrJitterDelayRequired)
	}

	return errors.Join(errs...)
}

// ServeEntry represents the entry point of the middleware chain
type ServeEntry struct {
	client  ProxyClient
	timeout time.Duration
}

// NewServeFromConfig constructs a middleware chain based on configuration.
// The middleware chain is constructed in the following order:
// 1. Request wrapping (Entry)
// 2. Metrics collection (Observer)
// 3. Request spreading (Jitter)
// 4. Adaptive rate limiting (Backpressure)
// 6. Final handler (Exit)
func NewServeFromConfig(cfg Config, next http.HandlerFunc) *ServeEntry {
	return &ServeEntry{
		client:  NewFromConfig(cfg, &ServeExit{next}),
		timeout: cfg.ClientTimeout,
	}
}

func NewServeFuncFromConfig(cfg Config, next http.HandlerFunc) http.HandlerFunc {
	return NewServeFromConfig(cfg, next).ServeHTTP
}

func NewFromConfig(cfg Config, client ProxyClient) ProxyClient {
	if cfg.EnableBackpressure {
		client = NewBackpressure(client, cfg.BackpressureConfig)
	}

	if cfg.EnableJitter {
		client = NewJitterer(client, cfg.JitterDelay, cfg.EnableCriticality)
	}

	if cfg.EnableBlocker {
		client = NewBlocker(client, cfg.BlockerConfig)
	}

	if cfg.EnableObserver {
		client = NewObserver(client)
	}

	return client
}

// ServeHTTP processes requests through the middleware chain
func (se *ServeEntry) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if se.timeout > 0 {
		var cancel func()
		ctx, cancel = context.WithTimeout(ctx, se.timeout)
		defer cancel()
	}

	rr := &RequestResponseWrapper{
		w:   w,
		req: r.WithContext(ctx),
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
		return fmt.Errorf("request is of type %T not ResponseWriter", rr)
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

func NewRoundTripperFromConfig(cfg Config, rt http.RoundTripper) *RoundTripperEntry {
	client := NewFromConfig(cfg, &RoundTripperExit{rt})
	return &RoundTripperEntry{client}
}

func (rte *RoundTripperEntry) RoundTrip(req *http.Request) (*http.Response, error) {
	rr := &RequestResponseWrapper{
		req: req,
	}

	if err := rte.client.Next(rr); err != nil {
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
		return fmt.Errorf("request is of type %T not Response", rr)
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
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)

	response := APIErrorResponse{
		Status:    "error",
		ErrorType: "throttle-proxy",
		Error:     errorMessage,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("error: Failed to encode error response: %v", err)
	}
}

func DupRequest(req *http.Request) (*http.Request, error) {
	clone := req.Clone(req.Context())
	if req.Body == nil {
		return clone, nil
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}

	req.Body = io.NopCloser(bytes.NewReader(body))
	clone.Body = io.NopCloser(bytes.NewBuffer(body))
	return clone, nil
}
