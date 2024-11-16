# Prometheus Backpressure Proxy

A Go middleware that implements an adaptive congestion control algorithm to protect backend services using Prometheus metrics as backpressure signals.

## Overview

This proxy acts as a protective layer for your backend services by dynamically adjusting the number of concurrent requests based on custom Prometheus metrics.

## Features

- Dynamic congestion window adjustment using AIMD algorithm
- Prometheus metrics-based backpressure detection
- Configurable minimum and maximum concurrent request limits
- Multiple backpressure signals support
- Non-blocking metric collection
- Thread-safe implementation

## How It Works

1. **Initialization**: The system starts with a minimum congestion window size.

2. **Metric Monitoring**:

   - Continuously monitors specified Prometheus metrics
   - Each metric is monitored independently in separate goroutines
   - Updates occur at a configurable cadence (default: 1 minute)

3. **Window Adjustment**:

   - If no backpressure signals are firing: Window size increases by 1 (Additive Increase)
   - If any backpressure signal fires: Window size is halved (Multiplicative Decrease)
   - Window size is always kept between configured min and max values

4. **Request Handling**:
   - Each incoming request checks against the current window size
   - Requests are rejected with a backpressure error if the window is full
   - Successfully processed requests trigger window size adjustments

### Example Configuration

```go
config := BackpressureConfig{
    EnableBackpressure:        true,
    BackpressureMonitoringURL: "http://prometheus:9090/api/v1/query",
    BackpressureQueries: []string{
        `sum(rate(http_server_requests_seconds_count{status="429"}[5m])) > 0.5`,
        `avg(system_cpu_usage) > 0.8`,
    },
    CongestionWindowMin: 10,
    CongestionWindowMax: 100,
}
```

## Examples

See `examples/http/README.md` to get started

## Example Prometheus Queries

Here are some example PromQL queries you might use as backpressure signals:

```promql
# High error rate
sum(rate(http_server_errors_total[5m])) / sum(rate(http_server_requests_total[5m])) > 0.1

# High latency
histogram_quantile(0.95, sum(rate(http_request_duration_seconds_bucket[5m])) by (le)) > 1.0

# High CPU usage
avg(rate(process_cpu_seconds_total[5m])) > 0.8

# Memory pressure
process_resident_memory_bytes / process_virtual_memory_bytes > 0.85
```

## Best Practices

1. **Choose Appropriate Metrics**:

   - Use metrics that directly indicate system stress
   - Combine multiple signals for better accuracy
   - Consider both resource metrics (CPU, memory) and application metrics (latency, errors)

2. **Window Size Configuration**:

   - Set minimum window size based on your system's baseline capacity
   - Set maximum window size based on your system's peak capacity
   - Consider your application's typical request patterns

3. **Monitoring**:
   - Monitor the proxy's behavior using its own metrics
   - Track rejected requests and window size changes
   - Adjust configuration based on observed patterns

## Limitations

- Requires a running Prometheus instance with relevant metrics
- Metric query latency can affect responsiveness
- All queries must return boolean results (firing/not firing)
