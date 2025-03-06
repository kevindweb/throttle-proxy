package proxymw

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	errCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "proxymw_error_count",
	})
	blockCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "proxymw_block_count",
		},
		[]string{"mw_type"},
	)
	reqCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "proxymw_request_count",
	})

	ms          = float64(time.Millisecond.Milliseconds())
	minute      = float64(time.Minute.Milliseconds())
	latencyHist = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "proxymw_request_latency_ms",
		Buckets: prometheus.ExponentialBucketsRange(ms, 10*minute, 12),
	})

	activeGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "proxymw_active_requests",
	})
)

// Observer wraps a ProxyClient to emit metrics such as error rate and blocked requests.
// Each client that blocks requests should tag their errors with a client type to filter metrics.
type Observer struct {
	client       ProxyClient
	errCounter   prometheus.Counter
	blockCounter *prometheus.CounterVec
	reqCounter   prometheus.Counter
	latencyHist  prometheus.Histogram
	activeGauge  prometheus.Gauge
}

var _ ProxyClient = &Observer{}

// NewObserver creates a new Observer wrapping the provided ProxyClient.
func NewObserver(client ProxyClient) *Observer {
	return &Observer{
		client:       client,
		errCounter:   errCounter,
		blockCounter: blockCounter,
		reqCounter:   reqCounter,
		latencyHist:  latencyHist,
		activeGauge:  activeGauge,
	}
}

// Init initializes the underlying ProxyClient.
func (o *Observer) Init(ctx context.Context) {
	o.client.Init(ctx)
}

// Next processes the request and records relevant metrics.
func (o *Observer) Next(rr Request) error {
	o.activeGauge.Inc()
	defer o.activeGauge.Dec()

	start := time.Now()
	err := o.executeNext(rr)

	o.reqCounter.Inc()
	o.latencyHist.Observe(float64(time.Since(start).Milliseconds()))

	if err != nil {
		var blocked *RequestBlockedError
		if errors.As(err, &blocked) {
			o.blockCounter.WithLabelValues(blocked.Type).Inc()
		} else {
			o.errCounter.Inc()
		}
	}

	return err
}

// executeNext runs the underlying client's Next method in a goroutine to handle potential hangs.
func (o *Observer) executeNext(rr Request) error {
	errc := make(chan error, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				errc <- fmt.Errorf("panic calling Next: %v", r)
			}
			close(errc)
		}()
		errc <- o.client.Next(rr)
	}()

	ctx := rr.Request().Context()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
