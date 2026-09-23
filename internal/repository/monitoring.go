package repository

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrNotFound = gorm.ErrRecordNotFound
var ErrJiraInitMapped = errors.New("jira INIT is already registered")
var ErrQaseRunMapped = errors.New("Qase run is already registered")

type Project struct {
	ID              string     `gorm:"primaryKey" json:"id"`
	JiraInitID      *string    `gorm:"uniqueIndex" json:"jiraInitId,omitempty"`
	JiraInitKey     string     `gorm:"uniqueIndex" json:"jiraInitKey"`
	Name            string     `json:"name"`
	Status          string     `json:"status"`
	Health          string     `json:"health"`
	QAOwner         string     `json:"qaOwner"`
	QaseProjectCode string     `gorm:"index" json:"qaseProjectCode"`
	QaseTestRunID   int64      `gorm:"index" json:"qaseTestRunId"`
	StagingStartAt  *time.Time `json:"stagingStartAt"`
	StagingEndAt    *time.Time `json:"stagingEndAt"`
	BetaStartAt     *time.Time `json:"betaStartAt"`
	BetaEndAt       *time.Time `json:"betaEndAt"`
	CreatedAt       time.Time  `json:"-"`
	UpdatedAt       time.Time  `json:"-"`
}
type Member struct {
	ID                  string  `gorm:"primaryKey" json:"id"`
	Name                string  `json:"name"`
	JiraAccountID       string  `gorm:"index" json:"jiraAccountId"`
	QaseMemberID        string  `gorm:"index" json:"qaseMemberId"`
	WeeklyCapacityHours float64 `json:"weeklyCapacityHours"`
}
type Allocation struct {
	ID           string    `gorm:"primaryKey"`
	ProjectID    string    `gorm:"index"`
	MemberID     string    `gorm:"index"`
	WeekStart    time.Time `gorm:"index"`
	PlannedHours float64
}
type JiraIssue struct {
	ID              string     `gorm:"primaryKey" json:"id"`
	ExternalID      string     `gorm:"uniqueIndex" json:"jiraIssueId"`
	Key             string     `gorm:"index" json:"key"`
	ProjectID       string     `gorm:"index" json:"projectId"`
	Summary         string     `json:"summary"`
	IssueType       string     `gorm:"index" json:"issueType"`
	Severity        string     `json:"severity"`
	Status          string     `json:"status"`
	Creator         string     `json:"creator"`
	Reporter        string     `json:"reporter"`
	Assignee        string     `json:"assignee"`
	SourceUpdatedAt *time.Time `json:"updatedAt"`
	FetchedAt       time.Time  `json:"-"`
}
type QaseCase struct {
	ID          string `gorm:"primaryKey"`
	ProjectCode string `gorm:"uniqueIndex:ux_case"`
	CaseID      int64  `gorm:"uniqueIndex:ux_case"`
	Title       string
	UpdatedAt   time.Time
}
type QaseRun struct {
	ID          string `gorm:"primaryKey"`
	ProjectCode string `gorm:"uniqueIndex:ux_run"`
	RunID       int64  `gorm:"uniqueIndex:ux_run"`
	Title       string
	Status      string
	Platform    string
	Environment string
	StartedAt   *time.Time
	FinishedAt  *time.Time
	FetchedAt   time.Time
}
type QaseRunCase struct {
	ID          string `gorm:"primaryKey"`
	ProjectCode string `gorm:"uniqueIndex:ux_run_case"`
	RunID       int64  `gorm:"uniqueIndex:ux_run_case"`
	CaseID      int64  `gorm:"uniqueIndex:ux_run_case"`
}
type QaseResult struct {
	ID               string `gorm:"primaryKey"`
	ProjectCode      string `gorm:"uniqueIndex:ux_result"`
	ResultID         string `gorm:"uniqueIndex:ux_result"`
	RunID            int64  `gorm:"index"`
	CaseID           int64  `gorm:"index"`
	ConfigurationKey string `gorm:"index"`
	Status           string `gorm:"index"`
	MemberID         string `gorm:"index"`
	Environment      string
	Platform         string
	StartedAt        *time.Time
	EndedAt          *time.Time `gorm:"index"`
	FetchedAt        time.Time
}
type SyncJob struct {
	ID          string     `gorm:"primaryKey" json:"id"`
	RequestKey  string     `gorm:"uniqueIndex" json:"-"`
	Trigger     string     `json:"trigger"`
	Status      string     `gorm:"index" json:"status"`
	ProjectID   string     `gorm:"index" json:"projectId,omitempty"`
	Sources     string     `json:"-"`
	RequestedAt time.Time  `gorm:"index" json:"requestedAt"`
	StartedAt   *time.Time `json:"startedAt"`
	FinishedAt  *time.Time `json:"finishedAt"`
	HeartbeatAt *time.Time `json:"-"`
	Attempts    int        `json:"-"`
	Actor       string     `json:"-"`
}
type SyncStep struct {
	ID         string     `gorm:"primaryKey" json:"id"`
	JobID      string     `gorm:"index" json:"-"`
	Source     string     `json:"source"`
	Status     string     `json:"status"`
	Fetched    int        `json:"fetched"`
	Inserted   int        `json:"inserted"`
	Updated    int        `json:"updated"`
	Skipped    int        `json:"skipped"`
	ErrorCode  string     `json:"errorCode,omitempty"`
	StartedAt  *time.Time `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt"`
}
type SyncEvent struct {
	ID         string    `gorm:"primaryKey" json:"id"`
	JobID      string    `gorm:"index" json:"-"`
	StepID     string    `json:"stepId,omitempty"`
	OccurredAt time.Time `gorm:"index" json:"occurredAt"`
	Level      string    `json:"level"`
	Code       string    `json:"code"`
	Message    string    `json:"message"`
}
type SyncCursor struct {
	ID        string `gorm:"primaryKey"`
	Source    string `gorm:"uniqueIndex:ux_cursor"`
	ScopeKey  string `gorm:"uniqueIndex:ux_cursor"`
	Resource  string `gorm:"uniqueIndex:ux_cursor"`
	Watermark time.Time
	UpdatedAt time.Time
}

