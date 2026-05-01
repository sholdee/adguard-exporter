package metrics

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestUpdateMetricsTracksAllowedQuery(t *testing.T) {
	resetMetricsForTest(t)
	collector := NewMetricsCollector()

	collector.UpdateMetrics(queryLogEntry("allowed.example", "A", false, ReasonNotFilteredNotFound, 25_000_000, "1.1.1.1"))

	if got := testutil.ToFloat64(DNSQueries.Counter); got != 1 {
		t.Fatalf("expected DNS query count 1, got %f", got)
	}
	if got := testutil.ToFloat64(QueryTypes.WithLabelValues("A")); got != 1 {
		t.Fatalf("expected A query type count 1, got %f", got)
	}
	if got := hostCountsByName(collector.topHosts.GetTop())["allowed.example"]; got != 1 {
		t.Fatalf("expected allowed host count 1, got %d", got)
	}
	if got := len(collector.topBlockedHosts.GetTop()); got != 0 {
		t.Fatalf("expected no blocked hosts, got %d", got)
	}
	if got := testutil.ToFloat64(TopQueryHosts.WithLabelValues("allowed.example")); got != 1 {
		t.Fatalf("expected top query host metric 1, got %f", got)
	}
	if got := testutil.ToFloat64(AverageResponseTime); got != 25 {
		t.Fatalf("expected average response time 25ms, got %f", got)
	}
	if got := testutil.ToFloat64(AverageUpstreamResponseTime.WithLabelValues("1.1.1.1")); got != 25 {
		t.Fatalf("expected upstream average response time 25ms, got %f", got)
	}
}

func TestUpdateMetricsDefaultsOmittedAllowedReason(t *testing.T) {
	resetMetricsForTest(t)
	collector := NewMetricsCollector()
	entry := queryLogEntry("allowed.example", "A", false, ReasonNotFilteredNotFound, 25_000_000, "1.1.1.1")
	entry.Result.Reason = nil

	collector.UpdateMetrics(entry)

	if got := testutil.ToFloat64(DNSQueries.Counter); got != 1 {
		t.Fatalf("expected omitted reason on allowed query to count DNS query, got %f", got)
	}
	if got := testutil.ToFloat64(QueryFilteringReasons.WithLabelValues("not_filtered_not_found")); got != 1 {
		t.Fatalf("expected omitted allowed reason to default to not filtered, got %f", got)
	}
	if got := testutil.ToFloat64(QueryLogEntriesSkipped.WithLabelValues("missing_reason")); got != 0 {
		t.Fatalf("expected omitted allowed reason not to be skipped, got %f", got)
	}
}

func TestUpdateMetricsTracksBlockedQuerySeparately(t *testing.T) {
	resetMetricsForTest(t)
	collector := NewMetricsCollector()

	collector.UpdateMetrics(queryLogEntry("blocked.example", "AAAA", true, ReasonFilteredBlockList, 10_000_000, "9.9.9.9"))

	if got := testutil.ToFloat64(BlockedQueries.Counter); got != 1 {
		t.Fatalf("expected blocked query count 1, got %f", got)
	}
	if got := len(collector.topHosts.GetTop()); got != 0 {
		t.Fatalf("expected blocked query not to be counted as allowed host, got %d hosts", got)
	}
	if got := hostCountsByName(collector.topBlockedHosts.GetTop())["blocked.example"]; got != 1 {
		t.Fatalf("expected blocked host count 1, got %d", got)
	}
	if got := testutil.ToFloat64(TopBlockedQueryHosts.WithLabelValues("blocked.example")); got != 1 {
		t.Fatalf("expected top blocked query host metric 1, got %f", got)
	}
	if got := testutil.ToFloat64(QueryFilteringReasons.WithLabelValues("filtered_blocklist")); got != 1 {
		t.Fatalf("expected filtered blocklist reason count 1, got %f", got)
	}
}

