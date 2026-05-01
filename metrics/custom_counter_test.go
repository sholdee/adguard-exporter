package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestCustomCounterAdd(t *testing.T) {
	counter := NewCustomCounter(prometheus.CounterOpts{
		Name: "test_custom_counter_total",
		Help: "Test custom counter",
	})

	counter.Add(3)

	if got := testutil.ToFloat64(counter.Counter); got != 3 {
		t.Fatalf("expected counter value 3, got %f", got)
	}
}

func TestCustomCounterVecWithLabels(t *testing.T) {
	counterVec := NewCustomCounterVec(prometheus.CounterOpts{
		Name: "test_custom_counter_vec_total",
		Help: "Test custom counter vec",
	}, []string{"label"})

	counterVec.With(prometheus.Labels{"label": "value"}).Add(4)

	if got := testutil.ToFloat64(counterVec.CounterVec.WithLabelValues("value")); got != 4 {
		t.Fatalf("expected labeled counter value 4, got %f", got)
	}
}
