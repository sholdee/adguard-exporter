package metrics

import (
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

type CustomCounterVec struct {
	CounterVec *prometheus.CounterVec
	CreatedVec *prometheus.GaugeVec
	created    map[string]struct{}
	lock       sync.Mutex
}

func NewCustomCounterVec(opts prometheus.CounterOpts, labelNames []string) *CustomCounterVec {
	counterVec := prometheus.NewCounterVec(opts, labelNames)
	createdOpts := prometheus.GaugeOpts{
		Name: opts.Name[:len(opts.Name)-6] + "_created",
		Help: opts.Help + " (created timestamp)",
	}
	createdVec := prometheus.NewGaugeVec(createdOpts, labelNames)
	return &CustomCounterVec{
		CounterVec: counterVec,
		CreatedVec: createdVec,
		created:    make(map[string]struct{}),
	}
}

func (ccv *CustomCounterVec) WithLabelValues(lvs ...string) prometheus.Counter {
	ccv.setCreatedOnce(labelValuesKey(lvs), func() {
		ccv.CreatedVec.WithLabelValues(lvs...).SetToCurrentTime()
	})
	return ccv.CounterVec.WithLabelValues(lvs...)
}

func (ccv *CustomCounterVec) With(labels prometheus.Labels) prometheus.Counter {
	ccv.setCreatedOnce(labelsKey(labels), func() {
		ccv.CreatedVec.With(labels).SetToCurrentTime()
	})
	return ccv.CounterVec.With(labels)
}

func (ccv *CustomCounterVec) setCreatedOnce(key string, setCreated func()) {
	ccv.lock.Lock()
	defer ccv.lock.Unlock()

	if _, exists := ccv.created[key]; exists {
		return
	}
	setCreated()
	ccv.created[key] = struct{}{}
}

func labelValuesKey(values []string) string {
	var builder strings.Builder
	for _, value := range values {
		builder.WriteString(strconv.Itoa(len(value)))
		builder.WriteByte(':')
		builder.WriteString(value)
		builder.WriteByte(';')
	}
	return builder.String()
}

func labelsKey(labels prometheus.Labels) string {
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var builder strings.Builder
	for _, key := range keys {
		builder.WriteString(strconv.Itoa(len(key)))
		builder.WriteByte(':')
		builder.WriteString(key)
		builder.WriteByte('=')
		value := labels[key]
		builder.WriteString(strconv.Itoa(len(value)))
		builder.WriteByte(':')
		builder.WriteString(value)
		builder.WriteByte(';')
	}
	return builder.String()
}
