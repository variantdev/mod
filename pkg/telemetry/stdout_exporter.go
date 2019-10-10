package telemetry

import (
	"context"
	"fmt"
	"go.opentelemetry.io/sdk/export"
	"io"
	"os"
)

// Options are the options to be used when initializing a stdout export.
type Options struct {
	// PrettyPrint will pretty the json representation of the span,
	// making it print "pretty". Default is false.
	PrettyPrint bool
}

// Exporter is an implementation of trace.Exporter that writes spans to stdout.
type Exporter struct {
	pretty       bool
	outputWriter io.Writer
}

func NewExporter(o Options) (*Exporter, error) {
	return &Exporter{
		pretty:       o.PrettyPrint,
		outputWriter: os.Stdout,
	}, nil
}

// ExportSpan writes a SpanData in json format to stdout.
func (e *Exporter) ExportSpan(ctx context.Context, data *export.SpanData) {
	traceId := data.SpanContext.TraceIDString()
	spanId := data.SpanContext.SpanIDString()
	spanName := data.Name
	parentSpanId := fmt.Sprintf("%.16x", data.ParentSpanID)
	startTime := data.StartTime.String()
	endTime := data.EndTime.String()

	jsonSpan := []byte(fmt.Sprintf(`{"traceId":%q,"parentSpanId":%q,"spanId":%q,"spanName":%q,"startTime":%q,"endTime":%q}`, traceId, parentSpanId, spanId, spanName, startTime, endTime))

	_, _ = e.outputWriter.Write(append(jsonSpan, byte('\n')))
}