func TestUpdateMetricsTracksSafeSearchSeparatelyFromBlockedQueries(t *testing.T) {
	resetMetricsForTest(t)
	collector := NewMetricsCollector()

	collector.UpdateMetrics(queryLogEntry("safe.example", "A", true, ReasonFilteredSafeSearch, 15_000_000, "8.8.8.8"))

	if got := testutil.ToFloat64(SafeSearchEnforcedHosts.WithLabelValues("safe.example")); got != 1 {
		t.Fatalf("expected safe search host count 1, got %f", got)
	}
	if got := testutil.ToFloat64(BlockedQueries.Counter); got != 0 {
		t.Fatalf("expected safe search query not to increment blocked query count, got %f", got)
	}
	if got := len(collector.topBlockedHosts.GetTop()); got != 0 {
		t.Fatalf("expected safe search query not to be counted as blocked host, got %d hosts", got)
	}
	if got := testutil.ToFloat64(QueryFilteringReasons.WithLabelValues("filtered_safe_search")); got != 1 {
		t.Fatalf("expected safe search reason count 1, got %f", got)
	}
}

func TestUpdateMetricsDoesNotCountSafeBrowsingAsBlockedQuery(t *testing.T) {
	resetMetricsForTest(t)
	collector := NewMetricsCollector()

	collector.UpdateMetrics(queryLogEntry("safebrowsing.example", "A", true, ReasonFilteredSafeBrowsing, 15_000_000, "8.8.8.8"))

	if got := testutil.ToFloat64(BlockedQueries.Counter); got != 0 {
		t.Fatalf("expected safe browsing not to increment blocked query count, got %f", got)
	}
	if got := len(collector.topHosts.GetTop()); got != 0 {
		t.Fatalf("expected safe browsing query not to be counted as allowed host, got %d hosts", got)
	}
	if got := len(collector.topBlockedHosts.GetTop()); got != 0 {
		t.Fatalf("expected safe browsing query not to be counted as blocked host, got %d hosts", got)
	}
	if got := testutil.ToFloat64(QueryFilteringReasons.WithLabelValues("filtered_safe_browsing")); got != 1 {
		t.Fatalf("expected safe browsing reason count 1, got %f", got)
	}
}

func TestUpdateMetricsExcludesUnknownUpstreamFromUpstreamAverages(t *testing.T) {
	resetMetricsForTest(t)
	collector := NewMetricsCollector()

	collector.UpdateMetrics(queryLogEntry("unknown.example", "A", false, ReasonNotFilteredNotFound, 20_000_000, "unknown"))
	collector.UpdateMetrics(queryLogEntry("empty.example", "A", false, ReasonNotFilteredNotFound, 30_000_000, ""))

	if len(collector.upstreamResponseTimes) != 0 {
		t.Fatalf("expected unknown upstreams to be excluded, got %#v", collector.upstreamResponseTimes)
	}
}

func TestUpdateMetricsExcludesCachedQueriesFromUpstreamAverages(t *testing.T) {
	resetMetricsForTest(t)
	collector := NewMetricsCollector()

	entry := queryLogEntry("cached.example", "A", false, ReasonNotFilteredNotFound, 20_000_000, "1.1.1.1")
	entry.Cached = true
	collector.UpdateMetrics(entry)

	if len(collector.upstreamResponseTimes) != 0 {
		t.Fatalf("expected cached query to be excluded from upstream averages, got %#v", collector.upstreamResponseTimes)
	}
	if got := testutil.ToFloat64(AverageResponseTime); got != 20 {
		t.Fatalf("expected cached query to remain in overall response average, got %f", got)
	}
}

func TestUpdateMetricsUsesQueryLogTimestampForRollingAverages(t *testing.T) {
	resetMetricsForTest(t)
	collector := NewMetricsCollector()
	entry := queryLogEntry("old.example", "A", false, ReasonNotFilteredNotFound, 20_000_000, "1.1.1.1")
	entry.Time = time.Now().Add(-10 * time.Minute)

	collector.UpdateMetrics(entry)

	if got := testutil.ToFloat64(DNSQueries.Counter); got != 1 {
		t.Fatalf("expected old query to count toward total DNS queries, got %f", got)
	}
	if got := testutil.ToFloat64(AverageResponseTime); got != 0 {
		t.Fatalf("expected old query not to affect rolling average, got %f", got)
	}
	if len(collector.responseTimes) != 0 {
		t.Fatalf("expected old query to be pruned from response time window, got %#v", collector.responseTimes)
	}
}

