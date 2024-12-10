FROM golang:1.23-alpine AS builder
ENV GOBIN=/tmp
RUN go install github.com/kevindweb/throttle-proxy@latest

FROM gcr.io/distroless/base-nossl-debian12:nonroot
COPY --from=builder /tmp/throttle-proxy /usr/bin/throttle-proxy
ENTRYPOINT [ "/usr/bin/throttle-proxy" ]
