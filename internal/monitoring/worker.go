package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Beyondtech-ID/ms-monitoring-qa-be/internal/repository"
	"github.com/google/uuid"
)

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
	case "JIRA_CONFIG_MISSING", "QASE_CONFIG_MISSING", "UPSTREAM_ACCESS_DENIED", "UPSTREAM_UNAVAILABLE", "UPSTREAM_RETRY_EXHAUSTED", "UPSTREAM_BAD_RESPONSE", "UPSTREAM_JSON_INVALID", "QASE_REJECTED", "QASE_RESPONSE_INVALID", "QASE_OFFSET_LIMIT", "QASE_PAGE_INCOMPLETE", "QASE_RUN_CASES_MISSING", "QASE_RUN_CASES_INVALID", "QASE_CASE_ID_MISSING", "QASE_RUN_ID_MISSING", "QASE_RESULT_ID_MISSING", "QASE_RESULT_RUN_ID_MISSING", "QASE_RESULT_CASE_ID_MISSING", "JIRA_ID_MISSING", "JIRA_CURSOR_MISSING", "JIRA_CURSOR_LOOP", "JIRA_PAGE_LIMIT":
		return err.Error()
	default:
		return "IMPORT_FAILED"
	}
}
func (w Worker) event(jobID, stepID, level, code, msg string) error {
	return w.Repo.Event(&SyncEvent{ID: uuid.NewString(), JobID: jobID, StepID: stepID, OccurredAt: time.Now().UTC(), Level: level, Code: code, Message: msg})
}
func (w Worker) syncJira(ctx context.Context, job *SyncJob, step *SyncStep) error {
	newest := time.Time{}
	activeIDs := make([]string, 0)
	importIssues := func(jql string, projects bool) error {
		return w.Connector.JiraPages(ctx, jql, func(item jiraIssue) error {
			step.Fetched++
			updated := parseTime(item.Fields.Updated)
			if updated != nil && updated.After(newest) {
				newest = *updated
			}
			existing, found, err := w.Repo.JiraIssue(item.ID)
			if err != nil {
				return err
			}
			if !found {
				existing = JiraIssue{ID: uuid.NewString(), ExternalID: item.ID}
				step.Inserted++
			} else {
				step.Updated++
			}
			existing.Key = item.Key
			existing.Summary = item.Fields.Summary
			existing.Status = item.Fields.Status.Name
			existing.IssueType = item.Fields.IssueType.Name
			existing.Creator = item.Fields.Creator.AccountID
			existing.Reporter = item.Fields.Reporter.AccountID
			existing.Assignee = item.Fields.Assignee.AccountID
			existing.SourceUpdatedAt = updated
			existing.FetchedAt = time.Now().UTC()
			for _, link := range item.Fields.IssueLinks {
				for _, target := range [](*struct {
					ID string `json:"id"`
				}){link.InwardIssue, link.OutwardIssue} {
					if target != nil {
						related, found, err := w.Repo.ProjectByJiraID(target.ID)
						if err != nil {
							return err
						}
						if found {
							existing.ProjectID = related.ID
						}
					}
				}
			}
			if existing.Severity == "" {
				existing.Severity = "Unknown"
			}
			if err := w.Repo.SaveJiraIssue(&existing); err != nil {
				return err
			}
			// The approved JQL selects active INIT project issues. Create an unmapped project
			// so Qase scope can be attached explicitly instead of inferred from a title.
			if projects {
				activeIDs = append(activeIDs, item.ID)
				p, found, err := w.Repo.ProjectByJiraID(item.ID)
				if err != nil {
					return err
				}
				if !found {
					p = Project{ID: uuid.NewString(), JiraInitID: item.ID, JiraInitKey: item.Key, Name: item.Fields.Summary, Status: item.Fields.Status.Name, Health: "unknown"}
				} else {
					p.JiraInitKey = item.Key
					p.Name = item.Fields.Summary
					p.Status = item.Fields.Status.Name
				}
				if err := w.Repo.SaveProject(&p); err != nil {
					return err
				}
			}
			return nil
		})
	}
	err := importIssues(w.Connector.JiraJQL, true)
	if err != nil {
		return err
	}
	if err := w.Repo.MarkInactiveProjects(activeIDs); err != nil {
		return err
	}
	if w.Connector.JiraBugJQL != "" {
		if err := importIssues(w.Connector.JiraBugJQL, false); err != nil {
			return err
		}
	}
	return w.cursor("jira", "global", "issues", newest)
}
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
func (w Worker) syncQaseProject(ctx context.Context, code string, step *SyncStep) error {
	for _, resource := range []string{"case", "run", "result"} {
		var newest time.Time
		err := w.Connector.QasePages(ctx, resource, code, func(raw json.RawMessage) error {
			step.Fetched++
			var item map[string]json.RawMessage
			if err := json.Unmarshal(raw, &item); err != nil {
				return err
			}
			switch resource {
			case "case":
				id := number(item, "id")
				if id == 0 {
					return errors.New("QASE_CASE_ID_MISSING")
				}
				row := QaseCase{ID: uuid.NewString(), ProjectCode: code, CaseID: id, Title: stringValue(item, "title"), UpdatedAt: time.Now().UTC()}
				return w.upsertCase(&row, step)
			case "run":
				id := number(item, "id")
				if id == 0 {
					return errors.New("QASE_RUN_ID_MISSING")
				}
				row := QaseRun{ID: uuid.NewString(), ProjectCode: code, RunID: id, Title: stringValue(item, "title"), Status: stringValue(item, "status"), Platform: runPlatform(stringValue(item, "title")), Environment: stringValue(item, "environment"), StartedAt: timeValue(item, "start_time"), FinishedAt: timeValue(item, "end_time"), FetchedAt: time.Now().UTC()}
				if err := w.upsertRun(&row, step); err != nil {
					return err
				}
				caseIDs, err := qaseCaseIDs(item["cases"])
				if err != nil {
					caseIDs, err = w.Connector.QaseRunCases(ctx, code, id)
					if err != nil {
						return err
					}
				}
				return w.replaceRunCases(code, id, caseIDs)
			case "result":
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
				return w.upsertResult(&row, step)
			}
			return nil
		})
		if err != nil {
			return err
		}
		if err := w.cursor("qase", code, resource, newest); err != nil {
			return err
		}
	}
	return nil
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
func runPlatform(title string) string {
	title = strings.ToUpper(title)
	for _, p := range []string{"AOS", "IOS", "BO", "DB", "APO"} {
		if strings.Contains(title, "[STG] "+p) {
			return p
		}
	}
	return "unknown"
}
