package observability

import "github.com/prometheus/client_golang/prometheus"

type Metrics struct {
	Operations *prometheus.CounterVec
	Duration   *prometheus.HistogramVec
	QueueDepth prometheus.Gauge
}

func New(reg prometheus.Registerer) *Metrics {
	m := &Metrics{Operations: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "chatops_operations_total", Help: "Operations by kind and result."}, []string{"kind", "result"}), Duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "chatops_operation_duration_seconds", Help: "Kubernetes operation duration.", Buckets: prometheus.DefBuckets}, []string{"kind"}), QueueDepth: prometheus.NewGauge(prometheus.GaugeOpts{Name: "chatops_queue_depth", Help: "Queued operations."})}
	reg.MustRegister(m.Operations, m.Duration, m.QueueDepth)
	return m
}
