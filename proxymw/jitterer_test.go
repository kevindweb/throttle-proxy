package proxymw

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestJitterGetDelay(t *testing.T) {
	_, parseErr := time.ParseDuration("invalid")
	t.Parallel()
	for _, tt := range []struct {
		name      string
		req       Request
		jitter    *Jitterer
		wantDelay time.Duration
		wantErr   error
	}{
		{
			name: "criticality not enabled ignore headers",
			jitter: &Jitterer{
				criticality: false,
				delay:       time.Second,
			},
			req: &Mocker{
				RequestFunc: func() *http.Request {
					return &http.Request{
						Header: http.Header{
							string(HeaderCriticality): []string{CriticalityCriticalPlus},
						},
					}
				},
			},
			wantDelay: time.Second,
		},
		{
			name: "critical plus gets no jitter",
			jitter: &Jitterer{
				criticality: true,
				delay:       time.Second,
			},
			req: &Mocker{
				RequestFunc: func() *http.Request {
					return &http.Request{
						Header: http.Header{
							string(HeaderCriticality): []string{CriticalityCriticalPlus},
						},
					}
				},
			},
			wantDelay: NoJitter,
		},
		{
			name: "can wait is less than min configured jitter",
			jitter: &Jitterer{
				criticality: true,
				delay:       time.Second,
			},
			req: &Mocker{
				RequestFunc: func() *http.Request {
					return &http.Request{
						Header: http.Header{
							string(HeaderCriticality): []string{CriticalityCritical},
							string(HeaderCanWait):     []string{"1ms"},
						},
					}
				},
			},
			wantDelay: time.Second,
		},
		{
			name: "can wait is more than min configured jitter",
			jitter: &Jitterer{
				criticality: true,
				delay:       time.Second,
			},
			req: &Mocker{
				RequestFunc: func() *http.Request {
					return &http.Request{
						Header: http.Header{
							string(HeaderCanWait): []string{"2m"},
						},
					}
				},
			},
			wantDelay: time.Minute * 2,
		},
		{
			name: "invalid can wait",
			jitter: &Jitterer{
				criticality: true,
				delay:       time.Second,
			},
			req: &Mocker{
				RequestFunc: func() *http.Request {
					return &http.Request{
						Header: http.Header{
							string(HeaderCanWait): []string{"invalid"},
						},
					}
				},
			},
			wantErr: parseErr,
		},
		{
			name: "no can wait set",
			jitter: &Jitterer{
				criticality: true,
				delay:       time.Second,
			},
			req: &Mocker{
				RequestFunc: func() *http.Request {
					return &http.Request{}
				},
			},
			wantDelay: time.Second,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			delay, err := tt.jitter.getDelay(tt.req)
			require.Equal(t, tt.wantDelay, delay)
			require.Equal(t, tt.wantErr, err)
		})
	}
}

func TestJitterSleep(t *testing.T) {
	longCtx, longCancel := context.WithTimeout(context.Background(), time.Hour)
	shortCtx, shortCancel := context.WithTimeout(context.Background(), time.Millisecond)

	t.Parallel()
	for _, tt := range []struct {
		name    string
		req     Request
		delay   time.Duration
		jitter  *Jitterer
		cleanup func()
	}{
		{
			name:  "massive context timeout",
			delay: time.Millisecond,
			req: &Mocker{
				RequestFunc: func() *http.Request {
					return (&http.Request{}).WithContext(longCtx)
				},
			},
			cleanup: func() {
				longCancel()
			},
		},
		{
			name:  "massive jitter",
			delay: time.Hour,
			req: &Mocker{
				RequestFunc: func() *http.Request {
					return (&http.Request{}).WithContext(shortCtx)
				},
			},
			cleanup: func() {
				shortCancel()
			},
		},
		{
			name:  "no jitter",
			delay: NoJitter,
			req: &Mocker{
				RequestFunc: func() *http.Request {
					return (&http.Request{}).WithContext(longCtx)
				},
			},
			cleanup: func() {},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.jitter.sleep(tt.req, tt.delay)
			tt.cleanup()
		})
	}
}