func TestProcessMetricsDropsExpiredResponseTimes(t *testing.T) {
	resetMetricsForTest(t)
	collector := NewMetricsCollector()
	now := time.Now().Unix()
	collector.responseTimes = []TimeValue{
		{Time: now - 600, Value: 100},
		{Time: now, Value: 50},
	}
	collector.upstreamResponseTimes["1.1.1.1"] = []TimeValue{
		{Time: now - 600, Value: 100},
		{Time: now, Value: 30},
	}

	collector.ProcessMetrics()

	if len(collector.responseTimes) != 1 {
		t.Fatalf("expected only recent response time to remain, got %#v", collector.responseTimes)
	}
	if collector.responseTimes[0].Value != 50 {
		t.Fatalf("expected recent response value 50, got %f", collector.responseTimes[0].Value)
	}
	if len(collector.upstreamResponseTimes["1.1.1.1"]) != 1 {
		t.Fatalf("expected only recent upstream response time to remain, got %#v", collector.upstreamResponseTimes["1.1.1.1"])
	}
	if got := testutil.ToFloat64(AverageResponseTime); got != 50 {
		t.Fatalf("expected average response time 50ms, got %f", got)
	}
	if got := testutil.ToFloat64(AverageUpstreamResponseTime.WithLabelValues("1.1.1.1")); got != 30 {
		t.Fatalf("expected upstream average response time 30ms, got %f", got)
	}
}

func TestProcessMetricsClearsExpiredResponseTimeGauges(t *testing.T) {
	resetMetricsForTest(t)
	collector := NewMetricsCollector()
	now := time.Now().Unix()
	collector.responseTimes = []TimeValue{{Time: now - 600, Value: 100}}
	collector.upstreamResponseTimes["1.1.1.1"] = []TimeValue{{Time: now - 600, Value: 100}}
	AverageResponseTime.Set(42)
	AverageUpstreamResponseTime.WithLabelValues("1.1.1.1").Set(42)

	collector.ProcessMetrics()

	if len(collector.responseTimes) != 0 {
		t.Fatalf("expected expired response times to be removed, got %#v", collector.responseTimes)
	}
	if _, exists := collector.upstreamResponseTimes["1.1.1.1"]; exists {
		t.Fatalf("expected expired upstream response times to be removed, got %#v", collector.upstreamResponseTimes)
	}
	if got := testutil.ToFloat64(AverageResponseTime); got != 0 {
		t.Fatalf("expected expired average response time gauge to clear to 0, got %f", got)
	}
	if got := testutil.ToFloat64(AverageUpstreamResponseTime.WithLabelValues("1.1.1.1")); got != 0 {
		t.Fatalf("expected expired upstream average response time gauge to clear to 0, got %f", got)
	}
}

