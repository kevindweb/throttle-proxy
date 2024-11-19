package proxymw

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/promql/parser"
)

const (
	BackpressureProxyType     = "backpressure"
	BackpressureUpdateCadence = time.Second
	MonitorQueryTimeout       = 10 * time.Second
	DefaultThrottleCurve      = 4.0
	DefaultMaxIdleConns       = 100
	DefaultIdleConnTimeout    = 90 * time.Second
	InstantQueryEndpoint      = "/api/v1/query"
)

type PrometheusResponse struct {
	Data struct {
		Result model.Vector `json:"result"`
	} `json:"data"`
}

type BackpressureQuery struct {
	// Query is the PromQL to monitor system load or usage
	Query string
	// WarningThreshold is the load value at which throttling begins (e.g., 80% capacity)
	WarningThreshold float64
	// EmergencyThreshold is the load value at which the max num of requests are blocked (e.g., 100% capacity). Still lets through CongestionWindowMin
	EmergencyThreshold float64
	// ThrottlingCurve is a constant controlling the aggressiveness of throttling (e.g., default 4.0 for steep growth)
	ThrottlingCurve float64
}

func (q BackpressureQuery) Validate() error {
	if _, err := parser.ParseExpr(q.Query); err != nil {
		return fmt.Errorf("invalid PromQL query %q: %w", q.Query, err)
	}
	if q.ThrottlingCurve < 0 {
		return ErrNegativeThrottleCurve
	}
	if q.WarningThreshold < 0 || q.EmergencyThreshold < 0 {
		return ErrNegativeQueryThresholds
	}
	if q.EmergencyThreshold <= q.WarningThreshold {
		return ErrEmergencyBelowWarnThreshold
	}
	return nil
}

func (q BackpressureQuery) throttlePercent(curr float64) float64 {
	if curr <= q.WarningThreshold {
		return 0.0
	}

	if curr >= q.EmergencyThreshold {
		return 1.0
	}

	curve := q.ThrottlingCurve
	if curve == 0 {
		curve = DefaultThrottleCurve
	}

	loadFactor := (curr - q.WarningThreshold) / (q.EmergencyThreshold - q.WarningThreshold)
	// exponential decay throttling formula: 1-e^(-c * loadFactor)
	return 1 - math.Exp(-curve*loadFactor)
}

type BackpressureConfig struct {
	EnableBackpressure        bool
	BackpressureMonitoringURL string
	BackpressureQueries       []BackpressureQuery
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
		if err := q.Validate(); err != nil {
			return err
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
	queries       []BackpressureQuery
	throttleFlags sync.Map
	allowance     float64

	client ProxyClient
}

var _ ProxyClient = &Backpressure{}

func NewBackpressure(
	client ProxyClient,
	minWindow,
	maxWindow int,
	queries []BackpressureQuery,
	monitorURL string,
) *Backpressure {
	return &Backpressure{
		watermark: minWindow,
		min:       minWindow,
		max:       maxWindow,
		allowance: 1,
		monitorClient: &http.Client{
			Timeout: MonitorQueryTimeout,
			Transport: &http.Transport{
				MaxIdleConns:    DefaultMaxIdleConns,
				IdleConnTimeout: DefaultIdleConnTimeout,
			},
		},
		monitorURL: monitorURL,
		queries:    queries,
		client:     client,
	}
}

// metricsLoop creates a goroutine for each backpressure signal to avoid one slow query from
// preventing the other signals from actioning the congestion window.
func (bp *Backpressure) metricsLoop(ctx context.Context) {
	for _, q := range bp.queries {
		go func(query BackpressureQuery) {
			bp.metricLoop(ctx, query)
		}(q)
	}
}

// metricLoop pulls one PromQL metric on a loop to update whether requests should be throttled.
// we only drop the global throttle when all metrics have dropped their own throttle flag
func (bp *Backpressure) metricLoop(ctx context.Context, q BackpressureQuery) {
	ticker := time.NewTicker(BackpressureUpdateCadence)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			curr, err := bp.metricFired(ctx, q.Query)
			if err != nil {
				log.Printf("querying metric '%s' returned error: %v", q.Query, err)
				continue
			}

			bp.updateThrottle(q, curr)
		}
	}
}

func (bp *Backpressure) updateThrottle(q BackpressureQuery, curr float64) {
	bp.throttleFlags.Store(q, q.throttlePercent(curr))

	throttlePercent := 0.0
	var err error
	bp.throttleFlags.Range(func(_, value interface{}) bool {
		val, ok := value.(float64)
		if !ok {
			log.Printf("error updating query '%s' throttle to %f: %v", q.Query, curr, err)
			return true
		}

		throttlePercent = max(throttlePercent, val)
		return true
	})

	bp.mu.Lock()
	bp.allowance = 1 - throttlePercent
	bp.mu.Unlock()
}

// queryMetric checks if the PromQL expression returns a non-empty response (backpressure is firing)
func (bp *Backpressure) metricFired(ctx context.Context, query string) (float64, error) {
	u, err := url.Parse(bp.monitorURL + InstantQueryEndpoint)
	if err != nil {
		return 0, fmt.Errorf("parse monitor URL: %w", err)
	}

	q := u.Query()
	q.Set("query", query)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}

	resp, err := bp.monitorClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var prometheusResp PrometheusResponse
	if err := json.NewDecoder(resp.Body).Decode(&prometheusResp); err != nil {
		return 0, fmt.Errorf("decode response: %w", err)
	}

	results := prometheusResp.Data.Result
	if len(results) != 1 {
		return 0, fmt.Errorf("query must return exactly one value: %s", query)
	}

	return float64(results[0].Value), nil
}

func (bp *Backpressure) Init(ctx context.Context) {
	bp.metricsLoop(ctx)
	bp.client.Init(ctx)
}

func (bp *Backpressure) Next(rr Request) error {
	if err := bp.check(); err != nil {
		return err
	}

	defer bp.release()
	return bp.client.Next(rr)
}

// check ensures the number of concurrent active requests stays within the allowed window.
// If the active count exceeds the current watermark, the request is denied.
func (bp *Backpressure) check() error {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	if bp.active >= bp.watermark {
		return ErrBackpressureBackoff
	}

	bp.active++
	return nil
}

// release adjusts the watermark and active request count:
// 1. Decrements the active request count, ensuring it doesn't go below zero.
//
// 2. Increases the watermark by one, unless throttling (allowance < 1) reduces it.
//
//   - Throttling can significantly lower the watermark, but watermark won't exceed max.
//
// 3. Ensures the watermark never falls below the configured minimum.
func (bp *Backpressure) release() {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	bp.active = max(0, bp.active-1)
	bp.watermark = min(bp.watermark+1, int(float64(bp.max)*bp.allowance))
	bp.watermark = max(bp.watermark, bp.min)
}
