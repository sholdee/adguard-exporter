FROM --platform=$BUILDPLATFORM golang:1.27.3@sha256:957861185dbf29d93668ace41e011d091c6e779953d47f0316ef2cc58e28fe2b AS builder
ARG TARGETOS TARGETARCH

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . ./
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-s -w" -o /adguard-exporter .

FROM gcr.io/distroless/static:nonroot@sha256:516757106de35205efb70b91ba03f6f1298424a0678c603255cf2852a7000087
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
