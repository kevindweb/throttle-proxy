package proxymw

import (
	"net/http"
)

// Mocker simply mocks the main http.HandlerFunc methods for unit testing
type Mocker struct {
	ServeHTTPFunc func(w http.ResponseWriter, r *http.Request)
}

var _ http.HandlerFunc = (&Mocker{}).ServeHTTP

func (m *Mocker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.ServeHTTPFunc(w, r)
}
