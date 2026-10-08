package monitoring

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Beyondtech-ID/ms-monitoring-qa-be/internal/repository"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type API struct {
	Repo             *repository.Monitoring
	Connector        Connector
	Solr             Solr
	AlertEmailSender AlertEmailSender
}
type sourceState struct {
	Status   string     `json:"status"`
	SyncedAt *time.Time `json:"syncedAt"`
}
type envelope struct {
	AsOf    *time.Time             `json:"asOf"`
	Sources map[string]sourceState `json:"sources"`
	Data    any                    `json:"data"`
}
type projectView struct {
	Project
	Counts           repository.Counts                 `json:"counts"`
	CountsAvailable  bool                              `json:"countsAvailable"`
	Runs             []repository.ProjectRun           `json:"runs"`
	TesterProgress   []repository.TesterProgress       `json:"testerProgress"`
	AssigneeProgress []repository.TesterProgress       `json:"assigneeProgress"`
	DailyExecutions  []repository.TesterDailyExecution `json:"dailyExecutions"`
	StagingCounts    repository.Counts                 `json:"stagingCounts"`
	BetaCounts       repository.Counts                 `json:"betaCounts"`
	BugSummary       repository.ProjectBugSummary      `json:"bugSummary"`
}

// allowedProjectSizes is plan §6.1's fixed size enum, mirrored by the
// projects.project_size CHECK constraint (migrations/20260928_bug_dashboard_additive.up.sql).
var allowedProjectSizes = map[string]bool{"S": true, "M": true, "L": true, "XL": true, "2XL": true, "3XL": true, "4L": true, "5L": true}

type projectRegistration struct {
	JiraInitKey        string  `json:"jiraInitKey"`
	Name               string  `json:"name"`
	QaseProjectCode    string  `json:"qaseProjectCode"`
	QAOwner            string  `json:"qaOwner"`
	ProjectSize        *string `json:"projectSize"`
	StagingMtttMinutes *int    `json:"stagingMtttMinutes"`
	BetaMtttMinutes    *int    `json:"betaMtttMinutes"`
	StagingStartAt     string  `json:"stagingStartAt"`
	StagingEndAt       string  `json:"stagingEndAt"`
	BetaStartAt        string  `json:"betaStartAt"`
	BetaEndAt          string  `json:"betaEndAt"`
}
type projectSchedule struct {
	stagingStart, stagingEnd, betaStart, betaEnd time.Time
}

func (r *projectRegistration) validate() (projectSchedule, error) {
	r.JiraInitKey = strings.ToUpper(strings.TrimSpace(r.JiraInitKey))
	r.Name = strings.TrimSpace(r.Name)
	r.QaseProjectCode = strings.ToUpper(strings.TrimSpace(r.QaseProjectCode))
	r.QAOwner = strings.TrimSpace(r.QAOwner)
	if !jiraKeyPattern.MatchString(r.JiraInitKey) || r.Name == "" || r.QaseProjectCode == "" || r.QAOwner == "" {
		return projectSchedule{}, errors.New("missing required project field")
	}
	if r.ProjectSize != nil {
		size := strings.ToUpper(strings.TrimSpace(*r.ProjectSize))
		if size == "" {
			r.ProjectSize = nil
		} else if !allowedProjectSizes[size] {
			return projectSchedule{}, errors.New("invalid project size")
		} else {
			r.ProjectSize = &size
		}
	}
	if r.StagingMtttMinutes != nil && *r.StagingMtttMinutes < 0 {
		return projectSchedule{}, errors.New("stagingMtttMinutes must not be negative")
	}
	if r.BetaMtttMinutes != nil && *r.BetaMtttMinutes < 0 {
		return projectSchedule{}, errors.New("betaMtttMinutes must not be negative")
	}
	values := []string{r.StagingStartAt, r.StagingEndAt, r.BetaStartAt, r.BetaEndAt}
	times := make([]time.Time, len(values))
	for i, value := range values {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return projectSchedule{}, errors.New("project dates must use RFC3339")
		}
		times[i] = parsed.UTC()
	}
	if times[1].Before(times[0]) || times[3].Before(times[2]) {
		return projectSchedule{}, errors.New("project end date precedes start date")
	}
	return projectSchedule{times[0], times[1], times[2], times[3]}, nil
}

