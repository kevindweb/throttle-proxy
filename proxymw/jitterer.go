package proxymw

import (
	"context"
	"math/rand"
	"time"
)

const (
	NoJitter time.Duration = 0
)

// Jitterer sleeps for a random amount of jitter before passing the request through.
// When EnableCriticality is set
//
// 1. CRITICAL_PLUS requests do not get jittered
//
// 2. Use max(X-Can-Wait, default) jitter if header is set
type Jitterer struct {
	delay       time.Duration
	client      ProxyClient
	criticality bool
}

var _ ProxyClient = &Jitterer{}

func NewJitterer(client ProxyClient, delay time.Duration, criticality bool) *Jitterer {
	return &Jitterer{
		delay:       delay,
		client:      client,
		criticality: criticality,
	}
}

func (j *Jitterer) Init(ctx context.Context) {
	j.client.Init(ctx)
}

func (j *Jitterer) Next(rr Request) error {
	delay, err := j.getDelay(rr)
	if err != nil {
		return err
	}

	j.sleep(rr, delay)
	return j.client.Next(rr)
}

func (j *Jitterer) sleep(rr Request, delay time.Duration) {
	if delay == 0 {
		return
	}

	// nolint:gosec // rand not used for security purposes
	jitter := time.Duration(rand.Intn(int(delay.Nanoseconds())))
	select {
	case <-rr.Request().Context().Done():
	case <-time.After(jitter):
	}
}

func (j *Jitterer) getDelay(rr Request) (time.Duration, error) {
	if j.criticality && ParseHeaderKey(rr, HeaderCriticality) == CriticalityCriticalPlus {
		// do not jitter if request is critical
		return NoJitter, nil
	}

	delay := j.delay
	canWait := ParseHeaderKey(rr, HeaderCanWait)
	if canWait == "" {
		return delay, nil
	}

	wait, err := time.ParseDuration(canWait)
	if err != nil {
		return 0, err
	}

	return max(wait, delay), nil
}
