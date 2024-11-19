package proxymw

import (
	"context"
	"math/rand"
	"time"
)

// Jitterer sleeps for a random amount of jitter before passing the request through.
type Jitterer struct {
	delay  time.Duration
	client ProxyClient
}

var _ ProxyClient = &Jitterer{}

func NewJitterer(client ProxyClient, delay time.Duration) *Jitterer {
	return &Jitterer{
		delay:  delay,
		client: client,
	}
}

func (j *Jitterer) Init(ctx context.Context) {
	j.client.Init(ctx)
}

func (j *Jitterer) Next(rr Request) error {
	j.sleep()
	return j.client.Next(rr)
}

func (j *Jitterer) sleep() {
	if j.delay == 0 {
		return
	}

	// nolint:gosec // rand not used for security purposes
	jitter := time.Duration(rand.Intn(int(j.delay.Nanoseconds())))
	time.Sleep(jitter)
}
