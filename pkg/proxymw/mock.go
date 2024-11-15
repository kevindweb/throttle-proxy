package proxymw

import (
	"net/http"
)

// Mocker simply mocks the main ThanosQuerier methods for unit testing
type Mocker struct {
	ServeHTTPFunc func(w http.ResponseWriter, r *http.Request) error
}

var _ ProxyClient = &Mocker{}

func (m *Mocker) ServeHTTP(w http.ResponseWriter, r *http.Request) error {
	return m.ServeHTTPFunc(w, r)
}
