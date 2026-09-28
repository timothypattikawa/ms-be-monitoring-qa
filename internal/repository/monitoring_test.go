package repository

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestMonitoringModelInventory(t *testing.T) {
	models := Models()
	if len(models) != 14 {
		t.Fatalf("got %d monitoring models, want 14", len(models))
	}
	seen := map[reflect.Type]bool{}
	for _, model := range models {
		typ := reflect.TypeOf(model)
		if typ.Kind() != reflect.Pointer || seen[typ] {
			t.Fatalf("model inventory contains invalid or duplicate type %v", typ)
		}
		// ProjectQAAssignment is a junction table with a composite primary
		// key (project_id, jira_account_id) per the plan, so it has no
		// singular ID field.
		if typ == reflect.TypeOf(&ProjectQAAssignment{}) {
			seen[typ] = true
			continue
		}
		id, ok := typ.Elem().FieldByName("ID")
		if !ok || strings.Contains(id.Tag.Get("gorm"), "type:uuid") {
			t.Fatalf("%v must keep string IDs compatible with relation columns", typ)
		}
		seen[typ] = true
	}
}

func TestProjectPersistenceContract(t *testing.T) {
	typ := reflect.TypeOf(Project{})
	want := map[string]reflect.Type{
		"JiraInitID": reflect.TypeOf((*string)(nil)), "JiraInitKey": reflect.TypeOf(""),
		"QaseProjectCode": reflect.TypeOf(""), "QAOwner": reflect.TypeOf(""),
		"StagingStartAt": reflect.TypeOf((*time.Time)(nil)), "StagingEndAt": reflect.TypeOf((*time.Time)(nil)),
		"BetaStartAt": reflect.TypeOf((*time.Time)(nil)), "BetaEndAt": reflect.TypeOf((*time.Time)(nil)),
	}
	for name, fieldType := range want {
		field, ok := typ.FieldByName(name)
		if !ok || field.Type != fieldType {
			t.Fatalf("Project.%s type = %v, want %v", name, field.Type, fieldType)
		}
	}
	key, _ := typ.FieldByName("JiraInitKey")
	if !strings.Contains(key.Tag.Get("gorm"), "uniqueIndex") {
		t.Fatal("JiraInitKey must have a database unique index")
	}
}

func TestWorkflowQueryIsCumulativeRunState(t *testing.T) {
	for _, required := range []string{
		"count(*) FROM qase_run_cases", "ar.active=true", "DISTINCT ON (r.case_id)",
		"SELECT DISTINCT date_trunc('day', r.ended_at)", "r.ended_at < d.day + interval '1 day'", "p.total AS total",
	} {
		if !strings.Contains(workflowQuery, required) {
			t.Fatalf("workflow query lost cumulative membership semantics: missing %q", required)
		}
	}
	if strings.Contains(workflowQuery, "generate_series") {
		t.Fatal("workflow must not invent calendar days without Qase activity")
	}
}