type memberRegistration struct {
	Name                string  `json:"name"`
	JiraEmail           string  `json:"jiraEmail"`
	QaseDisplayName     string  `json:"qaseDisplayName"`
	WeeklyCapacityHours float64 `json:"weeklyCapacityHours"`
}
type memberResponse struct {
	repository.Member
	Warnings map[string]string `json:"warnings,omitempty"`
}

func (r *memberRegistration) validate() error {
	r.Name = strings.TrimSpace(r.Name)
	r.JiraEmail = strings.TrimSpace(r.JiraEmail)
	r.QaseDisplayName = strings.TrimSpace(r.QaseDisplayName)
	if r.Name == "" || r.WeeklyCapacityHours < 0 {
		return errors.New("missing required member field")
	}
	return nil
}

// resolveJiraEmail resolves a Jira login email to an accountId. A
// not-found email yields a soft warning; any other error (network/auth)
// is skipped silently so a flaky third-party API never blocks the save.
func (a API) resolveJiraEmail(ctx context.Context, email string) (accountID, warning string) {
	if email == "" {
		return "", ""
	}
	id, err := a.Connector.JiraUserByEmail(ctx, email)
	if err != nil {
		if err.Error() == "JIRA_USER_NOT_FOUND" {
			return "", "Email '" + email + "' tidak ditemukan di Jira."
		}
		return "", ""
	}
	return id, ""
}

// resolveQaseDisplayName checks the given name against the live QA
// PIC/Tester/Tester Android/Tester IOS option titles and returns a soft
// warning when it doesn't exactly match any of them.
func (a API) resolveQaseDisplayName(ctx context.Context, name string) (warning string) {
	if name == "" {
		return ""
	}
	fields, err := a.Connector.QaseCustomFields(ctx)
	if err != nil {
		return ""
	}
	wanted := map[int64]bool{qaseFieldIDPic: true, qaseFieldIDTester: true, qaseFieldIDTesterAndroid: true, qaseFieldIDTesterIos: true}
	for _, f := range fields {
		if !wanted[f.ID] {
			continue
		}
		for _, v := range f.Value {
			if v.Title == name {
				return ""
			}
		}
	}
	return "Nama '" + name + "' tidak ditemukan di opsi QA PIC/QA Tester di Qase saat ini."
}

func invalidMember(c echo.Context) error {
	return c.JSON(http.StatusBadRequest, map[string]string{"code": "INVALID_MEMBER", "message": "name is required and weeklyCapacityHours must not be negative"})
}