type Counts struct {
	Passed  int64 `json:"passed"`
	Failed  int64 `json:"failed"`
	Blocked int64 `json:"blocked"`
	Total   int64 `json:"total"`
}
type ProjectRun struct {
	RunID          int64      `json:"runId"`
	Title          string     `json:"title"`
	Environment    string     `json:"environment"`
	Platform       string     `json:"platform"`
	Scope          string     `json:"scope"`
	OwnerID        string     `json:"ownerId"`
	Passed         int64      `json:"passed"`
	Failed         int64      `json:"failed"`
	Blocked        int64      `json:"blocked"`
	Total          int64      `json:"total"`
	StartedAt      *time.Time `json:"startedAt"`
	FinishedAt     *time.Time `json:"finishedAt"`
	ElapsedSeconds *int64     `json:"elapsedSeconds"`
}
type WorkflowDay struct {
	Date      string `json:"date"`
	ProjectID string `json:"projectId"`
	Passed    int64  `json:"passed"`
	Failed    int64  `json:"failed"`
	Blocked   int64  `json:"blocked"`
	Total     int64  `json:"total"`
}
type ExecutionDay struct {
	Date     string `json:"date"`
	Executed int64  `json:"executed"`
	Passed   int64  `json:"passed"`
	Failed   int64  `json:"failed"`
	Blocked  int64  `json:"blocked"`
}
type WorkloadMember struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	PlannedHours    float64        `json:"plannedHours"`
	CapacityHours   float64        `json:"capacityHours"`
	QaseExecutions  int64          `json:"qaseExecutions"`
	DailyExecutions []ExecutionDay `json:"dailyExecutions"`
}
type JobDetails struct {
	SyncJob
	Steps  []SyncStep  `json:"steps"`
	Events []SyncEvent `json:"events,omitempty"`
}

