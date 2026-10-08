package repository

import (
	"errors"
	"fmt"
	"sort"
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
	ID              string  `gorm:"primaryKey" json:"id"`
	JiraInitID      *string `gorm:"uniqueIndex" json:"jiraInitId,omitempty"`
	JiraInitKey     string  `gorm:"uniqueIndex" json:"jiraInitKey"`
	Name            string  `json:"name"`
	Status          string  `json:"status"`
	Health          string  `json:"health"`
	QAOwner         string  `json:"qaOwner"`
	QaseProjectCode string  `gorm:"index" json:"qaseProjectCode"`
	ProjectSize        *string `json:"projectSize"`
	StagingMtttMinutes *int   `json:"stagingMtttMinutes"`
	BetaMtttMinutes    *int   `json:"betaMtttMinutes"`
	// TimelinePlanDays is the QA lead's manual plan (man days) compared
	// against the auto-computed STG working days on the Bugs page widget.
	TimelinePlanDays *float64 `json:"timelinePlanDays"`
	// QaseTotalCases/QaseTotalSuites come straight from Qase's own project
	// summary (GET /v1/project/{code}.counts) — the project's full scope,
	// not derived by paginating every run's cases.
	QaseTotalCases  int64 `json:"qaseTotalCases"`
	QaseTotalSuites int64 `json:"qaseTotalSuites"`
	// ponytail: qase_test_run_id column is orphaned (field removed, run
	// membership is now auto-discovered per sync) — AutoMigrate never drops
	// columns, harmless to leave behind.
	StagingStartAt *time.Time `json:"stagingStartAt"`
	StagingEndAt   *time.Time `json:"stagingEndAt"`
	BetaStartAt    *time.Time `json:"betaStartAt"`
	BetaEndAt      *time.Time `json:"betaEndAt"`
	CreatedAt      time.Time  `json:"-"`
	UpdatedAt      time.Time  `json:"-"`
}
type Member struct {
	ID                  string  `gorm:"primaryKey" json:"id"`
	Name                string  `json:"name"`
	JiraEmail           string  `json:"jiraEmail"`
	JiraAccountID       string  `gorm:"index" json:"-"`
	QaseDisplayName     string  `gorm:"column:qase_member_id;index" json:"qaseDisplayName"`
	WeeklyCapacityHours float64 `json:"weeklyCapacityHours"`
	// ponytail: no gorm "default:true" tag — gorm treats a zero-value bool
	// (false) with a default tag as "unset" and silently overwrites it with
	// the schema default on insert, so an explicit Active:false would never
	// persist. Callers (e.g. the qa-members create handler) must set
	// Active:true explicitly for new members instead of relying on a DB default.
	Active bool `gorm:"not null" json:"active"`
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
	ParentKey       string     `gorm:"index" json:"parentKey"`
	Environment     string     `gorm:"index" json:"environment"`
	CreatedAt       *time.Time `json:"createdAt"`
	Active          bool       `gorm:"not null;default:true;index" json:"active"`
	SyncScope       string     `gorm:"index" json:"-"`
	SourceUpdatedAt *time.Time `json:"updatedAt"`
	FetchedAt       time.Time  `json:"-"`
}
// JiraInitQA is one (INIT issue, QA) pair from Jira's QAs field. It covers
// every INIT with a QA in any status — not just projects registered on the
// dashboard — and is replaced wholesale on each QA-portfolio sync.
type JiraInitQA struct {
	InitKey       string    `gorm:"primaryKey" json:"initKey"`
	JiraAccountID string    `gorm:"primaryKey" json:"jiraAccountId"`
	DisplayName   string    `json:"displayName"`
	InitName      string    `json:"initName"`
	InitStatus    string    `json:"initStatus"`
	SyncedAt      time.Time `json:"syncedAt"`
}
type QaseCase struct {
	ID                string `gorm:"primaryKey"`
	ProjectCode       string `gorm:"uniqueIndex:ux_case"`
	CaseID            int64  `gorm:"uniqueIndex:ux_case"`
	Title             string
	PicNames          string
	TesterName        string
	TesterAndroidName string
	TesterIosName     string
	BetaTesterName    string
	UpdatedAt         time.Time
}
type QaseDefect struct {
	ID            string `gorm:"primaryKey"`
	ProjectCode   string `gorm:"uniqueIndex:ux_defect"`
	DefectID      int64  `gorm:"uniqueIndex:ux_defect"`
	Title         string
	Status        string
	Environment   string
	JiraKey       string
	JiraAssignee  string
	JiraReporter  string
	JiraCreator   string
	JiraPriority  string
	JiraStatus    string
	JiraCreatedAt *time.Time
	QaseCreatedAt *time.Time
	UpdatedAt     time.Time
}
type QaseRun struct {
	ID          string `gorm:"primaryKey"`
	ProjectCode string `gorm:"uniqueIndex:ux_run"`
	RunID       int64  `gorm:"uniqueIndex:ux_run"`
	Title       string
	Status      string
	Platform    string
	Environment string
	Active      bool
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
	// TesterName is a snapshot of the case's environment-appropriate tester
	// field (QaseCase.TesterName for STAGING, .BetaTesterName for BETA) taken
	// the first time this result is synced — see UpsertResult. Without this,
	// "who ran this on this date" was read live off the case's *current*
	// tester assignment, so reassigning a case in Qase today silently
	// rewrote every past date's PIC in the Detailed Execution History table.
	TesterName string
	StartedAt  *time.Time
	EndedAt    *time.Time `gorm:"index"`
	FetchedAt  time.Time
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
	Passed     int64 `json:"passed"`
	Failed     int64 `json:"failed"`
	Blocked    int64 `json:"blocked"`
	Skipped    int64 `json:"skipped"`
	Retest     int64 `json:"retest"`
	Invalid    int64 `json:"invalid"`
	InProgress int64 `json:"inProgress"`
	Cancelled  int64 `json:"cancelled"`
	Total      int64 `json:"total"`
}
type EnvironmentStats struct {
	Environment string `json:"environment"`
	ActiveRuns  int64  `json:"activeRuns"`
	Total       int64  `json:"total"`
	Executed    int64  `json:"executed"`
	Passed      int64  `json:"passed"`
	Failed      int64  `json:"failed"`
	Blocked     int64  `json:"blocked"`
	NotRun      int64  `json:"notRun"`
	Progress    int64  `json:"progress"`
}
type ProjectBugSummary struct {
	Staging                        int64 `json:"staging"`
	Beta                           int64 `json:"beta"`
	BetaOverThirtyPercentOfStaging bool  `json:"betaOverThirtyPercentOfStaging"`
	Total                          int64 `json:"total"`
	Canceled                       int64 `json:"canceled"`
}
type ProjectRun struct {
	RunID          int64      `json:"runId"`
	Title          string     `json:"title"`
	Environment    string     `json:"environment"`
	Platform       string     `json:"platform"`
	Scope          string     `json:"scope"`
	Testers        []string   `json:"testers"`
	Passed         int64      `json:"passed"`
	Failed         int64      `json:"failed"`
	Blocked        int64      `json:"blocked"`
	Skipped        int64      `json:"skipped"`
	Retest         int64      `json:"retest"`
	Invalid        int64      `json:"invalid"`
	InProgress     int64      `json:"inProgress"`
	Cancelled      int64      `json:"cancelled"`
	Total          int64      `json:"total"`
	StartedAt      *time.Time `json:"startedAt"`
	FinishedAt     *time.Time `json:"finishedAt"`
	ElapsedSeconds *int64     `json:"elapsedSeconds"`
}

