package telemetry

import (
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"time"
)

type MetricSet struct {
	LabelNames              []string
	StartedCounter          *prometheus.CounterVec
	HandledCounter          *prometheus.CounterVec
	HandledHistogramEnabled bool
	HandledHistogramOpts    prometheus.HistogramOpts
	HandledHistogram        *prometheus.HistogramVec
}

func NewMetricSet(app string, labelNames []string, counterOpts ...CounterOption) *MetricSet {
	opts := counterOptions(counterOpts)
	tpe := labelNames[len(labelNames)-1]
	return &MetricSet{
		LabelNames: labelNames,
		StartedCounter: prometheus.NewCounterVec(
			opts.apply(prometheus.CounterOpts{
				Name: fmt.Sprintf("%s_%s_started_total", app, tpe),
				Help: "Total number of operations started.",
			}), labelNames),
		HandledCounter: prometheus.NewCounterVec(
			opts.apply(prometheus.CounterOpts{
				Name: fmt.Sprintf("%s_%s_handled_total", app, tpe),
				Help: "Total number of operations completed, regardless of success or failure.",
			}), append(append([]string{}, labelNames...), "status")),
		HandledHistogramEnabled: false,
		HandledHistogramOpts: prometheus.HistogramOpts{
			Name:    fmt.Sprintf("%s_%s_handling_seconds", app, tpe),
			Help:    "Histogram of response latency (seconds) of operation.",
			Buckets: prometheus.DefBuckets,
		},
		HandledHistogram: nil,
	}
}

// EnableHandlingTimeHistogram enables histograms being registered when
// registering the Metrics on a Prometheus registry. Histograms can be
// expensive on Prometheus servers. It takes options to configure histogram
// options such as the defined buckets.
func (m *MetricSet) EnableHandlingTimeHistogram(opts ...HistogramOption) {
	for _, o := range opts {
		o(&m.HandledHistogramOpts)
	}
	if !m.HandledHistogramEnabled {
		m.HandledHistogram = prometheus.NewHistogramVec(
			m.HandledHistogramOpts,
			m.LabelNames,
		)
	}
	m.HandledHistogramEnabled = true
}

func (m *MetricSet) Observe(startTime, endTime time.Time, status string, labelValues []string) {
	m.StartedCounter.WithLabelValues(labelValues...).Inc()
	counterLabels := append([]string{}, labelValues...)
	counterLabels = append(counterLabels, status)
	m.HandledCounter.WithLabelValues(counterLabels...).Inc()
	m.HandledHistogram.WithLabelValues(labelValues...).Observe(endTime.Sub(startTime).Seconds())
}

// Describe sends the super-set of all possible descriptors of metrics
// collected by this Collector to the provided channel and returns once
// the last descriptor has been sent.
func (m *MetricSet) Describe(ch chan<- *prometheus.Desc) {
	m.StartedCounter.Describe(ch)
	m.HandledCounter.Describe(ch)
	if m.HandledHistogramEnabled {
		m.HandledHistogram.Describe(ch)
	}
}

// Collect is called by the Prometheus registry when collecting
// metrics. The implementation sends each collected metric via the
// provided channel and returns once the last metric has been sent.
func (m *MetricSet) Collect(ch chan<- prometheus.Metric) {
	m.StartedCounter.Collect(ch)
	m.HandledCounter.Collect(ch)
	if m.HandledHistogramEnabled {
		m.HandledHistogram.Collect(ch)
	}
}
