package proxymw

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

func TestObserverNextError(t *testing.T) {
	blockErrInitCalls := 0
	normalErrInitCalls := 0
	noErrInitCalls := 0
	for _, tt := range []struct {
		name     string
		err      string
		observer *Observer
		check    func(*testing.T, *Observer)
	}{
		{
			name: "block error",
			observer: &Observer{
				errCounter: prometheus.NewCounter(
					prometheus.CounterOpts{Name: "block_test_error_count"},
				),
				blockCounter: prometheus.NewCounterVec(
					prometheus.CounterOpts{Name: "block_test_block_count"}, []string{"mw_type"},
				),
				reqCounter: prometheus.NewCounter(
					prometheus.CounterOpts{Name: "block_test_request_count"},
				),
				latencyHist: prometheus.NewHistogram(
					prometheus.HistogramOpts{Name: "block_test_request_latency_ms"},
				),
				activeGauge: prometheus.NewGauge(
					prometheus.GaugeOpts{Name: "block_test_active_requests"},
				),
				client: &Mocker{
					NextFunc: func(_ Request) error {
						return ErrBackpressureBackoff
					},
					InitFunc: func(_ context.Context) {
						blockErrInitCalls++
					},
				},
			},
			err: ErrBackpressureBackoff.Error(),
			check: func(t *testing.T, obs *Observer) {
				require.Equal(t, 1, blockErrInitCalls)
				metric := obs.blockCounter.WithLabelValues(BackpressureProxyType)
				var metricWriter dto.Metric
				metric.Write(&metricWriter)
				value := metricWriter.Counter.GetValue()
				require.Equal(t, float64(1), value)
			},
		},
		{
			name: "next panic",
			observer: &Observer{
				errCounter: prometheus.NewCounter(
					prometheus.CounterOpts{Name: "block_test_error_count"},
				),
				blockCounter: prometheus.NewCounterVec(
					prometheus.CounterOpts{Name: "block_test_block_count"}, []string{"mw_type"},
				),
				reqCounter: prometheus.NewCounter(
					prometheus.CounterOpts{Name: "block_test_request_count"},
				),
				latencyHist: prometheus.NewHistogram(
					prometheus.HistogramOpts{Name: "block_test_request_latency_ms"},
				),
				activeGauge: prometheus.NewGauge(
					prometheus.GaugeOpts{Name: "block_test_active_requests"},
				),
				client: &Mocker{
					NextFunc: func(_ Request) error {
						panic("here")
					},
					InitFunc: func(_ context.Context) {},
				},
			},
			err:   "panic calling Next: here",
			check: func(_ *testing.T, _ *Observer) {},
		},
		{
			name: "normal error",
			observer: &Observer{
				errCounter: prometheus.NewCounter(
					prometheus.CounterOpts{Name: "normal_err_test_error_count"},
				),
				blockCounter: prometheus.NewCounterVec(
					prometheus.CounterOpts{Name: "normal_err_test_block_count"}, []string{"mw_type"},
				),
				reqCounter: prometheus.NewCounter(
					prometheus.CounterOpts{Name: "normal_err_test_request_count"},
				),
				latencyHist: prometheus.NewHistogram(
					prometheus.HistogramOpts{Name: "normal_err_test_request_latency_ms"},
				),
				activeGauge: prometheus.NewGauge(
					prometheus.GaugeOpts{Name: "normal_err_test_active_requests"},
				),
				client: &Mocker{
					NextFunc: func(r Request) error {
						return errors.New("fail")
					},
					InitFunc: func(_ context.Context) {
						normalErrInitCalls++
					},
				},
			},
			err: "fail",
			check: func(t *testing.T, obs *Observer) {
				require.Equal(t, 1, normalErrInitCalls)
				metric := obs.errCounter
				var metricWriter dto.Metric
				metric.Write(&metricWriter)
				value := metricWriter.Counter.GetValue()
				require.Equal(t, float64(1), value)
			},
		},
		{
			name: "no error",
			observer: &Observer{
				errCounter: prometheus.NewCounter(
					prometheus.CounterOpts{Name: "no_err_test_error_count"},
				),
				blockCounter: prometheus.NewCounterVec(
					prometheus.CounterOpts{Name: "no_err_test_block_count"}, []string{"mw_type"},
				),
				reqCounter: prometheus.NewCounter(
					prometheus.CounterOpts{Name: "no_err_test_request_count"},
				),
				latencyHist: prometheus.NewHistogram(
					prometheus.HistogramOpts{Name: "no_err_test_request_latency_ms"},
				),
				activeGauge: prometheus.NewGauge(
					prometheus.GaugeOpts{Name: "no_err_test_active_requests"},
				),
				client: &Mocker{
					NextFunc: func(r Request) error {
						return nil
					},
					InitFunc: func(_ context.Context) {
						noErrInitCalls++
					},
				},
			},
			err: "",
			check: func(t *testing.T, obs *Observer) {
				require.Equal(t, 1, noErrInitCalls)
				errCounter := obs.errCounter
				var errorWriter dto.Metric
				errCounter.Write(&errorWriter)
				errors := errorWriter.Counter.GetValue()
				require.Equal(t, float64(0), errors)

				blockCounter := obs.blockCounter.WithLabelValues(BackpressureProxyType)
				var blockWriter dto.Metric
				blockCounter.Write(&blockWriter)
				blocked := blockWriter.Counter.GetValue()
				require.Equal(t, float64(0), blocked)
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			tt.observer.Init(ctx)
			rr := &Mocker{
				RequestFunc: func() *http.Request {
					return (&http.Request{}).WithContext(ctx)
				},
			}
			err := tt.observer.Next(rr)
			errStr := ""
			if err != nil {
				errStr = err.Error()
			}
			require.Equal(t, tt.err, errStr)
			tt.check(t, tt.observer)
		})
	}
}
