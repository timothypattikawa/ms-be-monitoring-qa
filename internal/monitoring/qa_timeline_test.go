package monitoring

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Beyondtech-ID/ms-monitoring-qa-be/internal/repository"
	"github.com/labstack/echo/v4"
)

func timelineAPI(t *testing.T) *echo.Echo {
	t.Helper()
	repo := repository.NewSQLiteForTest(t)
	if err := repo.SeedNationalHolidays(); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveProject(&repository.Project{ID: "project-1", JiraInitKey: "INIT-1", Name: "Checkout", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	api := API{Repo: repo}
	e := echo.New()
	api.Register(e)
	return e
}

func TestUpdateProjectPlanning(t *testing.T) {
	e := timelineAPI(t)
	patch := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/projects/project-1", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}

	rec := patch(`{"timelinePlanDays":12.5,"projectSize":"l"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body.String())
	}
	var saved repository.Project
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.TimelinePlanDays == nil || *saved.TimelinePlanDays != 12.5 || saved.ProjectSize == nil || *saved.ProjectSize != "L" {
		t.Fatalf("unexpected saved values: %+v", saved)
	}

	if rec := patch(`{"projectSize":"HUGE"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid size must be rejected: %d", rec.Code)
	}
	if rec := patch(`{"timelinePlanDays":-1}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("negative plan must be rejected: %d", rec.Code)
	}

	rec = patch(`{"timelinePlanDays":null,"projectSize":null}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("clear: %d %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.TimelinePlanDays != nil || saved.ProjectSize != nil {
		t.Fatalf("explicit null must clear fields: %+v", saved)
	}
}

func TestQaTimelineEndpointReturnsProjects(t *testing.T) {
	e := timelineAPI(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/qa-timeline", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("qa-timeline: %d %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Data []repository.ProjectTimeline `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) != 1 || payload.Data[0].ProjectID != "project-1" {
		t.Fatalf("unexpected timeline payload: %+v", payload.Data)
	}
	if payload.Data[0].QAStartAt != nil || payload.Data[0].WorkingDays != 0 {
		t.Fatalf("project without STG runs must have no window: %+v", payload.Data[0])
	}
}
