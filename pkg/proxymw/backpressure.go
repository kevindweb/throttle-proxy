package proxymw

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	BackpressureQuerierType = "ai_md"

	BackpressureUpdateCadence = time.Minute

	MonitorQueryTimeout = 10 * time.Second
)

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
	throttleFlags map[string]bool
	throttle      bool

	client ProxyClient
}

var _ ProxyClient = &Backpressure{}

func NewBackpressure(querier ProxyClient, minWindow, maxWindow int, queries []string, monitorURL string) *Backpressure {
	bp := &Backpressure{
		mu:        sync.Mutex{},
		watermark: minWindow,
		min:       minWindow,
		max:       maxWindow,
		active:    0,

		monitorClient: &http.Client{Timeout: MonitorQueryTimeout},
		monitorURL:    monitorURL,
		queries:       queries,
		throttleFlags: map[string]bool{},
		throttle:      false,

		client: querier,
	}
	bp.metricsLoop()
	return bp
}

// metricsLoop creates a goroutine for each backpressure signal to avoid one slow query from
// preventing the other signals from actioning the congestion window.
func (bp *Backpressure) metricsLoop() {
	for _, q := range bp.queries {
		go bp.metricLoop(q)
	}
}

// metricLoop pulls one PromQL metric on a loop to update whether requests should be throttled.
// we only drop the global throttle when all metrics have dropped their own throttle flag
func (bp *Backpressure) metricLoop(q string) {
	for {
		queryFired, err := bp.metricFired(q)
		if err != nil {
			log.Printf("querying metric '%s' returned error: %v", q, err)
			continue
		}

		bp.mu.Lock()
		bp.throttleFlags[q] = queryFired
		throttle := false
		for _, toThrottle := range bp.throttleFlags {
			if toThrottle {
				throttle = true
			}
		}
		bp.throttle = throttle
		bp.mu.Unlock()
	}
}

// queryMetric checks if the PromQL expression returns a non-empty response (backpressure is firing)
func (bp *Backpressure) metricFired(query string) (bool, error) {
	u, err := url.Parse(bp.monitorURL)
	if err != nil {
		return false, err
	}

	q := u.Query()
	q.Set("query", query)
	u.RawQuery = q.Encode()

	resp, err := bp.monitorClient.Get(u.String())
	if err != nil {
		return false, err
	}

	defer resp.Body.Close()

	type PrometheusResponse struct {
		Data struct {
			Result []struct {
				Metric map[string]string `json:"metric"`
				Value  [2]interface{}    `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}

	var prometheusResp PrometheusResponse
	if err := json.NewDecoder(resp.Body).Decode(&prometheusResp); err != nil {
		return false, fmt.Errorf("error decoding JSON response: %w", err)
	}

	return len(prometheusResp.Data.Result) > 0, nil
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

	if bp.throttle {
		bp.watermark = max(bp.min, bp.watermark/2)
	} else {
		bp.watermark = min(bp.max, bp.watermark+1)
	}
}
