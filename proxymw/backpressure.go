package proxymw

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/common/model"

	"github.com/kevindweb/throttle-proxy/internal/util"
)

const (
	BackpressureProxyType     = "backpressure"
	BackpressureUpdateCadence = 30 * time.Second
	MonitorQueryTimeout       = 15 * time.Second
	DefaultThrottleCurve      = 4.0
)

var (
	bpMinGauge       = promauto.NewGauge(prometheus.GaugeOpts{Name: "proxymw_bp_cwdn_min"})
	bpMaxGauge       = promauto.NewGauge(prometheus.GaugeOpts{Name: "proxymw_bp_cwdn_max"})
	bpWatermarkGauge = promauto.NewGauge(prometheus.GaugeOpts{Name: "proxymw_bp_watermark"})
	bpAllowanceGauge = promauto.NewGauge(prometheus.GaugeOpts{Name: "proxymw_bp_allowance"})

	bpMetricLabels    = []string{"query_name"}
	bpQueryErrCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{Name: "proxymw_bp_query_error_count"}, bpMetricLabels,
	)
	bpQueryWarnGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{Name: "proxymw_bp_query_warn"}, bpMetricLabels,
	)
	bpQueryEmergencyGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{Name: "proxymw_bp_query_emergency"}, bpMetricLabels,
	)
	bpQueryValGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{Name: "proxymw_bp_query_value"}, bpMetricLabels,
	)
)

type PrometheusResponse struct {
	Data struct {
		Result model.Vector `json:"result"`
	} `json:"data"`
}

type BackpressureQuery struct {
	// Name is an optional human readable field used to emit tagged metrics.
	// When unset, operational metrics are omitted.
	// When set, read warn_threshold as proxymw_bp_warn_threshold{query_name="<name>"}
	Name string `yaml:"name,omitempty"`
	// Query is the PromQL to monitor system load or usage
	Query string `yaml:"query"`
	// WarningThreshold is the load value at which throttling begins (e.g., 80% capacity)
	WarningThreshold float64 `yaml:"warning_threshold"`
	// EmergencyThreshold is the load value at which the max num of requests are blocked (e.g., 100% capacity). Still lets through CongestionWindowMin
	EmergencyThreshold float64 `yaml:"emergency_threshold"`
	// ThrottlingCurve is a constant controlling the aggressiveness of throttling (e.g., default 4.0 for steep growth)
	ThrottlingCurve float64 `yaml:"throttling_curve"`
}

