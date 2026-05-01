package loghandler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/sholdee/adguard-exporter/metrics"
)

func TestNewLogHandlerInitialLoadProcessesExistingLogFile(t *testing.T) {
	resetMetricsForLogHandlerTest(t)
	logFilePath := filepath.Join(t.TempDir(), "querylog.json")
	writeLogFile(t, logFilePath,
		queryLogLine("first.example", "A", false, 0, 25_000_000, "1.1.1.1"),
		queryLogLine("second.example", "AAAA", false, 0, 50_000_000, ""),
	)

	handler := NewLogHandler(logFilePath, metrics.NewMetricsCollector())

	if !handler.fileExists {
		t.Fatal("expected handler to mark existing log file as present")
	}
	if !handler.IsHealthy() {
		t.Fatal("expected handler to remain healthy after loading valid log file")
	}
	if handler.lastPosition == 0 {
		t.Fatal("expected handler to advance last read position after initial load")
	}
	if got := testutil.ToFloat64(metrics.DNSQueries.Counter); got != 2 {
		t.Fatalf("expected two DNS queries from initial load, got %f", got)
	}
	if got := testutil.ToFloat64(metrics.QueryTypes.WithLabelValues("A")); got != 1 {
		t.Fatalf("expected one A query, got %f", got)
	}
	if got := testutil.ToFloat64(metrics.QueryTypes.WithLabelValues("AAAA")); got != 1 {
		t.Fatalf("expected one AAAA query, got %f", got)
	}
}

func TestProcessNewLinesOnlyReadsAppendedLines(t *testing.T) {
	resetMetricsForLogHandlerTest(t)
	logFilePath := filepath.Join(t.TempDir(), "querylog.json")
	writeLogFile(t, logFilePath, queryLogLine("first.example", "A", false, 0, 10_000_000, "1.1.1.1"))
	handler := NewLogHandler(logFilePath, metrics.NewMetricsCollector())

	appendLogLine(t, logFilePath, queryLogLine("second.example", "AAAA", false, 0, 20_000_000, "1.1.1.1"))
	handler.processNewLines()

	if got := testutil.ToFloat64(metrics.DNSQueries.Counter); got != 2 {
		t.Fatalf("expected appended read to avoid replaying old lines, got %f DNS queries", got)
	}
	if got := testutil.ToFloat64(metrics.QueryTypes.WithLabelValues("A")); got != 1 {
		t.Fatalf("expected original A query to be counted once, got %f", got)
	}
	if got := testutil.ToFloat64(metrics.QueryTypes.WithLabelValues("AAAA")); got != 1 {
		t.Fatalf("expected appended AAAA query to be counted once, got %f", got)
	}
}

func TestProcessNewLinesSkipsMalformedJSONAndStaysHealthy(t *testing.T) {
	resetMetricsForLogHandlerTest(t)
	logFilePath := filepath.Join(t.TempDir(), "querylog.json")
	writeLogFile(t, logFilePath,
		"{not-json",
		queryLogLine("valid.example", "A", false, 0, 10_000_000, "1.1.1.1"),
	)

	handler := NewLogHandler(logFilePath, metrics.NewMetricsCollector())

	if !handler.IsHealthy() {
		t.Fatal("expected malformed JSON lines to be skipped without marking handler unhealthy")
	}
	if got := testutil.ToFloat64(metrics.DNSQueries.Counter); got != 1 {
		t.Fatalf("expected only valid JSON line to update metrics, got %f", got)
	}
}

func TestProcessNewLinesHandlesLargeQueryLogRecords(t *testing.T) {
	resetMetricsForLogHandlerTest(t)
	logFilePath := filepath.Join(t.TempDir(), "querylog.json")
	writeLogFile(t, logFilePath,
		largeQueryLogLine("large.example", "A", strings.Repeat("a", 70*1024)),
		queryLogLine("after-large.example", "AAAA", false, 0, 10_000_000, "1.1.1.1"),
	)

	handler := NewLogHandler(logFilePath, metrics.NewMetricsCollector())

	if !handler.IsHealthy() {
		t.Fatal("expected large query log record to be processed without making handler unhealthy")
	}
	if got := testutil.ToFloat64(metrics.DNSQueries.Counter); got != 2 {
		t.Fatalf("expected large and following records to update metrics, got %f", got)
	}
	if handler.lastPosition == 0 {
		t.Fatal("expected handler to advance last read position after large record")
	}
}

