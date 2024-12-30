package proxymw_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kevindweb/throttle-proxy/proxymw"
)

func TestMetricFired(t *testing.T) {
	u := "http://localhost:9090"
	for _, tt := range []struct {
		name     string
		err      error
		val      float64
		query    string
		endpoint string
		client   *http.Client
	}{
		{
			name:  "error response",
			err:   errors.New("backpressure query must return exactly one value: sum(throughput)"),
			query: "sum(throughput)",
			client: &http.Client{
				Transport: &proxymw.Mocker{
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
		{
			name: "negative float error",
			err: errors.New(
				"backpressure query (sum(throughput)) must have non-negative value: -90.000000",
			),
			query: "sum(throughput)",
			client: &http.Client{
				Transport: &proxymw.Mocker{
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
														"value": [1731988543.752, "-90"]
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
		{
			name:     "bad status code throws error",
			err:      fmt.Errorf("unexpected status code: %d", http.StatusBadGateway),
			endpoint: u,
			client: &http.Client{
				Transport: &proxymw.Mocker{
					RoundTripFunc: func(_ *http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: http.StatusBadGateway,
						}, nil
					},
				},
			},
		},
		{
			name:     "valid request and response",
			query:    "sum(throughput)",
			val:      90,
			endpoint: u,
			client: &http.Client{
				Transport: &proxymw.Mocker{
					RoundTripFunc: func(r *http.Request) (*http.Response, error) {
						url := u + proxymw.InstantQueryEndpoint + "?query=sum%28throughput%29"
						require.Equal(t, url, r.URL.String())
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
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			val, err := proxymw.ValueFromPromQL(ctx, tt.client, tt.endpoint, tt.query)
			require.Equal(t, tt.err, err)
			require.Equal(t, tt.val, val)
		})
	}
}
