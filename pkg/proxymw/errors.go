package proxymw

import (
	"errors"
	"fmt"
)

var (
	ErrJitterDelayRequired         = errors.New("delay must be non-empty when jitter is enabled")
	ErrBackpressureQueryRequired   = errors.New("must provide at least one backpressure query when backpressure is enabled")
	ErrCongestionWindowMinBelowOne = errors.New("backpressure min window < 1")
	ErrCongestionWindowMaxBelowMin = errors.New("backpressure max window <= min window")
	ErrRegistryRequired            = errors.New("prometheus registry is required when observer is enabled")

	ErrBackpressureBackoff = BlockErr(BackpressureQuerierType, "congestion window closed, backoff from backpressure")
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
