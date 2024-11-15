package proxymw

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	errCountMetric   = "querymw_error_count"
	blockCountMetric = "querymw_block_count"
	reqCountMetric   = "querymw_request_count"
	latencyMetric    = "querymw_request_latency_ms"
)

// Observer emits metrics such as error rate and how often queriers are blocking requests.
// Each querier that blocks requests should tag their errors with a querier type to filter metrics.
type Observer struct {
	now    func() time.Time
	since  func(time.Time) time.Duration
	client ProxyClient

	errCounter     prometheus.Counter
	blockCounter   *prometheus.CounterVec
	reqCounter     prometheus.Counter
	latencyCounter prometheus.Counter
}

var _ ProxyClient = &Observer{}

func NewObserver(querier ProxyClient, reg *prometheus.Registry) *Observer {
	o := &Observer{
		now:    time.Now,
		since:  time.Since,
		client: querier,

		errCounter:     prometheus.NewCounter(prometheus.CounterOpts{Name: errCountMetric}),
		blockCounter:   prometheus.NewCounterVec(prometheus.CounterOpts{Name: blockCountMetric}, []string{"mw_type"}),
		reqCounter:     prometheus.NewCounter(prometheus.CounterOpts{Name: reqCountMetric}),
		latencyCounter: prometheus.NewCounter(prometheus.CounterOpts{Name: latencyMetric}),
	}

	reg.MustRegister(o.errCounter, o.blockCounter, o.reqCounter, o.latencyCounter)
	return o
}

func (o *Observer) Init(ctx context.Context) {
	o.client.Init(ctx)
}

func (o *Observer) ServeHTTP(w http.ResponseWriter, r *http.Request) error {
	start := o.now()
	err := o.client.ServeHTTP(w, r)
	o.handleMetrics(err, start)
	return err
}

func (o *Observer) handleMetrics(err error, start time.Time) {
	if err != nil {
		var blocked *RequestBlockedError
		if !errors.As(err, &blocked) {
			o.blockCounter.WithLabelValues(blocked.Type).Inc()
		} else {
			o.errCounter.Inc()
		}
	}

	o.reqCounter.Inc()
	o.latencyCounter.Add(float64(o.since(start).Milliseconds()))
}
