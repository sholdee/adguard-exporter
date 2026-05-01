package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sholdee/adguard-exporter/loghandler"
)

type stubLogHealth struct {
	healthy bool
}

func (s stubLogHealth) IsHealthy() bool {
	return s.healthy
}

type stubLogWatcherFactory struct {
	watcher loghandler.LogWatcher
	err     error
}

func (s stubLogWatcherFactory) NewLogWatcher() (loghandler.LogWatcher, error) {
	return s.watcher, s.err
}

type stubLogWatcher struct {
	runStarted  chan struct{}
	closeCalled chan struct{}
}

func (s stubLogWatcher) Run(ctx context.Context) {
	close(s.runStarted)
	<-ctx.Done()
}

func (s stubLogWatcher) Close() error {
	close(s.closeCalled)
	return nil
}

func TestStartLogWatcherReturnsSetupError(t *testing.T) {
	wantErr := errors.New("watch setup failed")

	done, err := startLogWatcher(context.Background(), stubLogWatcherFactory{err: wantErr}.NewLogWatcher)

	if !errors.Is(err, wantErr) {
		t.Fatalf("expected setup error %v, got %v", wantErr, err)
	}
	if done != nil {
		t.Fatal("expected no watcher done channel when setup fails")
	}
}

func TestStartLogWatcherRunsUntilContextCanceledAndClosesWatcher(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	watcher := stubLogWatcher{
		runStarted:  make(chan struct{}),
		closeCalled: make(chan struct{}),
	}

	done, err := startLogWatcher(ctx, stubLogWatcherFactory{watcher: watcher}.NewLogWatcher)
	if err != nil {
		t.Fatalf("start watcher: %v", err)
	}

	select {
	case <-watcher.runStarted:
	case <-time.After(time.Second):
		t.Fatal("expected watcher to start running")
	}

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("expected watcher to stop after cancellation")
	}

	select {
	case <-watcher.closeCalled:
	default:
		t.Fatal("expected watcher to be closed after Run returns")
	}
}

func TestWaitForLogWatcherTimesOut(t *testing.T) {
	done := make(chan struct{})

	if waitForLogWatcher(done, time.Nanosecond) {
		t.Fatal("expected watcher wait to time out")
	}
}

func TestWaitForLogWatcherReturnsWhenDone(t *testing.T) {
	done := make(chan struct{})
	close(done)

	if !waitForLogWatcher(done, time.Second) {
		t.Fatal("expected watcher wait to observe closed done channel")
	}
}

func TestNewHTTPServerConfiguresTimeouts(t *testing.T) {
	server := newHTTPServer(":8000", http.NewServeMux())

	if server.ReadHeaderTimeout == 0 {
		t.Fatal("expected ReadHeaderTimeout to be configured")
	}
	if server.ReadTimeout == 0 {
		t.Fatal("expected ReadTimeout to be configured")
	}
	if server.WriteTimeout == 0 {
		t.Fatal("expected WriteTimeout to be configured")
	}
	if server.IdleTimeout == 0 {
		t.Fatal("expected IdleTimeout to be configured")
	}
}

func TestNewMetricsMuxReportsLivenessFromLogHandler(t *testing.T) {
	tests := []struct {
		name       string
		healthy    bool
		wantStatus int
		wantBody   string
	}{
		{
			name:       "healthy",
			healthy:    true,
			wantStatus: http.StatusOK,
			wantBody:   "Alive",
		},
		{
			name:       "unhealthy",
			healthy:    false,
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   "Unhealthy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux := newMetricsMux(stubLogHealth{healthy: tt.healthy}, http.NotFoundHandler())
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/livez", nil)

			mux.ServeHTTP(response, request)

			if response.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d", tt.wantStatus, response.Code)
			}
			if body := response.Body.String(); body != tt.wantBody {
				t.Fatalf("expected body %q, got %q", tt.wantBody, body)
			}
		})
	}
}

func TestNewMetricsMuxReportsReadiness(t *testing.T) {
	mux := newMetricsMux(stubLogHealth{healthy: false}, http.NotFoundHandler())
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
	if body := response.Body.String(); body != "Ready" {
		t.Fatalf("expected body %q, got %q", "Ready", body)
	}
}

func TestNewMetricsMuxUsesProvidedMetricsHandler(t *testing.T) {
	mux := newMetricsMux(stubLogHealth{healthy: true}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprint(w, "metrics")
	}))
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, response.Code)
	}
	if body := response.Body.String(); body != "metrics" {
		t.Fatalf("expected body %q, got %q", "metrics", body)
	}
}
