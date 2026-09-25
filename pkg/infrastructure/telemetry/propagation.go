package telemetry

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/trace"
)

// InjectTraceparent injects standard W3C TRACEPARENT and TRACESTATE environment
// variables into the given environment slice when an active valid span is present.
func InjectTraceparent(ctx context.Context, env []string) []string {
	span := trace.SpanFromContext(ctx)
	if !span.SpanContext().IsValid() {
		return env
	}
	sc := span.SpanContext()
	traceparent := fmt.Sprintf("00-%s-%s-%s", sc.TraceID().String(), sc.SpanID().String(), sc.TraceFlags().String())
	env = append(env, "TRACEPARENT="+traceparent)
	if sc.TraceState().Len() > 0 {
		env = append(env, "TRACESTATE="+sc.TraceState().String())
	}
	return env
}
