package telemetry

import (
	"context"
	"fmt"
	"go.opentelemetry.io/api/core"
	"go.opentelemetry.io/sdk/export"
	"io"
	"os"
)

// Options are the options to be used when initializing a stdout export.
type PromOptions struct {
	// PrettyPrint will pretty the json representation of the span,
	// making it print "pretty". Default is false.
	PrettyPrint bool
}

// Exporter is an implementation of trace.Exporter that writes spans to stdout.
type PromExporter struct {
	pretty       bool
	outputWriter io.Writer
	metrics      *Metrics
}

func NewPromExporter(m *Metrics, o PromOptions) (*PromExporter, error) {
	return &PromExporter{
		pretty:       o.PrettyPrint,
		outputWriter: os.Stdout,
		metrics:      m,
	}, nil
}

// ExportSpan writes a SpanData in json format to stdout.
func (e *PromExporter) ExportSpan(ctx context.Context, data *export.SpanData) {
	startTime := data.StartTime
	endTime := data.EndTime
	spanName := data.Name

	kind := attrGetKind(data.Attributes)

	labelValues := []string{}
	for i := 0; i < len(e.metrics.labelNames)-1; i++ {
		labelName := e.metrics.labelNames[i]
		if labelName == kind {
			break
		}
		labelValues = append(labelValues, contextGetNameForKind(ctx, labelName))
	}
	labelValues = append(labelValues, spanName)

	e.metrics.kindToMetricSets[kind].Observe(startTime, endTime, data.Status.String(), labelValues)
}

func attrGetKind(attrs []core.KeyValue) string {
	m := coreAttributesToMap(attrs)

	kind := m[AttributeKeyKind]

	return kind
}

func contextGetNameForKind(ctx context.Context, kind string) string {
	name, _ := ctx.Value(kindToContextKey(kind)).(string)
	return name
}

func coreAttributesToMap(kvs []core.KeyValue) map[string]string {
	m := map[string]string{}
	for _, kv := range kvs {
		switch kv.Value.Type {
		case core.STRING:
			m[kv.Key.Name] = kv.Value.String
		case core.BOOL:
			m[kv.Key.Name] = fmt.Sprintf("%v", kv.Value.Bool)
		case core.INT32, core.INT64:
			m[kv.Key.Name] = fmt.Sprintf("%d", kv.Value.Int64)
		case core.FLOAT32, core.FLOAT64:
			m[kv.Key.Name] = fmt.Sprintf("%f", kv.Value.Float64)
		}
	}
	return m
}
