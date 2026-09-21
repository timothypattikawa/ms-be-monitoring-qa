package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Worker struct {
	DB        *gorm.DB
	Connector Connector
}

func (w Worker) Run(ctx context.Context) error {
	if err := w.DB.AutoMigrate(Models()...); err != nil {
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
	return w.DB.Transaction(func(tx *gorm.DB) error {
		var jobs []SyncJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? AND heartbeat_at < ? AND attempts >= ?", "running", now.Add(-10*time.Minute), 3).
			Find(&jobs).Error; err != nil {
			return err
		}
		for _, job := range jobs {
			if err := tx.Model(&SyncStep{}).Where("job_id = ? AND status <> ?", job.ID, "succeeded").Updates(map[string]any{
				"status": "failed", "error_code": "WORKER_RETRY_EXHAUSTED", "finished_at": now,
			}).Error; err != nil {
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
	return w.DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "request_key"}}, DoNothing: true}).Create(&job)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		for _, source := range []string{"jira", "qase"} {
			if err := tx.Create(&SyncStep{ID: uuid.NewString(), JobID: job.ID, Source: source, Status: "queued"}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
func (w Worker) claim() (*SyncJob, error) {
	var job SyncJob
	err := w.DB.Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("status = ? OR (status = ? AND heartbeat_at < ? AND attempts < 3)", "queued", "running", time.Now().UTC().Add(-10*time.Minute)).Order("requested_at").First(&job).Error
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		job.Status = "running"
		job.StartedAt = &now
		job.HeartbeatAt = &now
		job.Attempts++
		return tx.Save(&job).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &job, nil
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
				w.DB.Model(&SyncJob{}).Where("id = ? AND status = ?", job.ID, "running").Update("heartbeat_at", time.Now().UTC())
			}
		}
	}()
	if err := w.event(job.ID, "", "info", "SYNC_STARTED", "Sync started"); err != nil {
		return err
	}
	var steps []SyncStep
	if err := w.DB.Where("job_id = ?", job.ID).Find(&steps).Error; err != nil {
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
		if err := w.DB.Save(&step).Error; err != nil {
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
		if err := w.DB.Save(&step).Error; err != nil {
			return err
		}
		if err := w.DB.Model(&job).Updates(map[string]any{"heartbeat_at": finished}).Error; err != nil {
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
	if err := w.DB.Save(&job).Error; err != nil {
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
	return w.DB.Create(&SyncEvent{ID: uuid.NewString(), JobID: jobID, StepID: stepID, OccurredAt: time.Now().UTC(), Level: level, Code: code, Message: msg}).Error
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
			var existing JiraIssue
			err := w.DB.Where("external_id = ?", item.ID).First(&existing).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if errors.Is(err, gorm.ErrRecordNotFound) {
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
						var related Project
						err := w.DB.Where("jira_init_id = ?", target.ID).First(&related).Error
						if err == nil {
							existing.ProjectID = related.ID
						} else if !errors.Is(err, gorm.ErrRecordNotFound) {
							return err
						}
					}
				}
			}
			if existing.Severity == "" {
				existing.Severity = "Unknown"
			}
			if err := w.DB.Save(&existing).Error; err != nil {
				return err
			}
			// The approved JQL selects active INIT project issues. Create an unmapped project
			// so Qase scope can be attached explicitly instead of inferred from a title.
			if projects {
				activeIDs = append(activeIDs, item.ID)
				var p Project
				err := w.DB.Where("jira_init_id = ?", item.ID).First(&p).Error
				if errors.Is(err, gorm.ErrRecordNotFound) {
					p = Project{ID: uuid.NewString(), JiraInitID: item.ID, JiraInitKey: item.Key, Name: item.Fields.Summary, Status: item.Fields.Status.Name, Health: "unknown"}
					if err := w.DB.Create(&p).Error; err != nil {
						return err
					}
				} else if err == nil {
					p.JiraInitKey = item.Key
					p.Name = item.Fields.Summary
					p.Status = item.Fields.Status.Name
					if err := w.DB.Save(&p).Error; err != nil {
						return err
					}
				} else {
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
	inactive := w.DB.Model(&Project{}).Where("jira_init_id <> ''")
	if len(activeIDs) > 0 {
		inactive = inactive.Where("jira_init_id NOT IN ?", activeIDs)
	}
	if err := inactive.Update("status", "inactive").Error; err != nil {
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
	var projects []Project
	q := w.DB.Model(&Project{}).Where("qase_project_code <> ''")
	if job.ProjectID != "" {
		q = q.Where("id = ?", job.ProjectID)
	}
	if err := q.Find(&projects).Error; err != nil {
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
	return w.DB.Transaction(func(tx *gorm.DB) error {
		q := tx.Where("project_code = ? AND run_id = ?", code, runID)
		if len(caseIDs) > 0 {
			q = q.Where("case_id NOT IN ?", caseIDs)
		}
		if err := q.Delete(&QaseRunCase{}).Error; err != nil {
			return err
		}
		for _, caseID := range caseIDs {
			if caseID <= 0 {
				return errors.New("QASE_RUN_CASES_INVALID")
			}
			link := QaseRunCase{ID: uuid.NewString(), ProjectCode: code, RunID: runID, CaseID: caseID}
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "project_code"}, {Name: "run_id"}, {Name: "case_id"}}, DoNothing: true}).Create(&link).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
func (w Worker) upsertCase(row *QaseCase, step *SyncStep) error {
	var old QaseCase
	err := w.DB.Where("project_code = ? AND case_id = ?", row.ProjectCode, row.CaseID).First(&old).Error
	if err == nil {
		row.ID = old.ID
		step.Updated++
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		step.Inserted++
	} else {
		return err
	}
	return w.DB.Save(row).Error
}
func (w Worker) upsertRun(row *QaseRun, step *SyncStep) error {
	var old QaseRun
	err := w.DB.Where("project_code = ? AND run_id = ?", row.ProjectCode, row.RunID).First(&old).Error
	if err == nil {
		row.ID = old.ID
		step.Updated++
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		step.Inserted++
	} else {
		return err
	}
	return w.DB.Save(row).Error
}
func (w Worker) upsertResult(row *QaseResult, step *SyncStep) error {
	var old QaseResult
	err := w.DB.Where("project_code = ? AND result_id = ?", row.ProjectCode, row.ResultID).First(&old).Error
	if err == nil {
		row.ID = old.ID
		step.Updated++
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		step.Inserted++
	} else {
		return err
	}
	return w.DB.Save(row).Error
}
func (w Worker) cursor(source, scope, resource string, watermark time.Time) error {
	if watermark.IsZero() {
		watermark = time.Now().UTC()
	}
	var row SyncCursor
	err := w.DB.Where("source=? AND scope_key=? AND resource=?", source, scope, resource).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = SyncCursor{ID: uuid.NewString(), Source: source, ScopeKey: scope, Resource: resource}
	} else if err != nil {
		return err
	}
	row.Watermark = watermark
	return w.DB.Save(&row).Error
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
