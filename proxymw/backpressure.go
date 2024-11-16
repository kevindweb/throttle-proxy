package proxymw

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/prometheus/promql/parser"
)

const (
	BackpressureProxyType = "backpressure"

	BackpressureUpdateCadence = time.Minute

	MonitorQueryTimeout = 10 * time.Second
)

// PrometheusResponse represents the structure of Prometheus query responses
type PrometheusResponse struct {
	Data struct {
		Result []struct {
			Metric map[string]string `json:"metric"`
			Value  [2]interface{}    `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

// Backpressure uses Additive Increase Multiplicative Decrease which
// is a congestion control algorithm to back off of expensive queries and is modeled after TCP's
// https://en.wikipedia.org/wiki/Additive_increase/multiplicative_decrease. Backpressure signals
// are derived from PromQL metric signals and the system will never let less than a minimum
// number of queries through at one time.
// How does it work?
// 1. Start a background thread to keep backpressure metrics updated
// 2. On each request, set the "window" for how many concurrent requests are allowed
// 3. if we are within bounds, allow the request
// 4. if backpressure is not spiking, widen the window by one (additive)
// 5. if backpressure signals fire, cut the window by half (multiplicative)
type Backpressure struct {
	mu        sync.Mutex
	watermark int
	active    int
	min, max  int

	monitorClient *http.Client
	monitorURL    string
	queries       []string
	throttleFlags sync.Map
	throttle      atomic.Bool

	client ProxyClient
}

type BackpressureConfig struct {
	EnableBackpressure        bool
	BackpressureMonitoringURL string
	BackpressureQueries       []string
	CongestionWindowMin       int
	CongestionWindowMax       int
}

func (c BackpressureConfig) Validate() error {
	if !c.EnableBackpressure {
		return nil
	}

	if len(c.BackpressureQueries) == 0 {
		return ErrBackpressureQueryRequired
	}

	for _, q := range c.BackpressureQueries {
		if _, err := parser.ParseExpr(q); err != nil {
			return fmt.Errorf("invalid PromQL query %q: %w", q, err)
		}
	}

	if _, err := url.Parse(c.BackpressureMonitoringURL); err != nil {
		return fmt.Errorf("invalid monitoring URL: %w", err)
	}

	if c.CongestionWindowMin < 1 {
		return ErrCongestionWindowMinBelowOne
	}

	if c.CongestionWindowMax < c.CongestionWindowMin {
		return ErrCongestionWindowMaxBelowMin
	}

	return nil
}

var _ ProxyClient = &Backpressure{}

func NewBackpressure(querier ProxyClient, minWindow, maxWindow int, queries []string, monitorURL string) *Backpressure {
	return &Backpressure{
		watermark: minWindow,
		min:       minWindow,
		max:       maxWindow,
		monitorClient: &http.Client{
			Timeout: MonitorQueryTimeout,
			Transport: &http.Transport{
				MaxIdleConns:    100,
				IdleConnTimeout: 90 * time.Second,
			},
		},
		monitorURL: monitorURL,
		queries:    queries,
		client:     querier,
	}
}

// metricsLoop creates a goroutine for each backpressure signal to avoid one slow query from
// preventing the other signals from actioning the congestion window.
func (bp *Backpressure) metricsLoop(ctx context.Context) {
	for _, q := range bp.queries {
		go func(query string) {
			bp.metricLoop(ctx, query)
		}(q)
	}
}

// metricLoop pulls one PromQL metric on a loop to update whether requests should be throttled.
// we only drop the global throttle when all metrics have dropped their own throttle flag
func (bp *Backpressure) metricLoop(ctx context.Context, query string) {
	ticker := time.NewTicker(BackpressureUpdateCadence)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			queryFired, err := bp.metricFired(ctx, query)
			if err != nil {
				log.Printf("querying metric '%s' returned error: %v", query, err)
				continue
			}

			bp.throttleFlags.Store(query, queryFired)

			throttle := false
			bp.throttleFlags.Range(func(_, value interface{}) bool {
				if value.(bool) {
					throttle = true
					return false
				}
				return true
			})

			bp.throttle.Store(throttle)
		}
	}
}

// queryMetric checks if the PromQL expression returns a non-empty response (backpressure is firing)
func (bp *Backpressure) metricFired(ctx context.Context, query string) (bool, error) {
	u, err := url.Parse(bp.monitorURL)
	if err != nil {
		return false, fmt.Errorf("parse monitor URL: %w", err)
	}

	q := u.Query()
	q.Set("query", query)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return false, fmt.Errorf("create request: %w", err)
	}

	resp, err := bp.monitorClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var prometheusResp PrometheusResponse
	if err := json.NewDecoder(resp.Body).Decode(&prometheusResp); err != nil {
		return false, fmt.Errorf("decode response: %w", err)
	}

	return len(prometheusResp.Data.Result) > 0, nil
}

func (bp *Backpressure) Init(ctx context.Context) {
	bp.metricsLoop(ctx)
	bp.client.Init(ctx)
}

func (bp *Backpressure) ServeHTTP(w http.ResponseWriter, r *http.Request) error {
	if err := bp.check(); err != nil {
		return err
	}

	defer bp.release()
	return bp.client.ServeHTTP(w, r)
}

func (bp *Backpressure) check() error {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	if bp.active >= bp.watermark {
		return ErrBackpressureBackoff
	}

	bp.active++
	return nil
}

func (bp *Backpressure) release() {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	bp.active--
	if bp.active < 0 {
		bp.active = 0
	}

	if bp.throttle.Load() {
		bp.watermark = max(bp.min, bp.watermark/2)
	} else {
		bp.watermark = min(bp.max, bp.watermark+1)
	}
}