// TesterProgress is a project's active-run progress broken down by the real
// per-case tester (see QaseCase.TesterName/TesterAndroidName/TesterIosName),
// replacing the broken Qase shared-account member_id attribution.
type TesterProgress struct {
	Environment string `json:"environment,omitempty"`
	Name        string `json:"name"`
	ActiveRuns  int64  `json:"activeRuns"`
	Passed      int64  `json:"passed"`
	Failed      int64  `json:"failed"`
	Blocked     int64  `json:"blocked"`
	Executed    int64  `json:"executed"`
	Total       int64  `json:"total"`
	NotRun      int64  `json:"notRun"`
	Progress    int64  `json:"progress"`
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
	ActiveProjects  int64          `json:"activeProjects"`
	TotalProjects   int64          `json:"totalProjects"`
	// QaseMapped is false until the QA lead sets the member's Qase display
	// name; until then the card shows Jira data only.
	QaseMapped bool `json:"qaseMapped"`
	// QaseName is the display name Qase records this QA's runs under (run
	// testers are matched against it, not Name).
	QaseName string `json:"qaseName"`
	// QaseProjects are the dashboard-registered projects this QA executed in
	// during the window — the Qase-side counterpart of Projects (Jira).
	QaseProjects []WorkloadProject `json:"qaseProjects"`
	NextProject *WorkloadProject  `json:"nextProject"`
	Projects    []WorkloadProject `json:"projects"`
}
type WorkloadProject struct {
	ID             string     `json:"id"`
	Key            string     `json:"key"`
	Name           string     `json:"name"`
	Status         string     `json:"status"`
	StagingStartAt *time.Time `json:"stagingStartAt"`
}
type JobDetails struct {
	SyncJob
	Steps  []SyncStep  `json:"steps"`
	Events []SyncEvent `json:"events,omitempty"`
}

func Models() []any {
	return []any{&Project{}, &Member{}, &Allocation{}, &JiraIssue{}, &JiraInitQA{}, &QaseCase{}, &QaseRun{}, &QaseRunCase{}, &QaseResult{}, &QaseDefect{}, &SyncJob{}, &SyncStep{}, &SyncEvent{}, &SyncCursor{}, &NationalHoliday{}}
}

type Monitoring struct{ db *gorm.DB }

func (r *Monitoring) AutoMigrate() error {
	if err := r.db.AutoMigrate(Models()...); err != nil {
		return err
	}
	return r.SeedNationalHolidays()
}
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
func (r *Monitoring) ReplaceProductionBugs(v []JiraIssue) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		active := make([]string, 0, len(v))
		for i := range v {
			active = append(active, v[i].ExternalID)
			var old JiraIssue
			err := tx.Where("external_id = ?", v[i].ExternalID).First(&old).Error
			if err == nil {
				v[i].ID = old.ID
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err := tx.Save(&v[i]).Error; err != nil {
				return err
			}
		}
		q := tx.Where("project_id = ?", "BUG")
		if len(active) > 0 {
			q = q.Where("external_id NOT IN ?", active)
		}
		return q.Delete(&JiraIssue{}).Error
	})
}

func (r *Monitoring) ReplaceProjectBugs(projectID, scope string, bugs []JiraIssue) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		active := make([]string, 0, len(bugs))
		for i := range bugs {
			active = append(active, bugs[i].ExternalID)
			bugs[i].ProjectID, bugs[i].SyncScope, bugs[i].Active = projectID, scope, true
			var old JiraIssue
			err := tx.Where("external_id = ?", bugs[i].ExternalID).First(&old).Error
			if err == nil {
				bugs[i].ID = old.ID
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err := tx.Save(&bugs[i]).Error; err != nil {
				return err
			}
		}
		q := tx.Model(&JiraIssue{}).Where("project_id = ? AND sync_scope = ?", projectID, scope)
		if len(active) > 0 {
			q = q.Where("external_id NOT IN ?", active)
		}
		return q.Update("active", false).Error
	})
}

// ReplaceJiraInitQAs swaps the whole QA-portfolio snapshot in one
// transaction, so an INIT a QA was removed from (or that left Jira's result)
// disappears from their portfolio.
func (r *Monitoring) ReplaceJiraInitQAs(rows []JiraInitQA) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&JiraInitQA{}).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.CreateInBatches(&rows, 200).Error
	})
}

// EnsureJiraMembers creates a QA member for every Jira QA that has none yet
// (matched by Jira accountId). New members start active with an empty Qase
// display name — the QA lead maps that later in the QA members page — and
// existing members are left untouched. It returns how many were created.
func (r *Monitoring) EnsureJiraMembers(users []Member) (int, error) {
	created := 0
	for _, u := range users {
		if u.JiraAccountID == "" {
			continue
		}
		var n int64
		if err := r.db.Model(&Member{}).Where("jira_account_id = ?", u.JiraAccountID).Count(&n).Error; err != nil {
			return created, err
		}
		if n > 0 {
			continue
		}
		u.ID, u.Active = uuid.NewString(), true
		if err := r.db.Create(&u).Error; err != nil {
			return created, err
		}
		created++
	}
	return created, nil
}
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
	q := r.db.Where("qase_project_code <> ''")
	if projectID != "" {
		q = q.Where("id = ?", projectID)
	}
	err := q.Find(&v).Error
	return v, err
}

// UpdateProjectQaseScope persists the project's full scope as reported by
// Qase's own project summary, so ProjectCounts doesn't need to derive it by
// paginating every run's cases.
func (r *Monitoring) UpdateProjectQaseScope(code string, cases, suites int64) error {
	return r.db.Model(&Project{}).Where("qase_project_code = ?", code).
		Updates(map[string]any{"qase_total_cases": cases, "qase_total_suites": suites}).Error
}
func (r *Monitoring) MarkRunsInactive(projectCode string, activeRunIDs []int64) error {
	q := r.db.Model(&QaseRun{}).Where("project_code = ?", projectCode)
	if len(activeRunIDs) > 0 {
		q = q.Where("run_id NOT IN ?", activeRunIDs)
	}
	return q.Update("active", false).Error
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
		// TesterName is a point-in-time snapshot: once set, a later resync
		// must never overwrite it even if the case's live tester field (or
		// this call's freshly-resolved value) has since changed.
		if old.TesterName != "" {
			v.TesterName = old.TesterName
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}
	return err == nil, r.db.Save(v).Error
}

func (r *Monitoring) BackfillResultTester(projectCode string, runID, caseID int64, tester string) error {
	if tester == "" {
		return nil
	}
	return r.db.Model(&QaseResult{}).
		Where("project_code = ? AND run_id = ? AND case_id = ? AND tester_name = ''", projectCode, runID, caseID).
		Update("tester_name", tester).Error
}

