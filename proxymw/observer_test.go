package proxymw

import (
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

func TestObserverNextError(t *testing.T) {
	for _, tt := range []struct {
		name     string
		err      string
		observer *Observer
		check    func(*testing.T, *Observer)
	}{
		{
			name: "block error",
			observer: &Observer{
				errCounter: prometheus.NewCounter(prometheus.CounterOpts{Name: "block_test_error_count"}),
				blockCounter: prometheus.NewCounterVec(
					prometheus.CounterOpts{Name: "block_test_block_count"}, []string{"mw_type"},
				),
				reqCounter:     prometheus.NewCounter(prometheus.CounterOpts{Name: "block_test_request_count"}),
				latencyCounter: prometheus.NewCounter(prometheus.CounterOpts{Name: "block_test_request_latency_ms"}),
				activeCounter:  prometheus.NewGauge(prometheus.GaugeOpts{Name: "block_test_active_requests"}),
				client: &Mocker{
					NextFunc: func(_ Request) error {
						return ErrBackpressureBackoff
					},
				},
			},
			err: ErrBackpressureBackoff.Error(),
			check: func(t *testing.T, obs *Observer) {
				metric := obs.blockCounter.WithLabelValues(BackpressureProxyType)
				var metricWriter dto.Metric
				metric.Write(&metricWriter)
				value := metricWriter.Counter.GetValue()
				require.Equal(t, float64(1), value)
			},
		},
		{
			name: "normal error",
			observer: &Observer{
				errCounter: prometheus.NewCounter(prometheus.CounterOpts{Name: "normal_test_error_count"}),
				blockCounter: prometheus.NewCounterVec(
					prometheus.CounterOpts{Name: "normal_test_block_count"}, []string{"mw_type"},
				),
				reqCounter:     prometheus.NewCounter(prometheus.CounterOpts{Name: "normal_test_request_count"}),
				latencyCounter: prometheus.NewCounter(prometheus.CounterOpts{Name: "normal_test_request_latency_ms"}),
				activeCounter:  prometheus.NewGauge(prometheus.GaugeOpts{Name: "normal_test_active_requests"}),
				client: &Mocker{
					NextFunc: func(r Request) error {
						require.Equal(t, nil, r)
						return errors.New("fail")
					},
				},
			},
			err: "fail",
			check: func(t *testing.T, obs *Observer) {
				metric := obs.errCounter
				var metricWriter dto.Metric
				metric.Write(&metricWriter)
				value := metricWriter.Counter.GetValue()
				require.Equal(t, float64(1), value)
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.err, tt.observer.Next(nil).Error())
			tt.check(t, tt.observer)
		})
	}
}
