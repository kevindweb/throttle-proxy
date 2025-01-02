# Examples

## Go HTTP Server

`go install github.com/kevindweb/throttle-proxy@latest`

### Build Locally

`make build`

### Config File

```
throttle-proxy -config-file examples/config.yaml
```

### CLI Flags

```
throttle-proxy -upstream=http://localhost:9095 \
    -insecure-listen-address=0.0.0.0:7777 \
    -internal-listen-address=0.0.0.0:7776 \
    -proxy-paths=/api/v2/endpoint-to-proxy \
    -passthrough-paths=/api/v2/endpoint-to-passthrough \
    -proxy-read-timeout=30s \
    -proxy-write-timeout=30s \
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

## Go RoundTripper Client

```
CONFIG_FILE="examples/config.yaml" go run examples/roundtripper/main.go
```

## TypeScript Throttler Cloudflare Worker

Start with [Cloudflare Workers docs](https://developers.cloudflare.com/workers/get-started/guide/). The configuration is in `examples/throttler/wrangler.json`

### Develop Worker Locally

```
cd examples/throttler
wrangler dev
```

### Deploy Worker to your Production Environment

```
cd examples/throttler
wrangler deploy
```
