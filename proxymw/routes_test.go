package proxymw_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kevindweb/throttle-proxy/proxymw"
)

func TestInvalidJitterConfig(t *testing.T) {
	upstream, err := url.Parse("http://google.com")
	require.NoError(t, err)

	ctx := context.Background()
	cfg := proxymw.Config{
		EnableJitter: true,
		JitterDelay:  0,
	}

	routes, err := proxymw.NewRoutes(ctx, cfg, []string{}, []string{}, upstream)
	require.ErrorAs(t, err, &proxymw.ErrJitterDelayRequired)
	require.Nil(t, routes)
}

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
		EnableJitter:  false,
		ClientTimeout: time.Second,
	}

	ctx := context.Background()
	proxies := []string{"/test-proxy"}
	passthroughs := []string{"/test-passthrough"}
	routes, err := proxymw.NewRoutes(ctx, cfg, proxies, passthroughs, upstreamURL)
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
			path:           "/test-proxy",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Passthrough Path",
			path:           "/test-passthrough",
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

func TestNewDefaultPassthroughRoutes(t *testing.T) {
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
		EnableJitter:  false,
		ClientTimeout: time.Second,
	}

	ctx := context.Background()
	proxies := []string{"/test-proxy"}
	passthroughs := []string{}
	routes, err := proxymw.NewRoutes(ctx, cfg, proxies, passthroughs, upstreamURL)
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
			path:           "/test-proxy",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Passthrough Path",
			path:           "/test-passthrough",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Not a passthrough",
			path:           "/non-passthrough",
			expectedStatus: http.StatusOK,
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
