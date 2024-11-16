package proxymw

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func TestMiddlewareOrder(t *testing.T) {
	ctx := context.Background()
	config := Config{
		BackpressureConfig: BackpressureConfig{
			EnableBackpressure:        true,
			BackpressureQueries:       []string{"sum(rate(throughput[5m]))"},
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

	entry.Proxy().ServeHTTP(nil, nil)
	require.Equal(t, 1, calls)
}
