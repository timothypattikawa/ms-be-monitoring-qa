package monitoring

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Beyondtech-ID/ms-monitoring-qa-be/internal/repository"
)

func TestSyncJiraPersistsProductionBugsUsingExactJQL(t *testing.T) {
	repo := repository.NewSQLiteForTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/rest/api/3/search/jql" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var request struct {
			JQL string `json:"jql"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatal(err)
		}
		if request.JQL != productionBugJQL {
			t.Fatalf("JQL = %q, want %q", request.JQL, productionBugJQL)
		}
		_, _ = w.Write([]byte(`{"issues":[{"id":"1","key":"BUG-1","fields":{"summary":"Production failure","status":{"name":"Confirm"},"issuetype":{"name":"Bug"},"priority":{"name":"Highest"},"reporter":{"displayName":"Kiki"},"assignee":{"displayName":"Nadia"}}}],"isLast":true}`))
	}))
	defer srv.Close()
	w := Worker{Repo: repo, Connector: Connector{Client: srv.Client(), JiraBaseURL: srv.URL, JiraEmail: "qa@example.com", JiraToken: "token"}}
	step := &SyncStep{}
	if err := w.syncJira(t.Context(), &SyncJob{}, step); err != nil {
		t.Fatal(err)
	}
	page, err := repo.ProductionBugs(1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].Reporter != "Kiki" || page.Items[0].Assignee != "Nadia" {
		t.Fatalf("unexpected persisted page: %+v", page)
	}
}

// TestSyncJiraBugsPaginatesWithoutLosingIssues covers plan §16.1 item 11:
// a multi-page JQL result (nextPageToken, isLast=false then true) must
// persist every issue, not just the last page.
func TestSyncJiraBugsPaginatesWithoutLosingIssues(t *testing.T) {
	repo := repository.NewSQLiteForTest(t)
	project, err := repo.SaveProjectMapping("INIT-99", "Refund", "PAY", "Kiki", time.Now(), time.Now(), time.Now(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var request struct {
			JQL string `json:"jql"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatal(err)
		}
		if want := fmt.Sprintf(projectBugsJQLTemplate, "INIT-99"); request.JQL != want {
			t.Fatalf("JQL = %q, want %q", request.JQL, want)
		}
		if calls == 1 {
			_, _ = w.Write([]byte(`{"issues":[{"id":"1","key":"QASE-1","fields":{"summary":"first page","status":{"name":"Open"},"priority":{"name":"High"}}}],"isLast":false,"nextPageToken":"tok"}`))
			return
		}
		_, _ = w.Write([]byte(`{"issues":[{"id":"2","key":"QASE-2","fields":{"summary":"second page","status":{"name":"Open"},"priority":{"name":"High"}}}],"isLast":true}`))
	}))
	defer srv.Close()
	w := Worker{Repo: repo, Connector: Connector{Client: srv.Client(), JiraBaseURL: srv.URL, JiraEmail: "qa@example.com", JiraToken: "token"}}
	step := &SyncStep{}
	if err := w.syncJiraBugs(t.Context(), project, step); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 requests (one per page), got %d", calls)
	}
	page, err := repo.JiraBugs(1, 50, "", "", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 {
		t.Fatalf("expected both pages' issues persisted, got %+v", page.Items)
	}
}

