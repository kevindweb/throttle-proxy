package proxymw_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kevindweb/throttle-proxy/proxymw"
)

func TestBlockPatternValidation(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name     string
		patterns []string
		want     error
	}{
		{
			name:     "nil patterns no error",
			patterns: nil,
		},
		{
			name:     "no patterns no error",
			patterns: []string{},
		},
		{
			name: "valid patterns",
			patterns: []string{
				`X-block=value.*=here`,
				`X-custom-header=.*`,
			},
		},
		{
			name: "no header",
			patterns: []string{
				`=value.*here`,
			},
			want: errors.New(`header is empty for pattern "=value.*here"`),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, proxymw.ValidateBlockPatterns(tt.patterns))
		})
	}
}

func TestBlocker(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		cfg  proxymw.BlockerConfig
		req  proxymw.Request
		want error
	}{
		{
			name: "request not blocked",
			req: &proxymw.Mocker{
				RequestFunc: func() *http.Request {
					ctx := context.Background()
					r, err := http.NewRequestWithContext(
						ctx, http.MethodGet, "http://google.com", http.NoBody,
					)
					require.NoError(t, err)
					r.Header.Add("X-User-Agent", "safe-user")
					return r
				},
			},
			cfg: proxymw.BlockerConfig{
				EnableBlocker: true,
				BlockPatterns: []string{
					`X-User-Agent=service`,
				},
			},
		},
		{
			name: "request blocked",
			req: &proxymw.Mocker{
				RequestFunc: func() *http.Request {
					ctx := context.Background()
					r, err := http.NewRequestWithContext(
						ctx, http.MethodGet, "http://google.com", http.NoBody,
					)
					require.NoError(t, err)
					r.Header.Add("X-User-Agent", "service1")
					return r
				},
			},
			cfg: proxymw.BlockerConfig{
				EnableBlocker: true,
				BlockPatterns: []string{
					`X-User-Agent=service.*`,
				},
			},
			want: proxymw.BlockErr(
				proxymw.BlockerProxyType,
				"header X-User-Agent, value service1 blocked by regex service.*",
			),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client := &proxymw.Mocker{
				NextFunc: func(_ proxymw.Request) error { return nil },
			}
			blocker := proxymw.NewBlocker(client, tt.cfg)
			require.Equal(t, tt.want, blocker.Next(tt.req))
		})
	}
}
