// Package throttleproxy provides an adaptive backpressure proxy mechanism for dynamically managing
// traffic and protecting backend services using Prometheus metrics.
//
// Usage Example:
//
//		config := proxymw.BackpressureConfig{
//		    EnableBackpressure: true,
//		    BackpressureQueries: []BackpressureQuery{
//		        {
//		            Query:              `sum(rate(http_server_errors_total[5m]))`,
//	                Name:               "http_error_rate"
//		            WarningThreshold:   0.5,
//		            EmergencyThreshold: 0.8,
//		        }
//		    },
//		    CongestionWindowMin: 10,
//		    CongestionWindowMax: 100,
//		}
//
// Use Cases:
//   - Protecting microservices from traffic spikes
//   - Preventing cascading failures
//   - Maintaining system stability under unpredictable load
//
// The package supports both server-side HTTP proxy and client-side RoundTripper
// implementations, providing flexible integration options.
package main // import "github.com/kevindweb/throttle-proxy"
