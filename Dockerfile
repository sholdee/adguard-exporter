FROM golang:1.25.4-alpine3.22 AS builder

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies
RUN go mod download

# Copy the source code
COPY . ./

# Build the application with static linking
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags="-s -w" -o adguard-exporter .

# Use distroless as the base image
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/adguard-exporter .

# Expose the metrics port
EXPOSE 8000

# Set environment variables (can be overridden at runtime)
ENV LOG_FILE_PATH=/opt/adguardhome/work/data/querylog.json
ENV METRICS_PORT=8000
ENV LOG_LEVEL=INFO

# Use the nonroot user
USER nonroot:nonroot

# Run the exporter
CMD ["./adguard-exporter"]
