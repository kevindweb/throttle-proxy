package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kevindweb/throttle-proxy/proxymw"
)

func TestNewRoutes(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("upstream response"))
	}))
	defer upstream.Close()

	upstreamURL, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("Failed to parse upstream URL: %v", err)
	}

	cfg := proxymw.Config{
		EnableJitter: false,
	}

	ctx := context.Background()
	routes, err := NewRoutes(ctx, cfg, []string{"/test-passthrough"}, upstreamURL)
	if err != nil {
		t.Fatalf("Failed to create routes: %v", err)
	}

	testServer := httptest.NewServer(routes)
	defer testServer.Close()

	testCases := []struct {
		name           string
		path           string
		expectedStatus int
	}{
		{
			name:           "Health Check",
			path:           "/healthz",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Passthrough Path",
			path:           "/test-passthrough",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Prometheus Query",
			path:           "/api/v1/query",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Query Range",
			path:           "/api/v1/query_range",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Not a passthrough",
			path:           "/non-passthrough",
			expectedStatus: http.StatusNotFound,
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			u := testServer.URL + tt.path
			ctx := context.Background()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
			require.NoError(t, err)
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)

			defer resp.Body.Close()
			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, resp.StatusCode)
			}
		})
	}
}

func TestInvalidJitterConfig(t *testing.T) {
	upstream, err := url.Parse("http://google.com")
	require.NoError(t, err)

	ctx := context.Background()
	cfg := proxymw.Config{
		EnableJitter: true,
		JitterDelay:  0,
	}

	routes, err := NewRoutes(ctx, cfg, []string{}, upstream)
	require.ErrorAs(t, err, &proxymw.ErrJitterDelayRequired)
	require.Nil(t, routes)
}

func TestParseConfig(t *testing.T) {
	for _, tt := range []struct {
		name    string
		args    []string
		wantErr bool
		cfg     Config
	}{
		{
			name: "default config flags",
			args: []string{
				"test-program",
				"--upstream", "http://example.com",
				"--insecure-listen-address", ":8080",
			},
			wantErr: false,
			cfg: Config{
				Upstream:               "http://example.com",
				InsecureListenAddress:  ":8080",
				ReadTimeout:            (time.Minute * 5).String(),
				WriteTimeout:           (time.Minute * 5).String(),
				UnsafePassthroughPaths: []string{},
				ProxyConfig: proxymw.Config{
					EnableJitter:   false,
					EnableObserver: false,
					BackpressureConfig: proxymw.BackpressureConfig{
						EnableBackpressure:  false,
						BackpressureQueries: []proxymw.BackpressureQuery{},
					},
				},
			},
		},
		{
			name: "comprehensive config flags",
			args: []string{
				"test-program",
				"--upstream", "http://example.com",
				"--insecure-listen-address", ":8080",
				"--internal-listen-address", ":9090",
				"--unsafe-passthrough-paths", "/health,/metrics",
				"--proxy-read-timeout", "2m",
				"--proxy-write-timeout", "3m",
				"--enable-observer=true",
				"--enable-jitter",
				"--jitter-delay", "100ms",
				"--enable-bp",
				"--bp-monitoring-url", "http://metrics.example.com",
				"--bp-query", "up{job='prometheus'} == 0",
				"--bp-warn", "0.5",
				"--bp-emergency", "0.8",
				"--bp-min-window", "10",
				"--bp-max-window", "100",
				"--enable-observer",
			},
			wantErr: false,
			cfg: Config{
				Upstream:               "http://example.com",
				UnsafePassthroughPaths: []string{"/health", "/metrics"},
				InsecureListenAddress:  ":8080",
				InternalListenAddress:  ":9090",
				ReadTimeout:            "2m",
				WriteTimeout:           "3m",
				ProxyConfig: proxymw.Config{
					EnableJitter:   true,
					JitterDelay:    time.Millisecond * 100,
					EnableObserver: true,
					BackpressureConfig: proxymw.BackpressureConfig{
						EnableBackpressure:        true,
						BackpressureMonitoringURL: "http://metrics.example.com",
						CongestionWindowMin:       10,
						CongestionWindowMax:       100,
						BackpressureQueries: []proxymw.BackpressureQuery{
							{
								Query:              "up{job='prometheus'} == 0",
								WarningThreshold:   0.5,
								EmergencyThreshold: 0.8,
							},
						},
					},
				},
			},
		},
		{
			name: "bad arguments fail parsing",
			args: []string{
				"test-program",
				"--enable-observer   true",
			},
			wantErr: true,
		},
		{
			name: "empty passthrough path",
			args: []string{
				"test-program",
				"--upstream", "http://example.com",
				"--insecure-listen-address", ":8080",
				"--unsafe-passthrough-paths", ",,,",
			},
			wantErr: true,
		},
		{
			name: "invalid passthrough path",
			args: []string{
				"test-program",
				"--upstream", "http://example.com",
				"--insecure-listen-address", ":8080",
				"--unsafe-passthrough-paths", "invalid path",
			},
			wantErr: true,
		},
		{
			name: "missing emergency threshold",
			args: []string{
				"test-program",
				"--upstream", "http://example.com",
				"--insecure-listen-address", ":8080",
				"--enable-bp",
				"--bp-query", "up{job='prometheus'} == 0",
				"--bp-warn", "0.5",
			},
			wantErr: true,
		},
		{
			name: "missing warn threshold",
			args: []string{
				"test-program",
				"--upstream", "http://example.com",
				"--insecure-listen-address", ":8080",
				"--unsafe-passthrough-paths", "/api",
				"--enable-bp",
				"--bp-query", "up{job='prometheus'} == 0",
				"--bp-emergency", "0.5",
			},
			wantErr: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			oldArgs := os.Args
			defer func() { os.Args = oldArgs }()
			os.Args = tt.args
			cfg, err := parseConfigs()
			require.Equal(t, err != nil, tt.wantErr)
			require.Equal(t, cfg, tt.cfg)
		})
	}
}