func Models() []any {
	return []any{&Project{}, &Member{}, &Allocation{}, &JiraIssue{}, &QaseCase{}, &QaseRun{}, &QaseRunCase{}, &QaseResult{}, &SyncJob{}, &SyncStep{}, &SyncEvent{}, &SyncCursor{}}
}

type Monitoring struct{ db *gorm.DB }

func (r *Monitoring) AutoMigrate() error { return r.db.AutoMigrate(Models()...) }
func (r *Monitoring) FailExhausted(now time.Time) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var jobs []SyncJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("status = ? AND heartbeat_at < ? AND attempts >= ?", "running", now.Add(-10*time.Minute), 3).Find(&jobs).Error; err != nil {
			return err
		}
		for _, job := range jobs {
			if err := tx.Model(&SyncStep{}).Where("job_id = ? AND status <> ?", job.ID, "succeeded").Updates(map[string]any{"status": "failed", "error_code": "WORKER_RETRY_EXHAUSTED", "finished_at": now}).Error; err != nil {
				return err
			}
			if err := tx.Model(&job).Updates(map[string]any{"status": "failed", "finished_at": now}).Error; err != nil {
				return err
			}
			if err := tx.Create(&SyncEvent{ID: uuid.NewString(), JobID: job.ID, OccurredAt: now, Level: "error", Code: "WORKER_RETRY_EXHAUSTED", Message: "Worker stopped after three interrupted attempts"}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
func (r *Monitoring) EnqueueIfAbsent(job *SyncJob, sources []string) (bool, error) {
	created := false
	err := r.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "request_key"}}, DoNothing: true}).Create(job)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			var existing SyncJob
			if err := tx.Where("request_key = ?", job.RequestKey).First(&existing).Error; err != nil {
				return err
			}
			*job = existing
			return nil
		}
		created = true
		for _, source := range sources {
			if err := tx.Create(&SyncStep{ID: uuid.NewString(), JobID: job.ID, Source: source, Status: "queued"}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return created, err
}
func (r *Monitoring) ClaimJob(now time.Time) (*SyncJob, error) {
	var job SyncJob
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("status = ? OR (status = ? AND heartbeat_at < ? AND attempts < 3)", "queued", "running", now.Add(-10*time.Minute)).Order("requested_at").First(&job).Error; err != nil {
			return err
		}
		job.Status = "running"
		job.StartedAt = &now
		job.HeartbeatAt = &now
		job.Attempts++
		return tx.Save(&job).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &job, err
}
func (r *Monitoring) UpdateHeartbeat(jobID string, at time.Time) error {
	return r.db.Model(&SyncJob{}).Where("id = ? AND status = ?", jobID, "running").Update("heartbeat_at", at).Error
}
func (r *Monitoring) JobSteps(jobID string) ([]SyncStep, error) {
	var v []SyncStep
	err := r.db.Where("job_id = ?", jobID).Find(&v).Error
	return v, err
}
func (r *Monitoring) SaveStep(v *SyncStep) error { return r.db.Save(v).Error }
func (r *Monitoring) SaveJob(v *SyncJob) error   { return r.db.Save(v).Error }
func (r *Monitoring) Event(v *SyncEvent) error   { return r.db.Create(v).Error }
func (r *Monitoring) JiraIssue(externalID string) (JiraIssue, bool, error) {
	var v JiraIssue
	err := r.db.Where("external_id = ?", externalID).First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return v, false, nil
	}
	return v, err == nil, err
}
func (r *Monitoring) SaveJiraIssue(v *JiraIssue) error { return r.db.Save(v).Error }
func (r *Monitoring) ProjectByJiraID(id string) (Project, bool, error) {
	var v Project
	err := r.db.Where("jira_init_id = ?", id).First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return v, false, nil
	}
	return v, err == nil, err
}
func (r *Monitoring) ProjectByJiraKey(key string) (Project, bool, error) {
	var v Project
	err := r.db.Where("jira_init_key = ?", key).First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return v, false, nil
	}
	return v, err == nil, err
}
func (r *Monitoring) SaveProject(v *Project) error { return r.db.Save(v).Error }
func (r *Monitoring) MarkInactiveProjects(active []string) error {
	q := r.db.Model(&Project{}).Where("jira_init_id <> ''")
	if len(active) > 0 {
		q = q.Where("jira_init_id NOT IN ?", active)
	}
	return q.Update("status", "inactive").Error
}
func (r *Monitoring) QaseProjects(projectID string) ([]Project, error) {
	var v []Project
	q := r.db.Where("qase_project_code <> '' AND qase_test_run_id > 0")
	if projectID != "" {
		q = q.Where("id = ?", projectID)
	}
	err := q.Find(&v).Error
	return v, err
}
func (r *Monitoring) ReplaceRunCases(code string, runID int64, caseIDs []int64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		q := tx.Where("project_code = ? AND run_id = ?", code, runID)
		if len(caseIDs) > 0 {
			q = q.Where("case_id NOT IN ?", caseIDs)
		}
		if err := q.Delete(&QaseRunCase{}).Error; err != nil {
			return err
		}
		for _, id := range caseIDs {
			if id <= 0 {
				return errors.New("QASE_RUN_CASES_INVALID")
			}
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "project_code"}, {Name: "run_id"}, {Name: "case_id"}}, DoNothing: true}).Create(&QaseRunCase{ID: uuid.NewString(), ProjectCode: code, RunID: runID, CaseID: id}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
func (r *Monitoring) UpsertCase(v *QaseCase) (bool, error) {
	var old QaseCase
	err := r.db.Where("project_code = ? AND case_id = ?", v.ProjectCode, v.CaseID).First(&old).Error
	if err == nil {
		v.ID = old.ID
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}
	return err == nil, r.db.Save(v).Error
}
func (r *Monitoring) UpsertRun(v *QaseRun) (bool, error) {
	var old QaseRun
	err := r.db.Where("project_code = ? AND run_id = ?", v.ProjectCode, v.RunID).First(&old).Error
	if err == nil {
		v.ID = old.ID
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}
	return err == nil, r.db.Save(v).Error
}
func (r *Monitoring) UpsertResult(v *QaseResult) (bool, error) {
	var old QaseResult
	err := r.db.Where("project_code = ? AND result_id = ?", v.ProjectCode, v.ResultID).First(&old).Error
	if err == nil {
		v.ID = old.ID
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}
	return err == nil, r.db.Save(v).Error
}
func (r *Monitoring) SaveCursor(source, scope, resource string, watermark time.Time) error {
	var v SyncCursor
	err := r.db.Where("source=? AND scope_key=? AND resource=?", source, scope, resource).First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		v = SyncCursor{ID: uuid.NewString(), Source: source, ScopeKey: scope, Resource: resource}
	} else if err != nil {
		return err
	}
	v.Watermark = watermark
	return r.db.Save(&v).Error
}

