# HTTP Proxy Example

### Run

```
go run examples/http/main.go -upstream http://localhost:8080 \
    -insecure-listen-address 127.0.0.1:7777 \
    -enable-jitter=true \
    -jitter-delay 3s \
    -enable-observer=true \
    -enable-backpressure=true \
    -backpressure-queries 'sum(rate(throughput[1m]))' \
    -backpressure-monitoring-url http://localhost:9090 \
    -backpressure-min-window 1 \
    -backpressure-max-window 1000
```

Ensure the server is listening at http://localhost:7777/healthz

```
{"ok":true}
```
