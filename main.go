package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sholdee/adguard-exporter/config"
	"github.com/sholdee/adguard-exporter/loghandler"
	"github.com/sholdee/adguard-exporter/metrics"
)

func main() {
	cfg := config.LoadConfig()

	// Set up logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("Using log file path: %s", cfg.LogFilePath)
	log.Printf("Using metrics port: %d", cfg.MetricsPort)
	log.Printf("Log level set to: %s", cfg.LogLevel)

	// Register metrics
	metricsCollector := metrics.NewMetricsCollector()
	metricsCtx, stopMetrics := context.WithCancel(context.Background())
	metricsCollector.StartProcessing(metricsCtx, 30*time.Second)
	prometheus.MustRegister(metrics.DNSQueries.Counter, metrics.DNSQueries.Created)
	prometheus.MustRegister(metrics.BlockedQueries.Counter, metrics.BlockedQueries.Created)
	prometheus.MustRegister(metrics.QueryTypes.CounterVec, metrics.QueryTypes.CreatedVec)
	prometheus.MustRegister(metrics.TopQueryHosts.CounterVec, metrics.TopQueryHosts.CreatedVec)
	prometheus.MustRegister(metrics.TopBlockedQueryHosts.CounterVec, metrics.TopBlockedQueryHosts.CreatedVec)
	prometheus.MustRegister(metrics.SafeSearchEnforcedHosts.CounterVec, metrics.SafeSearchEnforcedHosts.CreatedVec)
	prometheus.MustRegister(metrics.AverageResponseTime)
	prometheus.MustRegister(metrics.AverageUpstreamResponseTime)

	logHandler := loghandler.NewLogHandler(cfg.LogFilePath, metricsCollector)

	// Start watching the log file
	watchCtx, stopWatch := context.WithCancel(context.Background())
	watchDone, err := startLogWatcher(watchCtx, logHandler.NewLogWatcher)
	if err != nil {
		stopMetrics()
		stopWatch()
		log.Fatalf("Error watching log file: %v", err)
	}

	// Set up HTTP server for metrics and health checks
	mux := newMetricsMux(logHandler, promhttp.Handler())

	// Start the HTTP server
	serverAddr := fmt.Sprintf(":%d", cfg.MetricsPort)
	server := newHTTPServer(serverAddr, mux)

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Starting metrics server on %s", serverAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Error starting server: %v", err)
		}
	}()

	<-stop
	log.Println("Shutting down the server...")
	stopMetrics()
	stopWatch()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := server.Shutdown(ctx); err != nil {
		cancel()
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	cancel()
	if !waitForLogWatcher(watchDone, 5*time.Second) {
		log.Println("Timed out waiting for log watcher to stop")
	}

	log.Println("Server exiting")
}

func startLogWatcher(ctx context.Context, newWatcher func() (loghandler.LogWatcher, error)) (<-chan struct{}, error) {
	watcher, err := newWatcher()
	if err != nil {
		return nil, err
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if err := watcher.Close(); err != nil {
				log.Printf("Error closing log file watcher: %v", err)
			}
		}()
		watcher.Run(ctx)
	}()
	return done, nil
}

func waitForLogWatcher(done <-chan struct{}, timeout time.Duration) bool {
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

type logHealth interface {
	IsHealthy() bool
}

func newMetricsMux(logHandler logHealth, metricsHandler http.Handler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/metrics", metricsHandler)
	mux.HandleFunc("/livez", func(w http.ResponseWriter, r *http.Request) {
		if logHandler.IsHealthy() {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "Alive")
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, "Unhealthy")
		}
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "Ready")
	})
	return mux
}

func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}