func TestProcessNewLinesResetsOffsetWhenLogIsTruncated(t *testing.T) {
	resetMetricsForLogHandlerTest(t)
	logFilePath := filepath.Join(t.TempDir(), "querylog.json")
	writeLogFile(t, logFilePath, largeQueryLogLine("before-truncate.example", "A", strings.Repeat("a", 1024)))
	handler := NewLogHandler(logFilePath, metrics.NewMetricsCollector())

	writeLogFile(t, logFilePath, queryLogLine("after-truncate.example", "AAAA", false, 0, 20_000_000, "1.1.1.1"))
	handler.processNewLines()

	if !handler.IsHealthy() {
		t.Fatal("expected handler to remain healthy after processing truncated log")
	}
	if got := testutil.ToFloat64(metrics.DNSQueries.Counter); got != 2 {
		t.Fatalf("expected truncated log to be read from beginning, got %f DNS queries", got)
	}
	if got := testutil.ToFloat64(metrics.QueryTypes.WithLabelValues("AAAA")); got != 1 {
		t.Fatalf("expected query after truncation to be processed, got %f", got)
	}
}

func TestProcessNewLinesWaitsForPartialFinalLine(t *testing.T) {
	resetMetricsForLogHandlerTest(t)
	logFilePath := filepath.Join(t.TempDir(), "querylog.json")
	partial := `{"QH":"partial.example","QT":"A","Elapsed":10000000,"Upstream":"1.1.1.1","Result":{"IsFiltered":false`
	if err := os.WriteFile(logFilePath, []byte(partial), 0o600); err != nil {
		t.Fatalf("write partial log file: %v", err)
	}
	handler := NewLogHandler(logFilePath, metrics.NewMetricsCollector())

	if got := testutil.ToFloat64(metrics.DNSQueries.Counter); got != 0 {
		t.Fatalf("expected partial line not to be processed, got %f DNS queries", got)
	}

	appendLogFragment(t, logFilePath, `,"Reason":0}}`+"\n")
	handler.processNewLines()

	if got := testutil.ToFloat64(metrics.DNSQueries.Counter); got != 1 {
		t.Fatalf("expected completed partial line to be processed once, got %f", got)
	}
	if got := testutil.ToFloat64(metrics.QueryTypes.WithLabelValues("A")); got != 1 {
		t.Fatalf("expected completed partial A query to be processed once, got %f", got)
	}
}

func TestProcessNewLinesResetsOffsetWhenTruncatedLogRegrowsPastOldOffset(t *testing.T) {
	resetMetricsForLogHandlerTest(t)
	logFilePath := filepath.Join(t.TempDir(), "querylog.json")
	writeLogFile(t, logFilePath, largeQueryLogLine("old.example", "A", strings.Repeat("a", 128)))
	handler := NewLogHandler(logFilePath, metrics.NewMetricsCollector())
	oldPosition := handler.lastPosition

	newLine := largeQueryLogLine("after-regrow.example", "AAAA", strings.Repeat("b", int(oldPosition)))
	writeLogFile(t, logFilePath, newLine)
	handler.processNewLines()

	if got := testutil.ToFloat64(metrics.DNSQueries.Counter); got != 2 {
		t.Fatalf("expected regrown truncated log to be read from beginning, got %f DNS queries", got)
	}
	if got := testutil.ToFloat64(metrics.QueryTypes.WithLabelValues("AAAA")); got != 1 {
		t.Fatalf("expected query after truncate/regrow to be processed, got %f", got)
	}
}

func TestNewLogHandlerMissingFileStartsHealthyAndAbsent(t *testing.T) {
	resetMetricsForLogHandlerTest(t)
	logFilePath := filepath.Join(t.TempDir(), "missing-querylog.json")

	handler := NewLogHandler(logFilePath, metrics.NewMetricsCollector())

	if handler.fileExists {
		t.Fatal("expected missing log file to be marked absent")
	}
	if !handler.IsHealthy() {
		t.Fatal("expected missing log file to start healthy while waiting for creation")
	}
	if got := testutil.ToFloat64(metrics.DNSQueries.Counter); got != 0 {
		t.Fatalf("expected missing log file not to update metrics, got %f", got)
	}
}