func (r *Monitoring) LatestGlobalStep(source string) (SyncStep, bool, error) {
	var v SyncStep
	err := r.db.Model(&SyncStep{}).Joins("JOIN sync_jobs ON sync_jobs.id = sync_steps.job_id").Where("sync_steps.source = ? AND sync_steps.status = ? AND sync_jobs.project_id = ''", source, "succeeded").Order("sync_steps.finished_at desc").First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return v, false, nil
	}
	return v, err == nil, err
}
func (r *Monitoring) Projects(limit int, includeInactive bool, search string) ([]Project, error) {
	var v []Project
	q := r.db.Order("created_at desc").Limit(limit)
	if !includeInactive {
		q = q.Where("status <> ?", "inactive")
	}
	if search != "" {
		q = q.Where("name ILIKE ? OR jira_init_key ILIKE ?", "%"+search+"%", "%"+search+"%")
	}
	err := q.Find(&v).Error
	return v, err
}
func (r *Monitoring) Project(id string) (Project, error) {
	var v Project
	err := r.db.First(&v, "id = ?", id).Error
	return v, err
}
func (r *Monitoring) ProjectCounts(p Project) (Counts, bool, error) {
	var out Counts
	if p.QaseProjectCode == "" {
		return out, false, nil
	}
	if p.QaseTestRunID <= 0 {
		return out, false, nil
	}
	var mappings, membership int64
	if err := r.db.Model(&Project{}).Where("qase_project_code = ? AND qase_test_run_id = ?", p.QaseProjectCode, p.QaseTestRunID).Count(&mappings).Error; err != nil {
		return out, false, err
	}
	if mappings != 1 {
		return out, false, nil
	}
	if err := r.db.Model(&QaseRunCase{}).Where("project_code = ? AND run_id = ?", p.QaseProjectCode, p.QaseTestRunID).Count(&membership).Error; err != nil {
		return out, false, err
	}
	if membership == 0 {
		return out, false, nil
	}
	q := `SELECT count(*) FILTER (WHERE status='passed') AS passed,count(*) FILTER (WHERE status='failed') AS failed,count(*) FILTER (WHERE status='blocked') AS blocked FROM (SELECT DISTINCT ON (r.case_id) r.status FROM qase_results r JOIN qase_run_cases rc ON rc.project_code=r.project_code AND rc.run_id=r.run_id AND rc.case_id=r.case_id WHERE r.project_code=? AND r.run_id=? ORDER BY r.case_id,r.ended_at DESC NULLS LAST,r.result_id DESC) latest`
	if err := r.db.Raw(q, p.QaseProjectCode, p.QaseTestRunID).Scan(&out).Error; err != nil {
		return Counts{}, false, err
	}
	out.Total = membership
	return out, true, nil
}
func (r *Monitoring) ProjectRuns(p Project) ([]ProjectRun, error) {
	out := make([]ProjectRun, 0, 1)
	if p.QaseProjectCode == "" || p.QaseTestRunID <= 0 {
		return out, nil
	}
	var run QaseRun
	err := r.db.Where("project_code = ? AND run_id = ?", p.QaseProjectCode, p.QaseTestRunID).First(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	counts, _, err := r.ProjectCounts(p)
	if err != nil {
		return nil, err
	}
	var latest QaseResult
	err = r.db.Where("project_code = ? AND run_id = ?", p.QaseProjectCode, p.QaseTestRunID).
		Order("ended_at DESC NULLS LAST, fetched_at DESC").First(&latest).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	environment := run.Environment
	platform := run.Platform
	ownerID := ""
	if err == nil {
		if environment == "" {
			environment = latest.Environment
		}
		if platform == "" {
			platform = latest.Platform
		}
		ownerID = latest.MemberID
	}
	if environment == "" {
		title := strings.ToUpper(run.Title)
		if strings.Contains(title, "[STG]") || strings.Contains(title, "[STAGING]") || strings.Contains(title, "[BETA]") {
			environment = title
		}
	}
	if strings.EqualFold(strings.TrimSpace(platform), "unknown") {
		platform = ""
	}
	summary := ProjectRun{
		RunID: p.QaseTestRunID, Title: run.Title, Environment: normalizeQaseEnvironment(environment),
		Platform: strings.TrimSpace(platform), Scope: strings.TrimSpace(platform), OwnerID: ownerID,
		Passed: counts.Passed, Failed: counts.Failed, Blocked: counts.Blocked, Total: counts.Total,
		StartedAt: run.StartedAt, FinishedAt: run.FinishedAt,
	}
	if run.StartedAt != nil && run.FinishedAt != nil && !run.FinishedAt.Before(*run.StartedAt) {
		seconds := int64(run.FinishedAt.Sub(*run.StartedAt).Seconds())
		summary.ElapsedSeconds = &seconds
	}
	return append(out, summary), nil
}

func normalizeQaseEnvironment(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	switch {
	case strings.Contains(value, "BETA"):
		return "BETA"
	case strings.Contains(value, "STAGING"), strings.Contains(value, "[STG]"), value == "STG", strings.HasPrefix(value, "STG "), strings.HasPrefix(value, "STG-"):
		return "STAGING"
	default:
		return value
	}
}
func (r *Monitoring) SaveProjectMapping(jiraKey, name, code string, runID int64, qaOwner string, stagingStart, stagingEnd, betaStart, betaEnd time.Time) (Project, error) {
	var p Project
	err := r.db.Transaction(func(tx *gorm.DB) error {
		err := tx.Where("jira_init_key = ?", jiraKey).First(&p).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil && p.QaseProjectCode != "" {
			return ErrJiraInitMapped
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			p = Project{ID: uuid.NewString(), Health: "unknown"}
		}
		var qaseDuplicate int64
		if err := tx.Model(&Project{}).Where("qase_project_code = ? AND qase_test_run_id = ? AND id <> ?", code, runID, p.ID).Count(&qaseDuplicate).Error; err != nil {
			return err
		}
		if qaseDuplicate > 0 {
			return ErrQaseRunMapped
		}
		p.JiraInitKey = jiraKey
		p.Name = name
		p.QaseProjectCode = code
		p.QaseTestRunID = runID
		p.QAOwner = qaOwner
		p.StagingStartAt = &stagingStart
		p.StagingEndAt = &stagingEnd
		p.BetaStartAt = &betaStart
		p.BetaEndAt = &betaEnd
		return tx.Save(&p).Error
	})
	return p, err
}

const workflowQuery = `
WITH project_scope AS (
  SELECT p.id, p.qase_project_code, p.qase_test_run_id,
         (SELECT count(*) FROM qase_run_cases rc WHERE rc.project_code=p.qase_project_code AND rc.run_id=p.qase_test_run_id) AS total
  FROM projects p
  WHERE p.qase_project_code <> '' AND p.qase_test_run_id > 0 AND (? = '' OR p.id = ?)
), days AS (
  SELECT DISTINCT date_trunc('day', r.ended_at) AS day, p.id AS project_id
  FROM qase_results r
  JOIN project_scope p ON p.qase_project_code=r.project_code AND p.qase_test_run_id=r.run_id
  JOIN qase_run_cases rc ON rc.project_code=r.project_code AND rc.run_id=r.run_id AND rc.case_id=r.case_id
  WHERE r.ended_at >= ? AND r.ended_at < ?
)
SELECT to_char(d.day, 'YYYY-MM-DD') AS date, p.id AS project_id,
       count(latest.status) FILTER (WHERE latest.status='passed') AS passed,
       count(latest.status) FILTER (WHERE latest.status='failed') AS failed,
       count(latest.status) FILTER (WHERE latest.status='blocked') AS blocked,
       p.total AS total
FROM days d JOIN project_scope p ON p.id=d.project_id
LEFT JOIN LATERAL (
  SELECT DISTINCT ON (r.case_id) r.status
  FROM qase_results r
  JOIN qase_run_cases rc ON rc.project_code=r.project_code AND rc.run_id=r.run_id AND rc.case_id=r.case_id
  WHERE r.project_code=p.qase_project_code AND r.run_id=p.qase_test_run_id AND r.ended_at < d.day + interval '1 day'
  ORDER BY r.case_id, r.ended_at DESC NULLS LAST, r.result_id DESC
) latest ON true
WHERE p.total > 0
GROUP BY d.day,p.id,p.total
ORDER BY d.day,p.id`

func (r *Monitoring) Workflow(from, to time.Time, projectID string) ([]WorkflowDay, error) {
	rows := make([]WorkflowDay, 0)
	err := r.db.Raw(workflowQuery, projectID, projectID, from, to).Scan(&rows).Error
	return rows, err
}
func (r *Monitoring) Workload(from, to time.Time, memberID string) ([]WorkloadMember, error) {
	var members []Member
	q := r.db.Order("name")
	if memberID != "" {
		q = q.Where("id = ?", memberID)
	}
	if err := q.Find(&members).Error; err != nil {
		return nil, err
	}
	out := make([]WorkloadMember, 0, len(members))
	for _, m := range members {
		v := WorkloadMember{ID: m.ID, Name: memberDisplayName(m), DailyExecutions: []ExecutionDay{}}
		if err := r.db.Model(&Allocation{}).Select("coalesce(sum(planned_hours),0)").Where("member_id = ? AND week_start >= ? AND week_start < ?", m.ID, from, to).Scan(&v.PlannedHours).Error; err != nil {
			return nil, err
		}
		if err := r.db.Model(&QaseResult{}).Where("member_id = ? AND ended_at >= ? AND ended_at < ?", m.QaseMemberID, from, to).Count(&v.QaseExecutions).Error; err != nil {
			return nil, err
		}
		if err := r.db.Table("qase_results").Select("to_char(ended_at, 'YYYY-MM-DD') as date,count(*) as executed,count(*) filter (where status='passed') as passed,count(*) filter (where status='failed') as failed,count(*) filter (where status='blocked') as blocked").Where("member_id = ? AND ended_at >= ? AND ended_at < ?", m.QaseMemberID, from, to).Group("date").Order("date").Scan(&v.DailyExecutions).Error; err != nil {
			return nil, err
		}
		v.CapacityHours = float64(to.Sub(from).Hours()/168) * m.WeeklyCapacityHours
		out = append(out, v)
	}
	return out, nil
}
func memberDisplayName(member Member) string {
	if name := strings.TrimSpace(member.Name); name != "" {
		return name
	}
	return member.ID
}
func (r *Monitoring) Bugs(limit int, projectID, severity, status, search string) ([]JiraIssue, error) {
	var v []JiraIssue
	q := r.db.Where("lower(issue_type) IN ?", []string{"bug", "defect"}).Order("source_updated_at desc").Limit(limit)
	if projectID != "" {
		q = q.Where("project_id = ?", projectID)
	}
	if severity != "" {
		q = q.Where("severity = ?", severity)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if search != "" {
		q = q.Where("summary ILIKE ? OR key ILIKE ?", "%"+search+"%", "%"+search+"%")
	}
	err := q.Find(&v).Error
	return v, err
}
func (r *Monitoring) ProjectExists(id string) (bool, error) {
	var n int64
	err := r.db.Model(&Project{}).Where("id = ?", id).Count(&n).Error
	return n > 0, err
}
func (r *Monitoring) JobByRequestKey(key string) (SyncJob, error) {
	var v SyncJob
	err := r.db.Where("request_key = ?", key).First(&v).Error
	return v, err
}
func (r *Monitoring) Job(id string) (SyncJob, error) {
	var v SyncJob
	err := r.db.First(&v, "id = ?", id).Error
	return v, err
}
func (r *Monitoring) Jobs(limit int) ([]SyncJob, error) {
	var v []SyncJob
	err := r.db.Order("requested_at desc").Limit(limit).Find(&v).Error
	return v, err
}
func (r *Monitoring) JobDetails(job SyncJob) (JobDetails, error) {
	out := JobDetails{SyncJob: job, Steps: []SyncStep{}, Events: []SyncEvent{}}
	if err := r.db.Where("job_id = ?", job.ID).Order("source").Find(&out.Steps).Error; err != nil {
		return out, err
	}
	if err := r.db.Where("job_id = ?", job.ID).Order("occurred_at").Find(&out.Events).Error; err != nil {
		return out, err
	}
	return out, nil
}
