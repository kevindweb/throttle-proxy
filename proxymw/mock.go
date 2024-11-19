package proxymw

import (
	"context"
	"net/http"
)

// Mocker simply mocks the main interfaces for unit testing
type Mocker struct {
	ServeHTTPFunc func(w http.ResponseWriter, r *http.Request)
	RoundTripFunc func(r *http.Request) (*http.Response, error)
	NextFunc      func(Request) error
	InitFunc      func(context.Context)
}

var _ http.Handler = &Mocker{}
var _ http.RoundTripper = &Mocker{}
var _ ProxyClient = &Mocker{}

func (m *Mocker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.ServeHTTPFunc(w, r)
}

func (m *Mocker) RoundTrip(r *http.Request) (*http.Response, error) {
	return m.RoundTripFunc(r)
}

func (m *Mocker) Init(ctx context.Context) {
	m.InitFunc(ctx)
}

func (m *Mocker) Next(rr Request) error {
	return m.NextFunc(rr)
}
