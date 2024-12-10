package proxymw

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func TestBackpressureRelease(t *testing.T) {
	belowMinWatermarkGauge := prometheus.NewGauge(
		prometheus.GaugeOpts{Name: "fake_wm_gauge_below_min"},
	)
	atAllowanceWatermarkGauge := prometheus.NewGauge(
		prometheus.GaugeOpts{Name: "fake_wm_gauge_at_allowance"},
	)
	belowAllowanceWatermarkGauge := prometheus.NewGauge(
		prometheus.GaugeOpts{Name: "fake_wm_gauge_below_allowance"},
	)
	for _, tt := range []struct {
		name   string
		bp     *Backpressure
		expect *Backpressure
	}{
		{
			name: "watermark below allowance",
			bp: &Backpressure{
				min:            10,
				watermark:      14,
				max:            100,
				allowance:      0.25,
				active:         1,
				watermarkGauge: belowAllowanceWatermarkGauge,
			},
			expect: &Backpressure{
				min:            10,
				watermark:      15,
				max:            100,
				allowance:      0.25,
				active:         0,
				watermarkGauge: belowAllowanceWatermarkGauge,
			},
		},
		{
			name: "watermark at allowance",
			bp: &Backpressure{
				min:            10,
				watermark:      100,
				max:            100,
				allowance:      0.99999999999,
				active:         0,
				watermarkGauge: atAllowanceWatermarkGauge,
			},
			expect: &Backpressure{
				min:            10,
				watermark:      99,
				max:            100,
				allowance:      0.99999999999,
				active:         0,
				watermarkGauge: atAllowanceWatermarkGauge,
			},
		},
		{
			name: "watermark below min",
			bp: &Backpressure{
				min:            10,
				watermark:      14,
				max:            100,
				allowance:      0.05,
				active:         9,
				watermarkGauge: belowMinWatermarkGauge,
			},
			expect: &Backpressure{
				min:            10,
				watermark:      10,
				max:            100,
				allowance:      0.05,
				active:         8,
				watermarkGauge: belowMinWatermarkGauge,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt.bp.release()
			require.Equal(t, tt.expect, tt.bp)
		})
	}
}

func TestMetricFired(t *testing.T) {
	u := "http://localhost:9090"
	for _, tt := range []struct {
		name  string
		err   error
		val   float64
		query string
		bp    *Backpressure
	}{
		{
			name:  "error response",
			err:   errors.New("backpressure query must return exactly one value: sum(throughput)"),
			query: "sum(throughput)",
			bp: &Backpressure{
				monitorURL: u,
				monitorClient: &http.Client{
					Transport: &Mocker{
						RoundTripFunc: func(r *http.Request) (*http.Response, error) {
							return &http.Response{
								Body: io.NopCloser(bytes.NewBufferString(
									`{
									  "status": "success",
									  "data": {
									    "resultType": "vector",
									    "result": [
									      {
									        "metric": {},
									        "value": [1731988543.752, "90"]
									      },
										  {
									        "metric": {},
									        "value": [1731988543.752, "95"]
									      }
									    ]
									  }
									}`)),
								StatusCode: http.StatusOK,
							}, nil
						},
					},
				},
			},
		},
		{
			name: "bad status code throws error",
			err:  fmt.Errorf("unexpected status code: %d", http.StatusBadGateway),
			bp: &Backpressure{
				monitorURL: u,
				monitorClient: &http.Client{
					Transport: &Mocker{
						RoundTripFunc: func(_ *http.Request) (*http.Response, error) {
							return &http.Response{
								StatusCode: http.StatusBadGateway,
							}, nil
						},
					},
				},
			},
		},
		{
			name:  "valid request and response",
			query: "sum(throughput)",
			val:   90,
			bp: &Backpressure{
				monitorURL: u,
				monitorClient: &http.Client{
					Transport: &Mocker{
						RoundTripFunc: func(r *http.Request) (*http.Response, error) {
							require.Equal(t, u+InstantQueryEndpoint+"?query=sum%28throughput%29", r.URL.String())
							return &http.Response{
								Body: io.NopCloser(bytes.NewBufferString(
									`{
									  "status": "success",
									  "data": {
									    "resultType": "vector",
									    "result": [
									      {
									        "metric": {},
									        "value": [1731988543.752, "90"]
									      }
									    ]
									  }
									}`)),
								StatusCode: http.StatusOK,
							}, nil
						},
					},
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			val, err := tt.bp.metricFired(context.Background(), tt.query)
			require.Equal(t, tt.err, err)
			require.Equal(t, tt.val, val)
		})
	}
}

func TestUpdateThrottle(t *testing.T) {
	testGauge := prometheus.NewGauge(
		prometheus.GaugeOpts{Name: "fake_gauge_sensitive_bp_query"},
	)
	for _, tt := range []struct {
		name   string
		bp     *Backpressure
		setup  func(*Backpressure)
		query  BackpressureQuery
		update float64
		expect *Backpressure
	}{
		{
			name: "new query over emergency",
			bp: &Backpressure{
				min:            10,
				watermark:      80,
				max:            100,
				allowance:      0.2,
				throttleFlags:  sync.Map{},
				watermarkGauge: testGauge,
				allowanceGauge: testGauge,
			},
			setup: func(b *Backpressure) {},
			query: BackpressureQuery{
				Query:              `sum(rate(http_requests))`,
				WarningThreshold:   10,
				EmergencyThreshold: 100,
				ThrottlingCurve:    DefaultThrottleCurve,
			},
			update: 1000,
			expect: &Backpressure{
				min:            10,
				watermark:      10,
				max:            100,
				allowance:      0,
				watermarkGauge: testGauge,
				allowanceGauge: testGauge,
			},
		},
		{
			name: "new query more sensitive than previous",
			bp: &Backpressure{
				min:            10,
				watermark:      80,
				max:            100,
				allowance:      0.2,
				throttleFlags:  sync.Map{},
				watermarkGauge: testGauge,
				allowanceGauge: testGauge,
			},
			setup: func(b *Backpressure) {
				b.throttleFlags.Store("previous", 0.8)
			},
			query: BackpressureQuery{
				Query:              `sum(rate(http_requests))`,
				WarningThreshold:   10,
				EmergencyThreshold: 100,
				ThrottlingCurve:    DefaultThrottleCurve,
			},
			update: 30,
			expect: &Backpressure{
				min:            10,
				watermark:      41,
				max:            100,
				allowance:      0.41111229050718745, // calculated from 1-e^(-c * loadFactor)
				watermarkGauge: testGauge,
				allowanceGauge: testGauge,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt.bp.updateThrottle(tt.query, tt.update)
			tt.bp.throttleFlags = sync.Map{}
			require.Equal(t, tt.expect, tt.bp)
		})
	}
}
