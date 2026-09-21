package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestQasePagesRejectsIncompletePage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": true, "result": map[string]any{"total": 500, "entities": []any{map[string]int{"id": 1}}}})
	}))
	defer srv.Close()
	x := Connector{Client: srv.Client(), QaseBaseURL: srv.URL, QaseToken: "secret"}
	err := x.QasePages(context.Background(), "run", "INIT", func(json.RawMessage) error { return nil })
	if err == nil || err.Error() != "QASE_PAGE_INCOMPLETE" {
		t.Fatalf("expected incomplete page error, got %v", err)
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
	for title, want := range map[string]string{"[STG] AOS": "AOS", "[STG] IOS": "IOS", "[STG] BO": "BO", "[STG] DB": "DB", "[STG] APO": "APO", "Regression": "unknown"} {
		if got := runPlatform(title); got != want {
			t.Errorf("%s: got %s, want %s", title, got, want)
		}
	}
}
