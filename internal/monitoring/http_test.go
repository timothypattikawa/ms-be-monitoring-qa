package monitoring

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Beyondtech-ID/ms-monitoring-qa-be/internal/repository"
	"github.com/google/uuid"
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
		JiraInitKey: " init-42 ", Name: " Refund ", QaseProjectCode: " init ", QAOwner: " Kiki ",
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
	invalid.QaseProjectCode = ""
	if _, err := invalid.validate(); err == nil {
		t.Fatal("expected missing Qase project code to fail")
	}
	invalid = valid
	invalid.BetaEndAt = "2026-09-02T00:00:00Z"
	if _, err := invalid.validate(); err == nil {
		t.Fatal("expected reversed beta range to fail")
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

func TestProductionBugsDefaultsToTenRowsPerPage(t *testing.T) {
	repo := repository.NewSQLiteForTest(t)
	base := time.Now().UTC()
	for i := 0; i < 11; i++ {
		row := repository.JiraIssue{
			ID: strconv.Itoa(i), ExternalID: strconv.Itoa(i), Key: "BUG-" + strconv.Itoa(i), ProjectID: "BUG",
			Reporter: "Reporter", Assignee: "Assignee", FetchedAt: base.Add(time.Duration(i) * time.Millisecond),
		}
		if err := repo.SaveJiraIssue(&row); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/production-bugs?page=1", nil)
	rec := httptest.NewRecorder()
	if err := (API{Repo: repo}).productionBugs(echo.New().NewContext(req, rec)); err != nil {
		t.Fatal(err)
	}
	var response struct {
		Data repository.ProductionBugPage `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.Page != 1 || response.Data.PageSize != 10 || response.Data.Total != 11 || len(response.Data.Items) != 10 {
		t.Fatalf("unexpected response: %+v", response.Data)
	}
}

// TestBugsHandlerContractFiltersPaginatesAndSourcesFromJiraIssues covers
// plan §9.5: GET /api/v1/bugs accepts projectId/environment/reporter/status
// /priority/q/page/pageSize and returns {items,page,pageSize,total} sourced
// from jira_issues (active, sync_scope "project-bugs"), not qase_defects —
// an issue with no Qase linkage still has to show up (plan §9.1's root cause).
func TestBugsHandlerContractFiltersPaginatesAndSourcesFromJiraIssues(t *testing.T) {
	repo := repository.NewSQLiteForTest(t)
	if err := repo.SaveProject(&repository.Project{ID: "project-1", JiraInitKey: "INIT-1", Name: "Checkout", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	// j3 starts active as if it matched a previous sync's JQL, then
	// ReplaceProjectBugs (the real reconciliation path) is run without it —
	// exactly what syncJiraBugs does once an issue drops out of the JQL —
	// so it ends up genuinely inactive rather than fighting gorm's
	// zero-value-with-default-tag quirk on a fresh insert.
	stale := repository.JiraIssue{ID: "j3", ExternalID: "3", Key: "QASE-3", ProjectID: "project-1", Summary: "Stale, no longer matched", Severity: "High", Status: "Open", Reporter: "Kiki", Environment: "STAGING", Active: true, SyncScope: "project-bugs", CreatedAt: &now}
	if err := repo.SaveJiraIssue(&stale); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceProjectBugs("project-1", "project-bugs", []repository.JiraIssue{
		{ID: uuid.NewString(), ExternalID: "1", Key: "QASE-1", Summary: "Login fails", Severity: "High", Status: "Open", Reporter: "Kiki", Environment: "STAGING", CreatedAt: &now},
		{ID: uuid.NewString(), ExternalID: "2", Key: "QASE-2", Summary: "Payment glitch", Severity: "Low", Status: "Invalid", Reporter: "Dea", Environment: "STAGING", CreatedAt: &now},
	}); err != nil {
		t.Fatal(err)
	}
	prodBug := repository.JiraIssue{ID: "j4", ExternalID: "prod-1", Key: "BUG-1", ProjectID: "BUG", Summary: "Unrelated production bug", Severity: "Critical", Status: "Open", Active: true, CreatedAt: &now}
	if err := repo.SaveJiraIssue(&prodBug); err != nil {
		t.Fatal(err)
	}
	a := API{Repo: repo}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/bugs?projectId=project-1&environment=staging&reporter=Kiki&page=1&pageSize=20", nil)
	rec := httptest.NewRecorder()
	if err := a.bugs(echo.New().NewContext(req, rec)); err != nil {
		t.Fatal(err)
	}
	var response struct {
		Data repository.BugPage `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.Total != 1 || len(response.Data.Items) != 1 || response.Data.Items[0].Key != "QASE-1" {
		t.Fatalf("expected only the active, matching, project-bugs-scoped issue, got %+v", response.Data)
	}
	if response.Data.Page != 1 || response.Data.PageSize != 20 {
		t.Fatalf("unexpected pagination echo: %+v", response.Data)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/bugs?priority=Low&status=Invalid", nil)
	rec = httptest.NewRecorder()
	if err := a.bugs(echo.New().NewContext(req, rec)); err != nil {
		t.Fatal(err)
	}
	response = struct {
		Data repository.BugPage `json:"data"`
	}{}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.Total != 1 || response.Data.Items[0].Key != "QASE-2" {
		t.Fatalf("expected priority param to filter against severity, got %+v", response.Data)
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
	body := `{"jiraInitKey":"INIT-3001","name":"Checkout","qaseProjectCode":"PAY","qaOwner":"Nadia","stagingStartAt":"2026-09-01T00:00:00Z","stagingEndAt":"2026-09-10T00:00:00Z","betaStartAt":"2026-09-11T00:00:00Z","betaEndAt":"2026-09-20T00:00:00Z"}`
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
	body := `{"jiraInitKey":"INIT-9999","name":"Checkout","qaseProjectCode":"PAY","qaOwner":"Nadia","stagingStartAt":"2026-09-01T00:00:00Z","stagingEndAt":"2026-09-10T00:00:00Z","betaStartAt":"2026-09-11T00:00:00Z","betaEndAt":"2026-09-20T00:00:00Z"}`
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

func TestCreateProjectPersistsJiraInitIDAndTriggersAutoSync(t *testing.T) {
	jira := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"10099","key":"INIT-3001","fields":{"summary":"x","status":{"name":"In Progress"},"issuetype":{"name":"Initiative"}}}`))
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
	body := `{"jiraInitKey":"INIT-3001","name":"Checkout","qaseProjectCode":"PAY","qaOwner":"Nadia","stagingStartAt":"2026-09-01T00:00:00Z","stagingEndAt":"2026-09-10T00:00:00Z","betaStartAt":"2026-09-11T00:00:00Z","betaEndAt":"2026-09-20T00:00:00Z"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	if err := api.createProject(e.NewContext(req, rec)); err != nil {
		t.Fatalf("unexpected handler error: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created Project
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created project: %v", err)
	}
	if created.JiraInitID == nil || *created.JiraInitID != "10099" || created.Status != "In Progress" {
		t.Fatalf("expected persisted JiraInitID/Status from the Jira lookup, got %+v", created)
	}

	jobs, err := repo.Jobs(10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, j := range jobs {
		if j.Trigger == "project_created" && j.ProjectID == created.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected an auto-enqueued project_created sync job scoped to the new project, got %+v", jobs)
	}
}

func TestCreateProjectReturns503WhenJiraNotConfigured(t *testing.T) {
	repo := repository.NewSQLiteForTest(t)
	api := API{Repo: repo, Connector: Connector{}} // no JiraBaseURL/Email/Token set
	e := echo.New()
	body := `{"jiraInitKey":"INIT-3001","name":"Checkout","qaseProjectCode":"PAY","qaOwner":"Nadia","stagingStartAt":"2026-09-01T00:00:00Z","stagingEndAt":"2026-09-10T00:00:00Z","betaStartAt":"2026-09-11T00:00:00Z","betaEndAt":"2026-09-20T00:00:00Z"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	if err := api.createProject(e.NewContext(req, rec)); err != nil {
		t.Fatalf("unexpected handler error: %v", err)
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateAndUpdateQaMember(t *testing.T) {
	repo := repository.NewSQLiteForTest(t)
	api := API{Repo: repo}
	e := echo.New()

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/qa-members", strings.NewReader(`{"name":"Nadia Putri","jiraEmail":"nadia@example.com","qaseDisplayName":"Nadia","weeklyCapacityHours":40}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	if err := api.createMember(e.NewContext(createReq, createRec)); err != nil {
		t.Fatalf("unexpected handler error: %v", err)
	}
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", createRec.Code, createRec.Body.String())
	}
	var created repository.Member
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created member: %v", err)
	}
	if created.ID == "" || !created.Active {
		t.Fatalf("expected an active member with an id, got %+v", created)
	}

	updateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/qa-members/"+created.ID, strings.NewReader(`{"active":false}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	ctx := e.NewContext(updateReq, updateRec)
	ctx.SetParamNames("id")
	ctx.SetParamValues(created.ID)
	if err := api.updateMember(ctx); err != nil {
		t.Fatalf("unexpected handler error: %v", err)
	}
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", updateRec.Code, updateRec.Body.String())
	}
	var updated repository.Member
	if err := json.Unmarshal(updateRec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode updated member: %v", err)
	}
	if updated.Active {
		t.Fatalf("expected member to be deactivated, got %+v", updated)
	}

	listed, err := repo.Members(false)
	if err != nil {
		t.Fatalf("list active members: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("expected deactivated member to be excluded from active list, got %+v", listed)
	}
}

func TestCreateJobQaseDetailWithoutProjectSyncsAllProjects(t *testing.T) {
	repo := repository.NewSQLiteForTest(t)
	api := API{Repo: repo}
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync-jobs", strings.NewReader(`{"sources":["qase-detail"]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	if err := api.createJob(e.NewContext(req, rec)); err != nil {
		t.Fatalf("unexpected handler error: %v", err)
	}
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for an unscoped qase-detail sync, got %d: %s", rec.Code, rec.Body.String())
	}
	var details repository.JobDetails
	if err := json.Unmarshal(rec.Body.Bytes(), &details); err != nil {
		t.Fatalf("decode job details: %v", err)
	}
	if details.ProjectID != "" {
		t.Fatalf("expected an unscoped job, got %+v", details)
	}
	if len(details.Steps) != 1 || details.Steps[0].Source != "qase-detail" {
		t.Fatalf("expected a single qase-detail step, got %+v", details.Steps)
	}
}

func TestCreateJobFullSyncRequestsAllThreeSources(t *testing.T) {
	repo := repository.NewSQLiteForTest(t)
	api := API{Repo: repo}
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync-jobs", strings.NewReader(`{"sources":["jira","qase","qase-detail"]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	if err := api.createJob(e.NewContext(req, rec)); err != nil {
		t.Fatalf("unexpected handler error: %v", err)
	}
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	var details repository.JobDetails
	if err := json.Unmarshal(rec.Body.Bytes(), &details); err != nil {
		t.Fatalf("decode job details: %v", err)
	}
	if len(details.Steps) != 3 {
		t.Fatalf("expected one step per source, got %+v", details.Steps)
	}
}

func TestCreateJobDetailScopesToQaseDetailSource(t *testing.T) {
	repo := repository.NewSQLiteForTest(t)
	api := API{Repo: repo}
	e := echo.New()

	project, err := repo.SaveProjectMapping("INIT-4001", "Checkout", "PAY", "Nadia", time.Now(), time.Now(), time.Now(), time.Now())
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}

	body := `{"scope":{"projectId":"` + project.ID + `"},"sources":["qase-detail"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync-jobs", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	if err := api.createJob(e.NewContext(req, rec)); err != nil {
		t.Fatalf("unexpected handler error: %v", err)
	}
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	var details repository.JobDetails
	if err := json.Unmarshal(rec.Body.Bytes(), &details); err != nil {
		t.Fatalf("decode job details: %v", err)
	}
	if details.ProjectID != project.ID {
		t.Fatalf("expected job scoped to project %s, got %+v", project.ID, details)
	}
	if len(details.Steps) != 1 || details.Steps[0].Source != "qase-detail" {
		t.Fatalf("expected a single qase-detail step, got %+v", details.Steps)
	}
}

func TestCreateJobRejectsUnknownSource(t *testing.T) {
	repo := repository.NewSQLiteForTest(t)
	api := API{Repo: repo}
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync-jobs", strings.NewReader(`{"sources":["bogus"]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	if err := api.createJob(e.NewContext(req, rec)); err != nil {
		t.Fatalf("unexpected handler error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected unknown source to be rejected, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateQaMemberRejectsMissingName(t *testing.T) {
	repo := repository.NewSQLiteForTest(t)
	api := API{Repo: repo}
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/qa-members", strings.NewReader(`{"weeklyCapacityHours":40}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	if err := api.createMember(e.NewContext(req, rec)); err != nil {
		t.Fatalf("unexpected handler error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}