func (a API) Register(e *echo.Echo) {
	g := e.Group("/api/v1")
	g.GET("/projects", a.projects)
	g.GET("/projects/:id", a.project)
	g.POST("/projects", a.createProject, a.managerAuth)
	g.PATCH("/projects/:id", a.updateProject, a.managerAuth)
	g.GET("/qa-timeline", a.qaTimeline)
	g.GET("/qa-alert-email/status", a.qaAlertEmailStatus)
	g.POST("/projects/:id/qa-alert-email", a.sendProjectQaAlertEmail, a.managerAuth)
	g.GET("/qa-members", a.members)
	g.GET("/qa-members/:id", a.member)
	g.POST("/qa-members", a.createMember, a.managerAuth)
	g.PATCH("/qa-members/:id", a.updateMember, a.managerAuth)
	g.GET("/workflow", a.workflow)
	g.GET("/workload", a.workload)
	g.GET("/bugs", a.bugs)
	g.GET("/production-bugs", a.productionBugs)
	g.POST("/sync-jobs", a.createJob, a.managerAuth)
	g.GET("/sync-jobs", a.jobs)
	g.GET("/sync-jobs/:id", a.job)
	a.RegisterDocumentationRoutes(g)
	a.RegisterKnowledgeRoutes(g)
}
func (a API) managerAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		key := os.Getenv("MONITOR_MANAGER_API_KEY")
		// ponytail: unset key means the gate is off (single QA manager uses
		// this dashboard, no RBAC exists) — set MONITOR_MANAGER_API_KEY to
		// turn it back on, no code change needed.
		if key == "" {
			return next(c)
		}
		provided := c.Request().Header.Get("X-Manager-Key")
		if subtle.ConstantTimeCompare([]byte(provided), []byte(key)) != 1 {
			return c.JSON(http.StatusForbidden, map[string]string{"code": "FORBIDDEN", "message": "manager authorization required"})
		}
		return next(c)
	}
}
func (a API) wrap(data any) (envelope, error) {
	out := envelope{Sources: map[string]sourceState{}, Data: data}
	complete := true
	for _, source := range []string{"jira", "qase"} {
		step, found, err := a.Repo.LatestGlobalStep(source)
		if err != nil {
			return out, err
		}
		state := sourceState{Status: "never_synced"}
		if found && step.FinishedAt != nil {
			state.SyncedAt = step.FinishedAt
			state.Status = "fresh"
			if time.Since(*step.FinishedAt) > 24*time.Hour {
				state.Status = "stale"
			}
			if out.AsOf == nil || step.FinishedAt.Before(*out.AsOf) {
				out.AsOf = step.FinishedAt
			}
		}
		out.Sources[source] = state
		if !found {
			complete = false
		}
	}
	if !complete {
		out.AsOf = nil
	}
	return out, nil
}
func (a API) send(c echo.Context, data any) error {
	out, err := a.wrap(data)
	if err != nil {
		return safeError(c, err)
	}
	return c.JSON(http.StatusOK, out)
}
func safeError(c echo.Context, err error) error {
	_ = err
	return c.JSON(http.StatusInternalServerError, map[string]string{"code": "INTERNAL", "message": "request failed"})
}
func limit(c echo.Context) int {
	n, _ := strconv.Atoi(c.QueryParam("limit"))
	if n < 1 || n > 500 {
		return 50
	}
	return n
}

