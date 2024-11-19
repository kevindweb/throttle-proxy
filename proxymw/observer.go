package proxymw

import (
	"context"
	"errors"
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
	activeCounter  = promauto.NewGauge(prometheus.GaugeOpts{Name: "proxymw_active_requests"})
)

// Observer emits metrics such as error rate and how often proxies are blocking requests.
// Each client that blocks requests should tag their errors with a client type to filter metrics.
type Observer struct {
	client ProxyClient

	errCounter     prometheus.Counter
	blockCounter   *prometheus.CounterVec
	reqCounter     prometheus.Counter
	latencyCounter prometheus.Counter
	activeCounter  prometheus.Gauge
}

var _ ProxyClient = &Observer{}

func NewObserver(client ProxyClient) *Observer {
	o := &Observer{
		client: client,

		errCounter:     errCounter,
		blockCounter:   blockCounter,
		reqCounter:     reqCounter,
		latencyCounter: latencyCounter,
		activeCounter:  activeCounter,
	}

	return o
}

func (o *Observer) Init(ctx context.Context) {
	o.client.Init(ctx)
}

func (o *Observer) Next(rr Request) error {
	o.activeCounter.Inc()
	start := time.Now()
	err := o.client.Next(rr)
	o.handleMetrics(err, start)
	return err
}

func (o *Observer) handleMetrics(err error, start time.Time) {
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
	o.activeCounter.Dec()
}