func TestHandleWatchEventProcessesWriteEventForLogFile(t *testing.T) {
	resetMetricsForLogHandlerTest(t)
	logFilePath := filepath.Join(t.TempDir(), "querylog.json")
	writeLogFile(t, logFilePath, queryLogLine("initial.example", "A", false, 0, 10_000_000, "1.1.1.1"))
	handler := NewLogHandler(logFilePath, metrics.NewMetricsCollector())

	appendLogLine(t, logFilePath, queryLogLine("written.example", "AAAA", false, 0, 20_000_000, "1.1.1.1"))
	handler.handleWatchEvent(fsnotify.Event{Name: logFilePath, Op: fsnotify.Write})

	if got := testutil.ToFloat64(metrics.DNSQueries.Counter); got != 2 {
		t.Fatalf("expected write event to process appended log line, got %f DNS queries", got)
	}
	if got := testutil.ToFloat64(metrics.QueryTypes.WithLabelValues("AAAA")); got != 1 {
		t.Fatalf("expected write event to process appended AAAA query, got %f", got)
	}
}

func TestHandleWatchEventProcessesCreateEventForLogFile(t *testing.T) {
	resetMetricsForLogHandlerTest(t)
	logFilePath := filepath.Join(t.TempDir(), "querylog.json")
	handler := NewLogHandler(logFilePath, metrics.NewMetricsCollector())

	writeLogFile(t, logFilePath, queryLogLine("created.example", "A", false, 0, 10_000_000, "1.1.1.1"))
	handler.handleWatchEvent(fsnotify.Event{Name: logFilePath, Op: fsnotify.Create})

	if !handler.fileExists {
		t.Fatal("expected create event to mark log file as present")
	}
	if got := testutil.ToFloat64(metrics.DNSQueries.Counter); got != 1 {
		t.Fatalf("expected create event to process new log file, got %f DNS queries", got)
	}
}

func TestHandleWatchEventIgnoresUnrelatedFiles(t *testing.T) {
	resetMetricsForLogHandlerTest(t)
	dir := t.TempDir()
	logFilePath := filepath.Join(dir, "querylog.json")
	writeLogFile(t, logFilePath, queryLogLine("initial.example", "A", false, 0, 10_000_000, "1.1.1.1"))
	handler := NewLogHandler(logFilePath, metrics.NewMetricsCollector())

	appendLogLine(t, logFilePath, queryLogLine("unread.example", "AAAA", false, 0, 20_000_000, "1.1.1.1"))
	handler.handleWatchEvent(fsnotify.Event{Name: filepath.Join(dir, "other.json"), Op: fsnotify.Write})

	if got := testutil.ToFloat64(metrics.DNSQueries.Counter); got != 1 {
		t.Fatalf("expected unrelated event to leave appended log unread, got %f DNS queries", got)
	}
}

func TestHandleWatchErrorMarksHandlerUnhealthy(t *testing.T) {
	resetMetricsForLogHandlerTest(t)
	logFilePath := filepath.Join(t.TempDir(), "missing-querylog.json")
	handler := NewLogHandler(logFilePath, metrics.NewMetricsCollector())

	handler.handleWatchError(errors.New("watch failed"))

	if handler.IsHealthy() {
		t.Fatal("expected watcher error to mark handler unhealthy")
	}
}

func TestWatchLogFileReturnsWhenContextCanceled(t *testing.T) {
	resetMetricsForLogHandlerTest(t)
	logFilePath := filepath.Join(t.TempDir(), "missing-querylog.json")
	handler := NewLogHandler(logFilePath, metrics.NewMetricsCollector())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	errc := make(chan error, 1)
	go func() {
		errc <- handler.WatchLogFile(ctx)
	}()

	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("expected canceled watcher to return cleanly, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("expected canceled watcher to return")
	}
}

func TestNewLogWatcherReturnsErrorForMissingDirectory(t *testing.T) {
	resetMetricsForLogHandlerTest(t)
	logFilePath := filepath.Join(t.TempDir(), "missing", "querylog.json")
	handler := NewLogHandler(logFilePath, metrics.NewMetricsCollector())

	watcher, err := handler.NewLogWatcher()

	if err == nil {
		if closeErr := watcher.Close(); closeErr != nil {
			t.Fatalf("close unexpected watcher: %v", closeErr)
		}
		t.Fatal("expected missing watch directory to return an error")
	}
	if watcher != nil {
		t.Fatal("expected no watcher when setup fails")
	}
	if handler.IsHealthy() {
		t.Fatal("expected watcher setup failure to mark handler unhealthy")
	}
}