// CaseByID looks up a single synced case, used to snapshot its current
// tester assignment onto a result at ingest time (see UpsertResult).
func (r *Monitoring) CaseByID(projectCode string, caseID int64) (QaseCase, error) {
	var out QaseCase
	err := r.db.Where("project_code = ? AND case_id = ?", projectCode, caseID).First(&out).Error
	return out, err
}
func (r *Monitoring) UpsertDefect(v *QaseDefect) (bool, error) {
	var old QaseDefect
	err := r.db.Where("project_code = ? AND defect_id = ?", v.ProjectCode, v.DefectID).First(&old).Error
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
func (r *Monitoring) Members(includeInactive bool) ([]Member, error) {
	var v []Member
	q := r.db.Order("name asc")
	if !includeInactive {
		q = q.Where("active = ?", true)
	}
	err := q.Find(&v).Error
	return v, err
}

func (r *Monitoring) Member(id string) (Member, error) {
	var v Member
	err := r.db.First(&v, "id = ?", id).Error
	return v, err
}

func (r *Monitoring) SaveMember(v *Member) error { return r.db.Save(v).Error }
func (r *Monitoring) activeRuns(projectCode string) ([]QaseRun, error) {
	var runs []QaseRun
	err := r.db.Where("project_code = ? AND active = ?", projectCode, true).Find(&runs).Error
	return runs, err
}

// runCounts computes Counts for a single run (latest result per case, gated
// on run-case membership) — used by ProjectRuns for the per-run breakdown.
func (r *Monitoring) runCounts(projectCode string, runID int64) (Counts, int64, error) {
	var out Counts
	var membership int64
	if err := r.db.Model(&QaseRunCase{}).Where("project_code = ? AND run_id = ?", projectCode, runID).Count(&membership).Error; err != nil {
		return out, 0, err
	}
	if membership == 0 {
		return out, 0, nil
	}
	// ponytail: ROW_NUMBER()+NULLS LAST instead of Postgres' DISTINCT ON so this
	// query also runs against the in-memory SQLite test DB (testing.go) — same
	// "latest result per case" semantics, portable across both.
	q := `SELECT
		count(*) FILTER (WHERE lower(trim(status))='passed') AS passed,
		count(*) FILTER (WHERE lower(trim(status))='failed') AS failed,
		count(*) FILTER (WHERE lower(trim(status))='blocked') AS blocked,
		count(*) FILTER (WHERE lower(trim(status))='skipped') AS skipped,
		count(*) FILTER (WHERE lower(trim(status))='retest') AS retest,
		count(*) FILTER (WHERE lower(trim(status))='invalid') AS invalid,
		count(*) FILTER (WHERE lower(trim(status)) IN ('in_progress','in progress')) AS in_progress,
		count(*) FILTER (WHERE lower(trim(status)) IN ('cancel','cancelled','canceled')) AS cancelled
	FROM (
		SELECT r.status, ROW_NUMBER() OVER (PARTITION BY r.case_id ORDER BY r.ended_at DESC NULLS LAST,r.result_id DESC) AS rn
		FROM qase_results r
		JOIN qase_run_cases rc ON rc.project_code=r.project_code AND rc.run_id=r.run_id AND rc.case_id=r.case_id
		WHERE r.project_code=? AND r.run_id=?
	) latest WHERE rn=1`
	if err := r.db.Raw(q, projectCode, runID).Scan(&out).Error; err != nil {
		return Counts{}, 0, err
	}
	return out, membership, nil
}
func (r *Monitoring) ProjectCounts(p Project) (Counts, bool, error) {
	var out Counts
	if p.QaseProjectCode == "" {
		return out, false, nil
	}
	runs, err := r.activeRuns(p.QaseProjectCode)
	if err != nil {
		return out, false, err
	}
	var runIDs []int64
	for _, run := range runs {
		if DashboardEnvironment(run.Title) == "" {
			continue
		}
		runIDs = append(runIDs, run.RunID)
	}
	found := false
	if len(runIDs) > 0 {
		var membership int64
		if err := r.db.Raw("SELECT count(DISTINCT case_id) FROM qase_run_cases WHERE project_code = ? AND run_id IN ?",
			p.QaseProjectCode, runIDs).Scan(&membership).Error; err != nil {
			return Counts{}, false, err
		}
		if membership > 0 {
			found = true
			out.Total = membership
			// Latest result per case across ALL active runs, not a per-run
			// sum — the same scenario runs on STG and BETA (often several
			// platforms), so summing runs counts it once per run and can
			// push Passed past the unique-case Total. See
			// ProjectEnvironmentCounts for the per-environment variant.
			q := `SELECT
				count(*) FILTER (WHERE lower(trim(status))='passed') AS passed,
				count(*) FILTER (WHERE lower(trim(status))='failed') AS failed,
				count(*) FILTER (WHERE lower(trim(status))='blocked') AS blocked,
				count(*) FILTER (WHERE lower(trim(status))='skipped') AS skipped,
				count(*) FILTER (WHERE lower(trim(status))='retest') AS retest,
				count(*) FILTER (WHERE lower(trim(status))='invalid') AS invalid,
				count(*) FILTER (WHERE lower(trim(status)) IN ('in_progress','in progress')) AS in_progress,
				count(*) FILTER (WHERE lower(trim(status)) IN ('cancel','cancelled','canceled')) AS cancelled
			FROM (
				SELECT r.status, ROW_NUMBER() OVER (PARTITION BY r.case_id ORDER BY r.ended_at DESC NULLS LAST,r.result_id DESC) AS rn
				FROM qase_results r
				JOIN qase_run_cases rc ON rc.project_code=r.project_code AND rc.run_id=r.run_id AND rc.case_id=r.case_id
				WHERE r.project_code=? AND r.run_id IN ?
			) latest WHERE rn=1`
			// Scan into a separate dest for the same reason as
			// ProjectEnvironmentCounts (gorm would zero out.Total).
			var executed struct {
				Passed     int64
				Failed     int64
				Blocked    int64
				Skipped    int64
				Retest     int64
				Invalid    int64
				InProgress int64 `gorm:"column:in_progress"`
				Cancelled  int64
			}
			if err := r.db.Raw(q, p.QaseProjectCode, runIDs).Scan(&executed).Error; err != nil {
				return Counts{}, false, err
			}
			out.Passed, out.Failed, out.Blocked = executed.Passed, executed.Failed, executed.Blocked
			out.Skipped, out.Retest, out.Invalid = executed.Skipped, executed.Retest, executed.Invalid
			out.InProgress, out.Cancelled = executed.InProgress, executed.Cancelled
		}
	}
	// Denominator is the distinct case membership of tracked runs — repo
	// cases never attached to a run don't count as "untested" (Qase's
	// project-wide stat can also include stale/deleted cases). Only when
	// nothing is attached do we fall back to the repo count so an untouched
	// project still reports its real total.
	if out.Total == 0 && p.QaseTotalCases > 0 {
		out.Total = p.QaseTotalCases
		found = true
	}
	return out, found, nil
}

// ProjectEnvironmentCounts aggregates one environment's (STAGING or BETA)
// executed counts, deduping by case ID across every active run that
// resolves to it. The same scenario often runs on multiple platforms (e.g.
// "[STG] IOS" and "[STG] Android" both carry the same case), so summing
// each run's totals independently double-counts it and can push the
// percentage past 100%.
func (r *Monitoring) ProjectEnvironmentCounts(p Project, environment string) (Counts, error) {
	var out Counts
	if p.QaseProjectCode == "" {
		return out, nil
	}
	runs, err := r.activeRuns(p.QaseProjectCode)
	if err != nil {
		return out, err
	}
	var runIDs []int64
	for _, run := range runs {
		if DashboardEnvironment(run.Title) == environment {
			runIDs = append(runIDs, run.RunID)
		}
	}
	if len(runIDs) == 0 {
		return out, nil
	}
	if err := r.db.Raw("SELECT count(DISTINCT case_id) FROM qase_run_cases WHERE project_code = ? AND run_id IN ?",
		p.QaseProjectCode, runIDs).Scan(&out.Total).Error; err != nil {
		return Counts{}, err
	}
	q := `SELECT
		count(*) FILTER (WHERE lower(trim(status))='passed') AS passed,
		count(*) FILTER (WHERE lower(trim(status))='failed') AS failed,
		count(*) FILTER (WHERE lower(trim(status))='blocked') AS blocked,
		count(*) FILTER (WHERE lower(trim(status))='skipped') AS skipped,
		count(*) FILTER (WHERE lower(trim(status))='retest') AS retest,
		count(*) FILTER (WHERE lower(trim(status))='invalid') AS invalid,
		count(*) FILTER (WHERE lower(trim(status)) IN ('in_progress','in progress')) AS in_progress,
		count(*) FILTER (WHERE lower(trim(status)) IN ('cancel','cancelled','canceled')) AS cancelled
	FROM (
		SELECT r.status, ROW_NUMBER() OVER (PARTITION BY r.case_id ORDER BY r.ended_at DESC NULLS LAST,r.result_id DESC) AS rn
		FROM qase_results r
		JOIN qase_run_cases rc ON rc.project_code=r.project_code AND rc.run_id=r.run_id AND rc.case_id=r.case_id
		WHERE r.project_code=? AND r.run_id IN ?
	) latest WHERE rn=1`
	// A separate dest: scanning the "total"-less result set of this query
	// straight into &out would zero the Total we just set above (gorm
	// clears struct fields with no matching column on each Scan).
	var executed struct {
		Passed     int64
		Failed     int64
		Blocked    int64
		Skipped    int64
		Retest     int64
		Invalid    int64
		InProgress int64 `gorm:"column:in_progress"`
		Cancelled  int64
	}
	if err := r.db.Raw(q, p.QaseProjectCode, runIDs).Scan(&executed).Error; err != nil {
		return Counts{}, err
	}
	out.Passed, out.Failed, out.Blocked = executed.Passed, executed.Failed, executed.Blocked
	out.Skipped, out.Retest, out.Invalid = executed.Skipped, executed.Retest, executed.Invalid
	out.InProgress, out.Cancelled = executed.InProgress, executed.Cancelled
	return out, nil
}

func (r *Monitoring) ProjectEnvironmentStats(p Project, environment string) (EnvironmentStats, error) {
	out := EnvironmentStats{Environment: environment}
	counts, err := r.ProjectEnvironmentCounts(p, environment)
	if err != nil {
		return out, err
	}
	out.Total, out.Passed, out.Failed, out.Blocked = counts.Total, counts.Passed, counts.Failed, counts.Blocked
	runs, err := r.activeRuns(p.QaseProjectCode)
	if err != nil {
		return out, err
	}
	var runIDs []int64
	for _, run := range runs {
		if DashboardEnvironment(run.Title) == environment {
			runIDs = append(runIDs, run.RunID)
		}
	}
	out.ActiveRuns = int64(len(runIDs))
	if len(runIDs) > 0 {
		q := `SELECT count(*) FROM (SELECT r.case_id, ROW_NUMBER() OVER (PARTITION BY r.case_id ORDER BY r.ended_at DESC NULLS LAST,r.result_id DESC) rn FROM qase_results r JOIN qase_run_cases rc ON rc.project_code=r.project_code AND rc.run_id=r.run_id AND rc.case_id=r.case_id WHERE r.project_code=? AND r.run_id IN ?) latest WHERE rn=1`
		if err := r.db.Raw(q, p.QaseProjectCode, runIDs).Scan(&out.Executed).Error; err != nil {
			return EnvironmentStats{}, err
		}
	}
	if out.Executed > out.Total {
		out.Executed = out.Total
	}
	out.NotRun = out.Total - out.Executed
	if out.Total > 0 {
		out.Progress = out.Executed * 100 / out.Total
	}
	return out, nil
}

func (r *Monitoring) ProjectAssigneeProgress(p Project) ([]TesterProgress, error) {
	if p.QaseProjectCode == "" {
		return []TesterProgress{}, nil
	}
	runs, err := r.activeRuns(p.QaseProjectCode)
	if err != nil {
		return nil, err
	}
	type state struct {
		progress TesterProgress
		cases    map[int64]bool
		runs     map[int64]bool
	}
	states := map[string]*state{}
	for _, run := range runs {
		environment := DashboardEnvironment(run.Title)
		if environment == "" {
			continue
		}
		var cases []QaseCase
		if err := r.db.Model(&QaseCase{}).
			Joins("JOIN qase_run_cases rc ON rc.project_code=qase_cases.project_code AND rc.case_id=qase_cases.case_id").
			Where("rc.project_code=? AND rc.run_id=?", p.QaseProjectCode, run.RunID).Find(&cases).Error; err != nil {
			return nil, err
		}
		for _, testCase := range cases {
			name := ResolveAssignedTester(run, testCase)
			key := environment + "\x00" + name
			if states[key] == nil {
				states[key] = &state{progress: TesterProgress{Environment: environment, Name: name}, cases: map[int64]bool{}, runs: map[int64]bool{}}
			}
			states[key].cases[testCase.CaseID] = true
			states[key].runs[run.RunID] = true
		}
	}
	var results []QaseResult
	if err := r.db.Where("project_code = ?", p.QaseProjectCode).
		Order("ended_at DESC NULLS LAST, result_id DESC").Find(&results).Error; err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, result := range results {
		name := strings.TrimSpace(result.TesterName)
		if name == "" {
			name = "Unassigned"
		}
		var run QaseRun
		if err := r.db.Where("project_code=? AND run_id=? AND active=?", p.QaseProjectCode, result.RunID, true).First(&run).Error; err != nil {
			continue
		}
		environment := DashboardEnvironment(run.Title)
		key := environment + "\x00" + name
		s := states[key]
		if environment == "" || s == nil || seen[key+fmt.Sprint("\x00", result.CaseID)] {
			continue
		}
		seen[key+fmt.Sprint("\x00", result.CaseID)] = true
		s.progress.Executed++
		switch strings.ToLower(result.Status) {
		case "passed":
			s.progress.Passed++
		case "failed":
			s.progress.Failed++
		case "blocked":
			s.progress.Blocked++
		}
	}
	out := make([]TesterProgress, 0, len(states))
	for _, s := range states {
		s.progress.Total = int64(len(s.cases))
		s.progress.ActiveRuns = int64(len(s.runs))
		if s.progress.Executed > s.progress.Total {
			s.progress.Executed = s.progress.Total
		}
		s.progress.NotRun = s.progress.Total - s.progress.Executed
		if s.progress.Total > 0 {
			s.progress.Progress = s.progress.Executed * 100 / s.progress.Total
		}
		out = append(out, s.progress)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Environment == out[j].Environment {
			return out[i].Name < out[j].Name
		}
		return out[i].Environment < out[j].Environment
	})
	return out, nil
}
func (r *Monitoring) ProjectRuns(p Project) ([]ProjectRun, error) {
	out := make([]ProjectRun, 0)
	if p.QaseProjectCode == "" {
		return out, nil
	}
	runs, err := r.activeRuns(p.QaseProjectCode)
	if err != nil {
		return nil, err
	}
	for _, run := range runs {
		if DashboardEnvironment(run.Title) == "" {
			continue
		}
		counts, membership, err := r.runCounts(p.QaseProjectCode, run.RunID)
		if err != nil {
			return nil, err
		}
		if membership == 0 {
			continue
		}
		var latest QaseResult
		err = r.db.Where("project_code = ? AND run_id = ?", p.QaseProjectCode, run.RunID).
			Order("ended_at DESC NULLS LAST, fetched_at DESC").First(&latest).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		environment := run.Environment
		platform := run.Platform
		if err == nil {
			if environment == "" {
				environment = latest.Environment
			}
			if platform == "" {
				platform = latest.Platform
			}
		}
		if strings.EqualFold(strings.TrimSpace(platform), "unknown") {
			platform = ""
		}
		testers, err := r.runTesterNames(p.QaseProjectCode, run.RunID, run.Platform)
		if err != nil {
			return nil, err
		}
		summary := ProjectRun{
			RunID: run.RunID, Title: run.Title, Environment: DashboardEnvironment(run.Title),
			Platform: strings.TrimSpace(platform), Scope: runScopeLabel(run.Title, platform), Testers: testers,
			Passed: counts.Passed, Failed: counts.Failed, Blocked: counts.Blocked,
			Skipped: counts.Skipped, Retest: counts.Retest, Invalid: counts.Invalid,
			InProgress: counts.InProgress, Cancelled: counts.Cancelled, Total: membership,
			StartedAt: run.StartedAt, FinishedAt: run.FinishedAt,
		}
		if run.StartedAt != nil && run.FinishedAt != nil && !run.FinishedAt.Before(*run.StartedAt) {
			seconds := int64(run.FinishedAt.Sub(*run.StartedAt).Seconds())
			summary.ElapsedSeconds = &seconds
		}
		out = append(out, summary)
	}
	return out, nil
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

// DashboardEnvironment accepts only product run prefixes. Other Qase runs
// remain synced but never contribute to dashboard statistics.
func DashboardEnvironment(title string) string {
	value := strings.ToUpper(strings.TrimSpace(title))
	switch {
	case strings.HasPrefix(value, "[STG]"), strings.HasPrefix(value, "[STAGING]"), value == "STG", strings.HasPrefix(value, "STG "), strings.HasPrefix(value, "STG-"):
		return "STAGING"
	case strings.HasPrefix(value, "[BETA]"), value == "BETA", strings.HasPrefix(value, "BETA "), strings.HasPrefix(value, "BETA-"):
		return "BETA"
	default:
		return ""
	}
}

func ResolveAssignedTester(run QaseRun, testCase QaseCase) string {
	var tester string
	switch DashboardEnvironment(run.Title) {
	case "STAGING":
		platform := strings.ToUpper(strings.TrimSpace(run.Platform))
		switch platform {
		case "IOS":
			tester = testCase.TesterIosName
		case "ANDROID", "AOS":
			tester = testCase.TesterAndroidName
		default:
			tester = testCase.TesterName
		}
	case "BETA":
		tester = testCase.BetaTesterName
	default:
		return ""
	}
	if tester = strings.TrimSpace(tester); tester == "" {
		return "Unassigned"
	}
	return tester
}

// runScopeLabel returns a run's full display name, stripping only a leading
// "[TAG] " environment marker (e.g. "[BETA] APO Main" -> "APO Main") so two
// runs sharing a platform bucket (e.g. two APO runs) stay distinguishable —
// unlike Platform, which collapses to a coarse bucket like "APO" for both.
func runScopeLabel(title, platform string) string {
	t := strings.TrimSpace(title)
	if strings.HasPrefix(t, "[") {
		if idx := strings.Index(t, "] "); idx != -1 {
			t = strings.TrimSpace(t[idx+2:])
		}
	}
	if t != "" {
		return t
	}
	return strings.TrimSpace(platform)
}

// ResolveEnvironment normalizes a Qase run's environment, falling back to a
// title marker ([STG]/[STAGING]/[BETA]) when Qase's own environment field is
// blank. Shared by ProjectRuns and the defect-sync environment lookup
// (worker.go's syncQaseDefects, via a linked run's QaseRun row).
func ResolveEnvironment(environment, title string) string {
	if environment == "" {
		title = strings.ToUpper(title)
		if strings.Contains(title, "[STG]") || strings.Contains(title, "[STAGING]") || strings.Contains(title, "[BETA]") {
			environment = title
		}
	}
	return normalizeQaseEnvironment(environment)
}

// testerPlatformColumns is the single source for "which QaseCase tester
// column applies to this run platform" — IOS/AOS get their own tester
// field, everything else falls back to the shared tester_name field. Feeds
// both testerColumnFor (single known platform, e.g. per-run lookups) and
// testerColumnCaseSQL (row-varying platform, e.g. the workload queries).
var testerPlatformColumns = []struct{ platform, column string }{
	{"IOS", "tester_ios_name"},
	{"AOS", "tester_android_name"},
}

const testerNameDefaultColumn = "tester_name"

func testerColumnFor(platform string) string {
	platform = strings.ToUpper(strings.TrimSpace(platform))
	for _, pc := range testerPlatformColumns {
		if pc.platform == platform {
			return pc.column
		}
	}
	return testerNameDefaultColumn
}

func testerColumnCaseSQL(caseAlias, runAlias string) string {
	expr := "CASE "
	for _, pc := range testerPlatformColumns {
		expr += "WHEN " + runAlias + ".platform = '" + pc.platform + "' THEN " + caseAlias + "." + pc.column + " "
	}
	expr += "ELSE " + caseAlias + "." + testerNameDefaultColumn + " END"
	return expr
}

type TesterDailyExecution struct {
	Date        string `json:"date"`
	Tester      string `json:"tester"`
	Environment string `json:"environment"`
	Executed    int64  `json:"executed"`
	Passed      int64  `json:"passed"`
	Failed      int64  `json:"failed"`
	Blocked     int64  `json:"blocked"`
	Skipped     int64  `json:"skipped"`
	Retest      int64  `json:"retest"`
	InProgress  int64  `json:"inProgress"`
	Invalid     int64  `json:"invalid"`
	Cancelled   int64  `json:"cancelled"`
}

// projectTesterDailyQuery buckets every result (including retries — each
// attempt is its own qase_results row) by the day it finished and by
// qase_results.tester_name — a snapshot of the case's environment-appropriate
// tester field taken when the result was first synced (see UpsertResult),
// not a live join to qase_cases. Reading the case live would retroactively
// repaint a past date's PIC whenever that case gets reassigned in Qase
// afterward.
const projectTesterDailyQuery = `
SELECT to_char(qase_results.ended_at, 'YYYY-MM-DD') as date,
  COALESCE(NULLIF(TRIM(qase_results.tester_name), ''), 'Unassigned') as tester,
  count(*) as executed,
  count(*) filter (where lower(qase_results.status)='passed') as passed,
  count(*) filter (where lower(qase_results.status)='failed') as failed,
  count(*) filter (where lower(qase_results.status)='blocked') as blocked,
  count(*) filter (where lower(qase_results.status)='skipped') as skipped,
  count(*) filter (where lower(qase_results.status)='retest') as retest,
  count(*) filter (where lower(qase_results.status)='in_progress') as in_progress,
  count(*) filter (where lower(qase_results.status)='invalid') as invalid,
  count(*) filter (where lower(qase_results.status)='cancel') as cancelled
FROM qase_results
WHERE qase_results.project_code = ? AND qase_results.run_id IN ? AND qase_results.ended_at >= ? AND qase_results.ended_at < ?
GROUP BY date, tester
ORDER BY date`

// ProjectTesterDailyExecutions only looks back 5 days — the trend chart and
// detailed history table only ever show a handful of days, and scanning
// every historical result made the project detail dialog noticeably slow
// to load.
const projectTesterDailyWindow = 5 * 24 * time.Hour

func (r *Monitoring) ProjectTesterDailyExecutions(p Project) ([]TesterDailyExecution, error) {
	to := time.Now().UTC()
	return r.ProjectTesterDailyExecutionsRange(p, to.Add(-30*24*time.Hour), to)
}

func (r *Monitoring) ProjectTesterDailyExecutionsRange(p Project, from, to time.Time) ([]TesterDailyExecution, error) {
	out := make([]TesterDailyExecution, 0)
	if p.QaseProjectCode == "" {
		return out, nil
	}
	runs, err := r.activeRuns(p.QaseProjectCode)
	if err != nil {
		return nil, err
	}
	var stagingRunIDs, betaRunIDs []int64
	for _, run := range runs {
		switch DashboardEnvironment(run.Title) {
		case "STAGING":
			stagingRunIDs = append(stagingRunIDs, run.RunID)
		case "BETA":
			betaRunIDs = append(betaRunIDs, run.RunID)
		}
	}
	staging, err := r.testerDailyExecutionsForRuns(p.QaseProjectCode, stagingRunIDs, "STAGING", from, to)
	if err != nil {
		return nil, err
	}
	beta, err := r.testerDailyExecutionsForRuns(p.QaseProjectCode, betaRunIDs, "BETA", from, to)
	if err != nil {
		return nil, err
	}
	out = append(out, staging...)
	out = append(out, beta...)
	return out, nil
}

func (r *Monitoring) testerDailyExecutionsForRuns(projectCode string, runIDs []int64, environment string, from, to time.Time) ([]TesterDailyExecution, error) {
	if len(runIDs) == 0 {
		return nil, nil
	}
	var rows []TesterDailyExecution
	if err := r.db.Raw(projectTesterDailyQuery, projectCode, runIDs, from, to).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].Environment = environment
	}
	return rows, nil
}

