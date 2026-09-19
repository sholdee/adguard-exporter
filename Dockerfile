FROM --platform=$BUILDPLATFORM golang:1.27.1@sha256:3680233e3204827fbdc66088528ae6d4b3d034f51d03a99d454f6de034888244 AS builder
ARG TARGETOS TARGETARCH

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . ./
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-s -w" -o /adguard-exporter .

FROM gcr.io/distroless/static:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3
LABEL org.opencontainers.image.title="adguard-exporter"
LABEL org.opencontainers.image.description="Prometheus exporter for AdGuard Home query logs"
LABEL org.opencontainers.image.source="https://github.com/sholdee/adguard-exporter"

COPY --from=builder /adguard-exporter /adguard-exporter

EXPOSE 8000

ENV LOG_FILE_PATH=/opt/adguardhome/work/data/querylog.json
ENV METRICS_PORT=8000
ENV LOG_LEVEL=INFO

USER nonroot:nonroot

ENTRYPOINT ["/adguard-exporter"]
