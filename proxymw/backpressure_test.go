package proxymw

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBackpressureRelease(t *testing.T) {
	for _, tt := range []struct {
		name   string
		bp     *Backpressure
		expect *Backpressure
	}{
		{
			name: "watermark below allowance",
			bp: &Backpressure{
				min:       10,
				watermark: 14,
				max:       100,
				allowance: 0.25,
				active:    1,
			},
			expect: &Backpressure{
				min:       10,
				watermark: 15,
				max:       100,
				allowance: 0.25,
				active:    0,
			},
		},
		{
			name: "watermark below min",
			bp: &Backpressure{
				min:       10,
				watermark: 14,
				max:       100,
				allowance: 0.05,
				active:    9,
			},
			expect: &Backpressure{
				min:       10,
				watermark: 10,
				max:       100,
				allowance: 0.05,
				active:    8,
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
			err:   errors.New("query must return exactly one value: sum(throughput)"),
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
