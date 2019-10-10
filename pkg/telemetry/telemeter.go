package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/api/core"
	"go.opentelemetry.io/api/trace"
	"google.golang.org/grpc/codes"
	"io"
	"log"
	"os"
)

func New(jobName string, kinds []SpanKind) *Telemeter {
	labelNames := make([]string, len(kinds))
	for i := range kinds {
		labelNames[i] = string(kinds[i])
	}

	infoLog := log.New(&writer{w: os.Stdout,}, "", log.Ldate|log.Ltime|log.Lshortfile)
	errLog := log.New(&writer{w: os.Stderr,}, "", log.Ldate|log.Ltime|log.Lshortfile)
	debugLog := log.New(&writer{w: os.Stderr,}, "", log.Ldate|log.Ltime|log.Lshortfile)

	r := &Telemeter{
		jobName:          jobName,
		labelNames:       labelNames,
		nonLeafKinds:     labelNamesToNonLeafKinds(labelNames),
		kinds:            labelNamesToKinds(labelNames),
		promPushEndpoint: "http://localhost:9091",
		InfoLog:          infoLog,
		ErrorLog:         errLog,
		DebugLog:         debugLog,
		CurrentSpan:      trace.CurrentSpan,
		Print: func(log *log.Logger, msg string, kvs ...string) {
			m := make(map[string]interface{}, len(kvs)/2+1)
			m["message"] = msg
			for i := 0; i+1 < len(kvs); i += 2 {
				m[kvs[i]] = kvs[i+1]
			}
			j, err := json.Marshal(m)
			if err != nil {
				panic(err)
			}
			log.Output(4, string(j))
		},
	}

	m := NewMetrics(jobName, labelNames, WithConstLabels(prometheus.Labels{"constkey": "v"}))
	m.EnableHandlingTimeHistogram()

	r.Metrics = m

	SetupDefaultGlobalTracer(m)

	r.Tracer = trace.GlobalTracer()

	return r
}

type Telemeter struct {
	jobName          string
	promPushEndpoint string

	labelNames   []string
	nonLeafKinds map[string]struct{}
	kinds        map[string]struct{}

	Tracer trace.Tracer

	Metrics *Metrics

	InfoLog  *log.Logger
	ErrorLog *log.Logger
	DebugLog *log.Logger

	CurrentSpan func(ctx context.Context) trace.Span

	Print func(*log.Logger, string, ...string)
}

func (r *Telemeter) Info(ctx context.Context, msg string, kvs ...string) {
	r.print(r.InfoLog, ctx, msg, "level", "info")
}

func (r *Telemeter) Error(ctx context.Context, msg string, kvs ...string) {
	r.print(r.ErrorLog, ctx, msg, "level", "error")
}

func (r *Telemeter) Debug(ctx context.Context, msg string, kvs ...string) {
	r.print(r.DebugLog, ctx, msg, "level", "debug")
}

func (r *Telemeter) print(log *log.Logger, ctx context.Context, msg string, kvs ...string) {
	traceId := r.CurrentSpan(ctx).SpanContext().TraceIDString()
	spanId := r.CurrentSpan(ctx).SpanContext().SpanIDString()

	r.Print(log, msg, "traceId", traceId, "spanId", spanId)
}

const (
	AttributeKeyKind = "kind"
)

func (r *Telemeter) WithNonLeafKind(ctx context.Context, kind, name string) context.Context {
	return context.WithValue(ctx, kindToContextKey(kind), name)
}

func kindToContextKey(kind string) string {
	return fmt.Sprintf("variant.%s", kind)
}

func labelNamesToNonLeafKinds(labelNames []string) map[string]struct{} {
	ctxLabels := map[string]struct{}{}
	for i := 0; i < len(labelNames)-1; i++ {
		ctxLabels[labelNames[i]] = struct{}{}
	}
	return ctxLabels
}

func labelNamesToKinds(labelNames []string) map[string]struct{} {
	kinds := map[string]struct{}{}
	for i := 0; i < len(labelNames); i++ {
		kinds[labelNames[i]] = struct{}{}
	}
	return kinds
}

type SpanKind string

func (r *Telemeter) WithSpan(ctx context.Context, k SpanKind, operation string, body func(ctx context.Context) error) error {
	kind := string(k)

	if _, ok := r.kinds[kind]; !ok {
		return fmt.Errorf("unregistered kind found: %q", kind)
	}

	// Propagate the non-leaf kind to the child spans
	// If known kinds are ["workflow", "task", "step"], the first two kinds are non-leaf kinds.
	// That is, each "step" span should be provided "workflow" and "task" which called the "step" so that
	// we can correlate which "step" called by which "task" is e.g. time-consuming or problematic in any mean
	if _, ok := r.nonLeafKinds[kind]; ok {
		ctx = r.WithNonLeafKind(ctx, kind, operation)
	}

	err := r.Tracer.WithSpan(ctx, operation, func(ctx context.Context) error {
		// This attribute is used to tell the metrics exporter about the kind
		r.SetAttribute(ctx, AttributeKeyKind, kind)

		return body(ctx)
	})
	if err != nil {
		r.CurrentSpan(ctx).SetStatus(1)
	} else {
		r.CurrentSpan(ctx).SetStatus(codes.OK)
	}
	return err
}

func (r *Telemeter) AddTraceEvent(ctx context.Context, msg string, attrs ...core.KeyValue) {
	kvs := make([]string, len(attrs)*2)
	for i := range attrs {
		ii := i * 2
		ij := ii + 1
		kvs[ii] = attrs[i].Key.Name
		kvs[ij] = attrs[i].Value.Emit()
	}
	r.Debug(ctx, msg, kvs...)

	r.CurrentSpan(ctx).AddEvent(ctx, msg, attrs...)
}

func (r *Telemeter) SetAttribute(ctx context.Context, k, v string) {
	r.CurrentSpan(ctx).SetAttribute(core.Key{Name: k}.String(v))
}

func (r *Telemeter) Push() error {
	return r.Metrics.Push(r.promPushEndpoint, r.jobName)
}

type writer struct {
	w io.Writer
}

func (w *writer) Write(p []byte) (int, error) {
	splits := bytes.SplitN(p, []byte(" "), 4)

	date, time, f, j := splits[0], splits[1], splits[2], splits[3]

	buf := &bytes.Buffer{}
	if _, err := buf.Write(j[:1]); err != nil {
		return 0, err
	}
	if _, err := buf.Write([]byte(fmt.Sprintf("%q:%q,", "date", date))); err != nil {
		return 0, err
	}
	if _, err := buf.Write([]byte(fmt.Sprintf("%q:%q,", "time", time))); err != nil {
		return 0, err
	}
	// f[:len(f)-1]) needed in order to trim the trailing `:` (It looks like "main.go:22:" usually!
	if _, err := buf.Write([]byte(fmt.Sprintf("%q:%q,", "file", f[:len(f)-1]))); err != nil {
		return 0, err
	}
	if _, err := buf.Write(j[1:]); err != nil {
		return 0, err
	}

	return w.w.Write(buf.Bytes())
}