// TestSyncJiraBugsReconciliationDeactivatesStaleIssues covers plan §16.1
// item 12: an issue present in a prior run but absent from the latest JQL
// pass must be marked inactive, not deleted, and drop out of JiraBugs.
func TestSyncJiraBugsReconciliationDeactivatesStaleIssues(t *testing.T) {
	repo := repository.NewSQLiteForTest(t)
	project, err := repo.SaveProjectMapping("INIT-98", "Refund", "PAY", "Kiki", time.Now(), time.Now(), time.Now(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			_, _ = w.Write([]byte(`{"issues":[{"id":"1","key":"QASE-1","fields":{"summary":"stays","status":{"name":"Open"},"priority":{"name":"High"}}},{"id":"2","key":"QASE-2","fields":{"summary":"drops out","status":{"name":"Open"},"priority":{"name":"High"}}}],"isLast":true}`))
			return
		}
		// Second run: issue "2" no longer matches the JQL.
		_, _ = w.Write([]byte(`{"issues":[{"id":"1","key":"QASE-1","fields":{"summary":"stays","status":{"name":"Open"},"priority":{"name":"High"}}}],"isLast":true}`))
	}))
	defer srv.Close()
	w := Worker{Repo: repo, Connector: Connector{Client: srv.Client(), JiraBaseURL: srv.URL, JiraEmail: "qa@example.com", JiraToken: "token"}}
	if err := w.syncJiraBugs(t.Context(), project, &SyncStep{}); err != nil {
		t.Fatal(err)
	}
	if err := w.syncJiraBugs(t.Context(), project, &SyncStep{}); err != nil {
		t.Fatal(err)
	}
	page, err := repo.JiraBugs(1, 50, "", "", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || page.Items[0].Key != "QASE-1" {
		t.Fatalf("expected only the still-matched issue to remain active, got %+v", page.Items)
	}
}