// runTesterNames returns the distinct testers assigned to a run's member
// cases, platform-aware via testerColumnFor — used for ProjectRun.Testers.
func (r *Monitoring) runTesterNames(projectCode string, runID int64, platform string) ([]string, error) {
	col := testerColumnFor(platform)
	q := "SELECT DISTINCT qc." + col + " AS name FROM qase_run_cases rc JOIN qase_cases qc ON qc.project_code = rc.project_code AND qc.case_id = rc.case_id WHERE rc.project_code = ? AND rc.run_id = ? AND qc." + col + " <> ''"
	var rows []struct{ Name string }
	if err := r.db.Raw(q, projectCode, runID).Scan(&rows).Error; err != nil {
		return nil, err
	}
	names := make([]string, len(rows))
	for i, row := range rows {
		names[i] = row.Name
	}
	sort.Strings(names)
	return names, nil
}

// RunByID looks up a single synced QaseRun by project code and Qase run ID,
// used by defect-sync to resolve a defect's environment from its linked runs.
func (r *Monitoring) RunByID(projectCode string, runID int64) (QaseRun, bool, error) {
	var v QaseRun
	err := r.db.Where("project_code = ? AND run_id = ?", projectCode, runID).First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return v, false, nil
	}
	return v, err == nil, err
}

