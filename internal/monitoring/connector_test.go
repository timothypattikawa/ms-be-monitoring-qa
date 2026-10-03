package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestQaseRunCaseParsingRejectsMissingOrMalformedMembership(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want []int64
		err  string
	}{
		{"ids", `[1,2]`, []int64{1, 2}, ""},
		{"objects", `[{"id":3},{"id":4}]`, []int64{3, 4}, ""},
		{"empty", `[]`, nil, ""},
		{"missing", ``, nil, "QASE_RUN_CASES_MISSING"},
		{"null", `null`, nil, "QASE_RUN_CASES_MISSING"},
		{"count", `2`, nil, "QASE_RUN_CASES_INVALID"},
		{"invalid id", `[{"id":0}]`, nil, "QASE_RUN_CASES_INVALID"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := qaseCaseIDs(json.RawMessage(tc.raw))
			if tc.err == "" && err != nil || tc.err != "" && (err == nil || err.Error() != tc.err) {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestQasePagesToleratesTotalMismatch(t *testing.T) {
	visited := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": true, "result": map[string]any{"total": 500, "entities": []any{map[string]int{"id": 1}}}})
	}))
	defer srv.Close()
	x := Connector{Client: srv.Client(), QaseBaseURL: srv.URL, QaseToken: "secret"}
	err := x.QasePages(context.Background(), "run", "INIT", func(json.RawMessage) error { visited++; return nil })
	if err != nil {
		t.Fatalf("expected the persistently short page to be accepted as final, got %v", err)
	}
	if visited != 1 {
		t.Fatalf("expected the single returned entity to still be visited, got %d", visited)
	}
}

func TestJiraPagesRejectsMissingCursor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"issues": []any{}, "isLast": false})
	}))
	defer srv.Close()
	x := Connector{Client: srv.Client(), JiraBaseURL: srv.URL, JiraEmail: "qa@example.test", JiraToken: "secret"}
	err := x.JiraPages(context.Background(), "project = INIT", func(jiraIssue) error { return nil })
	if err == nil || err.Error() != "JIRA_CURSOR_MISSING" {
		t.Fatalf("expected missing cursor error, got %v", err)
	}
}

func TestQasePagesLoadsEveryPage(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Token") != "secret" {
			t.Errorf("missing Qase token")
		}
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		n := 100
		if offset == 200 {
			n = 5
		}
		entities := make([]map[string]int, n)
		for i := range entities {
			entities[i] = map[string]int{"id": offset + i + 1}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": true, "result": map[string]any{"total": 205, "entities": entities}})
	}))
	defer srv.Close()
	x := Connector{Client: srv.Client(), QaseBaseURL: srv.URL, QaseToken: "secret"}
	count := 0
	if err := x.QasePages(context.Background(), "run", "INIT", func(raw json.RawMessage) error { count++; return nil }); err != nil {
		t.Fatal(err)
	}
	if count != 205 || requests != 3 {
		t.Fatalf("got %d records in %d requests", count, requests)
	}
}

func TestQaseResultPagesAreScopedToRegisteredRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("run"); got != "77" {
			t.Errorf("run filter = %q, want 77", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": true, "result": map[string]any{"total": 0, "entities": []any{}}})
	}))
	defer srv.Close()
	x := Connector{Client: srv.Client(), QaseBaseURL: srv.URL, QaseToken: "secret"}
	if err := x.QasePagesForRun(context.Background(), "result", "INIT", 77, func(json.RawMessage) error { return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestQaseRunFetchesSelectedRunDirectly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/run/INIT/77" || r.URL.Query().Get("include") != "cases" {
			t.Errorf("unexpected selected-run request: %s", r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": true, "result": map[string]any{"id": 77, "cases": []int{1, 2}}})
	}))
	defer srv.Close()
	x := Connector{Client: srv.Client(), QaseBaseURL: srv.URL, QaseToken: "secret"}
	raw, err := x.QaseRun(context.Background(), "INIT", 77)
	if err != nil {
		t.Fatal(err)
	}
	var run struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(raw, &run); err != nil || run.ID != 77 {
		t.Fatalf("unexpected selected run: id=%d err=%v", run.ID, err)
	}
}

