package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

type CustomCounterVec struct {
	CounterVec *prometheus.CounterVec
	CreatedVec *prometheus.GaugeVec
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
	}
}

func (ccv *CustomCounterVec) WithLabelValues(lvs ...string) prometheus.Counter {
	ccv.CreatedVec.WithLabelValues(lvs...).SetToCurrentTime()
	return ccv.CounterVec.WithLabelValues(lvs...)
}

func (ccv *CustomCounterVec) With(labels prometheus.Labels) prometheus.Counter {
	ccv.CreatedVec.With(labels).SetToCurrentTime()
	return ccv.CounterVec.With(labels)
}
