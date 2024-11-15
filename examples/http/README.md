# HTTP Proxy Example

### Run

```
cd examples
go run http/main.go -upstream https://thanos.io:9090 \
    -insecure-listen-address 127.0.0.1:7777 \
    -enable-jitter=true \
    -jitter-delay 2s \
    -enable-observer=true \
    -enable-backpressure=true \
    -backpressure-queries 'sum(rate(throughput[1m]))' \
    -backpressure-monitoring-url https://thanos.io \
    -backpressure-min-window 1 \
    -backpressure-max-window 10
```

Ensure the server is listening at http://localhost:7777/healthz

```
{"ok":true}
```
