package monitoring

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Beyondtech-ID/ms-monitoring-qa-be/internal/repository"
	"github.com/labstack/echo/v4"
)

func TestDateRangeRejectsReverseRange(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest("GET", "/api/v1/workflow?from=2026-09-21&to=2026-09-20", nil)
	if _, _, err := dateRange(e.NewContext(req, httptest.NewRecorder())); err == nil {
		t.Fatal("expected reverse date range to be rejected")
	}
}

func TestProjectRegistrationValidation(t *testing.T) {
	valid := projectRegistration{
		JiraInitKey: " init-42 ", Name: " Refund ", QaseProjectCode: " init ", QaseTestRunID: 77, QAOwner: " Kiki ",
		StagingStartAt: "2026-09-01T00:00:00Z", StagingEndAt: "2026-09-02T00:00:00Z",
		BetaStartAt: "2026-09-03T00:00:00Z", BetaEndAt: "2026-09-04T00:00:00Z",
	}
	schedule, err := valid.validate()
	if err != nil {
		t.Fatal(err)
	}
	if valid.JiraInitKey != "INIT-42" || valid.QaseProjectCode != "INIT" || valid.Name != "Refund" || valid.QAOwner != "Kiki" {
		t.Fatalf("registration was not normalized: %+v", valid)
	}
	if !schedule.stagingStart.Equal(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected staging start: %v", schedule.stagingStart)
	}

	invalid := valid
	invalid.QaseTestRunID = 0
	if _, err := invalid.validate(); err == nil {
		t.Fatal("expected missing Qase run to fail")
	}
	invalid = valid
	invalid.BetaEndAt = "2026-09-02T00:00:00Z"
	if _, err := invalid.validate(); err == nil {
		t.Fatal("expected reversed beta range to fail")
	}
}

func TestScopedJiraJQLUsesRegisteredKey(t *testing.T) {
	got, err := scopedJiraJQL("project = INIT AND status != Closed", "init-42")
	if err != nil {
		t.Fatal(err)
	}
	if got != `(project = INIT AND status != Closed) AND key = "INIT-42"` {
		t.Fatalf("unexpected scoped JQL: %s", got)
	}
	if _, err := scopedJiraJQL("project = INIT", `INIT-1" OR project=X`); err == nil {
		t.Fatal("expected unsafe Jira key to fail")
	}
}

func TestQaseRunCasesRejectsMissingConfig(t *testing.T) {
	if _, err := (Connector{}).QaseRunCases(t.Context(), "INIT", 1); err == nil || err.Error() != "QASE_CONFIG_MISSING" {
		t.Fatalf("expected QASE_CONFIG_MISSING, got %v", err)
	}
}

func TestProjectViewRunContractUsesStableEmptyArray(t *testing.T) {
	payload, err := json.Marshal(projectView{
		Project: Project{ID: "project-1", JiraInitKey: "INIT-42"},
		Counts:  repository.Counts{}, Runs: make([]repository.ProjectRun, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), `"runs":[]`) {
		t.Fatalf("project response must expose a stable runs array: %s", payload)
	}
}

func TestCreateProjectValidatesBeforeInsert(t *testing.T) {
	jira := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"1","key":"INIT-3001","fields":{"summary":"x","status":{"name":"Open"},"issuetype":{"name":"Initiative"}}}`))
	}))
	defer jira.Close()
	qase := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":true,"result":{"code":"PAY"}}`))
	}))
	defer qase.Close()

	repo := repository.NewSQLiteForTest(t)
	api := API{Repo: repo, Connector: Connector{Client: jira.Client(), JiraBaseURL: jira.URL, JiraEmail: "qa@example.com", JiraToken: "token", QaseBaseURL: qase.URL, QaseToken: "token"}}

	e := echo.New()
	body := `{"jiraInitKey":"INIT-3001","name":"Checkout","qaseProjectCode":"PAY","qaseTestRunId":181,"qaOwner":"Nadia","stagingStartAt":"2026-09-01T00:00:00Z","stagingEndAt":"2026-09-10T00:00:00Z","betaStartAt":"2026-09-11T00:00:00Z","betaEndAt":"2026-09-20T00:00:00Z"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	if err := api.createProject(e.NewContext(req, rec)); err != nil {
		t.Fatalf("unexpected handler error: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateProjectRejectsUnknownJiraKey(t *testing.T) {
	jira := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer jira.Close()
	repo := repository.NewSQLiteForTest(t)
	api := API{Repo: repo, Connector: Connector{Client: jira.Client(), JiraBaseURL: jira.URL, JiraEmail: "qa@example.com", JiraToken: "token"}}

	e := echo.New()
	body := `{"jiraInitKey":"INIT-9999","name":"Checkout","qaseProjectCode":"PAY","qaseTestRunId":181,"qaOwner":"Nadia","stagingStartAt":"2026-09-01T00:00:00Z","stagingEndAt":"2026-09-10T00:00:00Z","betaStartAt":"2026-09-11T00:00:00Z","betaEndAt":"2026-09-20T00:00:00Z"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	if err := api.createProject(e.NewContext(req, rec)); err != nil {
		t.Fatalf("unexpected handler error: %v", err)
	}
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
	}
}
