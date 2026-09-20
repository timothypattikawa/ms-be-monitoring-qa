package otel

import (
	"time"

	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Skipper mirrors echo middleware.Skipper: true skips instrumentation
// (e.g. health checks, infra probes).
type Skipper func(c echo.Context) bool

// NewMetricsMiddleware returns Echo middleware that records request count
// and duration for every non-skipped request.
func NewMetricsMiddleware(m *Metrics, skipper Skipper) echo.MiddlewareFunc {
	if skipper == nil {
		skipper = func(c echo.Context) bool { return false }
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if skipper(c) {
				return next(c)
			}

			start := time.Now()
			err := next(c)
			elapsedMs := float64(time.Since(start).Microseconds()) / 1000.0

			attrs := metric.WithAttributes(
				attribute.String("http.method", c.Request().Method),
				attribute.String("http.route", c.Path()),
				attribute.Int("http.status_code", c.Response().Status),
			)

			ctx := c.Request().Context()
			m.RequestCounter.Add(ctx, 1, attrs)
			m.RequestDuration.Record(ctx, elapsedMs, attrs)

			return err
		}
	}
}
