package metrics

import (
	"context"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
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
		Help: "Average query processing time for queries sent to each upstream server in milliseconds",
	}, []string{"server"})
)

type MetricsCollector struct {
	topHosts              *TopHosts
	topBlockedHosts       *TopHosts
	responseTimes         []TimeValue
	upstreamResponseTimes map[string][]TimeValue
	windowSize            int64
	processLock           sync.Mutex
	lock                  sync.Mutex
}

func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		topHosts:              NewTopHosts(100),
		topBlockedHosts:       NewTopHosts(100),
		responseTimes:         make([]TimeValue, 0),
		upstreamResponseTimes: make(map[string][]TimeValue),
		windowSize:            300, // 5 minute window
	}
}

func (mc *MetricsCollector) UpdateMetrics(entry QueryLogEntry) {
	if skipReason := entry.SkipReason(); skipReason != "" {
		QueryLogEntriesSkipped.WithLabelValues(skipReason).Inc()
		return
	}
	entry.Normalize()
	currentTime := entry.Timestamp().Unix()
	reason := *entry.Result.Reason

	DNSQueries.Inc()
	QueryTypes.WithLabelValues(entry.QType).Inc()
	QueryFilteringReasons.WithLabelValues(reason.Label()).Inc()

	if !entry.Result.IsFiltered {
		mc.topHosts.Add(entry.QHost)
	}

	elapsedMs := entry.ElapsedMilliseconds()

	mc.lock.Lock()
	mc.responseTimes = append(mc.responseTimes, TimeValue{Time: currentTime, Value: elapsedMs})
	// Exclude metrics with unknown upstreams
	if !entry.Cached && entry.Upstream != "unknown" {
		mc.upstreamResponseTimes[entry.Upstream] = append(mc.upstreamResponseTimes[entry.Upstream], TimeValue{Time: currentTime, Value: elapsedMs})
	}
	mc.lock.Unlock()

	if reason.IsSafeSearch() {
		SafeSearchEnforcedHosts.WithLabelValues(entry.QHost).Inc()
	} else if reason.IsBlocked() {
		BlockedQueries.Inc()
		mc.topBlockedHosts.Add(entry.QHost)
	}

	mc.ProcessMetrics()
}

func (mc *MetricsCollector) StartProcessing(ctx context.Context, interval time.Duration) <-chan struct{} {
	done := make(chan struct{})
	if interval <= 0 {
		close(done)
		return done
	}

	ticker := time.NewTicker(interval)
	go func() {
		defer close(done)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				mc.ProcessMetrics()
			case <-ctx.Done():
				return
			}
		}
	}()
	return done
}

func (mc *MetricsCollector) ProcessMetrics() {
	mc.processLock.Lock()
	defer mc.processLock.Unlock()

	currentTime := time.Now().Unix()
	cutoffTime := currentTime - mc.windowSize

	TopQueryHosts.CounterVec.Reset()
	for _, hc := range mc.topHosts.GetTop() {
		TopQueryHosts.WithLabelValues(hc.Host).Add(float64(hc.Count))
	}

	TopBlockedQueryHosts.CounterVec.Reset()
	for _, hc := range mc.topBlockedHosts.GetTop() {
		TopBlockedQueryHosts.WithLabelValues(hc.Host).Add(float64(hc.Count))
	}

	mc.lock.Lock()
	defer mc.lock.Unlock()

	recentResponseTimes := mc.filterRecentTimes(mc.responseTimes, cutoffTime)
	if len(recentResponseTimes) > 0 {
		avgResponseTime := mc.calculateAverage(recentResponseTimes)
		AverageResponseTime.Set(avgResponseTime)
	} else {
		AverageResponseTime.Set(0)
	}

	recentUpstreamResponseTimes := make(map[string][]TimeValue, len(mc.upstreamResponseTimes))
	for upstream, times := range mc.upstreamResponseTimes {
		if upstream == "unknown" || upstream == "" {
			continue // Skip processing for unknown upstreams
		}
		recentTimes := mc.filterRecentTimes(times, cutoffTime)
		if len(recentTimes) > 0 {
			avgUpstreamTime := mc.calculateAverage(recentTimes)
			AverageUpstreamResponseTime.WithLabelValues(upstream).Set(avgUpstreamTime)
			recentUpstreamResponseTimes[upstream] = recentTimes
		} else {
			AverageUpstreamResponseTime.DeleteLabelValues(upstream)
		}
	}

	mc.responseTimes = recentResponseTimes
	mc.upstreamResponseTimes = recentUpstreamResponseTimes
}

func (mc *MetricsCollector) filterRecentTimes(times []TimeValue, cutoffTime int64) []TimeValue {
	var recent []TimeValue
	for _, tv := range times {
		if tv.Time > cutoffTime {
			recent = append(recent, tv)
		}
	}
	return recent
}

func (mc *MetricsCollector) calculateAverage(times []TimeValue) float64 {
	var sum float64
	for _, tv := range times {
		sum += tv.Value
	}
	return sum / float64(len(times))
}