func (a API) projects(c echo.Context) error {
	rows, err := a.Repo.Projects(limit(c), c.QueryParam("includeInactive") == "true", c.QueryParam("q"))
	if err != nil {
		return safeError(c, err)
	}
	result := make([]projectView, 0, len(rows))
	for _, p := range rows {
		counts, available, err := a.Repo.ProjectCounts(p)
		if err != nil {
			return safeError(c, err)
		}
		runs, err := a.Repo.ProjectRuns(p)
		if err != nil {
			return safeError(c, err)
		}
		testerProgress, err := a.Repo.ProjectTesterBreakdown(p)
		if err != nil {
			return safeError(c, err)
		}
		dailyExecutions, err := a.Repo.ProjectTesterDailyExecutions(p)
		if err != nil {
			return safeError(c, err)
		}
		stagingCounts, err := a.Repo.ProjectEnvironmentCounts(p, "STAGING")
		if err != nil {
			return safeError(c, err)
		}
		betaCounts, err := a.Repo.ProjectEnvironmentCounts(p, "BETA")
		if err != nil {
			return safeError(c, err)
		}
		bugSummary, err := a.Repo.ProjectBugSummary(p)
		if err != nil {
			return safeError(c, err)
		}
		result = append(result, projectView{Project: p, Counts: counts, CountsAvailable: available, Runs: runs, TesterProgress: testerProgress, DailyExecutions: dailyExecutions, StagingCounts: stagingCounts, BetaCounts: betaCounts, BugSummary: bugSummary})
	}
	return a.send(c, result)
}
func (a API) project(c echo.Context) error {
	p, err := a.Repo.Project(c.Param("id"))
	if errors.Is(err, repository.ErrNotFound) {
		return c.JSON(http.StatusNotFound, map[string]string{"code": "NOT_FOUND", "message": "project not found"})
	}
	if err != nil {
		return safeError(c, err)
	}
	counts, available, err := a.Repo.ProjectCounts(p)
	if err != nil {
		return safeError(c, err)
	}
	runs, err := a.Repo.ProjectRuns(p)
	if err != nil {
		return safeError(c, err)
	}
	testerProgress, err := a.Repo.ProjectTesterBreakdown(p)
	if err != nil {
		return safeError(c, err)
	}
	assigneeProgress, err := a.Repo.ProjectAssigneeProgress(p)
	if err != nil {
		return safeError(c, err)
	}
	dailyExecutions, err := a.Repo.ProjectTesterDailyExecutions(p)
	if err != nil {
		return safeError(c, err)
	}
	stagingCounts, err := a.Repo.ProjectEnvironmentCounts(p, "STAGING")
	if err != nil {
		return safeError(c, err)
	}
	betaCounts, err := a.Repo.ProjectEnvironmentCounts(p, "BETA")
	if err != nil {
		return safeError(c, err)
	}
	bugSummary, err := a.Repo.ProjectBugSummary(p)
	if err != nil {
		return safeError(c, err)
	}
	return a.send(c, projectView{Project: p, Counts: counts, CountsAvailable: available, Runs: runs, TesterProgress: testerProgress, AssigneeProgress: assigneeProgress, DailyExecutions: dailyExecutions, StagingCounts: stagingCounts, BetaCounts: betaCounts, BugSummary: bugSummary})
}
func (a API) createProject(c echo.Context) error {
	var req projectRegistration
	if err := c.Bind(&req); err != nil {
		return invalidProject(c)
	}
	schedule, err := req.validate()
	if err != nil {
		return invalidProject(c)
	}
	ctx := c.Request().Context()
	issue, err := a.Connector.JiraIssue(ctx, req.JiraInitKey)
	if err != nil {
		if err.Error() == "JIRA_CONFIG_MISSING" {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{"code": "JIRA_CONFIG_MISSING", "message": "Jira integration is not configured"})
		}
		return c.JSON(http.StatusUnprocessableEntity, map[string]string{"code": "JIRA_INIT_NOT_FOUND", "message": "jiraInitKey could not be validated against Jira"})
	}
	if _, err := a.Connector.QaseProject(ctx, req.QaseProjectCode); err != nil {
		if err.Error() == "QASE_CONFIG_MISSING" {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{"code": "QASE_CONFIG_MISSING", "message": "Qase integration is not configured"})
		}
		return c.JSON(http.StatusUnprocessableEntity, map[string]string{"code": "QASE_PROJECT_NOT_FOUND", "message": "qaseProjectCode could not be validated against Qase"})
	}
	p, err := a.Repo.SaveProjectMappingWithMetadata(req.JiraInitKey, req.Name, req.QaseProjectCode, req.QAOwner, req.ProjectSize, req.StagingMtttMinutes, req.BetaMtttMinutes, schedule.stagingStart, schedule.stagingEnd, schedule.betaStart, schedule.betaEnd)
	if errors.Is(err, repository.ErrJiraInitMapped) {
		return c.JSON(http.StatusConflict, map[string]string{"code": "JIRA_INIT_ALREADY_REGISTERED", "message": "jiraInitKey is already registered"})
	}
	if errors.Is(err, repository.ErrQaseRunMapped) {
		return c.JSON(http.StatusConflict, map[string]string{"code": "QASE_RUN_ALREADY_REGISTERED", "message": "qaseProjectCode is already registered"})
	}
	if err != nil {
		return safeError(c, err)
	}
	p.JiraInitID = stringPtr(issue.ID)
	p.Status = issue.Fields.Status.Name
	if err := a.Repo.SaveProject(&p); err != nil {
		return safeError(c, err)
	}
	// Non-fatal: the project is already saved; a failed auto-enqueue just
	// means it waits for the next scheduled or manual sync instead.
	job := SyncJob{ID: uuid.NewString(), RequestKey: uuid.NewString(), Trigger: "project_created", Status: "queued", Sources: "jira,qase", ProjectID: p.ID, RequestedAt: time.Now().UTC(), Actor: "system"}
	_, _ = a.Repo.EnqueueIfAbsent(&job, []string{"jira", "qase"})
	return c.JSON(http.StatusCreated, p)
}
func (a API) members(c echo.Context) error {
	rows, err := a.Repo.Members(c.QueryParam("includeInactive") == "true")
	if err != nil {
		return safeError(c, err)
	}
	return a.send(c, rows)
}

