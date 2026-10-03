package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Beyondtech-ID/ms-monitoring-qa-be/internal/repository"
	"github.com/google/uuid"
)

// ponytail: field IDs hardcoded with env override, not dynamic title-based
// discovery — single Qase workspace, IDs don't change; if we ever support
// multiple workspaces, discover by title instead.
var (
	qaseFieldIDPic           = envInt64("QASE_FIELD_ID_PIC", 7)
	qaseFieldIDTester        = envInt64("QASE_FIELD_ID_TESTER", 15)
	qaseFieldIDTesterAndroid = envInt64("QASE_FIELD_ID_TESTER_ANDROID", 16)
	qaseFieldIDTesterIos     = envInt64("QASE_FIELD_ID_TESTER_IOS", 17)
	qaseFieldIDTesterBeta    = envInt64("QASE_FIELD_ID_TESTER_BETA", 21)
)

func envInt64(key string, def int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}

type Worker struct {
	Repo      *repository.Monitoring
	Connector Connector
}

func (w Worker) Run(ctx context.Context) error {
	if err := w.Repo.AutoMigrate(); err != nil {
		return err
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if err := w.failExhausted(time.Now().UTC()); err != nil {
			return err
		}
		if err := w.enqueueScheduled(time.Now()); err != nil {
			return err
		}
		job, err := w.claim()
		if err != nil {
			return err
		}
		if job != nil {
			if err := w.process(ctx, *job); err != nil {
				return err
			}
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (w Worker) failExhausted(now time.Time) error {
	return w.Repo.FailExhausted(now)
}
func (w Worker) enqueueScheduled(now time.Time) error {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return err
	}
	local := now.In(loc)
	if local.Hour() != 8 && local.Hour() != 17 {
		return nil
	}
	key := fmt.Sprintf("scheduled:%s:%02d", local.Format("2006-01-02"), local.Hour())
	job := SyncJob{ID: uuid.NewString(), RequestKey: key, Trigger: "scheduled", Status: "queued", Sources: "jira,qase", RequestedAt: now.UTC(), Actor: "scheduler"}
	_, err = w.Repo.EnqueueIfAbsent(&job, []string{"jira", "qase"})
	return err
}
func (w Worker) claim() (*SyncJob, error) {
	return w.Repo.ClaimJob(time.Now().UTC())
}
func (w Worker) process(ctx context.Context, job SyncJob) error {
	heartbeatDone := make(chan struct{})
	defer close(heartbeatDone)
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatDone:
				return
			case <-ticker.C:
				_ = w.Repo.UpdateHeartbeat(job.ID, time.Now().UTC())
			}
		}
	}()
	if err := w.event(job.ID, "", "info", "SYNC_STARTED", "Sync started"); err != nil {
		return err
	}
	steps, err := w.Repo.JobSteps(job.ID)
	if err != nil {
		return err
	}
	success := 0
	for _, step := range steps {
		if step.Status == "succeeded" {
			success++
			continue
		}
		now := time.Now().UTC()
		step.Status = "running"
		step.StartedAt = &now
		if err := w.Repo.SaveStep(&step); err != nil {
			return err
		}
		if err := w.event(job.ID, step.ID, "info", strings.ToUpper(step.Source)+"_STARTED", "Source import started"); err != nil {
			return err
		}
		stepCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
		var err error
		switch step.Source {
		case "jira":
			err = w.syncJira(stepCtx, &job, &step)
		case "qase":
			err = w.syncQase(stepCtx, &job, &step)
		case "qase-detail":
			err = w.syncQaseDetail(stepCtx, &job, &step)
		case "production-bugs":
			err = w.syncProductionBugs(stepCtx, &step)
		case "qa-portfolio":
			if os.Getenv("JIRA_QAS_FIELD_ID") == "" {
				err = errors.New("JIRA_QAS_FIELD_MISSING")
			} else {
				err = w.syncQAPortfolio(stepCtx, &step)
			}
		default:
			err = errors.New("UNKNOWN_SOURCE")
		}
		cancel()
		finished := time.Now().UTC()
		step.FinishedAt = &finished
		if err != nil {
			step.Status = "failed"
			step.ErrorCode = safeCode(err)
			if eventErr := w.event(job.ID, step.ID, "error", strings.ToUpper(step.Source)+"_FAILED", step.ErrorCode); eventErr != nil {
				return eventErr
			}
		} else {
			step.Status = "succeeded"
			success++
			if eventErr := w.event(job.ID, step.ID, "info", strings.ToUpper(step.Source)+"_DONE", "Source import completed"); eventErr != nil {
				return eventErr
			}
		}
		if err := w.Repo.SaveStep(&step); err != nil {
			return err
		}
		if err := w.Repo.UpdateHeartbeat(job.ID, finished); err != nil {
			return err
		}
	}
	job.FinishedAt = timePtr(time.Now().UTC())
	job.Status = "failed"
	if success == len(steps) {
		job.Status = "succeeded"
	} else if success > 0 {
		job.Status = "partial"
	}
	if err := w.Repo.SaveJob(&job); err != nil {
		return err
	}
	return w.event(job.ID, "", "info", "SYNC_"+strings.ToUpper(job.Status), "Sync completed")
}
func timePtr(t time.Time) *time.Time { return &t }
func safeCode(err error) string {
	switch err.Error() {
	case "JIRA_CONFIG_MISSING", "QASE_CONFIG_MISSING", "UPSTREAM_ACCESS_DENIED", "UPSTREAM_UNAVAILABLE", "UPSTREAM_RETRY_EXHAUSTED", "UPSTREAM_BAD_RESPONSE", "UPSTREAM_JSON_INVALID", "QASE_REJECTED", "QASE_RESPONSE_INVALID", "QASE_OFFSET_LIMIT", "QASE_RUN_CASES_MISSING", "QASE_RUN_CASES_INVALID", "QASE_CASE_ID_MISSING", "QASE_RUN_ID_MISSING", "QASE_RESULT_ID_MISSING", "QASE_RESULT_RUN_ID_MISSING", "QASE_RESULT_CASE_ID_MISSING", "JIRA_QAS_FIELD_MISSING", "JIRA_ID_MISSING", "JIRA_CURSOR_MISSING", "JIRA_CURSOR_LOOP", "JIRA_PAGE_LIMIT", "JIRA_ISSUE_NOT_FOUND":
		return err.Error()
	default:
		return "IMPORT_FAILED"
	}
}
func (w Worker) event(jobID, stepID, level, code, msg string) error {
	return w.Repo.Event(&SyncEvent{ID: uuid.NewString(), JobID: jobID, StepID: stepID, OccurredAt: time.Now().UTC(), Level: level, Code: code, Message: msg})
}

