FROM --platform=$BUILDPLATFORM golang:1.27.3@sha256:f20d83a83e18dbd2a4714b8ffd4b30b287b6243962dd75608e86abcc63ba4efc AS builder
ARG TARGETOS TARGETARCH

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . ./
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-s -w" -o /adguard-exporter .

FROM gcr.io/distroless/static:nonroot@sha256:18f4cc26c9ffa7f20a359298be7deadac5c607c8e03a02b8119f1a5fc2f82c99
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