func (a API) member(c echo.Context) error {
	v, err := a.Repo.Member(c.Param("id"))
	if errors.Is(err, repository.ErrNotFound) {
		return c.JSON(http.StatusNotFound, map[string]string{"code": "NOT_FOUND", "message": "qa member not found"})
	}
	if err != nil {
		return safeError(c, err)
	}
	return a.send(c, v)
}

func (a API) createMember(c echo.Context) error {
	var req memberRegistration
	if err := c.Bind(&req); err != nil {
		return invalidMember(c)
	}
	if err := req.validate(); err != nil {
		return invalidMember(c)
	}
	ctx := c.Request().Context()
	warnings := map[string]string{}
	accountID, warn := a.resolveJiraEmail(ctx, req.JiraEmail)
	if warn != "" {
		warnings["jiraEmail"] = warn
	}
	if warn := a.resolveQaseDisplayName(ctx, req.QaseDisplayName); warn != "" {
		warnings["qaseDisplayName"] = warn
	}
	v := Member{ID: uuid.NewString(), Name: req.Name, JiraEmail: req.JiraEmail, JiraAccountID: accountID, QaseDisplayName: req.QaseDisplayName, WeeklyCapacityHours: req.WeeklyCapacityHours, Active: true}
	if err := a.Repo.SaveMember(&v); err != nil {
		return safeError(c, err)
	}
	resp := memberResponse{Member: v}
	if len(warnings) > 0 {
		resp.Warnings = warnings
	}
	return c.JSON(http.StatusCreated, resp)
}

func (a API) updateMember(c echo.Context) error {
	v, err := a.Repo.Member(c.Param("id"))
	if errors.Is(err, repository.ErrNotFound) {
		return c.JSON(http.StatusNotFound, map[string]string{"code": "NOT_FOUND", "message": "qa member not found"})
	}
	if err != nil {
		return safeError(c, err)
	}
	var req struct {
		Name                *string  `json:"name"`
		JiraEmail           *string  `json:"jiraEmail"`
		QaseDisplayName     *string  `json:"qaseDisplayName"`
		WeeklyCapacityHours *float64 `json:"weeklyCapacityHours"`
		Active              *bool    `json:"active"`
	}
	if err := c.Bind(&req); err != nil {
		return invalidMember(c)
	}
	if req.Name != nil {
		v.Name = strings.TrimSpace(*req.Name)
	}
	ctx := c.Request().Context()
	warnings := map[string]string{}
	if req.JiraEmail != nil {
		email := strings.TrimSpace(*req.JiraEmail)
		v.JiraEmail = email
		accountID, warn := a.resolveJiraEmail(ctx, email)
		v.JiraAccountID = accountID
		if warn != "" {
			warnings["jiraEmail"] = warn
		}
	}
	if req.QaseDisplayName != nil {
		name := strings.TrimSpace(*req.QaseDisplayName)
		v.QaseDisplayName = name
		if warn := a.resolveQaseDisplayName(ctx, name); warn != "" {
			warnings["qaseDisplayName"] = warn
		}
	}
	if req.WeeklyCapacityHours != nil {
		v.WeeklyCapacityHours = *req.WeeklyCapacityHours
	}
	if req.Active != nil {
		v.Active = *req.Active
	}
	if v.Name == "" || v.WeeklyCapacityHours < 0 {
		return invalidMember(c)
	}
	if err := a.Repo.SaveMember(&v); err != nil {
		return safeError(c, err)
	}
	resp := memberResponse{Member: v}
	if len(warnings) > 0 {
		resp.Warnings = warnings
	}
	return c.JSON(http.StatusOK, resp)
}

// qaTimeline serves the Bugs-page "QA timeline & scenario" widget: one row
// per active project with the STG-run window, working-day breakdown, manual
// plan/size, and total scenarios.
func (a API) qaTimeline(c echo.Context) error {
	rows, err := a.Repo.QaTimelines()
	if err != nil {
		return safeError(c, err)
	}
	return a.send(c, rows)
}

