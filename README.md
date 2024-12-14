[![Latest Release](https://img.shields.io/github/release/kevindweb/throttle-proxy.svg?style=flat-square)](https://github.com/kevindweb/throttle-proxy/releases/latest) [![Go Report Card](https://goreportcard.com/badge/github.com/kevindweb/throttle-proxy)](https://goreportcard.com/report/github.com/kevindweb/throttle-proxy) [![Go Code reference](https://img.shields.io/badge/code%20reference-go.dev-darkblue.svg)](https://pkg.go.dev/github.com/kevindweb/throttle-proxy?tab=subdirectories)

# Prometheus Backpressure Proxy

**Adaptive Protection for Your Backend Services**

🛡️ Dynamically shield your services from traffic overload using smart, metrics-driven congestion control.

## Key Features

- 📊 **Adaptive Traffic Management**: Automatically adjusts request concurrency based on real-time Prometheus metrics
- 🔀 **Smart Scaling**: Uses Additive Increase/Multiplicative Decrease (AIMD) algorithm
- 🚦 **Configurable Limits**: Set min and max concurrent request thresholds
- 🔍 **Multi-Signal Monitoring**: Track system health across multiple metrics simultaneously

## Quick Example

```go
config := proxymw.BackpressureConfig{
    EnableBackpressure: true,
    BackpressureQueries: []BackpressureQuery{
        {
            Query:              `sum(rate(http_server_errors_total[5m]))`,
            // Start to throttle when error rate reaches 50%
            WarningThreshold:   0.5,
            // Hard throttling up to 100 CongestionWindowMax when error rate is >80%
            EmergencyThreshold: 0.8,
        }
    },
    CongestionWindowMin: 10,
    CongestionWindowMax: 100,
}
```

## How It Works

1. 🔭 Continuously monitor system metrics
2. 📈 Dynamically adjust request throughput
3. 🛑 Automatically throttle when system stress detected

## When to Use

- Protecting microservices from sudden traffic spikes
- Preventing cascading failures
- Maintaining system stability under unpredictable load

## Quick Start

1. Configure backpressure queries as Prometheus metrics
2. Define min/max request windows
3. Choose the [server-side http proxy](main.go) or [client-side roundtripper](examples/roundtripper/main.go)
4. Import the starter [Grafana dashboard](sandbox/grafana/provisioning/dashboards/throttle-proxy.json)
5. Let the proxy handle the rest!

## Development

### Installation

Build the docker-compose stack

```
make all
docker compose down
docker-compose up --build
```

- Generate fake traffic with `./sandbox/traffic.py`
- View metrics in the [local Grafana instance](http://localhost:3000/d/be68n82lvzg8wa/throttle-proxy-metrics)

### Lint and Test

```
make test
make lintfix
```

### Upgrade dependencies

```
make deps
```

### Contributing

[CONTRIBUTING.md](CONTRIBUTING.md)
