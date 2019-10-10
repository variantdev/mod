package telemetry

import (
	"go.opentelemetry.io/sdk/trace"
	"log"
)

func SetupDefaultGlobalTracer(m *Metrics) {
	trace.Register()

	stdoutExporter, err := NewExporter(Options{PrettyPrint: false})
	if err != nil {
		log.Fatal(err)
	}

	ssp := trace.NewSimpleSpanProcessor(stdoutExporter)
	trace.RegisterSpanProcessor(ssp)

	promExporter, err := NewPromExporter(m, PromOptions{PrettyPrint: false})
	if err != nil {
		log.Fatal(err)
	}
	ssp2 := trace.NewSimpleSpanProcessor(promExporter)
	trace.RegisterSpanProcessor(ssp2)

	// For the demonstration, use sdktrace.AlwaysSample sampler to sample all traces.
	// In a production application, use sdktrace.ProbabilitySampler with a desired probability.
	trace.ApplyConfig(trace.Config{DefaultSampler: trace.AlwaysSample()})
}
