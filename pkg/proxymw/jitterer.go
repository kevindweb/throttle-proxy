package proxymw

import (
	"math/rand"
	"net/http"
	"time"
)

// Jitterer sleeps for a random amount of jitter before passing the request through.
type Jitterer struct {
	delay  time.Duration
	client ProxyClient
}

var _ ProxyClient = &Jitterer{}

func NewJitterer(querier ProxyClient, delay time.Duration) *Jitterer {
	return &Jitterer{
		delay:  delay,
		client: querier,
	}
}

func (j *Jitterer) ServeHTTP(w http.ResponseWriter, r *http.Request) error {
	j.sleep()
	return j.client.ServeHTTP(w, r)
}

func (j *Jitterer) sleep() {
	if j.delay == 0 {
		return
	}

	// nolint:gosec // rand not used for security purposes
	jitter := time.Duration(rand.Intn(int(j.delay.Nanoseconds())))
	time.Sleep(jitter)
}
