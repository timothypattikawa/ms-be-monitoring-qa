package monitoring

import "time"

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

func Models() []any {
	return []any{&Project{}, &Member{}, &Allocation{}, &JiraIssue{}, &QaseCase{}, &QaseRun{}, &QaseRunCase{}, &QaseResult{}, &SyncJob{}, &SyncStep{}, &SyncEvent{}, &SyncCursor{}}
}
