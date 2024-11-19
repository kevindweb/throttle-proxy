package proxymw

import (
	"errors"
	"fmt"
)

var (
	ErrJitterDelayRequired       = errors.New("delay must be non-empty when jitter is enabled")
	ErrBackpressureQueryRequired = errors.New(
		"must provide at least one backpressure query when backpressure is enabled",
	)
	ErrCongestionWindowMinBelowOne = errors.New("backpressure min window < 1")
	ErrCongestionWindowMaxBelowMin = errors.New("backpressure max window <= min window")
	ErrNegativeThrottleCurve       = errors.New("throttle curve cannot be negative")
	ErrNegativeQueryThresholds     = errors.New("backpressure query thresholds cannot be negative")
	ErrEmergencyBelowWarnThreshold = errors.New("emergency threshold must be > warn threshold")

	ErrLatencyBackoff = BlockErr(
		LatencyProxyType, "congestion window closed, backoff from slow queries",
	)
	ErrBackpressureBackoff = BlockErr(
		BackpressureProxyType,
		"congestion window closed, backoff from backpressure",
	)

	ErrNilRequest        = errors.New("nil *http.Request")
	ErrNilResponseWriter = errors.New("nil http.ResponseWriter")
	ErrNilResponse       = errors.New("nil *http.Response")
)

type RequestBlockedError struct {
	Err  error
	Type string
}

func (e *RequestBlockedError) Error() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func BlockErr(t string, format string, a ...any) error {
	return &RequestBlockedError{
		Err:  fmt.Errorf(format, a...),
		Type: t,
	}
}
