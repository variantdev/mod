package main

import (
	"context"
	"fmt"
	"github.com/variantdev/mod/pkg/telemetry"
	"go.opentelemetry.io/api/key"
	"time"
)

func main() {
	jobName := "variantapp"

	SpanKindTask := telemetry.SpanKind("task")
	SpanKindStep := telemetry.SpanKind("step")

	t := telemetry.New(jobName, []telemetry.SpanKind{SpanKindTask, SpanKindStep})

	for i := 0; i < 3; i++ {
		t.WithSpan(context.Background(), SpanKindTask, fmt.Sprintf("mytask-%d", i+1),
			func(ctx context.Context) error {
				t.Info(ctx, "foo starting")
				t.WithSpan(ctx, SpanKindStep, "bar",
					func(ctx context.Context) error {
						t.Info(ctx, "bar starting")
						return nil
					},
				)
				t.WithSpan(ctx, SpanKindStep, "baz",
					func(ctx context.Context) error {
						t.Info(ctx, "baz starting")
						t.AddTraceEvent(ctx, "FOO", key.New("fookey1").String("fooval1"))
						time.Sleep(time.Duration(i+1) * time.Second)
						return nil
					},
				)
				return nil
			},
		)
	}

	// Pre-requisite: docker run --rm -p 9091:9091 prom/pushgateway
	if err := t.Push(); err != nil {
		t.Error(context.Background(), fmt.Sprintf("%v", err))
		panic(err)
	}

	// Run `curl http:/localhost:9091/metrics` to see pushed metrics
}