// updateProject edits the manual planning fields (projectSize,
// timelinePlanDays). Keys are presence-checked on the raw body so an
// explicit JSON null clears the field — omitting the key leaves it as-is.
func (a API) updateProject(c echo.Context) error {
	p, err := a.Repo.Project(c.Param("id"))
	if errors.Is(err, repository.ErrNotFound) {
		return c.JSON(http.StatusNotFound, map[string]string{"code": "NOT_FOUND", "message": "project not found"})
	}
	if err != nil {
		return safeError(c, err)
	}
	var req map[string]json.RawMessage
	if err := c.Bind(&req); err != nil {
		return invalidProjectUpdate(c)
	}
	if raw, ok := req["projectSize"]; ok {
		var size *string
		if err := json.Unmarshal(raw, &size); err != nil {
			return invalidProjectUpdate(c)
		}
		if size == nil || strings.TrimSpace(*size) == "" {
			p.ProjectSize = nil
		} else if v := strings.ToUpper(strings.TrimSpace(*size)); !allowedProjectSizes[v] {
			return invalidProjectUpdate(c)
		} else {
			p.ProjectSize = &v
		}
	}
	if raw, ok := req["timelinePlanDays"]; ok {
		var days *float64
		if err := json.Unmarshal(raw, &days); err != nil {
			return invalidProjectUpdate(c)
		}
		if days != nil && *days < 0 {
			return invalidProjectUpdate(c)
		}
		p.TimelinePlanDays = days
	}
	if err := a.Repo.SaveProject(&p); err != nil {
		return safeError(c, err)
	}
	return c.JSON(http.StatusOK, p)
}

func invalidProjectUpdate(c echo.Context) error {
	return c.JSON(http.StatusBadRequest, map[string]string{"code": "INVALID_PROJECT_UPDATE", "message": "projectSize must be a known size and timelinePlanDays must not be negative"})
}

func invalidProject(c echo.Context) error {
	return c.JSON(http.StatusBadRequest, map[string]string{"code": "INVALID_PROJECT", "message": "jiraInitKey, name, qaseProjectCode, qaOwner and valid staging/beta ranges are required"})
}
func dateRange(c echo.Context) (time.Time, time.Time, error) {
	now := time.Now().UTC()
	from := now.AddDate(0, 0, -30)
	to := now
	if v := c.QueryParam("from"); v != "" {
		x, e := time.Parse("2006-01-02", v)
		if e != nil {
			return from, to, e
		}
		from = x
	}
	if v := c.QueryParam("to"); v != "" {
		x, e := time.Parse("2006-01-02", v)
		if e != nil {
			return from, to, e
		}
		to = x.Add(24 * time.Hour)
	}
	if !from.Before(to) {
		return from, to, errors.New("invalid date range")
	}
	return from, to, nil
}
func (a API) workflow(c echo.Context) error {
	from, to, err := dateRange(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"code": "INVALID_DATE", "message": "use ISO dates"})
	}
	rows, err := a.Repo.Workflow(from, to, c.QueryParam("projectId"))
	if err != nil {
		return safeError(c, err)
	}
	return a.send(c, map[string]any{"days": rows})
}
func (a API) workload(c echo.Context) error {
	from, to, err := dateRange(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"code": "INVALID_DATE", "message": "use ISO dates"})
	}
	members, err := a.Repo.Workload(from, to, c.QueryParam("memberId"))
	if err != nil {
		return safeError(c, err)
	}
	return a.send(c, map[string]any{"period": map[string]string{"from": from.Format("2006-01-02"), "to": to.Add(-time.Nanosecond).Format("2006-01-02")}, "members": members})
}

