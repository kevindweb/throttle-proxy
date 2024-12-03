# Examples

## Install

`go install github.com/kevindweb/throttle-proxy@latest`

## Run

### Config File

`throttle-proxy -config-file examples/config.yaml`

### CLI Flags

```
throttle-proxy -upstream=http://localhost:9095 \
    -insecure-listen-address=0.0.0.0:7777 \
    -internal-listen-address=0.0.0.0:7776 \
    -unsafe-passthrough-paths=/api/v2 \
    -enable-jitter=true \
    -jitter-delay=3s \
    -enable-observer=true \
    -enable-bp=true \
    -bp-monitoring-url=http://localhost:9095 \
    -bp-min-window=1 \
    -bp-max-window=1000 \
    -bp-query='sum(rate(throughput[1m]))' \
    -bp-warn=5000 \
    -bp-emergency=8000 \
    -bp-query='sum(rate(error_rate[1m]))' \
    -bp-warn=0.40 \
    -bp-emergency=0.80
```
