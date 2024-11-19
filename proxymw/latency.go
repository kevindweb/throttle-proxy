package proxymw

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	LatencyProxyType        = "latency"
	DefaultPercentileWindow = 1000
)

var (
	p90LatencyGauge = promauto.NewGauge(prometheus.GaugeOpts{Name: "proxymw_p90_latency_ms"})
)

type LatencyTracker struct {
	client    ProxyClient
	mu        sync.Mutex
	watermark int
	active    int
	min, max  int
	p90Gauge  prometheus.Gauge

	// for percentile calculation
	latencies []float64
	maxSize   int
	currIndex int
	elements  int
}

var _ ProxyClient = &LatencyTracker{}

func NewLatencyTracker(client ProxyClient, minWindow, maxWindow int) *LatencyTracker {
	return &LatencyTracker{
		min:       minWindow,
		watermark: minWindow,
		max:       maxWindow,

		client:    client,
		latencies: make([]float64, DefaultPercentileWindow),
		maxSize:   DefaultPercentileWindow,
		p90Gauge:  p90LatencyGauge,
	}
}

func (l *LatencyTracker) Init(ctx context.Context) {
	l.client.Init(ctx)
}

func (l *LatencyTracker) Next(rr Request) error {
	if err := l.check(); err != nil {
		return err
	}

	n := time.Now()
	err := l.client.Next(rr)
	latency := float64(time.Since(n).Milliseconds())
	l.release(rr.Request().Context(), latency)
	return err
}

func (l *LatencyTracker) check() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.active >= l.watermark {
		return ErrLatencyBackoff
	}

	l.active++
	return nil
}

func (l *LatencyTracker) release(ctx context.Context, latency float64) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if ctx.Err() == context.DeadlineExceeded || (l.elements > 100 && latency > l.P90()) {
		l.watermark = max(l.min, l.watermark/2)
	} else {
		l.watermark = min(l.max, l.watermark+1)
	}

	l.active = max(0, l.active-1)
	l.AddValue(latency)
}

func (l *LatencyTracker) AddValue(latency float64) {
	l.latencies[l.currIndex] = latency
	l.currIndex = (l.currIndex + 1) % l.maxSize
	l.elements = min(l.elements+1, l.maxSize)
}

func (l *LatencyTracker) P90() float64 {
	sortedVals := make([]float64, l.elements)
	copy(sortedVals, l.latencies)
	sort.Float64s(sortedVals)
	p90Index := int(float64(l.elements) * 0.9)
	p90 := sortedVals[p90Index]
	l.p90Gauge.Set(p90)
	return p90
}