func (r *Monitoring) RunByResultID(projectCode, resultID string) (QaseRun, bool, error) {
	var run QaseRun
	err := r.db.Model(&QaseRun{}).
		Joins("JOIN qase_results ON qase_results.project_code = qase_runs.project_code AND qase_results.run_id = qase_runs.run_id").
		Where("qase_results.project_code = ? AND qase_results.result_id = ?", projectCode, resultID).
		First(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return run, false, nil
	}
	return run, err == nil, err
}

// ProjectTesterBreakdown reports the total scenarios (cases) each QA has
// created in this project, per product's own definition: attribution comes
// from the case's "QA PIC" field (who created it), not who executed it or
// which run it's attached to. A case with multiple PICs counts once for
// each. Passed/Failed are left zero — the frontend only reads Total.
func (r *Monitoring) ProjectTesterBreakdown(p Project) ([]TesterProgress, error) {
	out := make([]TesterProgress, 0)
	if p.QaseProjectCode == "" {
		return out, nil
	}
	var cases []QaseCase
	if err := r.db.Where("project_code = ? AND pic_names <> ''", p.QaseProjectCode).Find(&cases).Error; err != nil {
		return nil, err
	}
	counts := map[string]int64{}
	for _, c := range cases {
		for _, name := range strings.Split(c.PicNames, ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			counts[name]++
		}
	}
	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		out = append(out, TesterProgress{Name: name, Total: counts[name]})
	}
	return out, nil
}
func (r *Monitoring) SaveProjectMapping(jiraKey, name, code string, qaOwner string, stagingStart, stagingEnd, betaStart, betaEnd time.Time) (Project, error) {
	return r.SaveProjectMappingWithMetadata(jiraKey, name, code, qaOwner, nil, nil, nil, stagingStart, stagingEnd, betaStart, betaEnd)
}

