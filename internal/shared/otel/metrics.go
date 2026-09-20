package otel

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// Metrics bundles the HTTP-level instruments emitted by NewMetricsMiddleware.
type Metrics struct {
	RequestCounter  metric.Int64Counter
	RequestDuration metric.Float64Histogram
}

// NewMetrics creates the request counter and duration histogram, backed by
// the process-wide MeterProvider — call only after SetupOTelSDK has run.
func NewMetrics() (*Metrics, error) {
	meter := otel.Meter(ServiceInstrumentationName)

	requestCounter, err := meter.Int64Counter(
		"http.server.request_count",
		metric.WithDescription("Number of HTTP requests received"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}

	requestDuration, err := meter.Float64Histogram(
		"http.server.request_duration",
		metric.WithDescription("Duration of HTTP requests"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}

	return &Metrics{
		RequestCounter:  requestCounter,
		RequestDuration: requestDuration,
	}, nil
}
