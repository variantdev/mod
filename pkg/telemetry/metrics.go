// See:
//   https://godoc.org/github.com/prometheus/client_golang/prometheus/push#Pusher.Push
//   https://prometheus.io/docs/instrumenting/pushing/
package telemetry

import (
	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
)

// Metrics represents a collection of metrics to be registered on a
// Prometheus metrics registry for a gRPC server.
type Metrics struct {
	labelNames []string
	kindToMetricSets map[string]*MetricSet
}

// NewMetrics returns a Metrics object. Use a new instance of
// Metrics when not using the default Prometheus metrics registry, for
// example when wanting to control which metrics are added to a registry as
// opposed to automatically adding metrics via init functions.
//
// Recommended usage is with https://github.com/weaveworks/prom-aggregation-gateway for aggregating counts and histograms
func NewMetrics(name string, labelNames []string, counterOpts ...CounterOption) *Metrics {
	metricsets := map[string]*MetricSet{}
	for i := 0; i < len(labelNames); i++ {
		l := labelNames[i]
		metricsets[l] = NewMetricSet(name, labelNames[:i+1], counterOpts...)
	}

	return &Metrics{
		labelNames:       labelNames,
		kindToMetricSets: metricsets,
	}
}

// EnableHandlingTimeHistogram enables histograms being registered when
// registering the Metrics on a Prometheus registry. Histograms can be
// expensive on Prometheus servers. It takes options to configure histogram
// options such as the defined buckets.
func (m *Metrics) EnableHandlingTimeHistogram(opts ...HistogramOption) {
	for _, ms := range m.kindToMetricSets {
		ms.EnableHandlingTimeHistogram(opts...)
	}
}

// Describe sends the super-set of all possible descriptors of metrics
// collected by this Collector to the provided channel and returns once
// the last descriptor has been sent.
func (m *Metrics) Describe(ch chan<- *prom.Desc) {
	for _, ms := range m.kindToMetricSets {
		ms.Describe(ch)
	}
}

// Collect is called by the Prometheus registry when collecting
// metrics. The implementation sends each collected metric via the
// provided channel and returns once the last metric has been sent.
func (m *Metrics) Collect(ch chan<- prom.Metric) {
	for _, ms := range m.kindToMetricSets {
		ms.Collect(ch)
	}
}

// pushBase can be something like http://pushgateway:9091 (for pushgateway)
// or http://pushgateway:9091/api/ui (for weaveworks/prom-aggregation-gateway)
func (m *Metrics) Push(pushBase, job string) error {
	return push.New(pushBase, job).
		Collector(m).
		//Grouping("push", "g1").
		Push()
}
