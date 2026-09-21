package otel

import (
	"context"

	"go.opentelemetry.io/otel"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// ServiceName is the process-wide service name reported to the OTel collector.
const ServiceName = "ms-monitoring-qa-be"

// ServiceInstrumentationName identifies this module as an OTel instrumentation scope.
const ServiceInstrumentationName = "github.com/Beyondtech-ID/ms-monitoring-qa-be"

// Tracer is the process-wide tracer singleton for every manual span in
// usecase/integrations — obtain it only through this variable, never
// construct another TracerProvider/Tracer.
var Tracer = otel.Tracer(ServiceInstrumentationName)

// SpanContextFromContext exposes the active span's trace_id/span_id, used by
// the logger to attach correlation fields to log lines.
func SpanContextFromContext(ctx context.Context) oteltrace.SpanContext {
	return oteltrace.SpanContextFromContext(ctx)
}
