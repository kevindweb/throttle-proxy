package proxymw

import (
	"context"
	"sort"
	"sync"
	"time"
)

const (
	LatencyProxyType        = "latency"
	DefaultPercentileWindow = 1000
)

type LatencyTracker struct {
	client     ProxyClient
	mu         sync.Mutex
	watermark  int
	active     int
	min, max   int
	percentile *Percentile
}

var _ ProxyClient = &LatencyTracker{}

func NewLatencyTracker(client ProxyClient) *LatencyTracker {
	return &LatencyTracker{
		client:     client,
		percentile: NewPercentile(DefaultPercentileWindow),
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

	l.active--
	if l.active < 0 {
		l.active = 0
	}

	if ctx.Err() == context.DeadlineExceeded || l.percentile.Block(latency) {
		l.watermark = max(l.min, l.watermark/2)
	} else {
		l.watermark = min(l.max, l.watermark+1)
	}

	l.percentile.AddValue(latency)
}

type Percentile struct {
	values    []float64
	maxSize   int
	currIndex int
}

func NewPercentile(windowSize int) *Percentile {
	return &Percentile{
		values:  make([]float64, windowSize),
		maxSize: windowSize,
	}
}

func (p *Percentile) AddValue(latency float64) {
	p.values[p.currIndex] = latency
	p.currIndex = (p.currIndex + 1) % p.maxSize
}

func (p *Percentile) Block(latency float64) bool {
	return p.GetCurrentCount() > 100 && p.P90() < latency
}

func (p *Percentile) P90() float64 {
	sortedVals := make([]float64, p.maxSize)
	copy(sortedVals, p.values)
	sort.Float64s(sortedVals)
	p90Index := int(float64(p.maxSize) * 0.9)
	return sortedVals[p90Index]
}

func (p *Percentile) GetCurrentCount() int {
	return p.maxSize
}
