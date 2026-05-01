package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestUpdateMetricsTracksAllowedQuery(t *testing.T) {
	resetMetricsForTest(t)
	collector := NewMetricsCollector()

	collector.UpdateMetrics(queryLogEntry("allowed.example", "A", false, float64(0), 25_000_000, "1.1.1.1"))

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

func TestUpdateMetricsTracksBlockedQuerySeparately(t *testing.T) {
	resetMetricsForTest(t)
	collector := NewMetricsCollector()

	collector.UpdateMetrics(queryLogEntry("blocked.example", "AAAA", true, float64(2), 10_000_000, "9.9.9.9"))

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
}

func TestUpdateMetricsTracksSafeSearchSeparatelyFromBlockedQueries(t *testing.T) {
	resetMetricsForTest(t)
	collector := NewMetricsCollector()

	collector.UpdateMetrics(queryLogEntry("safe.example", "A", true, float64(7), 15_000_000, "8.8.8.8"))

	if got := testutil.ToFloat64(SafeSearchEnforcedHosts.WithLabelValues("safe.example")); got != 1 {
		t.Fatalf("expected safe search host count 1, got %f", got)
	}
	if got := testutil.ToFloat64(BlockedQueries.Counter); got != 0 {
		t.Fatalf("expected safe search query not to increment blocked query count, got %f", got)
	}
	if got := len(collector.topBlockedHosts.GetTop()); got != 0 {
		t.Fatalf("expected safe search query not to be counted as blocked host, got %d hosts", got)
	}
}

func TestUpdateMetricsExcludesUnknownUpstreamFromUpstreamAverages(t *testing.T) {
	resetMetricsForTest(t)
	collector := NewMetricsCollector()

	collector.UpdateMetrics(queryLogEntry("unknown.example", "A", false, float64(0), 20_000_000, "unknown"))
	collector.UpdateMetrics(queryLogEntry("empty.example", "A", false, float64(0), 30_000_000, ""))

	if len(collector.upstreamResponseTimes) != 0 {
		t.Fatalf("expected unknown upstreams to be excluded, got %#v", collector.upstreamResponseTimes)
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

func queryLogEntry(host, queryType string, blocked bool, reason interface{}, elapsedNs float64, upstream string) map[string]interface{} {
	return map[string]interface{}{
		"QH":       host,
		"QT":       queryType,
		"Elapsed":  elapsedNs,
		"Upstream": upstream,
		"Result": map[string]interface{}{
			"IsFiltered": blocked,
			"Reason":     reason,
		},
	}
}

func resetMetricsForTest(t *testing.T) {
	t.Helper()

	oldDNSQueries := DNSQueries
	oldBlockedQueries := BlockedQueries
	oldQueryTypes := QueryTypes
	oldTopQueryHosts := TopQueryHosts
	oldTopBlockedQueryHosts := TopBlockedQueryHosts
	oldSafeSearchEnforcedHosts := SafeSearchEnforcedHosts
	oldAverageResponseTime := AverageResponseTime
	oldAverageUpstreamResponseTime := AverageUpstreamResponseTime
	t.Cleanup(func() {
		DNSQueries = oldDNSQueries
		BlockedQueries = oldBlockedQueries
		QueryTypes = oldQueryTypes
		TopQueryHosts = oldTopQueryHosts
		TopBlockedQueryHosts = oldTopBlockedQueryHosts
		SafeSearchEnforcedHosts = oldSafeSearchEnforcedHosts
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
	AverageResponseTime = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "agh_dns_average_response_time",
		Help: "Average response time for DNS queries in milliseconds",
	})
	AverageUpstreamResponseTime = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "agh_dns_average_upstream_response_time",
		Help: "Average response time by upstream server",
	}, []string{"server"})
}
