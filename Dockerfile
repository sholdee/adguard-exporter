FROM golang:1.22-alpine3.20 AS builder

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies
RUN go mod download

# Copy the source code
COPY . ./

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags="-s -w" -o adguard-exporter .

# Create a minimal Alpine image
FROM alpine:3.20

WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/adguard-exporter .

# Expose the metrics port
EXPOSE 8000

# Set environment variables (can be overridden at runtime)
ENV LOG_FILE_PATH=/opt/adguardhome/work/data/querylog.json
ENV METRICS_PORT=8000
ENV LOG_LEVEL=INFO

# Run the exporter
CMD ["./adguard-exporter"]
