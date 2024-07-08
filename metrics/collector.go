package metrics

import (
	"fmt"
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
	// Gauges remain unchanged
	AverageResponseTime = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "agh_dns_average_response_time",
		Help: "Average response time for DNS queries in milliseconds",
	})
	AverageUpstreamResponseTime = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "agh_dns_average_upstream_response_time",
		Help: "Average response time by upstream server",
	}, []string{"server"})
)

type MetricsCollector struct {
	topHosts              *TopHosts
	topBlockedHosts       *TopHosts
	responseTimes         []TimeValue
	upstreamResponseTimes map[string][]TimeValue
	windowSize            int64
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

func (mc *MetricsCollector) UpdateMetrics(data map[string]interface{}) {
	currentTime := time.Now().Unix()
	host, _ := data["QH"].(string)
	queryType, _ := data["QT"].(string)
	result, _ := data["Result"].(map[string]interface{})
	isBlocked, _ := result["IsFiltered"].(bool)
	resultReason := fmt.Sprintf("%v", result["Reason"])
	elapsedNs, _ := data["Elapsed"].(float64)
	upstream, _ := data["Upstream"].(string)

	DNSQueries.Inc()
	QueryTypes.WithLabelValues(queryType).Inc()

	if !isBlocked {
		mc.topHosts.Add(host)
	}

	elapsedMs := elapsedNs / 1_000_000 // Convert nanoseconds to milliseconds

	mc.lock.Lock()
	mc.responseTimes = append(mc.responseTimes, TimeValue{Time: currentTime, Value: elapsedMs})
	// Exclude metrics with unknown upstreams
	if upstream != "unknown" && upstream != "" {
		mc.upstreamResponseTimes[upstream] = append(mc.upstreamResponseTimes[upstream], TimeValue{Time: currentTime, Value: elapsedMs})
	}
	mc.lock.Unlock()

	if isBlocked && resultReason == "7" {
		SafeSearchEnforcedHosts.WithLabelValues(host).Inc()
	} else if isBlocked {
		BlockedQueries.Inc()
		mc.topBlockedHosts.Add(host)
	}

	mc.ProcessMetrics()
}

func (mc *MetricsCollector) ProcessMetrics() {
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
	}

	for upstream, times := range mc.upstreamResponseTimes {
		if upstream == "unknown" || upstream == "" {
			continue // Skip processing for unknown upstreams
		}
		recentTimes := mc.filterRecentTimes(times, cutoffTime)
		if len(recentTimes) > 0 {
			avgUpstreamTime := mc.calculateAverage(recentTimes)
			AverageUpstreamResponseTime.WithLabelValues(upstream).Set(avgUpstreamTime)
		}
	}

	mc.responseTimes = recentResponseTimes
	for upstream := range mc.upstreamResponseTimes {
		mc.upstreamResponseTimes[upstream] = mc.filterRecentTimes(mc.upstreamResponseTimes[upstream], cutoffTime)
	}
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
