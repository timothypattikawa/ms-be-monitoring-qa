package monitoring

import (
	"crypto/subtle"
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
	Repo      *repository.Monitoring
	Connector Connector
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
	Counts          repository.Counts       `json:"counts"`
	CountsAvailable bool                    `json:"countsAvailable"`
	Runs            []repository.ProjectRun `json:"runs"`
}
type projectRegistration struct {
	JiraInitKey     string `json:"jiraInitKey"`
	Name            string `json:"name"`
	QaseProjectCode string `json:"qaseProjectCode"`
	QaseTestRunID   int64  `json:"qaseTestRunId"`
	QAOwner         string `json:"qaOwner"`
	StagingStartAt  string `json:"stagingStartAt"`
	StagingEndAt    string `json:"stagingEndAt"`
	BetaStartAt     string `json:"betaStartAt"`
	BetaEndAt       string `json:"betaEndAt"`
}
type projectSchedule struct {
	stagingStart, stagingEnd, betaStart, betaEnd time.Time
}

func (r *projectRegistration) validate() (projectSchedule, error) {
	r.JiraInitKey = strings.ToUpper(strings.TrimSpace(r.JiraInitKey))
	r.Name = strings.TrimSpace(r.Name)
	r.QaseProjectCode = strings.ToUpper(strings.TrimSpace(r.QaseProjectCode))
	r.QAOwner = strings.TrimSpace(r.QAOwner)
	if !jiraKeyPattern.MatchString(r.JiraInitKey) || r.Name == "" || r.QaseProjectCode == "" || r.QaseTestRunID <= 0 || r.QAOwner == "" {
		return projectSchedule{}, errors.New("missing required project field")
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

func (a API) Register(e *echo.Echo) {
	g := e.Group("/api/v1")
	g.GET("/projects", a.projects)
	g.GET("/projects/:id", a.project)
	g.POST("/projects", a.createProject, a.managerAuth)
	g.GET("/workflow", a.workflow)
	g.GET("/workload", a.workload)
	g.GET("/bugs", a.bugs)
	g.POST("/sync-jobs", a.createJob, a.managerAuth)
	g.GET("/sync-jobs", a.jobs)
	g.GET("/sync-jobs/:id", a.job)
}
func (a API) managerAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		key := os.Getenv("MONITOR_MANAGER_API_KEY")
		provided := c.Request().Header.Get("X-Manager-Key")
		if key == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(key)) != 1 {
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
	if n < 1 || n > 100 {
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
		result = append(result, projectView{Project: p, Counts: counts, CountsAvailable: available, Runs: runs})
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
	return a.send(c, projectView{Project: p, Counts: counts, CountsAvailable: available, Runs: runs})
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
	if _, err := a.Connector.JiraIssue(ctx, req.JiraInitKey); err != nil {
		return c.JSON(http.StatusUnprocessableEntity, map[string]string{"code": "JIRA_INIT_NOT_FOUND", "message": "jiraInitKey could not be validated against Jira"})
	}
	if _, err := a.Connector.QaseProject(ctx, req.QaseProjectCode); err != nil {
		return c.JSON(http.StatusUnprocessableEntity, map[string]string{"code": "QASE_PROJECT_NOT_FOUND", "message": "qaseProjectCode could not be validated against Qase"})
	}
	p, err := a.Repo.SaveProjectMapping(req.JiraInitKey, req.Name, req.QaseProjectCode, req.QaseTestRunID, req.QAOwner, schedule.stagingStart, schedule.stagingEnd, schedule.betaStart, schedule.betaEnd)
	if errors.Is(err, repository.ErrJiraInitMapped) {
		return c.JSON(http.StatusConflict, map[string]string{"code": "JIRA_INIT_ALREADY_REGISTERED", "message": "jiraInitKey is already registered"})
	}
	if errors.Is(err, repository.ErrQaseRunMapped) {
		return c.JSON(http.StatusConflict, map[string]string{"code": "QASE_RUN_ALREADY_REGISTERED", "message": "qaseProjectCode and qaseTestRunId are already registered"})
	}
	if err != nil {
		return safeError(c, err)
	}
	return c.JSON(http.StatusCreated, p)
}
func invalidProject(c echo.Context) error {
	return c.JSON(http.StatusBadRequest, map[string]string{"code": "INVALID_PROJECT", "message": "jiraInitKey, name, qaseProjectCode, qaseTestRunId, qaOwner and valid staging/beta ranges are required"})
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
func (a API) bugs(c echo.Context) error {
	rows, err := a.Repo.Bugs(limit(c), c.QueryParam("projectId"), c.QueryParam("severity"), c.QueryParam("status"), c.QueryParam("q"))
	if err != nil {
		return safeError(c, err)
	}
	return a.send(c, rows)
}

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
		if (s != "jira" && s != "qase") || seen[s] {
			return c.JSON(http.StatusBadRequest, map[string]string{"code": "INVALID_SOURCE", "message": "sources must contain jira and/or qase once"})
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