func TestNormalizeQaseEnvironment(t *testing.T) {
	tests := map[string]string{
		"staging":   "STAGING",
		"STG":       "STAGING",
		"stg-ios":   "STAGING",
		"[STG] AOS": "STAGING",
		"beta ios":  "BETA",
		"uat":       "UAT",
		"":          "",
	}
	for input, want := range tests {
		if got := normalizeQaseEnvironment(input); got != want {
			t.Errorf("normalizeQaseEnvironment(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestMemberActiveFilterRoundTrips(t *testing.T) {
	repo := NewSQLiteForTest(t)
	if err := repo.SaveMember(&Member{ID: "m1", Name: "Nadia Putri", WeeklyCapacityHours: 40, Active: true}); err != nil {
		t.Fatalf("save active member: %v", err)
	}
	if err := repo.SaveMember(&Member{ID: "m2", Name: "Old Member", WeeklyCapacityHours: 40, Active: false}); err != nil {
		t.Fatalf("save inactive member: %v", err)
	}
	active, err := repo.Members(false)
	if err != nil {
		t.Fatalf("list active members: %v", err)
	}
	if len(active) != 1 || active[0].ID != "m1" {
		t.Fatalf("expected only m1 in active list, got %+v", active)
	}
	all, err := repo.Members(true)
	if err != nil {
		t.Fatalf("list all members: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 members total, got %d", len(all))
	}
	got, err := repo.Member("m2")
	if err != nil {
		t.Fatalf("get member: %v", err)
	}
	if got.Name != "Old Member" || got.Active {
		t.Fatalf("unexpected member: %+v", got)
	}

	// Guards the exact bug the Active field's gorm tag was chosen to avoid:
	// there is no DB-level default, so an unset Active must persist as false,
	// not silently flip to true.
	if err := repo.SaveMember(&Member{ID: "m3", Name: "No Explicit Active"}); err != nil {
		t.Fatalf("save member with unset active: %v", err)
	}
	m3, err := repo.Member("m3")
	if err != nil {
		t.Fatalf("get member: %v", err)
	}
	if m3.Active {
		t.Fatalf("expected Active to persist as false when left unset, got %+v", m3)
	}
}

func TestWorkloadQaseQueriesMatchTesterNameByPlatform(t *testing.T) {
	repo := NewSQLiteForTest(t)
	now := time.Now().UTC()
	from, to := now.Add(-time.Hour), now.Add(time.Hour)

	if _, err := repo.UpsertCase(&QaseCase{ID: "c1", ProjectCode: "P", CaseID: 1, TesterIosName: "Alice"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpsertCase(&QaseCase{ID: "c2", ProjectCode: "P", CaseID: 2, TesterAndroidName: "Alice"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpsertCase(&QaseCase{ID: "c3", ProjectCode: "P", CaseID: 3, TesterName: "Alice"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpsertRun(&QaseRun{ID: "r10", ProjectCode: "P", RunID: 10, Platform: "IOS"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpsertRun(&QaseRun{ID: "r11", ProjectCode: "P", RunID: 11, Platform: "AOS"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpsertRun(&QaseRun{ID: "r12", ProjectCode: "P", RunID: 12, Platform: "BO"}); err != nil {
		t.Fatal(err)
	}
	results := []QaseResult{
		{ID: "res1", ProjectCode: "P", ResultID: "res1", RunID: 10, CaseID: 1, Status: "passed", EndedAt: &now}, // IOS run + tester_ios_name match -> counts
		{ID: "res2", ProjectCode: "P", ResultID: "res2", RunID: 11, CaseID: 2, Status: "passed", EndedAt: &now}, // AOS run + tester_android_name match -> counts
		{ID: "res3", ProjectCode: "P", ResultID: "res3", RunID: 12, CaseID: 3, Status: "passed", EndedAt: &now}, // non-mobile run + tester_name match -> counts
		{ID: "res4", ProjectCode: "P", ResultID: "res4", RunID: 10, CaseID: 2, Status: "passed", EndedAt: &now}, // IOS run but case only has tester_android_name -> no match
		{ID: "res5", ProjectCode: "P", ResultID: "res5", RunID: 12, CaseID: 1, Status: "passed", EndedAt: &now}, // non-mobile run but case only has tester_ios_name -> no match
	}
	for i := range results {
		if _, err := repo.UpsertResult(&results[i]); err != nil {
			t.Fatal(err)
		}
	}

	var count int64
	if err := repo.db.Raw(workloadQaseCountQuery, "Alice", from, to).Scan(&count).Error; err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 3 {
		t.Fatalf("got %d matching qase results, want 3 (only same-platform tester matches)", count)
	}

	var none int64
	if err := repo.db.Raw(workloadQaseCountQuery, "Someone Else", from, to).Scan(&none).Error; err != nil {
		t.Fatalf("count query: %v", err)
	}
	if none != 0 {
		t.Fatalf("got %d matches for an unrelated name, want 0", none)
	}
}

// TestUpsertResultPreservesTesterNameSnapshot guards the fix for PIC rows in
// the Detailed Execution History table silently rewriting themselves (e.g.
// "Irene magang" turning into "Amalia" for a past date) whenever the
// underlying case got reassigned in Qase after that date's result was
// synced. TesterName must be captured once, at first sync, and never
// overwritten by a later resync even if the caller now passes a different
// value (reflecting the case's current, changed assignment).
func TestUpsertResultPreservesTesterNameSnapshot(t *testing.T) {
	repo := NewSQLiteForTest(t)
	now := time.Now().UTC()

	first := QaseResult{ID: "r1", ProjectCode: "P", ResultID: "res1", RunID: 1, CaseID: 1, Status: "passed", TesterName: "Irene magang", EndedAt: &now}
	if _, err := repo.UpsertResult(&first); err != nil {
		t.Fatal(err)
	}

	resync := QaseResult{ProjectCode: "P", ResultID: "res1", RunID: 1, CaseID: 1, Status: "passed", TesterName: "Amalia", EndedAt: &now}
	if _, err := repo.UpsertResult(&resync); err != nil {
		t.Fatal(err)
	}
	if resync.TesterName != "Irene magang" {
		t.Fatalf("TesterName snapshot was overwritten on resync: got %q, want the original %q", resync.TesterName, "Irene magang")
	}

	var stored QaseResult
	if err := repo.db.Where("project_code = ? AND result_id = ?", "P", "res1").First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.TesterName != "Irene magang" {
		t.Fatalf("persisted TesterName = %q, want %q", stored.TesterName, "Irene magang")
	}
}

func TestProjectCountsAndRunsAggregateAllActiveRuns(t *testing.T) {
	repo := NewSQLiteForTest(t)
	now := time.Now().UTC()
	if _, err := repo.UpsertRun(&QaseRun{ID: "run1", ProjectCode: "ILTA", RunID: 1, Title: "[BETA] APO Main", Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpsertRun(&QaseRun{ID: "run2", ProjectCode: "ILTA", RunID: 2, Title: "[BETA] Android", Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpsertRun(&QaseRun{ID: "run3", ProjectCode: "ILTA", RunID: 3, Title: "[BETA] Closed", Active: false}); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceRunCases("ILTA", 1, []int64{1, 2}); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceRunCases("ILTA", 2, []int64{3}); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceRunCases("ILTA", 3, []int64{4}); err != nil {
		t.Fatal(err)
	}
	results := []QaseResult{
		{ID: "res1", ProjectCode: "ILTA", ResultID: "res1", RunID: 1, CaseID: 1, Status: "passed", EndedAt: &now},
		{ID: "res2", ProjectCode: "ILTA", ResultID: "res2", RunID: 1, CaseID: 2, Status: "failed", EndedAt: &now},
		{ID: "res3", ProjectCode: "ILTA", ResultID: "res3", RunID: 2, CaseID: 3, Status: "passed", EndedAt: &now},
		{ID: "res4", ProjectCode: "ILTA", ResultID: "res4", RunID: 3, CaseID: 4, Status: "blocked", EndedAt: &now},
	}
	for i := range results {
		if _, err := repo.UpsertResult(&results[i]); err != nil {
			t.Fatal(err)
		}
	}
	p := Project{QaseProjectCode: "ILTA"}
	counts, ok, err := repo.ProjectCounts(p)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected counts to be available")
	}
	// run3 (inactive) must not contribute: total across run1+run2 only = 3, not 4.
	if counts.Total != 3 || counts.Passed != 2 || counts.Failed != 1 || counts.Blocked != 0 {
		t.Fatalf("unexpected aggregated counts: %+v", counts)
	}
	runs, err := repo.ProjectRuns(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 active run summaries, got %d: %+v", len(runs), runs)
	}
	var total int64
	for _, r := range runs {
		total += r.Total
	}
	if total != 3 {
		t.Fatalf("run summaries total = %d, want 3 (inactive run3 excluded)", total)
	}

	// Now simulate a sync where only run 1 is still active: run2 should flip inactive.
	if err := repo.MarkRunsInactive("ILTA", []int64{1}); err != nil {
		t.Fatal(err)
	}
	afterCounts, ok, err := repo.ProjectCounts(p)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || afterCounts.Total != 2 {
		t.Fatalf("expected only run1's 2 cases to count after run2 goes inactive, got %+v (ok=%v)", afterCounts, ok)
	}
}

func TestProjectEnvironmentCountsDedupesSharedCasesAcrossRuns(t *testing.T) {
	repo := NewSQLiteForTest(t)
	now := time.Now().UTC()
	// Same real-world shape as ILTA: one scenario (case 1) is attached to
	// both the STAGING IOS run and the STAGING Android run, since it's the
	// same test run on two devices, not two different scenarios.
	if _, err := repo.UpsertRun(&QaseRun{ID: "run1", ProjectCode: "ILTA", RunID: 1, Title: "[STG] IOS", Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpsertRun(&QaseRun{ID: "run2", ProjectCode: "ILTA", RunID: 2, Title: "[STG] Android", Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpsertRun(&QaseRun{ID: "run3", ProjectCode: "ILTA", RunID: 3, Title: "[BETA] IOS", Active: true}); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceRunCases("ILTA", 1, []int64{1, 2}); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceRunCases("ILTA", 2, []int64{1, 3, 5}); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceRunCases("ILTA", 3, []int64{4}); err != nil {
		t.Fatal(err)
	}
	results := []QaseResult{
		{ID: "res1", ProjectCode: "ILTA", ResultID: "res1", RunID: 1, CaseID: 1, Status: "passed", EndedAt: &now},
		{ID: "res2", ProjectCode: "ILTA", ResultID: "res2", RunID: 1, CaseID: 2, Status: "passed", EndedAt: &now},
		{ID: "res3", ProjectCode: "ILTA", ResultID: "res3", RunID: 2, CaseID: 1, Status: "passed", EndedAt: &now},
		{ID: "res4", ProjectCode: "ILTA", ResultID: "res4", RunID: 2, CaseID: 3, Status: "failed", EndedAt: &now},
		{ID: "res5", ProjectCode: "ILTA", ResultID: "res5", RunID: 3, CaseID: 4, Status: "passed", EndedAt: &now},
		{ID: "res6", ProjectCode: "ILTA", ResultID: "res6", RunID: 2, CaseID: 5, Status: "blocked", EndedAt: &now},
	}
	for i := range results {
		if _, err := repo.UpsertResult(&results[i]); err != nil {
			t.Fatal(err)
		}
	}
	p := Project{QaseProjectCode: "ILTA"}
	staging, err := repo.ProjectEnvironmentCounts(p, "STAGING")
	if err != nil {
		t.Fatal(err)
	}
	// Naively summing run1 (2 cases) + run2 (3 cases) would give Total=5 and
	// Passed=3, i.e. >100%. Deduped by case ID, STAGING covers 3 unique
	// executed cases (1,2,3) plus blocked case 5. Executed is strictly
	// passed+failed = 3; blocked/not-run stay outside that numerator.
	if staging.Total != 4 || staging.Passed != 2 || staging.Failed != 1 || staging.Blocked != 1 || staging.Passed+staging.Failed > staging.Total {
		t.Fatalf("unexpected staging counts: %+v", staging)
	}
	beta, err := repo.ProjectEnvironmentCounts(p, "BETA")
	if err != nil {
		t.Fatal(err)
	}
	if beta.Total != 1 || beta.Passed != 1 {
		t.Fatalf("unexpected beta counts: %+v", beta)
	}
}

func TestProjectCountsPrefersQaseTotalCasesOverRunMembership(t *testing.T) {
	repo := NewSQLiteForTest(t)
	now := time.Now().UTC()
	saved, err := repo.SaveProjectMapping("INIT-683", "Live Tracking", "ILTA", "Kiki", now, now, now, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpsertRun(&QaseRun{ID: "run1", ProjectCode: "ILTA", RunID: 1, Title: "[BETA] APO Main", Active: true}); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceRunCases("ILTA", 1, []int64{1, 2}); err != nil {
		t.Fatal(err)
	}
	results := []QaseResult{
		{ID: "res1", ProjectCode: "ILTA", ResultID: "res1", RunID: 1, CaseID: 1, Status: "passed", EndedAt: &now},
		{ID: "res2", ProjectCode: "ILTA", ResultID: "res2", RunID: 1, CaseID: 2, Status: "failed", EndedAt: &now},
	}
	for i := range results {
		if _, err := repo.UpsertResult(&results[i]); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.UpdateProjectQaseScope("ILTA", 542, 45); err != nil {
		t.Fatal(err)
	}
	p, err := repo.Project(saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if p.QaseTotalCases != 542 || p.QaseTotalSuites != 45 {
		t.Fatalf("scope not persisted: %+v", p)
	}
	counts, ok, err := repo.ProjectCounts(p)
	if err != nil {
		t.Fatal(err)
	}
	// Total reflects Qase's own project-wide case count (542), not the 2
	// cases attached to the currently tracked run. Passed/Failed still come
	// from actual executed results.
	if !ok || counts.Total != 542 || counts.Passed != 1 || counts.Failed != 1 {
		t.Fatalf("unexpected counts: %+v (ok=%v)", counts, ok)
	}
}

func TestMemberDisplayNameFallsBackToStableID(t *testing.T) {
	if got := memberDisplayName(Member{ID: "qase-member-42"}); got != "qase-member-42" {
		t.Fatalf("empty display name returned %q", got)
	}
	if got := memberDisplayName(Member{ID: "42", Name: "  Kiki  "}); got != "Kiki" {
		t.Fatalf("named member returned %q", got)
	}
}

// TestJiraBugsFiltersActiveProjectScopedRows exercises the source switch
// (plan §9.1/9.5): bugs now come straight from jira_issues (sync_scope
// "project-bugs", active=true), not qase_defects, so an issue never linked
// to a Qase defect still shows up. Rows outside that scope (an inactive
// issue, and a "BUG" project production-bug row written by the unrelated
// production-bugs sync) must never leak into the result.
func TestJiraBugsFiltersActiveProjectScopedRows(t *testing.T) {
	repo := NewSQLiteForTest(t)
	now := time.Now().UTC()
	if _, err := repo.SaveProjectMapping("INIT-1", "Refund", "PAY", "Kiki", now, now, now, now); err != nil {
		t.Fatal(err)
	}
	proj, found, err := repo.ProjectByJiraKey("INIT-1")
	if err != nil || !found {
		t.Fatalf("expected project by jira key, found=%v err=%v", found, err)
	}
	if err := repo.ReplaceProjectBugs(proj.ID, "project-bugs", []JiraIssue{
		{ID: uuid.NewString(), ExternalID: "1", Key: "QASE-1", Summary: "Checkout crash", Severity: "Medium", Status: "Open", Creator: "Dea", Reporter: "Kiki", Assignee: "Nadia", ParentKey: "INIT-1", Environment: "STAGING", CreatedAt: &now},
		{ID: uuid.NewString(), ExternalID: "2", Key: "QASE-2", Summary: "Beta glitch", Severity: "High", Status: "Confirm", Reporter: "Dea", ParentKey: "INIT-1", Environment: "BETA", CreatedAt: &now},
	}); err != nil {
		t.Fatal(err)
	}
	// Superseded next run: not in the new issue list, so it's deactivated and
	// must drop out of JiraBugs.
	if err := repo.ReplaceProjectBugs(proj.ID, "project-bugs", []JiraIssue{
		{ID: uuid.NewString(), ExternalID: "1", Key: "QASE-1", Summary: "Checkout crash", Severity: "Medium", Status: "Open", Creator: "Dea", Reporter: "Kiki", Assignee: "Nadia", ParentKey: "INIT-1", Environment: "STAGING", CreatedAt: &now},
	}); err != nil {
		t.Fatal(err)
	}
	// Unrelated production-bugs feature row: different scope entirely, must
	// never surface through JiraBugs.
	if err := repo.ReplaceProductionBugs([]JiraIssue{
		{ID: uuid.NewString(), ExternalID: "prod-1", Key: "BUG-1", ProjectID: "BUG", Summary: "Prod outage", Severity: "Critical", Status: "Open"},
	}); err != nil {
		t.Fatal(err)
	}

	page, err := repo.JiraBugs(1, 50, "", "", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("expected only the still-active project-bugs row, got %+v", page)
	}
	got := page.Items[0]
	if got.Key != "QASE-1" || got.Summary != "Checkout crash" || got.Severity != "Medium" || got.Status != "Open" || got.Creator != "Dea" || got.Reporter != "Kiki" || got.Assignee != "Nadia" || got.ProjectID != proj.ID {
		t.Fatalf("unexpected ApiBug shape: %+v", got)
	}
}

// TestJiraBugsFilterParamsAndPagination covers each optional query param
// (projectId/environment/reporter/status/priority/q) plus DB-side pagination
// (plan §9.5: "Pagination dilakukan di database").
func TestJiraBugsFilterParamsAndPagination(t *testing.T) {
	repo := NewSQLiteForTest(t)
	now := time.Now().UTC()
	if _, err := repo.SaveProjectMapping("INIT-2", "Checkout", "CHK", "Kiki", now, now, now, now); err != nil {
		t.Fatal(err)
	}
	proj, _, err := repo.ProjectByJiraKey("INIT-2")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceProjectBugs(proj.ID, "project-bugs", []JiraIssue{
		{ID: uuid.NewString(), ExternalID: "1", Key: "QASE-1", Summary: "Login fails", Severity: "High", Status: "Open", Reporter: "Kiki", ParentKey: "INIT-2", Environment: "STAGING", CreatedAt: &now},
		{ID: uuid.NewString(), ExternalID: "2", Key: "QASE-2", Summary: "Payment invalid", Severity: "Low", Status: "Invalid", Reporter: "Dea", ParentKey: "INIT-2", Environment: "STAGING", CreatedAt: &now},
	}); err != nil {
		t.Fatal(err)
	}

	if p, err := repo.JiraBugs(1, 50, proj.ID, "", "", "", "", ""); err != nil || p.Total != 2 {
		t.Fatalf("projectId filter: page=%+v err=%v", p, err)
	}
	if p, err := repo.JiraBugs(1, 50, "", "staging", "", "", "", ""); err != nil || p.Total != 2 {
		t.Fatalf("environment filter (case-insensitive): page=%+v err=%v", p, err)
	}
	if p, err := repo.JiraBugs(1, 50, "", "beta", "", "", "", ""); err != nil || p.Total != 0 {
		t.Fatalf("environment filter excludes non-matching rows: page=%+v err=%v", p, err)
	}
	if p, err := repo.JiraBugs(1, 50, "", "", "Dea", "", "", ""); err != nil || p.Total != 1 || p.Items[0].Key != "QASE-2" {
		t.Fatalf("reporter filter: page=%+v err=%v", p, err)
	}
	if p, err := repo.JiraBugs(1, 50, "", "", "", "Invalid", "", ""); err != nil || p.Total != 1 || p.Items[0].Key != "QASE-2" {
		t.Fatalf("status filter: page=%+v err=%v", p, err)
	}
	// "priority" is the frontend/query param name, filtered against the
	// Severity column (plan §9.5 contract).
	if p, err := repo.JiraBugs(1, 50, "", "", "", "", "High", ""); err != nil || p.Total != 1 || p.Items[0].Key != "QASE-1" {
		t.Fatalf("priority(severity) filter: page=%+v err=%v", p, err)
	}
	// ponytail: the "q" free-text filter uses ILIKE (Postgres-only, matching
	// the same pattern already used untested elsewhere in this file, e.g.
	// Projects' name/key search) — the in-memory SQLite test driver doesn't
	// support it, so it's exercised against real Postgres only.

	first, err := repo.JiraBugs(1, 1, "", "", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if first.Total != 2 || len(first.Items) != 1 || first.Page != 1 || first.PageSize != 1 {
		t.Fatalf("page 1 of 1: %+v", first)
	}
	second, err := repo.JiraBugs(2, 1, "", "", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if second.Total != 2 || len(second.Items) != 1 || second.Items[0].Key == first.Items[0].Key {
		t.Fatalf("page 2 of 1 must return the other row: page1=%+v page2=%+v", first, second)
	}
}

func TestResolveEnvironmentPrefersRunEnvironmentThenTitleMarker(t *testing.T) {
	tests := []struct {
		name        string
		environment string
		title       string
		want        string
	}{
		{"explicit environment wins", "beta", "[STG] AOS Main", "BETA"},
		{"blank falls back to STG title marker", "", "[STG] AOS Main", "STAGING"},
		{"blank falls back to BETA title marker", "", "[BETA] APO Main", "BETA"},
		{"blank with no marker stays blank", "", "Regression Suite", ""},
	}
	for _, tc := range tests {
		if got := ResolveEnvironment(tc.environment, tc.title); got != tc.want {
			t.Errorf("%s: ResolveEnvironment(%q, %q) = %q, want %q", tc.name, tc.environment, tc.title, got, tc.want)
		}
	}
}

func TestProjectRunsExposesPlatformAwareTesters(t *testing.T) {
	repo := NewSQLiteForTest(t)
	now := time.Now().UTC()
	if _, err := repo.UpsertCase(&QaseCase{ID: "c1", ProjectCode: "P", CaseID: 1, TesterIosName: "Alice"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpsertCase(&QaseCase{ID: "c2", ProjectCode: "P", CaseID: 2, TesterAndroidName: "Budi"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpsertRun(&QaseRun{ID: "r10", ProjectCode: "P", RunID: 10, Title: "[STG] IOS Main", Platform: "IOS", Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpsertRun(&QaseRun{ID: "r11", ProjectCode: "P", RunID: 11, Title: "[STG] AOS Main", Platform: "AOS", Active: true}); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceRunCases("P", 10, []int64{1}); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceRunCases("P", 11, []int64{2}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpsertResult(&QaseResult{ID: "res1", ProjectCode: "P", ResultID: "res1", RunID: 10, CaseID: 1, Status: "passed", EndedAt: &now}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpsertResult(&QaseResult{ID: "res2", ProjectCode: "P", ResultID: "res2", RunID: 11, CaseID: 2, Status: "passed", EndedAt: &now}); err != nil {
		t.Fatal(err)
	}
	runs, err := repo.ProjectRuns(Project{QaseProjectCode: "P"})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 active runs, got %+v", runs)
	}
	byRunID := map[int64][]string{}
	for _, r := range runs {
		byRunID[r.RunID] = r.Testers
	}
	if got := byRunID[10]; len(got) != 1 || got[0] != "Alice" {
		t.Fatalf("expected IOS run to expose Alice via tester_ios_name, got %v", got)
	}
	if got := byRunID[11]; len(got) != 1 || got[0] != "Budi" {
		t.Fatalf("expected AOS run to expose Budi via tester_android_name, got %v", got)
	}
}

func TestProjectTesterBreakdownCountsScenariosByPicField(t *testing.T) {
	repo := NewSQLiteForTest(t)
	if _, err := repo.UpsertCase(&QaseCase{ID: "c1", ProjectCode: "P", CaseID: 1, PicNames: "Alice"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpsertCase(&QaseCase{ID: "c2", ProjectCode: "P", CaseID: 2, PicNames: "Alice"}); err != nil {
		t.Fatal(err)
	}
	// A case with multiple PICs counts toward each of them.
	if _, err := repo.UpsertCase(&QaseCase{ID: "c3", ProjectCode: "P", CaseID: 3, PicNames: "Alice, Budi"}); err != nil {
		t.Fatal(err)
	}
	// Not a case-run/execution concern any more: no run or result rows at
	// all, and the count still reflects total scenarios created.
	if _, err := repo.UpsertCase(&QaseCase{ID: "c4", ProjectCode: "P", CaseID: 4}); err != nil {
		t.Fatal(err)
	}
	breakdown, err := repo.ProjectTesterBreakdown(Project{QaseProjectCode: "P"})
	if err != nil {
		t.Fatal(err)
	}
	if len(breakdown) != 2 {
		t.Fatalf("expected only Alice and Budi (case 4 has no PIC), got %+v", breakdown)
	}
	byName := map[string]TesterProgress{}
	for _, tp := range breakdown {
		byName[tp.Name] = tp
	}
	if alice := byName["Alice"]; alice.Total != 3 {
		t.Fatalf("unexpected Alice total: %+v", alice)
	}
	if budi := byName["Budi"]; budi.Total != 1 {
		t.Fatalf("unexpected Budi total: %+v", budi)
	}
}

func TestProjectBugSummaryCountsBothEnvironmentsAndCanceled(t *testing.T) {
	repo := NewSQLiteForTest(t)
	now := time.Now().UTC()
	defects := []QaseDefect{
		{ID: "d1", ProjectCode: "PAY", DefectID: 1, Environment: "STAGING", Status: "open", UpdatedAt: now},
		{ID: "d2", ProjectCode: "PAY", DefectID: 2, Environment: "STAGING", JiraStatus: "Cancelled", UpdatedAt: now},
		{ID: "d3", ProjectCode: "PAY", DefectID: 3, Environment: "BETA", JiraPriority: "Low", UpdatedAt: now},
		{ID: "d5", ProjectCode: "PAY", DefectID: 5, Status: "closed", UpdatedAt: now},
		{ID: "d4", ProjectCode: "OTHER", DefectID: 4, Environment: "BETA", UpdatedAt: now},
	}
	for i := range defects {
		if _, err := repo.UpsertDefect(&defects[i]); err != nil {
			t.Fatal(err)
		}
	}
	got, err := repo.ProjectBugSummary(Project{QaseProjectCode: "PAY"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Staging != 2 || got.Beta != 1 || got.Total != 4 || got.Canceled != 1 || !got.BetaOverThirtyPercentOfStaging {
		t.Fatalf("unexpected summary: %+v", got)
	}
}

func TestProductionBugsPaginatesInPersistedJQLOrder(t *testing.T) {
	repo := NewSQLiteForTest(t)
	base := time.Now().UTC()
	rows := []JiraIssue{
		{ID: "1", ExternalID: "1", Key: "BUG-1", ProjectID: "BUG", Summary: "First", Reporter: "R1", Assignee: "A1", FetchedAt: base},
		{ID: "2", ExternalID: "2", Key: "BUG-2", ProjectID: "BUG", Summary: "Second", Reporter: "R2", Assignee: "A2", FetchedAt: base.Add(time.Nanosecond)},
		{ID: "3", ExternalID: "3", Key: "INIT-1", ProjectID: "project-1", Summary: "Not production", FetchedAt: base},
	}
	for i := range rows {
		if err := repo.SaveJiraIssue(&rows[i]); err != nil {
			t.Fatal(err)
		}
	}
	got, err := repo.ProductionBugs(2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 2 || got.Page != 2 || got.PageSize != 1 || len(got.Items) != 1 {
		t.Fatalf("unexpected page: %+v", got)
	}
	if got.Items[0].Key != "BUG-2" || got.Items[0].Reporter != "R2" || got.Items[0].Assignee != "A2" {
		t.Fatalf("unexpected item: %+v", got.Items[0])
	}
}

// TestWorkloadPopulatesProjectPortfolioFields covers plan §10.3: ActiveProjects
// only counts active assignments on non-excluded-status projects, TotalProjects
// counts every assignment ever synced regardless of active/status, NextProject
// picks the nearest future staging start among active assignments, and Projects
// lists all of them.
func TestWorkloadPopulatesProjectPortfolioFields(t *testing.T) {
	repo := NewSQLiteForTest(t)
	now := time.Now().UTC()

	projA, err := repo.SaveProjectMapping("INIT-1", "Checkout", "CHK", "Kiki", now.Add(48*time.Hour), now, now, now)
	if err != nil {
		t.Fatal(err)
	}
	projB, err := repo.SaveProjectMapping("INIT-2", "Legacy Cleanup", "LEG", "Kiki", now, now, now, now)
	if err != nil {
		t.Fatal(err)
	}
	projB.Status = "Done"
	if err := repo.SaveProject(&projB); err != nil {
		t.Fatal(err)
	}
	projC, err := repo.SaveProjectMapping("INIT-3", "Old Project", "OLD", "Kiki", now, now, now, now)
	if err != nil {
		t.Fatal(err)
	}
	projD, err := repo.SaveProjectMapping("INIT-4", "Refund Flow", "RFD", "Kiki", now.Add(24*time.Hour), now, now, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveMember(&Member{ID: "m1", Name: "Nadia", JiraAccountID: "acc1", Active: true}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []Project{projA, projD} {
		if err := repo.ReplaceProjectAssignments(p.ID, []ProjectQAAssignment{{JiraAccountID: "acc1", DisplayName: "Nadia"}}, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.ReplaceProjectAssignments(projB.ID, []ProjectQAAssignment{{JiraAccountID: "acc1", DisplayName: "Nadia"}}, now); err != nil {
		t.Fatal(err)
	}
	// projC's assignment existed once but was dropped on a later sync (no
	// longer in Jira's QAs field) — it must still count toward TotalProjects
	// but not ActiveProjects.
	if err := repo.ReplaceProjectAssignments(projC.ID, []ProjectQAAssignment{{JiraAccountID: "acc1", DisplayName: "Nadia"}}, now); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceProjectAssignments(projC.ID, nil, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	members, err := repo.Workload(now.Add(-24*time.Hour), now.Add(24*time.Hour), "m1")
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 {
		t.Fatalf("got %d workload members, want 1", len(members))
	}
	got := members[0]
	if got.TotalProjects != 4 {
		t.Fatalf("TotalProjects = %d, want 4 (every assignment ever synced)", got.TotalProjects)
	}
	if got.ActiveProjects != 2 {
		t.Fatalf("ActiveProjects = %d, want 2 (excludes Done status and dropped assignment)", got.ActiveProjects)
	}
	if len(got.Projects) != 2 {
		t.Fatalf("Projects = %+v, want 2 entries", got.Projects)
	}
	gotKeys := map[string]bool{}
	for _, p := range got.Projects {
		gotKeys[p.Key] = true
	}
	if !gotKeys["INIT-1"] || !gotKeys["INIT-4"] {
		t.Fatalf("Projects keys = %v, want INIT-1 and INIT-4", gotKeys)
	}
	if got.NextProject == nil || got.NextProject.Key != "INIT-4" {
		t.Fatalf("NextProject = %+v, want the nearer-future INIT-4", got.NextProject)
	}
}