// productionBugJQL is product's production-bug query; its ORDER BY (newest
// created first) is the display order, persisted as-is via fetched_at.
const productionBugJQL = `project = BUG AND status in (Confirm, Done, "In Progress", "To Do") ORDER BY created DESC, cf[10019] ASC`

// syncProductionBugs replaces the DB-backed production bug snapshot using
// Jira's exact order. It is the whole "production-bugs" source (the Bugs page's
// dedicated sync button) and also the tail of an unscoped "jira" sync.
func (w Worker) syncProductionBugs(ctx context.Context, step *SyncStep) error {
	bugs := make([]JiraIssue, 0)
	err := w.Connector.JiraPages(ctx, productionBugJQL, func(issue jiraIssue) error {
		step.Fetched++
		bugs = append(bugs, JiraIssue{
			ID: uuid.NewString(), ExternalID: issue.ID, Key: issue.Key, ProjectID: "BUG", Summary: issue.Fields.Summary,
			IssueType: issue.Fields.IssueType.Name, Severity: issue.Fields.Priority.Name, Status: issue.Fields.Status.Name,
			Creator: issue.Fields.Creator.DisplayName, Reporter: issue.Fields.Reporter.DisplayName, Assignee: issue.Fields.Assignee.DisplayName,
			CreatedAt: parseTime(issue.Fields.Created), SourceUpdatedAt: parseTime(issue.Fields.Updated),
		})
		return nil
	})
	if err != nil {
		return err
	}
	fetchedAt := time.Now().UTC()
	for i := range bugs {
		bugs[i].FetchedAt = fetchedAt.Add(time.Duration(i) * time.Millisecond)
	}
	if err := w.Repo.ReplaceProductionBugs(bugs); err != nil {
		return err
	}
	step.Inserted += len(bugs)
	return nil
}