func TestJiraNextPageToken(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		u, p, ok := r.BasicAuth()
		if !ok || u != "qa@example.test" || p != "secret" {
			t.Error("missing Jira basic auth")
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		if req["jql"] != "project = INIT" {
			t.Error("wrong JQL")
		}
		fields, _ := req["fields"].([]any)
		hasPriority := false
		for _, field := range fields {
			hasPriority = hasPriority || field == "priority"
		}
		if !hasPriority {
			t.Errorf("Jira search fields must include priority: %v", fields)
		}
		if requests == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{"issues": []any{map[string]any{"id": "1", "key": "INIT-1"}}, "nextPageToken": "next", "isLast": false})
			return
		}
		if req["nextPageToken"] != "next" {
			t.Error("missing nextPageToken")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"issues": []any{map[string]any{"id": "2", "key": "INIT-2"}}, "isLast": true})
	}))
	defer srv.Close()
	x := Connector{Client: srv.Client(), JiraBaseURL: srv.URL, JiraEmail: "qa@example.test", JiraToken: "secret"}
	count := 0
	if err := x.JiraPages(context.Background(), "project = INIT", func(issue jiraIssue) error {
		count++
		if issue.ID != fmt.Sprint(count) {
			t.Error("wrong issue")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if count != 2 || requests != 2 {
		t.Fatalf("got %d records in %d requests", count, requests)
	}
}

func TestRunPlatformKeepsEnvironmentSeparate(t *testing.T) {
	for title, want := range map[string]string{
		"[STG] AOS": "AOS", "[STG] IOS": "IOS", "[STG] BO": "BO", "[STG] DB": "DB", "[STG] APO": "APO",
		"Regression": "unknown",
		// real ILTA run titles: human names, and a [BETA] tag the old
		// literal-"[STG] "-prefix check never recognized at all.
		"[BETA] APO Main": "APO", "[BETA] APO Mitra": "APO", "[BETA] Android": "AOS", "[BETA] IOS": "IOS",
		"[STG] Android": "AOS", "[STG] Webcom": "unknown",
	} {
		if got := runPlatform(title); got != want {
			t.Errorf("%s: got %s, want %s", title, got, want)
		}
	}
}

func TestJiraIssueFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/issue/INIT-2401" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"10042","key":"INIT-2401","fields":{"summary":"Checkout revamp","status":{"name":"In Progress"},"issuetype":{"name":"Initiative"}}}`))
	}))
	defer srv.Close()
	x := Connector{Client: srv.Client(), JiraBaseURL: srv.URL, JiraEmail: "qa@example.com", JiraToken: "token"}
	issue, err := x.JiraIssue(context.Background(), "INIT-2401")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issue.ID != "10042" || issue.Key != "INIT-2401" {
		t.Fatalf("unexpected issue: %+v", issue)
	}
}

func TestJiraIssueNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	x := Connector{Client: srv.Client(), JiraBaseURL: srv.URL, JiraEmail: "qa@example.com", JiraToken: "token"}
	if _, err := x.JiraIssue(context.Background(), "INIT-9999"); err == nil {
		t.Fatal("expected an error for a missing issue")
	}
}

func TestJiraIssueConfigMissing(t *testing.T) {
	x := Connector{}
	if _, err := x.JiraIssue(context.Background(), "INIT-2401"); err == nil || err.Error() != "JIRA_CONFIG_MISSING" {
		t.Fatalf("expected JIRA_CONFIG_MISSING, got %v", err)
	}
}

func TestQaseProjectFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/project/PAY" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":true,"result":{"code":"PAY","title":"Payments"}}`))
	}))
	defer srv.Close()
	x := Connector{Client: srv.Client(), QaseBaseURL: srv.URL, QaseToken: "token"}
	result, err := x.QaseProject(context.Background(), "PAY")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) == 0 {
		t.Fatal("expected a non-empty result payload")
	}
}

func TestQaseCustomFieldsResolvesOptions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/custom_field" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Token") != "secret" {
			t.Errorf("missing Qase token")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": true, "result": map[string]any{"total": 1, "entities": []any{
			map[string]any{"id": 7, "title": "QA PIC", "type": "Multiselect", "entity": "case", "value": []any{map[string]any{"id": 6, "title": "Melisa"}}},
		}}})
	}))
	defer srv.Close()
	x := Connector{Client: srv.Client(), QaseBaseURL: srv.URL, QaseToken: "secret"}
	fields, err := x.QaseCustomFields(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fields) != 1 || fields[0].ID != 7 || fields[0].Title != "QA PIC" || len(fields[0].Value) != 1 || fields[0].Value[0].Title != "Melisa" {
		t.Fatalf("unexpected fields: %+v", fields)
	}
}

func TestQaseCustomFieldsConfigMissing(t *testing.T) {
	x := Connector{}
	if _, err := x.QaseCustomFields(context.Background()); err == nil || err.Error() != "QASE_CONFIG_MISSING" {
		t.Fatalf("expected QASE_CONFIG_MISSING, got %v", err)
	}
}

func TestJiraUserByEmailFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/user/search" || r.URL.Query().Get("query") != "kiki.manurung@gli.id" {
			t.Fatalf("unexpected request: %s", r.URL.String())
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{{"accountId": "60e3cc8fc0db53006aa59eaf", "emailAddress": "kiki.manurung@gli.id"}})
	}))
	defer srv.Close()
	x := Connector{Client: srv.Client(), JiraBaseURL: srv.URL, JiraEmail: "qa@example.com", JiraToken: "token"}
	id, err := x.JiraUserByEmail(context.Background(), "kiki.manurung@gli.id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "60e3cc8fc0db53006aa59eaf" {
		t.Fatalf("got %q, want accountId", id)
	}
}

func TestJiraUserByEmailNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{})
	}))
	defer srv.Close()
	x := Connector{Client: srv.Client(), JiraBaseURL: srv.URL, JiraEmail: "qa@example.com", JiraToken: "token"}
	if _, err := x.JiraUserByEmail(context.Background(), "nobody@example.com"); err == nil || err.Error() != "JIRA_USER_NOT_FOUND" {
		t.Fatalf("expected JIRA_USER_NOT_FOUND, got %v", err)
	}
}

func TestQaseRunIDsIncludesFinishedRuns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/run/ILTA" || r.URL.Query().Get("status") != "" {
			t.Fatalf("unexpected request: %s", r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": true, "result": map[string]any{"total": 9, "entities": []any{
			map[string]any{"id": 1, "title": "[STG] IOS", "status": 3},
			map[string]any{"id": 2, "title": "[STG] Android", "status": 3},
			map[string]any{"id": 3, "title": "[STG] APO Mitra", "status": 1},
			map[string]any{"id": 4, "title": "[STG] APO Main", "status": 1},
			map[string]any{"id": 5, "title": "[STG] Webcom", "status": 1},
			map[string]any{"id": 8, "title": "[BETA] APO Main", "status": 0},
			map[string]any{"id": 9, "title": "[BETA] APO Mitra", "status": 0},
			map[string]any{"id": 10, "title": "[BETA] Android", "status": 0},
			map[string]any{"id": 11, "title": "[BETA] IOS", "status": 0},
		}}})
	}))
	defer srv.Close()
	x := Connector{Client: srv.Client(), QaseBaseURL: srv.URL, QaseToken: "secret"}
	ids, err := x.QaseRunIDs(context.Background(), "ILTA")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []int64{1, 2, 3, 4, 5, 8, 9, 10, 11}
	if len(ids) != len(want) {
		t.Fatalf("got %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("got %v, want %v", ids, want)
		}
	}
}

func TestQaseRunIDsConfigMissing(t *testing.T) {
	x := Connector{}
	if _, err := x.QaseRunIDs(context.Background(), "ILTA"); err == nil || err.Error() != "QASE_CONFIG_MISSING" {
		t.Fatalf("expected QASE_CONFIG_MISSING, got %v", err)
	}
}

func TestQaseDefectsLoadsEveryPage(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/v1/defect/PAY" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		n := 100
		if offset == 100 {
			n = 1
		}
		entities := make([]map[string]any, n)
		for i := range entities {
			id := offset + i + 1
			entities[i] = map[string]any{"id": id, "title": fmt.Sprintf("Bug %d", id), "status": "open", "severity": "major", "external_data": `{"jira-cloud":{"key":"QASE-1"}}`, "results": []string{"result-1"}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": true, "result": map[string]any{"total": 101, "entities": entities}})
	}))
	defer srv.Close()
	x := Connector{Client: srv.Client(), QaseBaseURL: srv.URL, QaseToken: "secret"}
	defects, err := x.QaseDefects(context.Background(), "PAY")
	if err != nil {
		t.Fatal(err)
	}
	if len(defects) != 101 || requests != 2 {
		t.Fatalf("got %d defects in %d requests", len(defects), requests)
	}
	if defects[0].Title != "Bug 1" || defects[0].Status != "open" {
		t.Fatalf("unexpected defect: %+v", defects[0])
	}
	if len(defects[0].Results) != 1 || defects[0].Results[0] != "result-1" {
		t.Fatalf("expected defect result relation, got %+v", defects[0].Results)
	}
}

func TestQaseDefectsConfigMissing(t *testing.T) {
	x := Connector{}
	if _, err := x.QaseDefects(context.Background(), "PAY"); err == nil || err.Error() != "QASE_CONFIG_MISSING" {
		t.Fatalf("expected QASE_CONFIG_MISSING, got %v", err)
	}
}

func TestJiraIssueIncludesEnrichmentFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("fields") != "summary,status,issuetype,assignee,reporter,creator,priority,created,customfield_10185" {
			t.Fatalf("unexpected fields param: %s", r.URL.Query().Get("fields"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"10042","key":"QASE-19216","fields":{"summary":"Checkout bug","status":{"name":"Open"},"issuetype":{"name":"Bug"},"assignee":{"accountId":"a1","displayName":"Nadia"},"reporter":{"accountId":"r1","displayName":"Kiki"},"creator":{"accountId":"c1","displayName":"Dea"},"priority":{"name":"Medium"},"created":"2024-01-15T10:30:00.000+0700"}}`))
	}))
	defer srv.Close()
	x := Connector{Client: srv.Client(), JiraBaseURL: srv.URL, JiraEmail: "qa@example.com", JiraToken: "token"}
	issue, err := x.JiraIssue(context.Background(), "QASE-19216")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issue.Fields.Assignee.DisplayName != "Nadia" || issue.Fields.Reporter.DisplayName != "Kiki" || issue.Fields.Creator.DisplayName != "Dea" || issue.Fields.Priority.Name != "Medium" || issue.Fields.Created != "2024-01-15T10:30:00.000+0700" {
		t.Fatalf("unexpected enrichment fields: %+v", issue.Fields)
	}
}

