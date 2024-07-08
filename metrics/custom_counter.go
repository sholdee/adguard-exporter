package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

type CustomCounter struct {
	Counter prometheus.Counter
	Created prometheus.Gauge
}

func NewCustomCounter(opts prometheus.CounterOpts) *CustomCounter {
	counter := prometheus.NewCounter(opts)
	createdOpts := prometheus.GaugeOpts{
		Name: opts.Name[:len(opts.Name)-6] + "_created",
		Help: opts.Help + " (created timestamp)",
	}
	created := prometheus.NewGauge(createdOpts)
	created.SetToCurrentTime()
	return &CustomCounter{
		Counter: counter,
		Created: created,
	}
}

func (cc *CustomCounter) Inc() {
	cc.Counter.Inc()
}

func (cc *CustomCounter) Add(v float64) {
	cc.Counter.Add(v)
}
