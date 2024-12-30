package proxyutil_test

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kevindweb/throttle-proxy/proxymw"
	"github.com/kevindweb/throttle-proxy/proxyutil"
)

func TestParseConfig(t *testing.T) {
	for _, tt := range []struct {
		name    string
		args    []string
		wantErr bool
		cfg     proxyutil.Config
	}{
		{
			name: "default config flags",
			args: []string{
				"test-program",
				"--upstream", "http://example.com",
				"--insecure-listen-address", ":8080",
			},
			wantErr: false,
			cfg: proxyutil.Config{
				Upstream:              "http://example.com",
				InsecureListenAddress: ":8080",
				ReadTimeout:           time.Minute * 5,
				WriteTimeout:          time.Minute * 5,
				ProxyPaths:            []string{},
				PassthroughPaths:      []string{},
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
				"--proxy-paths", "/api/v2",
				"--passthrough-paths", "/health,/metrics",
				"--proxy-read-timeout", "2m0s",
				"--proxy-write-timeout", "3m0s",
				"--enable-observer=true",
				"--enable-criticality=true",
				"--enable-jitter",
				"--jitter-delay", "100ms",
				"--enable-bp",
				"--bp-monitoring-url", "http://metrics.example.com",
				"--bp-query=sum(rate(http_request_count))",
				"--bp-query-name", "http_rps",
				"--bp-warn", "1000",
				"--bp-emergency", "5000",
				"--bp-query", "up{job='prometheus'} == 0",
				"--bp-query-name", "up_jobs",
				"--bp-warn", "0.5",
				"--bp-emergency", "0.8",
				"--bp-min-window", "10",
				"--bp-max-window", "100",
				"--enable-observer",
			},
			wantErr: false,
			cfg: proxyutil.Config{
				Upstream:              "http://example.com",
				ProxyPaths:            []string{"/api/v2"},
				PassthroughPaths:      []string{"/health", "/metrics"},
				InsecureListenAddress: ":8080",
				InternalListenAddress: ":9090",
				ReadTimeout:           2 * time.Minute,
				WriteTimeout:          3 * time.Minute,
				ProxyConfig: proxymw.Config{
					EnableCriticality: true,
					EnableJitter:      true,
					JitterDelay:       time.Millisecond * 100,
					EnableObserver:    true,
					BackpressureConfig: proxymw.BackpressureConfig{
						EnableBackpressure:        true,
						BackpressureMonitoringURL: "http://metrics.example.com",
						CongestionWindowMin:       10,
						CongestionWindowMax:       100,
						BackpressureQueries: []proxymw.BackpressureQuery{
							{
								Name:               "http_rps",
								Query:              "sum(rate(http_request_count))",
								WarningThreshold:   1000,
								EmergencyThreshold: 5000,
							},
							{
								Name:               "up_jobs",
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
			name: "empty passthrough path",
			args: []string{
				"test-program",
				"--upstream", "http://example.com",
				"--insecure-listen-address", ":8080",
				"--passthrough-paths", ",,,",
			},
			wantErr: true,
		},
		{
			name: "empty proxy path",
			args: []string{
				"test-program",
				"--upstream", "http://example.com",
				"--insecure-listen-address", ":8080",
				"--proxy-paths", ",,,",
			},
			wantErr: true,
		},
		{
			name: "invalid passthrough path",
			args: []string{
				"test-program",
				"--upstream", "http://example.com",
				"--insecure-listen-address", ":8080",
				"--passthrough-paths", "invalid path",
			},
			wantErr: true,
		},
		{
			name: "invalid query names",
			args: []string{
				"test-program",
				"--upstream", "http://example.com",
				"--insecure-listen-address", ":8080",
				"--passthrough-paths", "/api",
				"--enable-bp",
				"--bp-query", "up{job='prometheus'} == 0",
				"--bp-query-name", "instances_down",
				"--bp-emergency", "0.5",
				"--bp-warn", "0.7",
				"--bp-query", "up{job='prometheus'} == 1",
				"--bp-emergency", "0.5",
				"--bp-warn", "0.7",
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
				"--passthrough-paths", "/api",
				"--enable-bp",
				"--bp-query", "up{job='prometheus'} == 0",
				"--bp-emergency", "0.5",
			},
			wantErr: true,
		},
		{
			name: "simple config file",
			args: []string{
				"test-program",
				"--config-file", "testdata/simple.yaml",
			},
			cfg: proxyutil.Config{
				Upstream:              "http://localhost:9095",
				PassthroughPaths:      []string{"/api/v2"},
				InsecureListenAddress: "0.0.0.0:7777",
				InternalListenAddress: "0.0.0.0:7776",
				ReadTimeout:           5 * time.Second,
				WriteTimeout:          5 * time.Second,
				ProxyConfig: proxymw.Config{
					EnableJitter:   true,
					JitterDelay:    time.Second * 5,
					EnableObserver: true,
				},
			},
		},
		{
			name: "invalid config file",
			args: []string{
				"test-program",
				"--config-file", "testdata/invalid.yaml",
			},
			wantErr: true,
		},
		{
			name: "config file does not exist",
			args: []string{
				"test-program",
				"--config-file", "testdata/nonexistent.yaml",
			},
			wantErr: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			oldArgs := os.Args
			defer func() { os.Args = oldArgs }()
			os.Args = tt.args
			cfg, err := proxyutil.ParseConfigFlags()
			require.Equal(t, err != nil, tt.wantErr)
			require.Equal(t, cfg, tt.cfg)
		})
	}
}
