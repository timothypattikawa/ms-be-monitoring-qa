package monitoring

import (
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestDateRangeRejectsReverseRange(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest("GET", "/api/v1/workflow?from=2026-09-21&to=2026-09-20", nil)
	if _, _, err := dateRange(e.NewContext(req, httptest.NewRecorder())); err == nil {
		t.Fatal("expected reverse date range to be rejected")
	}
}

func TestQaseRunCasesRejectsMissingConfig(t *testing.T) {
	if _, err := (Connector{}).QaseRunCases(t.Context(), "INIT", 1); err == nil || err.Error() != "QASE_CONFIG_MISSING" {
		t.Fatalf("expected QASE_CONFIG_MISSING, got %v", err)
	}
}
