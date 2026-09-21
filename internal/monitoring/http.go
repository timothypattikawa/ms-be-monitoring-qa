package monitoring

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type API struct{ DB *gorm.DB }
type sourceState struct {
	Status   string     `json:"status"`
	SyncedAt *time.Time `json:"syncedAt"`
}
type envelope struct {
	AsOf    *time.Time             `json:"asOf"`
	Sources map[string]sourceState `json:"sources"`
	Data    any                    `json:"data"`
}
type counts struct {
	Passed  int64 `json:"passed"`
	Failed  int64 `json:"failed"`
	Blocked int64 `json:"blocked"`
	Total   int64 `json:"total"`
}
type projectView struct {
	Project
	Counts          counts `json:"counts"`
	CountsAvailable bool   `json:"countsAvailable"`
}
type jobView struct {
	SyncJob
	Steps  []SyncStep  `json:"steps"`
	Events []SyncEvent `json:"events,omitempty"`
}
type workflowDay struct {
	Date      string `json:"date"`
	ProjectID string `json:"projectId"`
	Passed    int64  `json:"passed"`
	Failed    int64  `json:"failed"`
	Blocked   int64  `json:"blocked"`
	Total     int64  `json:"total"`
}
type executionDay struct {
	Date     string `json:"date"`
	Executed int64  `json:"executed"`
	Passed   int64  `json:"passed"`
	Failed   int64  `json:"failed"`
	Blocked  int64  `json:"blocked"`
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
		var step SyncStep
		err := a.DB.Model(&SyncStep{}).
			Joins("JOIN sync_jobs ON sync_jobs.id = sync_steps.job_id").
			Where("sync_steps.source = ? AND sync_steps.status = ? AND sync_jobs.project_id = ''", source, "succeeded").
			Order("sync_steps.finished_at desc").First(&step).Error
		state := sourceState{Status: "never_synced"}
		if err == nil && step.FinishedAt != nil {
			state.SyncedAt = step.FinishedAt
			state.Status = "fresh"
			if time.Since(*step.FinishedAt) > 24*time.Hour {
				state.Status = "stale"
			}
			if out.AsOf == nil || step.FinishedAt.Before(*out.AsOf) {
				out.AsOf = step.FinishedAt
			}
		} else if err != nil && err != gorm.ErrRecordNotFound {
			return out, err
		}
		out.Sources[source] = state
		if state.Status == "never_synced" {
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
	var rows []Project
	q := a.DB.Order("created_at desc").Limit(limit(c))
	if c.QueryParam("includeInactive") != "true" {
		q = q.Where("status <> ?", "inactive")
	}
	if s := c.QueryParam("q"); s != "" {
		q = q.Where("name ILIKE ? OR jira_init_key ILIKE ?", "%"+s+"%", "%"+s+"%")
	}
	if err := q.Find(&rows).Error; err != nil {
		return safeError(c, err)
	}
	result := make([]projectView, 0, len(rows))
	for _, p := range rows {
		count, available, err := a.projectCounts(p)
		if err != nil {
			return safeError(c, err)
		}
		result = append(result, projectView{Project: p, Counts: count, CountsAvailable: available})
	}
	return a.send(c, result)
}
func (a API) project(c echo.Context) error {
	var p Project
	if err := a.DB.First(&p, "id = ?", c.Param("id")).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.JSON(404, map[string]string{"code": "NOT_FOUND", "message": "project not found"})
		}
		return safeError(c, err)
	}
	count, available, err := a.projectCounts(p)
	if err != nil {
		return safeError(c, err)
	}
	return a.send(c, projectView{Project: p, Counts: count, CountsAvailable: available})
}
func (a API) projectCounts(p Project) (counts, bool, error) {
	var out counts
	if p.QaseProjectCode == "" {
		return out, false, nil
	}
	var mappings int64
	if err := a.DB.Model(&Project{}).Where("qase_project_code = ?", p.QaseProjectCode).Count(&mappings).Error; err != nil {
		return out, false, err
	}
	if mappings != 1 {
		return out, false, nil
	}
	var membership int64
	if err := a.DB.Model(&QaseRunCase{}).Where("project_code = ?", p.QaseProjectCode).Count(&membership).Error; err != nil {
		return out, false, err
	}
	if membership == 0 {
		return out, false, nil
	}
	q := `SELECT count(*) FILTER (WHERE status='passed') AS passed,count(*) FILTER (WHERE status='failed') AS failed,count(*) FILTER (WHERE status='blocked') AS blocked FROM (SELECT DISTINCT ON (r.run_id,r.case_id,r.configuration_key) r.status FROM qase_results r JOIN qase_run_cases rc ON rc.project_code=r.project_code AND rc.run_id=r.run_id AND rc.case_id=r.case_id WHERE r.project_code=? ORDER BY r.run_id,r.case_id,r.configuration_key,r.ended_at DESC NULLS LAST,r.result_id DESC) latest`
	if err := a.DB.Raw(q, p.QaseProjectCode).Scan(&out).Error; err != nil {
		return counts{}, false, err
	}
	out.Total = membership
	return out, true, nil
}
func (a API) createProject(c echo.Context) error {
	var req struct {
		JiraInitID      string `json:"jiraInitId"`
		JiraInitKey     string `json:"jiraInitKey"`
		Name            string `json:"name"`
		QaseProjectCode string `json:"qaseProjectCode"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(400, map[string]string{"code": "INVALID_PROJECT", "message": "jiraInitId, jiraInitKey, name and qaseProjectCode are required"})
	}
	req.JiraInitID = strings.TrimSpace(req.JiraInitID)
	req.JiraInitKey = strings.TrimSpace(req.JiraInitKey)
	req.Name = strings.TrimSpace(req.Name)
	req.QaseProjectCode = strings.ToUpper(strings.TrimSpace(req.QaseProjectCode))
	if req.JiraInitID == "" || req.JiraInitKey == "" || req.Name == "" || req.QaseProjectCode == "" {
		return c.JSON(400, map[string]string{"code": "INVALID_PROJECT", "message": "jiraInitId, jiraInitKey, name and qaseProjectCode are required"})
	}
	var p Project
	if err := a.DB.Transaction(func(tx *gorm.DB) error {
		var duplicate int64
		if err := tx.Model(&Project{}).Where("qase_project_code = ? AND jira_init_id <> ?", req.QaseProjectCode, req.JiraInitID).Count(&duplicate).Error; err != nil {
			return err
		}
		if duplicate > 0 {
			return errQaseProjectMapped
		}
		err := tx.Where("jira_init_id = ?", req.JiraInitID).First(&p).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			p = Project{ID: uuid.NewString(), JiraInitID: req.JiraInitID, Health: "unknown"}
		}
		p.JiraInitKey = req.JiraInitKey
		p.Name = req.Name
		p.QaseProjectCode = req.QaseProjectCode
		p.Status = "pending_validation"
		if err := tx.Save(&p).Error; err != nil {
			return err
		}
		job := SyncJob{ID: uuid.NewString(), RequestKey: uuid.NewString(), Trigger: "project_validation", Status: "queued", ProjectID: p.ID, Sources: "jira,qase", RequestedAt: time.Now().UTC(), Actor: "manager"}
		if err := tx.Create(&job).Error; err != nil {
			return err
		}
		for _, source := range []string{"jira", "qase"} {
			if err := tx.Create(&SyncStep{ID: uuid.NewString(), JobID: job.ID, Source: source, Status: "queued"}).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		if errors.Is(err, errQaseProjectMapped) {
			return c.JSON(http.StatusConflict, map[string]string{"code": "QASE_PROJECT_ALREADY_MAPPED", "message": "qaseProjectCode is already mapped"})
		}
		return safeError(c, err)
	}
	return c.JSON(http.StatusCreated, p)
}

var errQaseProjectMapped = errors.New("qase project already mapped")

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
		return c.JSON(400, map[string]string{"code": "INVALID_DATE", "message": "use ISO dates"})
	}
	rows := make([]workflowDay, 0)
	latest := a.DB.Table("qase_results r").Select("DISTINCT ON (r.project_code,r.run_id,r.case_id,r.configuration_key) r.project_code,r.run_id,r.case_id,r.configuration_key,r.status,r.ended_at").Joins("JOIN qase_run_cases rc ON rc.project_code=r.project_code AND rc.run_id=r.run_id AND rc.case_id=r.case_id").Order("r.project_code,r.run_id,r.case_id,r.configuration_key,r.ended_at DESC NULLS LAST,r.result_id DESC")
	q := a.DB.Table("(?) latest", latest).Select("to_char(latest.ended_at, 'YYYY-MM-DD') as date, p.id as project_id, count(*) filter (where latest.status='passed') as passed, count(*) filter (where latest.status='failed') as failed, count(*) filter (where latest.status='blocked') as blocked, count(*) as total").Joins("join projects p on p.qase_project_code=latest.project_code").Where("p.qase_project_code IN (SELECT qase_project_code FROM projects GROUP BY qase_project_code HAVING count(*)=1)").Where("latest.ended_at >= ? AND latest.ended_at < ?", from, to).Group("date,p.id").Order("date")
	if id := c.QueryParam("projectId"); id != "" {
		q = q.Where("p.id = ?", id)
	}
	if err := q.Scan(&rows).Error; err != nil {
		return safeError(c, err)
	}
	return a.send(c, map[string]any{"days": rows})
}
func (a API) workload(c echo.Context) error {
	from, to, err := dateRange(c)
	if err != nil {
		return c.JSON(400, map[string]string{"code": "INVALID_DATE", "message": "use ISO dates"})
	}
	var members []Member
	q := a.DB.Order("name")
	if id := c.QueryParam("memberId"); id != "" {
		q = q.Where("id = ?", id)
	}
	if err := q.Find(&members).Error; err != nil {
		return safeError(c, err)
	}
	views := make([]map[string]any, 0, len(members))
	for _, m := range members {
		var executions int64
		var planned float64
		if err := a.DB.Model(&Allocation{}).Select("coalesce(sum(planned_hours),0)").Where("member_id = ? AND week_start >= ? AND week_start < ?", m.ID, from, to).Scan(&planned).Error; err != nil {
			return safeError(c, err)
		}
		if err := a.DB.Model(&QaseResult{}).Where("member_id = ? AND ended_at >= ? AND ended_at < ?", m.QaseMemberID, from, to).Count(&executions).Error; err != nil {
			return safeError(c, err)
		}
		daily := make([]executionDay, 0)
		if err := a.DB.Table("qase_results").Select("to_char(ended_at, 'YYYY-MM-DD') as date,count(*) as executed,count(*) filter (where status='passed') as passed,count(*) filter (where status='failed') as failed,count(*) filter (where status='blocked') as blocked").Where("member_id = ? AND ended_at >= ? AND ended_at < ?", m.QaseMemberID, from, to).Group("date").Order("date").Scan(&daily).Error; err != nil {
			return safeError(c, err)
		}
		views = append(views, map[string]any{"id": m.ID, "name": m.Name, "plannedHours": planned, "capacityHours": float64(to.Sub(from).Hours()/168) * m.WeeklyCapacityHours, "qaseExecutions": executions, "dailyExecutions": daily})
	}
	return a.send(c, map[string]any{"period": map[string]string{"from": from.Format("2006-01-02"), "to": to.Add(-time.Nanosecond).Format("2006-01-02")}, "members": views})
}
func (a API) bugs(c echo.Context) error {
	rows := make([]JiraIssue, 0)
	q := a.DB.Where("lower(issue_type) IN ?", []string{"bug", "defect"}).Order("source_updated_at desc").Limit(limit(c))
	if v := c.QueryParam("projectId"); v != "" {
		q = q.Where("project_id = ?", v)
	}
	if v := c.QueryParam("severity"); v != "" {
		q = q.Where("severity = ?", v)
	}
	if v := c.QueryParam("status"); v != "" {
		q = q.Where("status = ?", v)
	}
	if v := c.QueryParam("q"); v != "" {
		q = q.Where("summary ILIKE ? OR key ILIKE ?", "%"+v+"%", "%"+v+"%")
	}
	if err := q.Find(&rows).Error; err != nil {
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
		return c.JSON(400, map[string]string{"code": "INVALID_REQUEST", "message": "invalid JSON"})
	}
	if len(req.Sources) == 0 {
		req.Sources = []string{"jira", "qase"}
	}
	seen := map[string]bool{}
	for _, s := range req.Sources {
		if s != "jira" && s != "qase" || seen[s] {
			return c.JSON(400, map[string]string{"code": "INVALID_SOURCE", "message": "sources must contain jira and/or qase once"})
		}
		seen[s] = true
	}
	key := c.Request().Header.Get("Idempotency-Key")
	key = strings.TrimSpace(key)
	if key == "" {
		key = uuid.NewString()
	}
	if len(key) > 128 {
		return c.JSON(400, map[string]string{"code": "INVALID_KEY", "message": "idempotency key too long"})
	}
	var existing SyncJob
	err := a.DB.Where("request_key = ?", key).First(&existing).Error
	if err == nil {
		details, err := a.jobDetails(existing)
		if err != nil {
			return safeError(c, err)
		}
		return c.JSON(202, details)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return safeError(c, err)
	}
	job := SyncJob{ID: uuid.NewString(), RequestKey: key, Trigger: "manual", Status: "queued", Sources: strings.Join(req.Sources, ","), RequestedAt: time.Now().UTC(), Actor: "manager"}
	if req.Scope.ProjectID != nil {
		job.ProjectID = strings.TrimSpace(*req.Scope.ProjectID)
		var count int64
		if err := a.DB.Model(&Project{}).Where("id = ?", job.ProjectID).Count(&count).Error; err != nil {
			return safeError(c, err)
		}
		if count == 0 {
			return c.JSON(http.StatusBadRequest, map[string]string{"code": "INVALID_PROJECT", "message": "projectId does not exist"})
		}
	}
	err = a.DB.Transaction(func(tx *gorm.DB) error {
		created := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "request_key"}}, DoNothing: true}).Create(&job)
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected == 0 {
			return tx.Where("request_key = ?", key).First(&job).Error
		}
		for _, s := range req.Sources {
			if err := tx.Create(&SyncStep{ID: uuid.NewString(), JobID: job.ID, Source: s, Status: "queued"}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return safeError(c, err)
	}
	details, err := a.jobDetails(job)
	if err != nil {
		return safeError(c, err)
	}
	return c.JSON(202, details)
}
func (a API) jobDetails(job SyncJob) (jobView, error) {
	out := jobView{SyncJob: job, Steps: []SyncStep{}, Events: []SyncEvent{}}
	if err := a.DB.Where("job_id = ?", job.ID).Order("source").Find(&out.Steps).Error; err != nil {
		return out, err
	}
	if err := a.DB.Where("job_id = ?", job.ID).Order("occurred_at").Find(&out.Events).Error; err != nil {
		return out, err
	}
	return out, nil
}
func (a API) jobs(c echo.Context) error {
	var rows []SyncJob
	if err := a.DB.Order("requested_at desc").Limit(limit(c)).Find(&rows).Error; err != nil {
		return safeError(c, err)
	}
	out := make([]jobView, 0, len(rows))
	for _, row := range rows {
		details, err := a.jobDetails(row)
		if err != nil {
			return safeError(c, err)
		}
		out = append(out, details)
	}
	return a.send(c, out)
}
func (a API) job(c echo.Context) error {
	var row SyncJob
	if err := a.DB.First(&row, "id = ?", c.Param("id")).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.JSON(404, map[string]string{"code": "NOT_FOUND", "message": "job not found"})
		}
		return safeError(c, err)
	}
	details, err := a.jobDetails(row)
	if err != nil {
		return safeError(c, err)
	}
	return a.send(c, details)
}