func TestStartProcessingClearsExpiredAveragesWithoutNewQueries(t *testing.T) {
	resetMetricsForTest(t)
	collector := NewMetricsCollector()
	collector.responseTimes = []TimeValue{{Time: time.Now().Unix() - 600, Value: 100}}
	AverageResponseTime.Set(42)

	ctx, cancel := context.WithCancel(context.Background())
	done := collector.StartProcessing(ctx, time.Millisecond)
	defer func() {
		cancel()
		<-done
	}()

	deadline := time.After(200 * time.Millisecond)
	for {
		if got := testutil.ToFloat64(AverageResponseTime); got == 0 {
			return
		}

		select {
		case <-deadline:
			t.Fatalf("expected background processing to clear expired average, got %f", testutil.ToFloat64(AverageResponseTime))
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func TestProcessMetricsSerializesTopHostResetAndAdd(t *testing.T) {
	resetMetricsForTest(t)
	collector := NewMetricsCollector()
	for i := range 100 {
		host := fmt.Sprintf("host-%03d.example", i)
		collector.topHosts.Add(host)
	}

	for range 100 {
		var wg sync.WaitGroup
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				collector.ProcessMetrics()
			}()
		}
		wg.Wait()

		for _, hostCount := range collector.topHosts.GetTop() {
			if got := testutil.ToFloat64(TopQueryHosts.WithLabelValues(hostCount.Host)); got != float64(hostCount.Count) {
				t.Fatalf("expected serialized top-host metric for %s to equal %d, got %f", hostCount.Host, hostCount.Count, got)
			}
		}
	}
}

func queryLogEntry(host, queryType string, blocked bool, reason FilteringReason, elapsedNs int64, upstream string) QueryLogEntry {
	return QueryLogEntry{
		Time:     time.Now(),
		QHost:    host,
		QType:    queryType,
		Elapsed:  int64Ptr(elapsedNs),
		Upstream: upstream,
		Result: &QueryLogResult{
			IsFiltered: blocked,
			Reason:     filteringReasonPtr(reason),
		},
	}
}

func int64Ptr(value int64) *int64 {
	return &value
}

func filteringReasonPtr(reason FilteringReason) *FilteringReason {
	return &reason
}

func resetMetricsForTest(t *testing.T) {
	t.Helper()

	oldDNSQueries := DNSQueries
	oldBlockedQueries := BlockedQueries
	oldQueryTypes := QueryTypes
	oldTopQueryHosts := TopQueryHosts
	oldTopBlockedQueryHosts := TopBlockedQueryHosts
	oldSafeSearchEnforcedHosts := SafeSearchEnforcedHosts
	oldQueryFilteringReasons := QueryFilteringReasons
	oldQueryLogEntriesSkipped := QueryLogEntriesSkipped
	oldAverageResponseTime := AverageResponseTime
	oldAverageUpstreamResponseTime := AverageUpstreamResponseTime
	t.Cleanup(func() {
		DNSQueries = oldDNSQueries
		BlockedQueries = oldBlockedQueries
		QueryTypes = oldQueryTypes
		TopQueryHosts = oldTopQueryHosts
		TopBlockedQueryHosts = oldTopBlockedQueryHosts
		SafeSearchEnforcedHosts = oldSafeSearchEnforcedHosts
		QueryFilteringReasons = oldQueryFilteringReasons
		QueryLogEntriesSkipped = oldQueryLogEntriesSkipped
		AverageResponseTime = oldAverageResponseTime
		AverageUpstreamResponseTime = oldAverageUpstreamResponseTime
	})

	DNSQueries = NewCustomCounter(prometheus.CounterOpts{
		Name: "agh_dns_queries_total",
		Help: "Total number of DNS queries",
	})
	BlockedQueries = NewCustomCounter(prometheus.CounterOpts{
		Name: "agh_blocked_dns_queries_total",
		Help: "Total number of blocked DNS queries",
	})
	QueryTypes = NewCustomCounterVec(prometheus.CounterOpts{
		Name: "agh_dns_query_types_total",
		Help: "Types of DNS queries",
	}, []string{"query_type"})
	TopQueryHosts = NewCustomCounterVec(prometheus.CounterOpts{
		Name: "agh_dns_query_hosts_total",
		Help: "Top DNS query hosts",
	}, []string{"host"})
	TopBlockedQueryHosts = NewCustomCounterVec(prometheus.CounterOpts{
		Name: "agh_blocked_dns_query_hosts_total",
		Help: "Top blocked DNS query hosts",
	}, []string{"host"})
	SafeSearchEnforcedHosts = NewCustomCounterVec(prometheus.CounterOpts{
		Name: "agh_safe_search_enforced_hosts_total",
		Help: "Safe search enforced hosts",
	}, []string{"host"})
	QueryFilteringReasons = NewCustomCounterVec(prometheus.CounterOpts{
		Name: "agh_dns_filtering_reason_total",
		Help: "DNS query filtering reasons",
	}, []string{"reason"})
	QueryLogEntriesSkipped = NewCustomCounterVec(prometheus.CounterOpts{
		Name: "agh_querylog_entries_skipped_total",
		Help: "Query log entries skipped by reason",
	}, []string{"reason"})
	AverageResponseTime = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "agh_dns_average_response_time",
		Help: "Average response time for DNS queries in milliseconds",
	})
	AverageUpstreamResponseTime = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "agh_dns_average_upstream_response_time",
		Help: "Average response time by upstream server",
	}, []string{"server"})
}
