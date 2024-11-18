package proxymw

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func TestMiddlewareOrder(t *testing.T) {
	ctx := context.Background()
	config := Config{
		BackpressureConfig: BackpressureConfig{
			EnableBackpressure: true,
			BackpressureQueries: []BackpressureQuery{
				{
					Query:              "sum(rate(throughput[5m]))",
					WarningThreshold:   80,
					EmergencyThreshold: 100,
					ThrottlingCurve:    4,
				},
			},
			BackpressureMonitoringURL: "https://thanos.io",
			CongestionWindowMin:       2,
			CongestionWindowMax:       100,
		},

		EnableJitter: true,
		JitterDelay:  time.Second,

		EnableObserver:   true,
		ObserverRegistry: prometheus.NewRegistry(),
	}

	calls := 0
	mock := &Mocker{
		ServeHTTPFunc: func(w http.ResponseWriter, r *http.Request) {
			calls++
		},
	}

	entry, err := NewFromConfig(config, mock.ServeHTTP)
	require.NoError(t, err)

	entry.Init(ctx)

	c := entry.client
	observer := c.(*Observer)
	jitterer := observer.client.(*Jitterer)
	backpressure := jitterer.client.(*Backpressure)
	exit := backpressure.client.(*Exit)
	require.NotNil(t, exit.next)

	u, err := url.Parse("https://thanos.io")
	require.NoError(t, err)
	r := &http.Request{
		Method: http.MethodPost,
		URL:    u,
	}
	r = r.WithContext(ctx)
	entry.Proxy().ServeHTTP(nil, r)
	require.Equal(t, 1, calls)
	require.Equal(t, *r.Clone(ctx), *r)
}

func TestConfig(t *testing.T) {
	for _, tt := range []struct {
		name string
		cfg  Config
		err  error
	}{
		{
			name: "no jitter delay",
			cfg: Config{
				EnableJitter: true,
				JitterDelay:  0,
			},
			err: ErrJitterDelayRequired,
		},
		{
			name: "no prom registry",
			cfg: Config{
				EnableObserver:   true,
				ObserverRegistry: nil,
			},
			err: ErrRegistryRequired,
		},
		{
			name: "no backpressure queries",
			cfg: Config{
				BackpressureConfig: BackpressureConfig{
					EnableBackpressure:  true,
					BackpressureQueries: []BackpressureQuery{},
				},
			},
			err: ErrBackpressureQueryRequired,
		},
		{
			name: "inverted congestion window",
			cfg: Config{
				BackpressureConfig: BackpressureConfig{
					EnableBackpressure: true,
					BackpressureQueries: []BackpressureQuery{
						{
							Query:              "up",
							WarningThreshold:   80,
							EmergencyThreshold: 100,
						},
					},
					BackpressureMonitoringURL: "https://thanos.io",
					CongestionWindowMin:       10,
					CongestionWindowMax:       5,
				},
			},
			err: ErrCongestionWindowMaxBelowMin,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewFromConfig(tt.cfg, nil)
			require.ErrorIs(t, err, tt.err)
		})
	}
}