func (q BackpressureQuery) Validate() error {
	if q.Query == "" {
		return errors.New("empty backpressure query")
	}
	if wrappedInQuotes(q.Query) {
		return ErrExtraQueryQuotes
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

func wrappedInQuotes(query string) bool {
	if len(query) < 2 {
		return false
	}

	firstChar := query[0]
	lastChar := query[len(query)-1]
	return (firstChar == '\'' && lastChar == '\'') ||
		(firstChar == '"' && lastChar == '"')
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
	EnableBackpressure        bool                `yaml:"enable_backpressure"`
	BackpressureMonitoringURL string              `yaml:"backpressure_monitoring_url"`
	BackpressureQueries       []BackpressureQuery `yaml:"backpressure_queries"`
	CongestionWindowMin       int                 `yaml:"congestion_window_min"`
	CongestionWindowMax       int                 `yaml:"congestion_window_max"`
	// EnableLowCostBypass assumes proxy requests are Prometheus queries.
	// If the promQL will query data more than 2 hours ago, the query is considered high cost.
	// When enabled, low cost queries bypass the backpressure congestion control queue.
	EnableLowCostBypass bool `yaml:"enable_low_cost_bypass"`
}

func ParseBackpressureQueries(
	bpQueries, bpQueryNames []string, bpWarnThresholds, bpEmergencyThresholds []float64,
) ([]BackpressureQuery, error) {
	n := len(bpQueries)
	queries := make([]BackpressureQuery, n)
	if len(bpQueryNames) != n && len(bpQueryNames) != 0 {
		return nil, fmt.Errorf("number of backpressure query names should be 0 or %d", n)
	}

	if len(bpWarnThresholds) != n {
		return nil, fmt.Errorf("expected %d warn thresholds for %d backpressure queries", n, n)
	}

	if len(bpEmergencyThresholds) != n {
		return nil, fmt.Errorf(
			"expected %d emergency thresholds for %d backpressure queries", n, n,
		)
	}

	for i, query := range bpQueries {
		queryName := ""
		if len(bpQueryNames) > 0 {
			queryName = bpQueryNames[i]
		}
		queries[i] = BackpressureQuery{
			Name:               queryName,
			Query:              query,
			WarningThreshold:   bpWarnThresholds[i],
			EmergencyThreshold: bpEmergencyThresholds[i],
		}
	}
	return queries, nil
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
// 3. If we are within bounds, allow the request
// 4. If backpressure is not spiking, widen the window by one (additive)
// 5. if backpressure signals fire, cut the window in proportion to signal strength (multiplicative)
type Backpressure struct {
	mu             sync.Mutex
	watermark      int
	active         int
	min, max       int
	minGauge       prometheus.Gauge
	maxGauge       prometheus.Gauge
	watermarkGauge prometheus.Gauge
	allowanceGauge prometheus.Gauge

	queryErrCount  *prometheus.CounterVec
	warnGauge      *prometheus.GaugeVec
	emergencyGauge *prometheus.GaugeVec
	queryValGauge  *prometheus.GaugeVec

	monitorClient *http.Client
	monitorURL    string
	queries       []BackpressureQuery
	throttleFlags *util.SyncMap[BackpressureQuery, float64]
	allowance     float64

	lowCostBypass bool

	client ProxyClient
}

var _ ProxyClient = &Backpressure{}

func NewBackpressure(client ProxyClient, cfg BackpressureConfig) *Backpressure {
	return &Backpressure{
		watermark:      cfg.CongestionWindowMin,
		min:            cfg.CongestionWindowMin,
		max:            cfg.CongestionWindowMax,
		allowance:      1,
		minGauge:       bpMinGauge,
		maxGauge:       bpMaxGauge,
		watermarkGauge: bpWatermarkGauge,
		allowanceGauge: bpAllowanceGauge,

		queryErrCount:  bpQueryErrCounter,
		warnGauge:      bpQueryWarnGauge,
		emergencyGauge: bpQueryEmergencyGauge,
		queryValGauge:  bpQueryValGauge,
		throttleFlags:  util.NewSyncMap[BackpressureQuery, float64](),

		lowCostBypass: cfg.EnableLowCostBypass,

		monitorClient: &http.Client{
			Timeout:   MonitorQueryTimeout,
			Transport: http.DefaultTransport,
		},
		monitorURL: cfg.BackpressureMonitoringURL,
		queries:    cfg.BackpressureQueries,
		client:     client,
	}
}

func (bp *Backpressure) Init(ctx context.Context) {
	bp.minGauge.Set(float64(bp.min))
	bp.maxGauge.Set(float64(bp.max))
	bp.allowanceGauge.Set(bp.allowance)
	bp.watermarkGauge.Set(float64(bp.watermark))

	for _, q := range bp.queries {
		if q.Name != "" {
			bp.warnGauge.WithLabelValues(q.Name).Set(q.WarningThreshold)
			bp.emergencyGauge.WithLabelValues(q.Name).Set(q.EmergencyThreshold)
		}
	}

	bp.metricsLoop(ctx)
	bp.client.Init(ctx)
}

func (bp *Backpressure) Next(rr Request) error {
	if bp.lowCostBypass {
		if lowCost, err := LowCostRequest(rr); err != nil {
			return err
		} else if lowCost {
			return bp.client.Next(rr)
		}
	}

	if err := bp.check(); err != nil {
		return err
	}

	defer bp.release()
	return bp.client.Next(rr)
}

// metricsLoop creates a goroutine for each backpressure signal to avoid one slow query from
// preventing the other signals from actioning the congestion window.
func (bp *Backpressure) metricsLoop(ctx context.Context) {
	for _, q := range bp.queries {
		go func(q BackpressureQuery) {
			ticker := time.NewTicker(BackpressureUpdateCadence)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					curr, err := ValueFromPromQL(ctx, bp.monitorClient, bp.monitorURL, q.Query)
					if err != nil {
						bp.queryErrCount.WithLabelValues(q.Name).Inc()
						log.Printf("querying metric '%s' returned error: %v", q.Query, err)
						continue
					}

					bp.queryValGauge.WithLabelValues(q.Name).Set(curr)
					bp.updateThrottle(q, curr)
				}
			}
		}(q)
	}
}

func (bp *Backpressure) updateThrottle(q BackpressureQuery, curr float64) {
	bp.throttleFlags.Store(q, q.throttlePercent(curr))
	throttlePercent := 0.0
	bp.throttleFlags.Range(func(_ BackpressureQuery, value float64) bool {
		throttlePercent = max(throttlePercent, value)
		return true
	})

	bp.mu.Lock()
	bp.allowance = 1 - throttlePercent
	bp.allowanceGauge.Set(bp.allowance)
	bp.constrainWatermark()
	bp.mu.Unlock()
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
	bp.watermark++
	bp.constrainWatermark()
}

// constrainWatermark ensures that watermark never goes above the allowed max or below the min.
// Assumes the callsite already holds the lock and updates the metric gauge.
func (bp *Backpressure) constrainWatermark() {
	bp.watermark = min(bp.watermark, int(float64(bp.max)*bp.allowance))
	bp.watermark = max(bp.watermark, bp.min)
	bp.watermarkGauge.Set(float64(bp.watermark))
}