// TestSyncJiraBugsInvalidStatusCountsAsCanceledOnly covers plan §16.1 item
// 13 and §9.7 ("Invalid ditampilkan sebagai canceled/invalid, bukan active
// defect"): an Invalid/Staging issue is still fetched (the JQL's carve-out
// deliberately includes it) but must be reflected as a canceled bug in the
// project bug summary, not counted toward Staging/Beta as an in-flight defect.
func TestSyncJiraBugsInvalidStatusCountsAsCanceledOnly(t *testing.T) {
	repo := repository.NewSQLiteForTest(t)
	project, err := repo.SaveProjectMapping("INIT-97", "Refund", "PAY", "Kiki", time.Now(), time.Now(), time.Now(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"issues":[
			{"id":"1","key":"QASE-1","fields":{"summary":"open bug","status":{"name":"Open"},"priority":{"name":"High"},"customfield_10185":{"value":"Staging"}}},
			{"id":"2","key":"QASE-2","fields":{"summary":"invalid bug","status":{"name":"Invalid"},"priority":{"name":"Low"},"customfield_10185":{"value":"Staging"}}}
		],"isLast":true}`))
	}))
	defer srv.Close()
	w := Worker{Repo: repo, Connector: Connector{Client: srv.Client(), JiraBaseURL: srv.URL, JiraEmail: "qa@example.com", JiraToken: "token"}}
	if err := w.syncJiraBugs(t.Context(), project, &SyncStep{}); err != nil {
		t.Fatal(err)
	}
	summary, err := repo.ProjectBugSummary(project)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Total != 2 || summary.Staging != 2 {
		t.Fatalf("expected both issues fetched and environment-tagged, got %+v", summary)
	}
	if summary.Canceled != 1 {
		t.Fatalf("expected only the Invalid-status issue counted as canceled, got %+v", summary)
	}
}

func TestSyncQAPortfolioPersistsAssignmentsForRegisteredProjectsOnly(t *testing.T) {
	repo := repository.NewSQLiteForTest(t)
	now := time.Now().UTC()
	if _, err := repo.SaveProjectMapping("INIT-1", "Checkout Revamp", "CHK", "Kiki", now, now, now, now); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveMember(&repository.Member{ID: "m1", Name: "Nadia", JiraAccountID: "acc1", Active: true}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JIRA_QAS_FIELD_ID", "customfield_10099")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// INIT-1 is registered and has two QAs (one mapped to a member, one
		// not); INIT-UNREGISTERED has no matching registered project and
		// must be skipped entirely.
		_, _ = w.Write([]byte(`{"issues":[
			{"key":"INIT-1","fields":{"customfield_10099":[{"accountId":"acc1","displayName":"Nadia"},{"accountId":"acc-unmapped","displayName":"Ghost"}]}},
			{"key":"INIT-UNREGISTERED","fields":{"customfield_10099":[{"accountId":"acc1","displayName":"Nadia"}]}}
		],"isLast":true}`))
	}))
	defer srv.Close()
	w := Worker{Repo: repo, Connector: Connector{Client: srv.Client(), JiraBaseURL: srv.URL, JiraEmail: "qa@example.com", JiraToken: "token"}}
	step := &SyncStep{}
	if err := w.syncQAPortfolio(t.Context(), step); err != nil {
		t.Fatal(err)
	}

	members, err := repo.Workload(now.Add(-time.Hour), now.Add(time.Hour), "m1")
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 {
		t.Fatalf("got %d workload members, want 1", len(members))
	}
	if members[0].ActiveProjects != 1 || members[0].TotalProjects != 1 {
		t.Fatalf("unexpected project counts: %+v", members[0])
	}
	if len(members[0].Projects) != 1 || members[0].Projects[0].Key != "INIT-1" {
		t.Fatalf("unexpected projects: %+v", members[0].Projects)
	}
}

func TestSyncQAPortfolioNoopsWhenFieldIDUnset(t *testing.T) {
	repo := repository.NewSQLiteForTest(t)
	t.Setenv("JIRA_QAS_FIELD_ID", "")
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()
	w := Worker{Repo: repo, Connector: Connector{Client: srv.Client(), JiraBaseURL: srv.URL, JiraEmail: "qa@example.com", JiraToken: "token"}}
	step := &SyncStep{}
	if err := w.syncQAPortfolio(t.Context(), step); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("expected no Jira call when JIRA_QAS_FIELD_ID is unset")
	}
}

func TestProductionBugRankOrdersCriticalTierFirstThenOpenBeforeInProgressBeforeDone(t *testing.T) {
	cases := []struct {
		status, priority string
		want             int
	}{
		{"Open", "Highest", 0},
		{"Open", "Blocker", 0},
		{"In Progress", "Critical", 1},
		{"Done", "Highest", 2},
		{"Open", "High", 10},
		{"In Progress", "High", 11},
		{"Done", "High", 12},
		{"Confirm", "Medium", 23},
		{"Confirm", "Low", 23},
	}
	for _, c := range cases {
		if got := productionBugRank(c.status, c.priority); got != c.want {
			t.Errorf("productionBugRank(%q, %q) = %d, want %d", c.status, c.priority, got, c.want)
		}
	}
}

// fakeQaseServer serves the minimal set of Qase endpoints one project ("PAY")
// with one run (501) and one case (9001) touches, so syncQaseProject
// (overview) and syncQaseProjectDetail (detail) can be exercised without a
// real Qase account.
func fakeQaseServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/project/PAY", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":true,"result":{"counts":{"cases":1,"suites":1}}}`))
	})
	mux.HandleFunc("/v1/run/PAY", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":true,"result":{"total":1,"entities":[{"id":501}]}}`))
	})
	mux.HandleFunc("/v1/run/PAY/501", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":true,"result":{"id":501,"title":"[STG] Android","status":"active","cases":[9001]}}`))
	})
	mux.HandleFunc("/v1/result/PAY", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":true,"result":{"total":1,"entities":[{"id":1,"hash":"r1","run_id":501,"case_id":9001,"status":"passed","member_id":"m1"}]}}`))
	})
	mux.HandleFunc("/v1/case/PAY", func(w http.ResponseWriter, r *http.Request) {
		// ProjectTesterBreakdown ("Test cases assigned in Qase") counts by the
		// QA PIC field (id 7) — who created the scenario, not who ran it.
		_, _ = w.Write([]byte(`{"status":true,"result":{"total":1,"entities":[{"id":9001,"title":"Case A","custom_fields":[{"id":7,"value":"6"},{"id":16,"value":"6"}]}]}}`))
	})
	mux.HandleFunc("/v1/custom_field", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":true,"result":{"total":2,"entities":[{"id":7,"title":"QA PIC","entity":"test-case","value":"[{\"id\":6,\"title\":\"Melisa\"}]"},{"id":16,"title":"Tester Android","entity":"test-case","value":"[{\"id\":6,\"title\":\"Melisa\"}]"}]}}`))
	})
	mux.HandleFunc("/v1/defect/PAY", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":true,"result":{"total":1,"entities":[{"id":1,"title":"Bug 1","status":"open","runs":[],"results":["r1"]}]}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestSyncQaseProjectSplitsOverviewAndDetailTiers(t *testing.T) {
	repo := repository.NewSQLiteForTest(t)
	srv := fakeQaseServer(t)
	w := Worker{Repo: repo, Connector: Connector{Client: srv.Client(), QaseBaseURL: srv.URL, QaseToken: "secret"}}

	project, err := repo.SaveProjectMapping("INIT-4001", "Checkout", "PAY", "Nadia", time.Now(), time.Now(), time.Now(), time.Now())
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}

	overviewStep := &SyncStep{}
	if err := w.syncQaseProject(t.Context(), "PAY", overviewStep); err != nil {
		t.Fatalf("overview sync: %v", err)
	}
	runs, err := repo.ProjectRuns(project)
	if err != nil {
		t.Fatalf("project runs: %v", err)
	}
	if len(runs) != 1 || runs[0].Passed != 1 {
		t.Fatalf("expected overview tier to populate run/result data, got %+v", runs)
	}
	testerProgress, err := repo.ProjectTesterBreakdown(project)
	if err != nil {
		t.Fatalf("tester breakdown: %v", err)
	}
	if len(testerProgress) != 0 {
		t.Fatalf("expected overview tier NOT to populate case/tester data, got %+v", testerProgress)
	}
	summary, err := repo.ProjectBugSummary(project)
	if err != nil {
		t.Fatalf("bug summary: %v", err)
	}
	if summary.Total != 0 {
		t.Fatalf("expected overview tier NOT to import defects, got %+v", summary)
	}

	detailStep := &SyncStep{}
	if err := w.syncQaseProjectDetail(t.Context(), "PAY", detailStep); err != nil {
		t.Fatalf("detail sync: %v", err)
	}
	testerProgress, err = repo.ProjectTesterBreakdown(project)
	if err != nil {
		t.Fatalf("tester breakdown after detail sync: %v", err)
	}
	if len(testerProgress) == 0 {
		t.Fatalf("expected detail tier to populate case/tester data")
	}
	summary, err = repo.ProjectBugSummary(project)
	if err != nil {
		t.Fatalf("bug summary after detail sync: %v", err)
	}
	if summary.Total != 1 {
		t.Fatalf("expected detail tier to import the defect, got %+v", summary)
	}
	if summary.Staging != 1 {
		t.Fatalf("expected result-linked defect to inherit STAGING, got %+v", summary)
	}
}

func TestApplyQaseTesterFieldsResolvesAndSplitsMultiselect(t *testing.T) {
	fieldOptions := map[int64]map[int64]string{
		qaseFieldIDPic:           {6: "Melisa", 8: "Kiki"},
		qaseFieldIDTester:        {6: "Melisa"},
		qaseFieldIDTesterAndroid: {8: "Kiki"},
		qaseFieldIDTesterIos:     {9: "Nadia"},
	}
	raw := json.RawMessage(`[{"id":7,"value":"6,8"},{"id":15,"value":"6"},{"id":16,"value":"8"},{"id":17,"value":"9"},{"id":19,"value":"6"}]`)
	var row QaseCase
	applyQaseTesterFields(&row, raw, fieldOptions)
	if row.PicNames != "Melisa, Kiki" {
		t.Fatalf("PicNames = %q, want %q", row.PicNames, "Melisa, Kiki")
	}
	if row.TesterName != "Melisa" {
		t.Fatalf("TesterName = %q, want %q", row.TesterName, "Melisa")
	}
	if row.TesterAndroidName != "Kiki" {
		t.Fatalf("TesterAndroidName = %q, want %q", row.TesterAndroidName, "Kiki")
	}
	if row.TesterIosName != "Nadia" {
		t.Fatalf("TesterIosName = %q, want %q", row.TesterIosName, "Nadia")
	}
}

func TestJiraKeyFromExternalDataExtractsJiraCloudKey(t *testing.T) {
	if got := jiraKeyFromExternalData(`{"jira-cloud":{"id":"111290","key":"QASE-19216"}}`); got != "QASE-19216" {
		t.Fatalf("got %q, want QASE-19216", got)
	}
	if got := jiraKeyFromExternalData(""); got != "" {
		t.Fatalf("expected empty external_data to yield no key, got %q", got)
	}
	if got := jiraKeyFromExternalData(`{"other-tracker":{"key":"X-1"}}`); got != "" {
		t.Fatalf("expected missing jira-cloud key to yield no key, got %q", got)
	}
	if got := jiraKeyFromExternalData(`not json`); got != "" {
		t.Fatalf("expected malformed external_data to be skipped without crashing, got %q", got)
	}
}

func TestApplyQaseTesterFieldsSkipsUnresolvableValues(t *testing.T) {
	fieldOptions := map[int64]map[int64]string{qaseFieldIDTester: {6: "Melisa"}}
	var row QaseCase
	applyQaseTesterFields(&row, json.RawMessage(`[{"id":15,"value":"not-a-number"}]`), fieldOptions)
	if row.TesterName != "" {
		t.Fatalf("expected empty TesterName for unresolvable value, got %q", row.TesterName)
	}
	applyQaseTesterFields(&row, json.RawMessage(`not json`), fieldOptions)
	if row.TesterName != "" {
		t.Fatalf("expected malformed custom_fields to be skipped without crashing")
	}
	applyQaseTesterFields(&row, nil, fieldOptions)
}

// TestResultTesterSnapshotNoCrossEnvironmentFallback guards the fix where
// BETA execution rows were misattributed to STAGING-only "QA Tester"
// assignees (e.g. Rio, tagged QA Tester but never Field Beta Tester) — the
// snapshot must read TesterName for STAGING and BetaTesterName for BETA,
// with no fallback between them, and "" for a case that hasn't been synced
// by the detail-tier pass yet.
func TestResultTesterSnapshotNoCrossEnvironmentFallback(t *testing.T) {
	repo := repository.NewSQLiteForTest(t)
	if _, err := repo.UpsertCase(&repository.QaseCase{ID: "c1", ProjectCode: "P", CaseID: 1, TesterName: "Rio"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpsertCase(&repository.QaseCase{ID: "c2", ProjectCode: "P", CaseID: 2, BetaTesterName: "Hani"}); err != nil {
		t.Fatal(err)
	}
	w := Worker{Repo: repo}
	cache := map[int64]*repository.QaseCase{}

	if got := w.resultTesterSnapshot("P", 1, "STAGING", cache); got != "Rio" {
		t.Fatalf("STAGING tester = %q, want Rio", got)
	}
	if got := w.resultTesterSnapshot("P", 1, "BETA", cache); got != "" {
		t.Fatalf("Rio (QA Tester only) must not surface under BETA, got %q", got)
	}
	if got := w.resultTesterSnapshot("P", 2, "BETA", cache); got != "Hani" {
		t.Fatalf("BETA tester = %q, want Hani", got)
	}
	if got := w.resultTesterSnapshot("P", 2, "STAGING", cache); got != "" {
		t.Fatalf("Hani (Field Beta Tester only) must not surface under STAGING, got %q", got)
	}
	if got := w.resultTesterSnapshot("P", 999, "STAGING", cache); got != "" {
		t.Fatalf("unsynced case must yield empty tester, got %q", got)
	}
}
