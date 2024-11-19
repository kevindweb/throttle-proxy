# Client-Side HTTP RoundTripper Example

### Run

```
PROXY_URL="http://localhost:9095" \
PROXY_QUERY="vector(82)" \
PROXY_WARN="80" \
PROXY_EMERGENCY="100" \
CWIND_MIN="1" \
CWIND_MAX="100" \
JITTER_PROXY_DELAY="1s" \
    go run examples/roundtripper/main.go
```
