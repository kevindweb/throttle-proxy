package proxymw

import (
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func parseURL(t *testing.T, u string) *url.URL {
	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatalf("failed to parse: %s", u)
	}
	return parsed
}

func TestQueryCost(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name     string
		request  Request
		wantCost int
		wantErr  bool
	}{
		{
			name: "nil request should throw error",
			request: &Mocker{
				RequestFunc: func() *http.Request {
					return nil
				},
			},
			wantErr: true,
		},
		{
			name: "nil URL should throw error",
			request: &Mocker{
				RequestFunc: func() *http.Request {
					return &http.Request{
						URL: nil,
					}
				},
			},
			wantErr: true,
		},
		{
			name: "invalid range GET step throws error",
			request: &Mocker{
				RequestFunc: func() *http.Request {
					return &http.Request{
						URL:    parseURL(t, "http://localhost/api/v1/query_range?query=sum&start=1&end=2&step=fifty"),
						Method: http.MethodGet,
					}
				},
			},
			wantErr: true,
		},
		{
			name: "unexpected api url",
			request: &Mocker{
				RequestFunc: func() *http.Request {
					return &http.Request{
						URL:    parseURL(t, "http://localhost/api/v1/unexpected"),
						Method: http.MethodGet,
					}
				},
			},
			wantErr: true,
		},
		{
			name: "invalid range POST end throws error",
			request: &Mocker{
				RequestFunc: func() *http.Request {
					return &http.Request{
						URL:    parseURL(t, "http://localhost/api/v1/query_range"),
						Method: http.MethodPost,
						Form: url.Values{
							"query": []string{"sum"},
							"start": []string{"1"},
							"end":   []string{"invalid"},
							"step":  []string{"53"},
						},
						Body: io.NopCloser(strings.NewReader("")),
					}
				},
			},
			wantErr: true,
		},
		{
			name: "handle nil body",
			request: &Mocker{
				RequestFunc: func() *http.Request {
					return &http.Request{
						URL:    parseURL(t, "http://localhost/api/v1/query_range"),
						Method: http.MethodPost,
						Form: url.Values{
							"query": []string{"sum"},
							"start": []string{"1"},
							"end":   []string{"invalid"},
							"step":  []string{"53"},
						},
					}
				},
			},
			wantErr: true,
		},
		{
			name: "formatted timestamp",
			request: &Mocker{
				RequestFunc: func() *http.Request {
					return &http.Request{
						URL:    parseURL(t, "http://localhost/api/v1/query_range"),
						Method: http.MethodPost,
						Form: url.Values{
							"query": []string{"sum"},
							"start": []string{"2024-07-16T12:47:00Z"},
							"end":   []string{"2024-07-16T12:48:00Z"},
							"step":  []string{"2"},
						},
						Body: io.NopCloser(strings.NewReader("")),
					}
				},
			},
			wantCost: ObjectStorageThreshold,
			wantErr:  false,
		},
		{
			name: "invalid range GET time throws error",
			request: &Mocker{
				RequestFunc: func() *http.Request {
					return &http.Request{
						URL:    parseURL(t, "http://localhost/api/v1/query?query=missing(parens&time=invalid"),
						Method: http.MethodGet,
					}
				},
			},
			wantErr: true,
		},
		{
			name: "invalid instant empty promql query",
			request: &Mocker{
				RequestFunc: func() *http.Request {
					return &http.Request{
						URL:    parseURL(t, "http://localhost/api/v1/query?query=&time=1"),
						Method: http.MethodGet,
					}
				},
			},
			wantErr: true,
		},
		{
			name: "10 hours ago is costly",
			request: &Mocker{
				RequestFunc: func() *http.Request {
					return &http.Request{
						URL:    parseURL(t, "http://localhost/api/v1/query_range"),
						Method: http.MethodPost,
						Form: url.Values{
							"query": []string{"sum(rate(errors[5m]))"},
							"start": []string{timeAgo(10 * time.Hour)},
							"end":   []string{timeAgo(0)},
							"step":  []string{"53"},
						},
						Body: io.NopCloser(strings.NewReader("")),
					}
				},
			},
			wantCost: ObjectStorageThreshold,
			wantErr:  false,
		},
		{
			name: "default range query fields",
			request: &Mocker{
				RequestFunc: func() *http.Request {
					return &http.Request{
						URL:    parseURL(t, "http://localhost/api/v1/query_range"),
						Method: http.MethodPost,
						Form: url.Values{
							"query": []string{"sum(rate(errors[5m]))"},
						},
						Body: io.NopCloser(strings.NewReader("")),
					}
				},
			},
			wantCost: 0,
			wantErr:  false,
		},
		{
			name: "default instant query timestamp to time.Now",
			request: &Mocker{
				RequestFunc: func() *http.Request {
					return &http.Request{
						URL:    parseURL(t, "http://localhost/api/v1/query"),
						Method: http.MethodPost,
						Form: url.Values{
							"query": []string{"rate(errors[7d])"},
							"time":  []string{""},
						},
						Body: io.NopCloser(strings.NewReader("")),
					}
				},
			},
			wantCost: ObjectStorageThreshold,
			wantErr:  false,
		},
		{
			name: "2 minutes ago is not costly",
			request: &Mocker{
				RequestFunc: func() *http.Request {
					return &http.Request{
						URL:    parseURL(t, "http://localhost/api/v1/query"),
						Method: http.MethodPost,
						Form: url.Values{
							"query": []string{"count(errors)"},
							"time":  []string{timeAgo(2 * time.Minute)},
						},
						Body: io.NopCloser(strings.NewReader("")),
					}
				},
			},
			wantCost: 0,
			wantErr:  false,
		},
		{
			name: "long 4h range lookback",
			request: &Mocker{
				RequestFunc: func() *http.Request {
					return &http.Request{
						URL:    parseURL(t, "http://localhost/api/v1/query"),
						Method: http.MethodPost,
						Form: url.Values{
							"query": []string{"sum(avg_over_time(errors[4h]))"},
							"time":  []string{timeAgo(2 * time.Minute)},
						},
						Body: io.NopCloser(strings.NewReader("")),
					}
				},
			},
			wantCost: ObjectStorageThreshold,
			wantErr:  false,
		},
		{
			name: "not quite beyond 2 hour lookback",
			request: &Mocker{
				RequestFunc: func() *http.Request {
					return &http.Request{
						URL:    parseURL(t, "http://localhost/api/v1/query"),
						Method: http.MethodPost,
						Form: url.Values{
							"query": []string{"sum(rate(errors[1h]))"},
							"time":  []string{timeAgo(30 * time.Minute)},
						},
						Body: io.NopCloser(strings.NewReader("")),
					}
				},
			},
			wantCost: 0,
			wantErr:  false,
		},
		{
			name: "just over range",
			request: &Mocker{
				RequestFunc: func() *http.Request {
					return &http.Request{
						URL:    parseURL(t, "http://localhost/api/v1/query"),
						Method: http.MethodPost,
						Form: url.Values{
							"query": []string{"max_over_time(sum(operations{operation=~\"get\"})[3m:30s])"},
							"time":  []string{timeAgo(time.Hour + 58*time.Minute)},
						},
						Body: io.NopCloser(strings.NewReader("")),
					}
				},
			},
			wantCost: ObjectStorageThreshold,
			wantErr:  false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotCost, err := QueryCost(tt.request)
			require.Equal(t, tt.wantErr, err != nil, err)
			require.Equal(t, tt.wantCost, gotCost)
		})
	}
}

func timeAgo(duration time.Duration) string {
	ago := time.Now().UTC().Add(-duration).Unix()
	return strconv.FormatInt(ago, 10)
}
