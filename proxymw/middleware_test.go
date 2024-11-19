package proxymw

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

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

		EnableObserver: true,
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

	serve, err := NewServeFromConfig(config, mock.ServeHTTP)
	require.NoError(t, err)

	serve.Init(ctx)

	c := serve.client
	observer := c.(*Observer)
	jitterer := observer.client.(*Jitterer)
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
	serve.Proxy().ServeHTTP(w, r)
	require.Equal(t, 1, serveCalls)
	require.Equal(t, *r.Clone(ctx), *r)

	rt, err := NewRoundTripperFromConfig(config, mock)
	require.NoError(t, err)

	rt.Init(ctx)

	rtc := rt.client
	observer = rtc.(*Observer)
	jitterer = observer.client.(*Jitterer)
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
			_, err := NewServeFromConfig(tt.cfg, nil)
			require.ErrorIs(t, err, tt.err)
		})
	}
}