func TestJiraInitsWithQAs(t *testing.T) {
	for _, tc := range []struct {
		name       string
		fieldID    string
		wantCalled bool
		wantIssues []jiraIssueWithQAs
	}{
		{
			name:       "unset field id no-ops without calling Jira",
			fieldID:    "",
			wantCalled: false,
		},
		{
			name:       "parses key, summary, status and QAs array from configured field",
			fieldID:    "customfield_10099",
			wantCalled: true,
			wantIssues: []jiraIssueWithQAs{
				{Key: "INIT-1", Summary: "Checkout", Status: "QA", QAs: []jiraQAUser{{AccountID: "acc1", DisplayName: "Nadia", EmailAddress: "nadia@example.com"}, {AccountID: "acc2", DisplayName: "Kiki"}}},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("JIRA_QAS_FIELD_ID", tc.fieldID)
			called := false
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
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
				if request.JQL != qaPortfolioJQL {
					t.Fatalf("JQL = %q, want %q", request.JQL, qaPortfolioJQL)
				}
				_, _ = w.Write([]byte(`{"issues":[{"key":"INIT-1","fields":{"summary":"Checkout","status":{"name":"QA"},"customfield_10099":[{"accountId":"acc1","displayName":"Nadia","emailAddress":"nadia@example.com"},{"accountId":"acc2","displayName":"Kiki"}]}}],"isLast":true}`))
			}))
			defer srv.Close()
			x := Connector{Client: srv.Client(), JiraBaseURL: srv.URL, JiraEmail: "qa@example.com", JiraToken: "token"}
			var got []jiraIssueWithQAs
			err := x.JiraInitsWithQAs(context.Background(), func(issue jiraIssueWithQAs) error {
				got = append(got, issue)
				return nil
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if called != tc.wantCalled {
				t.Fatalf("called = %v, want %v", called, tc.wantCalled)
			}
			if len(got) != len(tc.wantIssues) {
				t.Fatalf("got %+v, want %+v", got, tc.wantIssues)
			}
			for i := range got {
				if got[i].Key != tc.wantIssues[i].Key || got[i].Summary != tc.wantIssues[i].Summary || got[i].Status != tc.wantIssues[i].Status || len(got[i].QAs) != len(tc.wantIssues[i].QAs) {
					t.Fatalf("got %+v, want %+v", got[i], tc.wantIssues[i])
				}
				for j := range got[i].QAs {
					if got[i].QAs[j] != tc.wantIssues[i].QAs[j] {
						t.Fatalf("got %+v, want %+v", got[i].QAs[j], tc.wantIssues[i].QAs[j])
					}
				}
			}
		})
	}
}

func TestQaseProjectNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":false,"errorMessage":"Project not found"}`))
	}))
	defer srv.Close()
	x := Connector{Client: srv.Client(), QaseBaseURL: srv.URL, QaseToken: "token"}
	if _, err := x.QaseProject(context.Background(), "MISSING"); err == nil || err.Error() != "QASE_PROJECT_NOT_FOUND" {
		t.Fatalf("expected QASE_PROJECT_NOT_FOUND, got %v", err)
	}
}
