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
	go logHandler.WatchLogFile()

	// Set up HTTP server for metrics and health checks
	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/livez", func(w http.ResponseWriter, r *http.Request) {
		if logHandler.IsHealthy() {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "Alive")
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, "Unhealthy")
		}
	})
	http.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "Ready")
	})

	// Start the HTTP server
	serverAddr := fmt.Sprintf(":%d", cfg.MetricsPort)
	server := &http.Server{
		Addr:    serverAddr,
		Handler: http.DefaultServeMux,
	}

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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exiting")
}