func (r *Monitoring) SaveProjectMappingWithMetadata(jiraKey, name, code, qaOwner string, projectSize *string, stagingMttt, betaMttt *int, stagingStart, stagingEnd, betaStart, betaEnd time.Time) (Project, error) {
	var p Project
	err := r.db.Transaction(func(tx *gorm.DB) error {
		// ponytail: check-then-insert race on duplicate jiraInitKey/qaseProjectCode is possible under concurrent requests; acceptable given this is a manager-only, low-volume endpoint — add a DB-level advisory lock if concurrent creates become a real issue
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
		if err := tx.Model(&Project{}).Where("qase_project_code = ? AND id <> ?", code, p.ID).Count(&qaseDuplicate).Error; err != nil {
			return err
		}
		if qaseDuplicate > 0 {
			return ErrQaseRunMapped
		}
		p.JiraInitKey = jiraKey
		p.Name = name
		p.QaseProjectCode = code
		p.QAOwner = qaOwner
		p.ProjectSize = projectSize
		p.StagingMtttMinutes = stagingMttt
		p.BetaMtttMinutes = betaMttt
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
  SELECT p.id, p.qase_project_code,
         (SELECT count(*) FROM qase_run_cases rc JOIN qase_runs ar ON ar.project_code=rc.project_code AND ar.run_id=rc.run_id AND ar.active=true WHERE rc.project_code=p.qase_project_code) AS total
  FROM projects p
  WHERE p.qase_project_code <> '' AND (? = '' OR p.id = ?)
), days AS (
  SELECT DISTINCT date_trunc('day', r.ended_at) AS day, p.id AS project_id
  FROM qase_results r
  JOIN project_scope p ON p.qase_project_code=r.project_code
  JOIN qase_runs ar ON ar.project_code=r.project_code AND ar.run_id=r.run_id AND ar.active=true
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
  JOIN qase_runs ar ON ar.project_code=r.project_code AND ar.run_id=r.run_id AND ar.active=true
  JOIN qase_run_cases rc ON rc.project_code=r.project_code AND rc.run_id=r.run_id AND rc.case_id=r.case_id
  WHERE r.project_code=p.qase_project_code AND r.ended_at < d.day + interval '1 day'
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

// Tester names match normalized (lower+trim) — see normTester in Workload.
var workloadQaseCountQuery = `
SELECT count(*) FROM qase_results
JOIN qase_cases qc ON qc.project_code = qase_results.project_code AND qc.case_id = qase_results.case_id
JOIN qase_runs qr2 ON qr2.project_code = qase_results.project_code AND qr2.run_id = qase_results.run_id
WHERE lower(trim(` + testerColumnCaseSQL("qc", "qr2") + `)) = ? AND qase_results.ended_at >= ? AND qase_results.ended_at < ? AND qase_results.project_code IN (SELECT qase_project_code FROM projects WHERE qase_project_code <> '')`

var workloadQaseDailyQuery = `
SELECT to_char(qase_results.ended_at, 'YYYY-MM-DD') as date,
  count(*) as executed,
  count(*) filter (where qase_results.status='passed') as passed,
  count(*) filter (where qase_results.status='failed') as failed,
  count(*) filter (where qase_results.status='blocked') as blocked
FROM qase_results
JOIN qase_cases qc ON qc.project_code = qase_results.project_code AND qc.case_id = qase_results.case_id
JOIN qase_runs qr2 ON qr2.project_code = qase_results.project_code AND qr2.run_id = qase_results.run_id
WHERE lower(trim(` + testerColumnCaseSQL("qc", "qr2") + `)) = ? AND qase_results.ended_at >= ? AND qase_results.ended_at < ? AND qase_results.project_code IN (SELECT qase_project_code FROM projects WHERE qase_project_code <> '')
GROUP BY date ORDER BY date`

// workloadQaseProjectsQuery lists the dashboard-registered projects a QA has
// recorded Qase results in during the window.
var workloadQaseProjectsQuery = `
SELECT DISTINCT p.id AS id, coalesce(nullif(p.jira_init_key,''), p.qase_project_code) AS key, p.name AS name
FROM qase_results
JOIN qase_cases qc ON qc.project_code = qase_results.project_code AND qc.case_id = qase_results.case_id
JOIN qase_runs qr2 ON qr2.project_code = qase_results.project_code AND qr2.run_id = qase_results.run_id
JOIN projects p ON p.qase_project_code = qase_results.project_code AND p.qase_project_code <> ''
WHERE lower(trim(` + testerColumnCaseSQL("qc", "qr2") + `)) = ? AND qase_results.ended_at >= ? AND qase_results.ended_at < ?
ORDER BY key`

// distinctSyncedTesterNamesQuery lists every name that has actually appeared
// as a tester on synced Qase cases, across every project — the roster of
// people who've really done QA work, independent of whether they've been
// registered as a QA member yet.
const distinctSyncedTesterNamesQuery = `
SELECT name FROM (
  SELECT DISTINCT tester_name AS name FROM qase_cases WHERE tester_name <> ''
  UNION
  SELECT DISTINCT tester_android_name FROM qase_cases WHERE tester_android_name <> ''
  UNION
  SELECT DISTINCT tester_ios_name FROM qase_cases WHERE tester_ios_name <> ''
  UNION
  SELECT DISTINCT beta_tester_name FROM qase_cases WHERE beta_tester_name <> ''
) t`

// workloadRoster is one row to compute a WorkloadMember for: either a
// registered Member (capacity/allocation apply) or a bare name discovered in
// synced Qase data with no matching qa_members row (no capacity concept).
type workloadRoster struct {
	id, name, qaseDisplayName string
	jiraAccountID             string
	weeklyCapacityHours       float64
	registered                bool
}

// Workload reports execution activity for every QA who has actually done
// synced work, not just those registered as QA members — a member who signed
// up first still lists first, but an unregistered tester found in Qase data
// still shows up (with no capacity/allocation, since none was ever set for
// them) instead of being silently dropped.
func (r *Monitoring) Workload(from, to time.Time, memberID string) ([]WorkloadMember, error) {
	var members []Member
	if err := r.db.Order("name").Find(&members).Error; err != nil {
		return nil, err
	}
	// Tester names are compared normalized (lower+trim): Qase stores whatever
	// casing was typed ("amalia"), while qa_members may register "Amalia" —
	// a case difference must not split one QA into two workload rows.
	normTester := func(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
	roster := make([]workloadRoster, 0, len(members))
	registeredQaseNames := map[string]bool{}
	for _, m := range members {
		if memberID != "" && m.ID != memberID {
			continue
		}
		// An inactive member's Qase name stays reserved so it doesn't resurface
		// below as a bare, unregistered Qase tester — inactive means hidden.
		if m.QaseDisplayName != "" {
			registeredQaseNames[normTester(m.QaseDisplayName)] = true
		}
		if !m.Active {
			continue
		}
		roster = append(roster, workloadRoster{id: m.ID, name: memberDisplayName(m), qaseDisplayName: m.QaseDisplayName, weeklyCapacityHours: m.WeeklyCapacityHours, jiraAccountID: m.JiraAccountID, registered: true})
	}
	if memberID == "" {
		var names []string
		if err := r.db.Raw(distinctSyncedTesterNamesQuery).Scan(&names).Error; err != nil {
			return nil, err
		}
		sort.Strings(names)
		for _, name := range names {
			if registeredQaseNames[normTester(name)] {
				continue
			}
			roster = append(roster, workloadRoster{id: name, name: name, qaseDisplayName: name})
		}
	}
	out := make([]WorkloadMember, 0, len(roster))
	for _, m := range roster {
		v := WorkloadMember{ID: m.id, Name: m.name, DailyExecutions: []ExecutionDay{}, QaseProjects: []WorkloadProject{}, QaseName: m.qaseDisplayName, QaseMapped: strings.TrimSpace(m.qaseDisplayName) != ""}
		if m.registered {
			if err := r.db.Model(&Allocation{}).Select("coalesce(sum(planned_hours),0)").Where("member_id = ? AND week_start >= ? AND week_start < ?", m.id, from, to).Scan(&v.PlannedHours).Error; err != nil {
				return nil, err
			}
			v.CapacityHours = float64(to.Sub(from).Hours()/168) * m.weeklyCapacityHours
		}
		if err := r.populateWorkloadProjects(m.jiraAccountID, &v); err != nil {
			return nil, err
		}
		if m.qaseDisplayName != "" {
			qaseName := normTester(m.qaseDisplayName)
			if err := r.db.Raw(workloadQaseCountQuery, qaseName, from, to).Scan(&v.QaseExecutions).Error; err != nil {
				return nil, err
			}
			if err := r.db.Raw(workloadQaseDailyQuery, qaseName, from, to).Scan(&v.DailyExecutions).Error; err != nil {
				return nil, err
			}
			if err := r.db.Raw(workloadQaseProjectsQuery, qaseName, from, to).Scan(&v.QaseProjects).Error; err != nil {
				return nil, err
			}
		}
		out = append(out, v)
	}
	return out, nil
}

// excludedProjectStatuses are the Jira statuses that mean an INIT is no longer
// active (same list as the product's own QA-portfolio query).
var excludedProjectStatuses = []string{"Cancel", "Done", "Postponed", "Backlog"}

func isActiveInitStatus(status string) bool {
	for _, s := range excludedProjectStatuses {
		if strings.EqualFold(strings.TrimSpace(status), s) {
			return false
		}
	}
	return true
}

// populateWorkloadProjects fills a QA's project portfolio straight from the
// Jira snapshot (jira_init_qas), matched by Jira accountId: TotalProjects is
// every INIT they are a QA on, and Projects/ActiveProjects only those whose
// Jira status isn't excluded. The INIT need not be registered on the
// dashboard. NextProject is the nearest upcoming staging start among the
// active INITs that are registered projects.
func (r *Monitoring) populateWorkloadProjects(jiraAccountID string, v *WorkloadMember) error {
	v.Projects = []WorkloadProject{}
	if jiraAccountID == "" {
		return nil
	}
	var rows []JiraInitQA
	if err := r.db.Where("jira_account_id = ?", jiraAccountID).Order("length(init_key) desc, init_key desc").Find(&rows).Error; err != nil {
		return err
	}
	v.TotalProjects = int64(len(rows))
	activeKeys := make([]string, 0, len(rows))
	for _, row := range rows {
		if !isActiveInitStatus(row.InitStatus) {
			continue
		}
		v.Projects = append(v.Projects, WorkloadProject{ID: row.InitKey, Key: row.InitKey, Name: row.InitName, Status: row.InitStatus})
		activeKeys = append(activeKeys, row.InitKey)
	}
	v.ActiveProjects = int64(len(v.Projects))
	if len(activeKeys) == 0 {
		return nil
	}
	var next Project
	err := r.db.Where("jira_init_key IN ? AND staging_start_at >= ?", activeKeys, time.Now().UTC()).Order("staging_start_at asc").First(&next).Error
	if err == nil {
		v.NextProject = &WorkloadProject{ID: next.ID, Key: next.JiraInitKey, Name: next.Name, StagingStartAt: next.StagingStartAt}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return nil
}
func memberDisplayName(member Member) string {
	if name := strings.TrimSpace(member.Name); name != "" {
		return name
	}
	return member.ID
}

// ApiBug is the wire shape the frontend already expects for /api/v1/bugs
// (see ApiBug in dashboard-api.service.ts); Bugs now sources it from Qase
// defects joined to Project by qase_project_code instead of JiraIssue.
type ApiBug struct {
	ID          string     `json:"id"`
	Key         string     `json:"key"`
	ProjectID   string     `json:"projectId"`
	Summary     string     `json:"summary"`
	Severity    string     `json:"severity"`
	Status      string     `json:"status"`
	Environment string     `json:"environment"`
	Creator     string     `json:"creator"`
	Reporter    string     `json:"reporter"`
	Assignee    string     `json:"assignee"`
	CreatedAt   *time.Time `json:"createdAt"`
	UpdatedAt   *time.Time `json:"updatedAt"`
}


func (r *Monitoring) ProjectBugSummary(p Project) (ProjectBugSummary, error) {
	var out ProjectBugSummary
	if p.ID == "" && p.QaseProjectCode == "" {
		return out, nil
	}
	var jiraBugs []JiraIssue
	if p.ID != "" {
		if err := r.db.Where("project_id = ? AND sync_scope = ? AND active = ?", p.ID, "project-bugs", true).Find(&jiraBugs).Error; err != nil {
			return out, err
		}
	}
	if len(jiraBugs) > 0 {
		for _, bug := range jiraBugs {
			// Canceled/Invalid bugs are noise: counted only in Canceled, never
			// in Total/Staging/Beta or the Beta>30% check.
			if strings.EqualFold(strings.TrimSpace(bug.Status), "Invalid") || strings.Contains(strings.ToLower(bug.Status), "cancel") {
				out.Canceled++
				continue
			}
			out.Total++
			switch strings.ToUpper(strings.TrimSpace(bug.Environment)) {
			case "STAGING":
				out.Staging++
			case "BETA":
				out.Beta++
			}
		}
		out.BetaOverThirtyPercentOfStaging = out.Beta*100 > out.Staging*30
		return out, nil
	}
	var defects []QaseDefect
	if err := r.db.Where("project_code = ?", p.QaseProjectCode).Find(&defects).Error; err != nil {
		return out, err
	}
	for _, defect := range defects {
		status := defect.JiraStatus
		if status == "" {
			status = defect.Status
		}
		if strings.Contains(strings.ToLower(status), "cancel") {
			out.Canceled++
			continue
		}
		out.Total++
		switch strings.ToUpper(strings.TrimSpace(defect.Environment)) {
		case "STAGING":
			out.Staging++
		case "BETA":
			out.Beta++
		}
	}
	out.BetaOverThirtyPercentOfStaging = out.Beta*100 > out.Staging*30
	return out, nil
}

type ProductionBugPage struct {
	Items    []ApiBug `json:"items"`
	Page     int      `json:"page"`
	PageSize int      `json:"pageSize"`
	Total    int64    `json:"total"`
}

type BugPage struct {
	Items    []ApiBug `json:"items"`
	Page     int      `json:"page"`
	PageSize int      `json:"pageSize"`
	Total    int64    `json:"total"`
}

func (r *Monitoring) JiraBugs(page, pageSize int, projectID, environment, reporter, status, priority, search string) (BugPage, error) {
	out := BugPage{Items: []ApiBug{}, Page: page, PageSize: pageSize}
	q := r.db.Model(&JiraIssue{}).Where("active = ? AND sync_scope = ?", true, "project-bugs")
	if projectID != "" {
		q = q.Where("project_id = ?", projectID)
	}
	if environment != "" {
		q = q.Where("UPPER(environment) = ?", strings.ToUpper(environment))
	}
	if reporter != "" {
		q = q.Where("reporter = ?", reporter)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if priority != "" {
		q = q.Where("severity = ?", priority)
	}
	if search != "" {
		q = q.Where("summary ILIKE ? OR key ILIKE ?", "%"+search+"%", "%"+search+"%")
	}
	if err := q.Count(&out.Total).Error; err != nil {
		return out, err
	}
	if err := q.Select("id, key, project_id, summary, severity, status, environment, creator, reporter, assignee, created_at, source_updated_at AS updated_at").
		Order("created_at DESC NULLS LAST, key DESC").Offset((page - 1) * pageSize).Limit(pageSize).Scan(&out.Items).Error; err != nil {
		return out, err
	}
	return out, nil
}

func (r *Monitoring) ProductionBugs(page, pageSize int) (ProductionBugPage, error) {
	out := ProductionBugPage{Items: make([]ApiBug, 0), Page: page, PageSize: pageSize}
	q := r.db.Model(&JiraIssue{}).Where("project_id = ?", "BUG")
	if err := q.Count(&out.Total).Error; err != nil {
		return out, err
	}
	if err := q.Select("id, key, project_id, summary, severity, status, creator, reporter, assignee, created_at, source_updated_at AS updated_at").
		Order("fetched_at asc").Offset((page - 1) * pageSize).Limit(pageSize).Scan(&out.Items).Error; err != nil {
		return out, err
	}
	return out, nil
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
