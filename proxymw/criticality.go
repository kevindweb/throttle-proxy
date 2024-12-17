package proxymw

const (
	// https://sre.google/sre-book/handling-overload/
	CriticalityCriticalPlus = "CRITICAL_PLUS"
	CriticalityCritical     = "CRITICAL"
	// CriticalityDefault is used when the client does not set the X-Request-Criticality header.
	CriticalityDefault = CriticalityCritical
)
