package metrics

import (
	"strconv"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

type CustomCounterVec struct {
	CounterVec *prometheus.CounterVec
	CreatedVec *prometheus.GaugeVec
	created    map[string]struct{}
	labelNames []string
	lock       sync.Mutex
}

func NewCustomCounterVec(opts prometheus.CounterOpts, labelNames []string) *CustomCounterVec {
	labelNamesCopy := append([]string(nil), labelNames...)
	counterVec := prometheus.NewCounterVec(opts, labelNamesCopy)
	createdOpts := prometheus.GaugeOpts{
		Name: opts.Name[:len(opts.Name)-6] + "_created",
		Help: opts.Help + " (created timestamp)",
	}
	createdVec := prometheus.NewGaugeVec(createdOpts, labelNamesCopy)
	return &CustomCounterVec{
		CounterVec: counterVec,
		CreatedVec: createdVec,
		created:    make(map[string]struct{}),
		labelNames: labelNamesCopy,
	}
}

func (ccv *CustomCounterVec) WithLabelValues(lvs ...string) prometheus.Counter {
	ccv.setCreatedOnce(labelValuesKey(ccv.labelNames, lvs), func() {
		ccv.CreatedVec.WithLabelValues(lvs...).SetToCurrentTime()
	})
	return ccv.CounterVec.WithLabelValues(lvs...)
}

func (ccv *CustomCounterVec) With(labels prometheus.Labels) prometheus.Counter {
	ccv.setCreatedOnce(labelsKey(ccv.labelNames, labels), func() {
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

func labelValuesKey(labelNames []string, values []string) string {
	var builder strings.Builder
	for index, labelName := range labelNames {
		value := ""
		if index < len(values) {
			value = values[index]
		}
		writeLabelKeyPart(&builder, labelName, value)
	}
	if len(values) > len(labelNames) {
		for _, value := range values[len(labelNames):] {
			writeLabelKeyPart(&builder, "", value)
		}
	}
	return builder.String()
}

func labelsKey(labelNames []string, labels prometheus.Labels) string {
	var builder strings.Builder
	for _, labelName := range labelNames {
		writeLabelKeyPart(&builder, labelName, labels[labelName])
	}
	return builder.String()
}

func writeLabelKeyPart(builder *strings.Builder, labelName, value string) {
	builder.WriteString(strconv.Itoa(len(labelName)))
	builder.WriteByte(':')
	builder.WriteString(labelName)
	builder.WriteByte('=')
	builder.WriteString(strconv.Itoa(len(value)))
	builder.WriteByte(':')
	builder.WriteString(value)
	builder.WriteByte(';')
}
