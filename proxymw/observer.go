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
	errCounter   = promauto.NewCounter(prometheus.CounterOpts{Name: "proxymw_error_count"})
	blockCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{Name: "proxymw_block_count"}, []string{"mw_type"},
	)
	reqCounter     = promauto.NewCounter(prometheus.CounterOpts{Name: "proxymw_request_count"})
	latencyCounter = promauto.NewCounter(prometheus.CounterOpts{Name: "proxymw_request_latency_ms"})
	activeGauge    = promauto.NewGauge(prometheus.GaugeOpts{Name: "proxymw_active_requests"})
)

// Observer emits metrics such as error rate and how often proxies are blocking requests.
// Each client that blocks requests should tag their errors with a client type to filter metrics.
type Observer struct {
	client ProxyClient

	errCounter     prometheus.Counter
	blockCounter   *prometheus.CounterVec
	reqCounter     prometheus.Counter
	latencyCounter prometheus.Counter
	activeGauge    prometheus.Gauge
}

var _ ProxyClient = &Observer{}

func NewObserver(client ProxyClient) *Observer {
	return &Observer{
		client: client,

		errCounter:     errCounter,
		blockCounter:   blockCounter,
		reqCounter:     reqCounter,
		latencyCounter: latencyCounter,
		activeGauge:    activeGauge,
	}
}

func (o *Observer) Init(ctx context.Context) {
	o.client.Init(ctx)
}

func (o *Observer) Next(rr Request) error {
	o.activeGauge.Inc()
	start := time.Now()
	err := o.executeNext(rr)
	if err != nil {
		var blocked *RequestBlockedError
		if errors.As(err, &blocked) {
			o.blockCounter.WithLabelValues(blocked.Type).Inc()
		} else {
			o.errCounter.Inc()
		}
	}

	o.reqCounter.Inc()
	o.latencyCounter.Add(float64(time.Since(start).Milliseconds()))
	o.activeGauge.Dec()
	return err
}

// executeNext runs next in a goroutine on the off chance Next hangs so we can still run cleanup
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