func queryLogLine(host, queryType string, blocked bool, reason int, elapsedNs int, upstream string) string {
	upstreamField := ""
	if upstream != "" {
		upstreamField = fmt.Sprintf(`,"Upstream":%q`, upstream)
	}

	return fmt.Sprintf(
		`{"QH":%q,"QT":%q,"Elapsed":%d%s,"Result":{"IsFiltered":%t,"Reason":%d}}`,
		host,
		queryType,
		elapsedNs,
		upstreamField,
		blocked,
		reason,
	)
}

func largeQueryLogLine(host, queryType string, answer string) string {
	return fmt.Sprintf(
		`{"QH":%q,"QT":%q,"Elapsed":10000000,"Upstream":"1.1.1.1","Answer":%q,"Result":{"IsFiltered":false,"Reason":0}}`,
		host,
		queryType,
		answer,
	)
}

func writeLogFile(t *testing.T, path string, lines ...string) {
	t.Helper()

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create log file: %v", err)
	}
	defer file.Close()

	for _, line := range lines {
		if _, err := fmt.Fprintln(file, line); err != nil {
			t.Fatalf("write log line: %v", err)
		}
	}
}

func appendLogLine(t *testing.T, path string, line string) {
	t.Helper()

	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open log file for append: %v", err)
	}
	defer file.Close()

	if _, err := fmt.Fprintln(file, line); err != nil {
		t.Fatalf("append log line: %v", err)
	}
}

func appendLogFragment(t *testing.T, path string, fragment string) {
	t.Helper()

	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open log file for append: %v", err)
	}
	defer file.Close()

	if _, err := file.WriteString(fragment); err != nil {
		t.Fatalf("append log fragment: %v", err)
	}
}

func resetMetricsForLogHandlerTest(t *testing.T) {
	t.Helper()

	oldDNSQueries := metrics.DNSQueries
	oldBlockedQueries := metrics.BlockedQueries
	oldQueryTypes := metrics.QueryTypes
	oldTopQueryHosts := metrics.TopQueryHosts
	oldTopBlockedQueryHosts := metrics.TopBlockedQueryHosts
	oldSafeSearchEnforcedHosts := metrics.SafeSearchEnforcedHosts
	oldAverageResponseTime := metrics.AverageResponseTime
	oldAverageUpstreamResponseTime := metrics.AverageUpstreamResponseTime
	t.Cleanup(func() {
		metrics.DNSQueries = oldDNSQueries
		metrics.BlockedQueries = oldBlockedQueries
		metrics.QueryTypes = oldQueryTypes
		metrics.TopQueryHosts = oldTopQueryHosts
		metrics.TopBlockedQueryHosts = oldTopBlockedQueryHosts
		metrics.SafeSearchEnforcedHosts = oldSafeSearchEnforcedHosts
		metrics.AverageResponseTime = oldAverageResponseTime
		metrics.AverageUpstreamResponseTime = oldAverageUpstreamResponseTime
	})

	metrics.DNSQueries = metrics.NewCustomCounter(prometheus.CounterOpts{
		Name: "agh_dns_queries_total",
		Help: "Total number of DNS queries",
	})
	metrics.BlockedQueries = metrics.NewCustomCounter(prometheus.CounterOpts{
		Name: "agh_blocked_dns_queries_total",
		Help: "Total number of blocked DNS queries",
	})
	metrics.QueryTypes = metrics.NewCustomCounterVec(prometheus.CounterOpts{
		Name: "agh_dns_query_types_total",
		Help: "Types of DNS queries",
	}, []string{"query_type"})
	metrics.TopQueryHosts = metrics.NewCustomCounterVec(prometheus.CounterOpts{
		Name: "agh_dns_query_hosts_total",
		Help: "Top DNS query hosts",
	}, []string{"host"})
	metrics.TopBlockedQueryHosts = metrics.NewCustomCounterVec(prometheus.CounterOpts{
		Name: "agh_blocked_dns_query_hosts_total",
		Help: "Top blocked DNS query hosts",
	}, []string{"host"})
	metrics.SafeSearchEnforcedHosts = metrics.NewCustomCounterVec(prometheus.CounterOpts{
		Name: "agh_safe_search_enforced_hosts_total",
		Help: "Safe search enforced hosts",
	}, []string{"host"})
	metrics.AverageResponseTime = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "agh_dns_average_response_time",
		Help: "Average response time for DNS queries in milliseconds",
	})
	metrics.AverageUpstreamResponseTime = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "agh_dns_average_upstream_response_time",
		Help: "Average response time by upstream server",
	}, []string{"server"})
}