// bugsPage clamps pageSize higher than documentPage's: the dashboard does a
// single bulk "load every registered-project bug" fetch (pageSize=1000) to
// populate client-side grouping/filtering, on top of the normal paginated
// Bugs page table (pageSize=20). The 2000 ceiling gives headroom above the
// current ~700-bug total as more projects/history accumulate.
func bugsPage(c echo.Context) (int, int) {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("pageSize"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 2000 {
		pageSize = 20
	}
	return page, pageSize
}
func (a API) bugs(c echo.Context) error {
	page, pageSize := bugsPage(c)
	rows, err := a.Repo.JiraBugs(page, pageSize, c.QueryParam("projectId"), c.QueryParam("environment"), c.QueryParam("reporter"), c.QueryParam("status"), c.QueryParam("priority"), c.QueryParam("q"))
	if err != nil {
		return safeError(c, err)
	}
	return a.send(c, rows)
}

func (a API) productionBugs(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.QueryParam("pageSize"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}
	rows, err := a.Repo.ProductionBugs(page, pageSize)
	if err != nil {
		return safeError(c, err)
	}
	return a.send(c, rows)
}

// createJob's sources may be any combination, each at most once, of: "jira"
// and "qase" (the cheap overview tier) and "qase-detail" (the expensive
// per-project case/tester/defect tier), plus "production-bugs" (only the Jira
// BUG-project snapshot, for the Bugs page's own sync button). scope.projectId is optional for all
// of them — omitted, a source fans out across every registered project;
// set, it scopes to just that one. The single global "Sync data" button
// requests all three sources unscoped in one job, so its progress can be
// polled from one place.
func (a API) createJob(c echo.Context) error {
	var req struct {
		Scope struct {
			ProjectID *string `json:"projectId"`
		} `json:"scope"`
		Sources []string `json:"sources"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"code": "INVALID_REQUEST", "message": "invalid JSON"})
	}
	if len(req.Sources) == 0 {
		req.Sources = []string{"jira", "qase"}
	}
	seen := map[string]bool{}
	for _, s := range req.Sources {
		if (s != "jira" && s != "qase" && s != "qase-detail" && s != "production-bugs" && s != "qa-portfolio") || seen[s] {
			return c.JSON(http.StatusBadRequest, map[string]string{"code": "INVALID_SOURCE", "message": "sources must be one or more of jira, qase, qase-detail, production-bugs, qa-portfolio (each once)"})
		}
		seen[s] = true
	}
	key := strings.TrimSpace(c.Request().Header.Get("Idempotency-Key"))
	if key == "" {
		key = uuid.NewString()
	}
	if len(key) > 128 {
		return c.JSON(http.StatusBadRequest, map[string]string{"code": "INVALID_KEY", "message": "idempotency key too long"})
	}
	existing, err := a.Repo.JobByRequestKey(key)
	if err == nil {
		return a.acceptedJob(c, existing)
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return safeError(c, err)
	}
	job := SyncJob{ID: uuid.NewString(), RequestKey: key, Trigger: "manual", Status: "queued", Sources: strings.Join(req.Sources, ","), RequestedAt: time.Now().UTC(), Actor: "manager"}
	if req.Scope.ProjectID != nil {
		job.ProjectID = strings.TrimSpace(*req.Scope.ProjectID)
		exists, err := a.Repo.ProjectExists(job.ProjectID)
		if err != nil {
			return safeError(c, err)
		}
		if !exists {
			return c.JSON(http.StatusBadRequest, map[string]string{"code": "INVALID_PROJECT", "message": "projectId does not exist"})
		}
	}
	if _, err := a.Repo.EnqueueIfAbsent(&job, req.Sources); err != nil {
		return safeError(c, err)
	}
	return a.acceptedJob(c, job)
}
func (a API) acceptedJob(c echo.Context, job SyncJob) error {
	details, err := a.Repo.JobDetails(job)
	if err != nil {
		return safeError(c, err)
	}
	return c.JSON(http.StatusAccepted, details)
}
func (a API) jobs(c echo.Context) error {
	rows, err := a.Repo.Jobs(limit(c))
	if err != nil {
		return safeError(c, err)
	}
	out := make([]repository.JobDetails, 0, len(rows))
	for _, row := range rows {
		details, err := a.Repo.JobDetails(row)
		if err != nil {
			return safeError(c, err)
		}
		out = append(out, details)
	}
	return a.send(c, out)
}
func (a API) job(c echo.Context) error {
	row, err := a.Repo.Job(c.Param("id"))
	if errors.Is(err, repository.ErrNotFound) {
		return c.JSON(http.StatusNotFound, map[string]string{"code": "NOT_FOUND", "message": "job not found"})
	}
	if err != nil {
		return safeError(c, err)
	}
	details, err := a.Repo.JobDetails(row)
	if err != nil {
		return safeError(c, err)
	}
	return a.send(c, details)
}
