package repository

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrNotFound = gorm.ErrRecordNotFound
var ErrQaseProjectMapped = errors.New("qase project already mapped")

type Project struct {
	ID              string     `gorm:"primaryKey;type:uuid" json:"id"`
	JiraInitID      string     `gorm:"uniqueIndex" json:"jiraInitId"`
	JiraInitKey     string     `gorm:"index" json:"jiraInitKey"`
	Name            string     `json:"name"`
	Status          string     `json:"status"`
	Health          string     `json:"health"`
	QAOwner         string     `json:"qaOwner"`
	QaseProjectCode string     `gorm:"index" json:"qaseProjectCode"`
	StagingDate     *time.Time `json:"stagingDate"`
	BetaDate        *time.Time `json:"betaDate"`
	CreatedAt       time.Time  `json:"-"`
	UpdatedAt       time.Time  `json:"-"`
}
type Member struct {
	ID                  string  `gorm:"primaryKey;type:uuid" json:"id"`
	Name                string  `json:"name"`
	JiraAccountID       string  `gorm:"index" json:"jiraAccountId"`
	QaseMemberID        string  `gorm:"index" json:"qaseMemberId"`
	WeeklyCapacityHours float64 `json:"weeklyCapacityHours"`
}
type Allocation struct {
	ID           string    `gorm:"primaryKey;type:uuid"`
	ProjectID    string    `gorm:"index"`
	MemberID     string    `gorm:"index"`
	WeekStart    time.Time `gorm:"index"`
	PlannedHours float64
}
type JiraIssue struct {
	ID              string     `gorm:"primaryKey;type:uuid" json:"id"`
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
	ID          string `gorm:"primaryKey;type:uuid"`
	ProjectCode string `gorm:"uniqueIndex:ux_case"`
	CaseID      int64  `gorm:"uniqueIndex:ux_case"`
	Title       string
	UpdatedAt   time.Time
}
type QaseRun struct {
	ID          string `gorm:"primaryKey;type:uuid"`
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
	ID          string `gorm:"primaryKey;type:uuid"`
	ProjectCode string `gorm:"uniqueIndex:ux_run_case"`
	RunID       int64  `gorm:"uniqueIndex:ux_run_case"`
	CaseID      int64  `gorm:"uniqueIndex:ux_run_case"`
}
type QaseResult struct {
	ID               string `gorm:"primaryKey;type:uuid"`
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
	ID          string     `gorm:"primaryKey;type:uuid" json:"id"`
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
	ID         string     `gorm:"primaryKey;type:uuid" json:"id"`
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
	ID         string    `gorm:"primaryKey;type:uuid" json:"id"`
	JobID      string    `gorm:"index" json:"-"`
	StepID     string    `json:"stepId,omitempty"`
	OccurredAt time.Time `gorm:"index" json:"occurredAt"`
	Level      string    `json:"level"`
	Code       string    `json:"code"`
	Message    string    `json:"message"`
}
type SyncCursor struct {
	ID        string `gorm:"primaryKey;type:uuid"`
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
	q := r.db.Where("qase_project_code <> ''")
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
	var mappings, membership int64
	if err := r.db.Model(&Project{}).Where("qase_project_code = ?", p.QaseProjectCode).Count(&mappings).Error; err != nil {
		return out, false, err
	}
	if mappings != 1 {
		return out, false, nil
	}
	if err := r.db.Model(&QaseRunCase{}).Where("project_code = ?", p.QaseProjectCode).Count(&membership).Error; err != nil {
		return out, false, err
	}
	if membership == 0 {
		return out, false, nil
	}
	q := `SELECT count(*) FILTER (WHERE status='passed') AS passed,count(*) FILTER (WHERE status='failed') AS failed,count(*) FILTER (WHERE status='blocked') AS blocked FROM (SELECT DISTINCT ON (r.run_id,r.case_id,r.configuration_key) r.status FROM qase_results r JOIN qase_run_cases rc ON rc.project_code=r.project_code AND rc.run_id=r.run_id AND rc.case_id=r.case_id WHERE r.project_code=? ORDER BY r.run_id,r.case_id,r.configuration_key,r.ended_at DESC NULLS LAST,r.result_id DESC) latest`
	if err := r.db.Raw(q, p.QaseProjectCode).Scan(&out).Error; err != nil {
		return Counts{}, false, err
	}
	out.Total = membership
	return out, true, nil
}
func (r *Monitoring) SaveProjectMapping(jiraID, jiraKey, name, code string) (Project, error) {
	var p Project
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var duplicate int64
		if err := tx.Model(&Project{}).Where("qase_project_code = ? AND jira_init_id <> ?", code, jiraID).Count(&duplicate).Error; err != nil {
			return err
		}
		if duplicate > 0 {
			return ErrQaseProjectMapped
		}
		err := tx.Where("jira_init_id = ?", jiraID).First(&p).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			p = Project{ID: uuid.NewString(), JiraInitID: jiraID, Health: "unknown"}
		}
		p.JiraInitKey = jiraKey
		p.Name = name
		p.QaseProjectCode = code
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
	})
	return p, err
}
func (r *Monitoring) Workflow(from, to time.Time, projectID string) ([]WorkflowDay, error) {
	var rows []WorkflowDay
	latest := r.db.Table("qase_results r").Select("DISTINCT ON (r.project_code,r.run_id,r.case_id,r.configuration_key) r.project_code,r.run_id,r.case_id,r.configuration_key,r.status,r.ended_at").Joins("JOIN qase_run_cases rc ON rc.project_code=r.project_code AND rc.run_id=r.run_id AND rc.case_id=r.case_id").Order("r.project_code,r.run_id,r.case_id,r.configuration_key,r.ended_at DESC NULLS LAST,r.result_id DESC")
	q := r.db.Table("(?) latest", latest).Select("to_char(latest.ended_at, 'YYYY-MM-DD') as date, p.id as project_id, count(*) filter (where latest.status='passed') as passed, count(*) filter (where latest.status='failed') as failed, count(*) filter (where latest.status='blocked') as blocked, count(*) as total").Joins("join projects p on p.qase_project_code=latest.project_code").Where("p.qase_project_code IN (SELECT qase_project_code FROM projects GROUP BY qase_project_code HAVING count(*)=1)").Where("latest.ended_at >= ? AND latest.ended_at < ?", from, to).Group("date,p.id").Order("date")
	if projectID != "" {
		q = q.Where("p.id = ?", projectID)
	}
	err := q.Scan(&rows).Error
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
		v := WorkloadMember{ID: m.ID, Name: m.Name, DailyExecutions: []ExecutionDay{}}
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