// syncJira refreshes registered INIT statuses and, for an unscoped sync,
// the production bug snapshot and QA portfolio.
func (w Worker) syncJira(ctx context.Context, job *SyncJob, step *SyncStep) error {
	var projects []Project
	if job.ProjectID != "" {
		p, err := w.Repo.Project(job.ProjectID)
		if err != nil {
			return err
		}
		projects = []Project{p}
	} else {
		var err error
		projects, err = w.Repo.Projects(-1, true, "")
		if err != nil {
			return err
		}
	}
	newest := time.Time{}
	for _, p := range projects {
		if p.JiraInitKey == "" {
			continue
		}
		step.Fetched++
		issue, err := w.Connector.JiraIssue(ctx, p.JiraInitKey)
		if err != nil {
			return err
		}
		step.Updated++
		p.JiraInitID = stringPtr(issue.ID)
		p.Status = issue.Fields.Status.Name
		if err := w.Repo.SaveProject(&p); err != nil {
			return err
		}
		if updated := parseTime(issue.Fields.Updated); updated != nil && updated.After(newest) {
			newest = *updated
		}
		if err := w.syncJiraBugs(ctx, p, step); err != nil {
			return err
		}
	}
	if job.ProjectID == "" {
		if err := w.syncProductionBugs(ctx, step); err != nil {
			return err
		}
	}
	if job.ProjectID == "" {
		if err := w.syncQAPortfolio(ctx, step); err != nil {
			return err
		}
	}
	scope := "global"
	if job.ProjectID != "" {
		scope = job.ProjectID
	}
	return w.cursor("jira", scope, "issues", newest)
}

// projectBugsJQLTemplate is product's own confirmed per-project bug query
// (one for Staging, one for Beta, merged here via testing-environment IN
// (...) so pagination only runs once per project), substituting the
// project's INIT key for the parent clause. Invalid applies to both
// environments equally, matching product's queries exactly.
const projectBugsJQLTemplate = `project = QASE AND parent = "%s" AND status IN (Confirm, "In Progress", Open, "Ready for QA", Resolved, Invalid, Reopened) AND "testing environment[dropdown]" IN (Staging, Beta) ORDER BY created DESC`

// syncJiraBugs makes Jira the source of truth for the bugs dashboard (plan
// §9.1): a Qase defect linkage is no longer required for an issue to show
// up. Issues no longer matched by the JQL after a full pagination pass are
// deactivated (ReplaceProjectBugs), not deleted, so history/audit stays intact.
func (w Worker) syncJiraBugs(ctx context.Context, project Project, step *SyncStep) error {
	if project.JiraInitKey == "" {
		return nil
	}
	jql := fmt.Sprintf(projectBugsJQLTemplate, project.JiraInitKey)
	fetchedAt := time.Now().UTC()
	issues := make([]JiraIssue, 0)
	err := w.Connector.JiraPages(ctx, jql, func(issue jiraIssue) error {
		step.Fetched++
		issues = append(issues, JiraIssue{
			ID:              uuid.NewString(),
			ExternalID:      issue.ID,
			Key:             issue.Key,
			Summary:         issue.Fields.Summary,
			IssueType:       issue.Fields.IssueType.Name,
			Severity:        issue.Fields.Priority.Name,
			Status:          issue.Fields.Status.Name,
			Creator:         issue.Fields.Creator.DisplayName,
			Reporter:        issue.Fields.Reporter.DisplayName,
			Assignee:        issue.Fields.Assignee.DisplayName,
			ParentKey:       project.JiraInitKey,
			Environment:     repository.ResolveEnvironment(issue.Fields.TestingEnvironment.Value, ""),
			CreatedAt:       parseTime(issue.Fields.Created),
			SourceUpdatedAt: parseTime(issue.Fields.Updated),
			FetchedAt:       fetchedAt,
		})
		return nil
	})
	if err != nil {
		return err
	}
	step.Inserted += len(issues)
	return w.Repo.ReplaceProjectBugs(project.ID, "project-bugs", issues)
}

var jiraKeyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*-[0-9]+$`)

// syncQAPortfolio refreshes the QA portfolio from Jira's QAs field: it swaps
// the jira_init_qas snapshot (every INIT with a QA, any status — registered
// on the dashboard or not) and creates a member for each Jira QA that has none
// yet (active, Qase display name left empty for the QA lead to map). It runs
// as the "qa-portfolio" source and at the tail of an unscoped "jira" sync, and
// no-ops while JIRA_QAS_FIELD_ID is unset. The snapshot is only replaced after
// the whole Jira query succeeded, so a failed run keeps the previous data.
func (w Worker) syncQAPortfolio(ctx context.Context, step *SyncStep) error {
	if os.Getenv("JIRA_QAS_FIELD_ID") == "" {
		return nil
	}
	syncedAt := time.Now().UTC()
	var rows []repository.JiraInitQA
	users := map[string]repository.Member{}
	seen := map[string]bool{}
	err := w.Connector.JiraInitsWithQAs(ctx, func(issue jiraIssueWithQAs) error {
		step.Fetched++
		for _, qa := range issue.QAs {
			if qa.AccountID == "" || seen[issue.Key+"|"+qa.AccountID] {
				continue
			}
			seen[issue.Key+"|"+qa.AccountID] = true
			rows = append(rows, repository.JiraInitQA{InitKey: issue.Key, JiraAccountID: qa.AccountID, DisplayName: qa.DisplayName, InitName: issue.Summary, InitStatus: issue.Status, SyncedAt: syncedAt})
			users[qa.AccountID] = repository.Member{Name: qa.DisplayName, JiraAccountID: qa.AccountID, JiraEmail: qa.EmailAddress}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := w.Repo.ReplaceJiraInitQAs(rows); err != nil {
		return err
	}
	list := make([]repository.Member, 0, len(users))
	for _, u := range users {
		list = append(list, u)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	created, err := w.Repo.EnsureJiraMembers(list)
	if err != nil {
		return err
	}
	step.Updated += len(rows)
	step.Inserted += created
	return nil
}

func stringPtr(value string) *string { return &value }

func (w Worker) syncQase(ctx context.Context, job *SyncJob, step *SyncStep) error {
	projects, err := w.Repo.QaseProjects(job.ProjectID)
	if err != nil {
		return err
	}
	codes := map[string]bool{}
	for _, p := range projects {
		codes[p.QaseProjectCode] = true
	}
	for code := range codes {
		if err := w.syncQaseProject(ctx, code, step); err != nil {
			return err
		}
	}
	if len(codes) == 0 {
		return errors.New("QASE_CONFIG_MISSING")
	}
	return nil
}

// syncQaseProjectScope pulls the project's own case/suite totals from a
// single lightweight Qase project-summary call instead of deriving them by
// paginating every run's cases. Non-fatal: a failure here just leaves the
// previously synced scope in place.
func (w Worker) syncQaseProjectScope(ctx context.Context, code string) {
	raw, err := w.Connector.QaseProject(ctx, code)
	if err != nil {
		return
	}
	var parsed struct {
		Counts struct {
			Cases  int64 `json:"cases"`
			Suites int64 `json:"suites"`
		} `json:"counts"`
	}
	if json.Unmarshal(raw, &parsed) != nil {
		return
	}
	_ = w.Repo.UpdateProjectQaseScope(code, parsed.Counts.Cases, parsed.Counts.Suites)
}

// syncQaseProject runs the "overview" tier only: project scope count, run
// rows, and the "result" resource (pass/fail/blocked per case) — everything
// the project-list page and its percentages need. It deliberately skips case
// titles/tester metadata and defects; see syncQaseProjectDetail for those.
func (w Worker) syncQaseProject(ctx context.Context, code string, step *SyncStep) error {
	w.syncQaseProjectScope(ctx, code)
	runIDs, err := w.Connector.QaseRunIDs(ctx, code)
	if err != nil {
		return err
	}
	for _, runID := range runIDs {
		if err := w.syncQaseRunOverview(ctx, code, runID, step); err != nil {
			return err
		}
	}
	return w.Repo.MarkRunsInactive(code, runIDs)
}

// syncQaseProjectDetail runs the expensive "detail" tier for one project:
// case titles + tester/PIC custom-field resolution for every run, plus the
// defect list (with per-defect Jira enrichment). Only requested when a user
// opens that project's detail dialog (Team Progress / Jira bugs tabs), never
// as part of the regular overview sync.
func (w Worker) syncQaseProjectDetail(ctx context.Context, code string, step *SyncStep) error {
	runIDs, err := w.Connector.QaseRunIDs(ctx, code)
	if err != nil {
		return err
	}
	// Non-fatal: if this call fails, the 4 tester-name columns just stay
	// blank for this sync — case titles etc. still sync fine. Resolved once
	// per project here rather than once per run.
	fieldOptions := map[int64]map[int64]string{}
	if fields, err := w.Connector.QaseCustomFields(ctx); err == nil {
		for _, f := range fields {
			opts := make(map[int64]string, len(f.Value))
			for _, v := range f.Value {
				opts[v.ID] = v.Title
			}
			fieldOptions[f.ID] = opts
		}
	}
	for _, runID := range runIDs {
		if err := w.syncQaseRunDetail(ctx, code, runID, fieldOptions, step); err != nil {
			return err
		}
	}
	return w.syncQaseDefects(ctx, code, step)
}

// syncQaseDetail is the "qase-detail" step dispatcher. Like syncQase's
// overview tier, an empty job.ProjectID fans out across every registered
// project (QaseProjects("") returns them all) — this is what the single
// global "Sync data" job uses for a full sync. A non-empty job.ProjectID
// scopes it to one project, kept for callers that only need one project's
// detail tier refreshed.
func (w Worker) syncQaseDetail(ctx context.Context, job *SyncJob, step *SyncStep) error {
	projects, err := w.Repo.QaseProjects(job.ProjectID)
	if err != nil {
		return err
	}
	codes := map[string]bool{}
	for _, p := range projects {
		codes[p.QaseProjectCode] = true
	}
	if len(codes) == 0 {
		return errors.New("QASE_CONFIG_MISSING")
	}
	for code := range codes {
		if err := w.syncQaseProjectDetail(ctx, code, step); err != nil {
			return err
		}
	}
	return nil
}

// syncQaseDefects imports Qase's per-project defect list, enriching each
// linked-Jira-issue defect with assignee/reporter/priority pulled from Jira
// (the source of truth for those fields, not Qase's own severity). Defects
// aren't run-scoped, so this runs once per project code.
func (w Worker) syncQaseDefects(ctx context.Context, code string, step *SyncStep) error {
	defects, err := w.Connector.QaseDefects(ctx, code)
	if err != nil {
		return err
	}
	for _, d := range defects {
		step.Fetched++
		row := QaseDefect{ID: uuid.NewString(), ProjectCode: code, DefectID: d.ID, Title: d.Title, Status: d.Status, UpdatedAt: time.Now().UTC()}
		row.QaseCreatedAt = parseTime(d.Created)
		if row.QaseCreatedAt == nil {
			row.QaseCreatedAt = parseTime(d.CreatedAt)
		}
		row.Environment = w.defectEnvironment(code, d.Runs, d.Results)
		if key := jiraKeyFromExternalData(d.ExternalData); key != "" {
			row.JiraKey = key
			// Non-fatal: if this single lookup fails, leave the Jira-sourced
			// fields blank and keep the defect row itself.
			if issue, err := w.Connector.JiraIssue(ctx, key); err == nil {
				row.JiraAssignee = issue.Fields.Assignee.DisplayName
				row.JiraReporter = issue.Fields.Reporter.DisplayName
				row.JiraCreator = issue.Fields.Creator.DisplayName
				row.JiraPriority = issue.Fields.Priority.Name
				row.JiraStatus = issue.Fields.Status.Name
				row.JiraCreatedAt = parseTime(issue.Fields.Created)
				// Jira's own "Testing Environment" field (customfield_10185)
				// is authoritative — it's set directly on the issue, unlike
				// the Qase run/result-linkage heuristic below, which only
				// covers defects Qase itself managed to link to a run.
				if env := repository.ResolveEnvironment(issue.Fields.TestingEnvironment.Value, ""); env != "" {
					row.Environment = env
				}
			}
		}
		existed, err := w.Repo.UpsertDefect(&row)
		if err != nil {
			return err
		}
		if existed {
			step.Updated++
		} else {
			step.Inserted++
		}
	}
	return nil
}

// defectEnvironment resolves a defect's environment from a directly linked
// run, or from its linked result when Qase omits runs (the common case).
// ponytail: first non-empty match wins even if linked runs span
// environments — good enough for defect tagging, not worth more nuance.
func (w Worker) defectEnvironment(code string, runIDs []int64, resultIDs []string) string {
	for _, id := range runIDs {
		run, found, err := w.Repo.RunByID(code, id)
		if err != nil || !found {
			continue
		}
		if env := repository.ResolveEnvironment(run.Environment, run.Title); env != "" {
			return env
		}
	}
	for _, id := range resultIDs {
		run, found, err := w.Repo.RunByResultID(code, id)
		if err != nil || !found {
			continue
		}
		if env := repository.ResolveEnvironment(run.Environment, run.Title); env != "" {
			return env
		}
	}
	return ""
}
func jiraKeyFromExternalData(raw string) string {
	if raw == "" {
		return ""
	}
	var parsed struct {
		JiraCloud struct {
			Key string `json:"key"`
		} `json:"jira-cloud"`
	}
	if json.Unmarshal([]byte(raw), &parsed) != nil {
		return ""
	}
	return parsed.JiraCloud.Key
}

// fetchRun fetches one run's metadata plus its case-ID membership from Qase.
// It's a single lightweight call (not a paginated listing), so both the
// overview and detail tiers can afford to call it independently.
func (w Worker) fetchRun(ctx context.Context, code string, runID int64, step *SyncStep) (QaseRun, []int64, error) {
	rawRun, err := w.Connector.QaseRun(ctx, code, runID)
	if err != nil {
		return QaseRun{}, nil, err
	}
	var runItem map[string]json.RawMessage
	if err := json.Unmarshal(rawRun, &runItem); err != nil {
		return QaseRun{}, nil, errors.New("QASE_RESPONSE_INVALID")
	}
	if number(runItem, "id") != runID {
		return QaseRun{}, nil, errors.New("QASE_RUN_ID_MISSING")
	}
	step.Fetched++
	run := QaseRun{ID: uuid.NewString(), ProjectCode: code, RunID: runID, Title: stringValue(runItem, "title"), Status: stringValue(runItem, "status"), Platform: runPlatform(stringValue(runItem, "title")), Environment: stringValue(runItem, "environment"), Active: true, StartedAt: timeValue(runItem, "start_time"), FinishedAt: timeValue(runItem, "end_time"), FetchedAt: time.Now().UTC()}
	caseIDs, err := qaseCaseIDs(runItem["cases"])
	if err != nil {
		caseIDs, err = w.Connector.QaseRunCases(ctx, code, runID)
		if err != nil {
			return QaseRun{}, nil, err
		}
	}
	return run, caseIDs, nil
}

// syncQaseRunOverview upserts the run row and imports only the "result"
// resource (pass/fail/blocked per case) — the cheap tier needed for the
// project-list page's run list and percentages. Case titles/tester metadata
// are NOT fetched here; see syncQaseRunDetail for that expensive tier.
func (w Worker) syncQaseRunOverview(ctx context.Context, code string, runID int64, step *SyncStep) error {
	run, caseIDs, err := w.fetchRun(ctx, code, runID, step)
	if err != nil {
		return err
	}
	if err := w.upsertRun(&run, step); err != nil {
		return err
	}
	if err := w.replaceRunCases(code, runID, caseIDs); err != nil {
		return err
	}
	if err := w.cursor("qase", fmt.Sprintf("%s:%d", code, runID), "run", time.Now().UTC()); err != nil {
		return err
	}
	var newest time.Time
	caseCache := map[int64]*repository.QaseCase{}
	err = w.Connector.QasePagesForRun(ctx, "result", code, runID, func(raw json.RawMessage) error {
		step.Fetched++
		var item map[string]json.RawMessage
		if err := json.Unmarshal(raw, &item); err != nil {
			return err
		}
		if number(item, "run_id") != runID {
			step.Skipped++
			return nil
		}
		rid := stringValue(item, "hash")
		if rid == "" {
			rid = stringValue(item, "id")
		}
		if rid == "" {
			rid = fmt.Sprint(number(item, "id"))
		}
		if rid == "" || rid == "0" {
			return errors.New("QASE_RESULT_ID_MISSING")
		}
		row := QaseResult{ID: uuid.NewString(), ProjectCode: code, ResultID: rid, RunID: number(item, "run_id"), CaseID: number(item, "case_id"), ConfigurationKey: string(item["param"]), Status: stringValue(item, "status"), MemberID: stringValue(item, "member_id"), Environment: stringValue(item, "environment"), StartedAt: timeValue(item, "start_time"), EndedAt: timeValue(item, "end_time"), FetchedAt: time.Now().UTC()}
		if row.RunID <= 0 {
			return errors.New("QASE_RESULT_RUN_ID_MISSING")
		}
		if row.CaseID <= 0 {
			return errors.New("QASE_RESULT_CASE_ID_MISSING")
		}
		if row.EndedAt != nil && row.EndedAt.After(newest) {
			newest = *row.EndedAt
		}
		row.TesterName = w.resultTesterSnapshot(code, row.CaseID, repository.ResolveEnvironment(row.Environment, run.Title), caseCache)
		return w.upsertResult(&row, step)
	})
	if err != nil {
		return err
	}
	return w.cursor("qase", fmt.Sprintf("%s:%d", code, runID), "result", newest)
}

// resultTesterSnapshot resolves the environment-appropriate tester name for
// a case (STAGING -> QaseCase.TesterName, i.e. "QA Tester"; BETA ->
// QaseCase.BetaTesterName, i.e. "Field Beta Tester" — never a fallback
// between the two, see betaTesterColumn's old fallback bug) at the moment a
// result is first synced. UpsertResult then locks this in permanently, so a
// later case reassignment in Qase can't rewrite history. Returns "" for any
// other environment, or if the case hasn't been synced yet (detail-tier sync
// runs separately from this overview-tier one, see syncQaseProjectDetail).
func (w Worker) resultTesterSnapshot(code string, caseID int64, environment string, cache map[int64]*repository.QaseCase) string {
	qc, ok := cache[caseID]
	if !ok {
		if found, err := w.Repo.CaseByID(code, caseID); err == nil {
			qc = &found
		}
		cache[caseID] = qc
	}
	if qc == nil {
		return ""
	}
	switch environment {
	case "STAGING":
		return qc.TesterName
	case "BETA":
		return qc.BetaTesterName
	default:
		return ""
	}
}

// syncQaseRunDetail imports the "case" resource (title + tester/PIC custom
// fields) for one run — the expensive part of a Qase sync (paginates every
// case and resolves tester names), only needed by the project detail
// dialog's Team Progress tab. fieldOptions is resolved once per project by
// the caller rather than once per run.
func (w Worker) syncQaseRunDetail(ctx context.Context, code string, runID int64, fieldOptions map[int64]map[int64]string, step *SyncStep) error {
	_, caseIDs, err := w.fetchRun(ctx, code, runID, step)
	if err != nil {
		return err
	}
	allowedCases := make(map[int64]bool, len(caseIDs))
	for _, caseID := range caseIDs {
		allowedCases[caseID] = true
	}
	return w.Connector.QasePagesForRun(ctx, "case", code, runID, func(raw json.RawMessage) error {
		step.Fetched++
		var item map[string]json.RawMessage
		if err := json.Unmarshal(raw, &item); err != nil {
			return err
		}
		id := number(item, "id")
		if !allowedCases[id] {
			step.Skipped++
			return nil
		}
		if id == 0 {
			return errors.New("QASE_CASE_ID_MISSING")
		}
		row := QaseCase{ID: uuid.NewString(), ProjectCode: code, CaseID: id, Title: stringValue(item, "title"), UpdatedAt: time.Now().UTC()}
		applyQaseTesterFields(&row, item["custom_fields"], fieldOptions)
		return w.upsertCase(&row, step)
	})
}
func (w Worker) replaceRunCases(code string, runID int64, caseIDs []int64) error {
	return w.Repo.ReplaceRunCases(code, runID, caseIDs)
}
func (w Worker) upsertCase(row *QaseCase, step *SyncStep) error {
	existed, err := w.Repo.UpsertCase(row)
	if err != nil {
		return err
	}
	if existed {
		step.Updated++
	} else {
		step.Inserted++
	}
	return nil
}
func (w Worker) upsertRun(row *QaseRun, step *SyncStep) error {
	existed, err := w.Repo.UpsertRun(row)
	if err != nil {
		return err
	}
	if existed {
		step.Updated++
	} else {
		step.Inserted++
	}
	return nil
}
func (w Worker) upsertResult(row *QaseResult, step *SyncStep) error {
	existed, err := w.Repo.UpsertResult(row)
	if err != nil {
		return err
	}
	if existed {
		step.Updated++
	} else {
		step.Inserted++
	}
	return nil
}
func (w Worker) cursor(source, scope, resource string, watermark time.Time) error {
	if watermark.IsZero() {
		watermark = time.Now().UTC()
	}
	return w.Repo.SaveCursor(source, scope, resource, watermark)
}
func number(m map[string]json.RawMessage, key string) int64 {
	var n int64
	if b := m[key]; len(b) > 0 {
		_ = json.Unmarshal(b, &n)
	}
	return n
}
func stringValue(m map[string]json.RawMessage, key string) string {
	var s string
	if b := m[key]; len(b) > 0 {
		if json.Unmarshal(b, &s) == nil {
			return s
		}
		return strings.Trim(string(b), "\"")
	}
	return ""
}
func timeValue(m map[string]json.RawMessage, key string) *time.Time {
	if s := stringValue(m, key); s != "" {
		if t := parseTime(s); t != nil {
			return t
		}
	}
	if n := number(m, key); n > 0 {
		return timePtr(time.Unix(n, 0).UTC())
	}
	return nil
}
func parseTime(s string) *time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.000-0700", "2006-01-02T15:04:05.000+0000", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t
		}
	}
	return nil
}
func applyQaseTesterFields(row *QaseCase, raw json.RawMessage, fieldOptions map[int64]map[int64]string) {
	if len(raw) == 0 {
		return
	}
	var entries []struct {
		ID    int64  `json:"id"`
		Value string `json:"value"`
	}
	if json.Unmarshal(raw, &entries) != nil {
		return
	}
	for _, e := range entries {
		opts := fieldOptions[e.ID]
		if opts == nil {
			continue
		}
		var titles []string
		for _, tok := range strings.Split(e.Value, ",") {
			tok = strings.TrimSpace(tok)
			if tok == "" {
				continue
			}
			optID, err := strconv.ParseInt(tok, 10, 64)
			if err != nil {
				continue
			}
			if title, ok := opts[optID]; ok {
				titles = append(titles, title)
			}
		}
		if len(titles) == 0 {
			continue
		}
		joined := strings.Join(titles, ", ")
		switch e.ID {
		case qaseFieldIDPic:
			row.PicNames = joined
		case qaseFieldIDTester:
			row.TesterName = joined
		case qaseFieldIDTesterAndroid:
			row.TesterAndroidName = joined
		case qaseFieldIDTesterIos:
			row.TesterIosName = joined
		case qaseFieldIDTesterBeta:
			row.BetaTesterName = joined
		}
	}
}

// runPlatform detects the platform from a run title regardless of which
// environment tag prefixes it ("[STG] ", "[BETA] ", or none) — real run
// titles use human names ("[BETA] APO Main", "[STG] Android"), not just the
// literal platform codes, so this strips any leading "[TAG] " and matches
// keywords in what's left.
func runPlatform(title string) string {
	upper := strings.ToUpper(title)
	if idx := strings.Index(upper, "] "); idx != -1 {
		upper = upper[idx+2:]
	}
	switch {
	case strings.Contains(upper, "IOS"):
		return "IOS"
	case strings.Contains(upper, "AOS"), strings.Contains(upper, "ANDROID"):
		return "AOS"
	case strings.Contains(upper, "APO"):
		return "APO"
	case strings.Contains(upper, "BO"):
		return "BO"
	case strings.Contains(upper, "DB"):
		return "DB"
	}
	return "unknown"
}
