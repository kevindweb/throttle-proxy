package proxymw

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
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

		BlockerConfig: BlockerConfig{
			EnableBlocker: true,
			BlockPatterns: []string{"X-block=user"},
		},

		EnableJitter: true,
		JitterDelay:  time.Second,

		EnableObserver: true,
		ClientTimeout:  time.Hour,
	}

	serveCalls := 0
	rtCalls := 0
	mock := &Mocker{
		ServeHTTPFunc: func(_ http.ResponseWriter, _ *http.Request) {
			serveCalls++
		},
		RoundTripFunc: func(_ *http.Request) (*http.Response, error) {
			rtCalls++
			return &http.Response{
				Body: http.NoBody,
			}, nil
		},
	}

	serve := NewServeFromConfig(config, mock.ServeHTTP)
	serve.Init(ctx)

	c := serve.client
	observer := c.(*Observer)
	blocker := observer.client.(*Blocker)
	jitterer := blocker.client.(*Jitterer)
	backpressure := jitterer.client.(*Backpressure)
	exit := backpressure.client.(*ServeExit)
	require.NotNil(t, exit.next)

	u, err := url.Parse("https://thanos.io")
	require.NoError(t, err)
	r := &http.Request{
		Method: http.MethodPost,
		URL:    u,
		Header: http.Header{},
	}
	r = r.WithContext(ctx)

	w := &httptest.ResponseRecorder{}
	serve.ServeHTTP(w, r)
	require.Equal(t, 1, serveCalls)
	require.Equal(t, *r.Clone(ctx), *r)

	rt := NewRoundTripperFromConfig(config, mock)
	rt.Init(ctx)

	rtc := rt.client
	observer = rtc.(*Observer)
	blocker = observer.client.(*Blocker)
	jitterer = blocker.client.(*Jitterer)
	backpressure = jitterer.client.(*Backpressure)
	rtExit := backpressure.client.(*RoundTripperExit)
	require.NotNil(t, rtExit.transport)

	require.NoError(t, err)
	r = r.WithContext(ctx)

	res, err := rt.RoundTrip(r.Clone(ctx))
	require.NoError(t, err)
	require.NotNil(t, res)
	if res.Body != nil {
		res.Body.Close()
	}
	require.Equal(t, 1, rtCalls)
	require.Equal(t, *r.Clone(ctx), *r)
}

func TestHangingClient(t *testing.T) {
	ctx := context.Background()
	config := Config{
		EnableObserver: true,
		ClientTimeout:  time.Millisecond,
	}

	var wg sync.WaitGroup
	var wgInternal sync.WaitGroup
	var wgInternal2 sync.WaitGroup
	var wgInternal3 sync.WaitGroup
	mock := &Mocker{
		ServeHTTPFunc: func(_ http.ResponseWriter, r *http.Request) {
			wgInternal2.Done()
			wg.Wait()
			defer wgInternal.Done()

			testShouldTimeout := time.Hour
			timer := time.NewTimer(testShouldTimeout)
			defer timer.Stop()
			select {
			case <-r.Context().Done():
				return
			case <-timer.C:
				return
			}
		},
	}

	serve := NewServeFromConfig(config, mock.ServeHTTP)

	c := serve.client
	observer := c.(*Observer)
	activeRequests := prometheus.NewGauge(prometheus.GaugeOpts{Name: "hanging_requests"})
	observer.activeGauge = activeRequests

	serve.Init(ctx)
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://thanos.io", http.NoBody)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	wgInternal.Add(1)
	wgInternal2.Add(1)
	wgInternal3.Add(1)
	wg.Add(1)
	go func() {
		serve.ServeHTTP(w, r)
		wgInternal3.Done()
	}()

	var metricWriter dto.Metric
	wgInternal2.Wait()
	observer.activeGauge.Write(&metricWriter)
	require.Equal(t, float64(1), metricWriter.Gauge.GetValue())

	wg.Done()
	wgInternal.Wait()
	wgInternal3.Wait()
	observer.activeGauge.Write(&metricWriter)
	require.Equal(t, float64(0), metricWriter.Gauge.GetValue())
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
			name: "promQL wrapped in extraneous quotes",
			cfg: Config{
				BackpressureConfig: BackpressureConfig{
					EnableBackpressure: true,
					BackpressureQueries: []BackpressureQuery{
						{
							Query: "'sum(rate(http_requests))'",
						},
					},
				},
			},
			err: ErrExtraQueryQuotes,
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
			require.ErrorIs(t, tt.cfg.Validate(), tt.err)
		})
	}
}
