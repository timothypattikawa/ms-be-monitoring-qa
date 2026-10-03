package monitoring

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Connector struct {
	Client                                                    *http.Client
	JiraBaseURL, JiraEmail, JiraToken, QaseBaseURL, QaseToken string
}

func NewConnector() Connector {
	return Connector{Client: &http.Client{Timeout: 30 * time.Second}, JiraBaseURL: strings.TrimRight(os.Getenv("JIRA_BASE_URL"), "/"), JiraEmail: os.Getenv("JIRA_EMAIL"), JiraToken: os.Getenv("JIRA_API_TOKEN"), QaseBaseURL: strings.TrimRight(defaultString(os.Getenv("QASE_BASE_URL"), "https://api.qase.io"), "/"), QaseToken: os.Getenv("QASE_API_TOKEN")}
}
func defaultString(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

type jiraIssue struct {
	ID     string `json:"id"`
	Key    string `json:"key"`
	Fields struct {
		Summary string `json:"summary"`
		Status  struct {
			Name string `json:"name"`
		} `json:"status"`
		IssueType struct {
			Name string `json:"name"`
		} `json:"issuetype"`
		Creator struct {
			AccountID   string `json:"accountId"`
			DisplayName string `json:"displayName"`
		} `json:"creator"`
		Reporter struct {
			AccountID   string `json:"accountId"`
			DisplayName string `json:"displayName"`
		} `json:"reporter"`
		Assignee struct {
			AccountID   string `json:"accountId"`
			DisplayName string `json:"displayName"`
		} `json:"assignee"`
		Priority struct {
			Name string `json:"name"`
		} `json:"priority"`
		// TestingEnvironment is Jira's own "Testing Environment" dropdown
		// (customfield_10185, confirmed via /rest/api/3/field) — the same
		// field product's JQL filters on ("testing environment[dropdown]").
		// Authoritative source for a defect's environment; the Qase
		// run/result-linkage heuristic only covers defects Qase itself
		// managed to link to a run.
		TestingEnvironment struct {
			Value string `json:"value"`
		} `json:"customfield_10185"`
		Updated    string `json:"updated"`
		Created    string `json:"created"`
		IssueLinks []struct {
			InwardIssue *struct {
				ID string `json:"id"`
			} `json:"inwardIssue"`
			OutwardIssue *struct {
				ID string `json:"id"`
			} `json:"outwardIssue"`
		} `json:"issuelinks"`
	} `json:"fields"`
}
type jiraPage struct {
	Issues        []jiraIssue `json:"issues"`
	NextPageToken string      `json:"nextPageToken"`
	IsLast        bool        `json:"isLast"`
}

func (x Connector) JiraPages(ctx context.Context, jql string, visit func(jiraIssue) error) error {
	if x.JiraBaseURL == "" || x.JiraEmail == "" || x.JiraToken == "" || jql == "" {
		return errors.New("JIRA_CONFIG_MISSING")
	}
	pageToken := ""
	seen := map[string]bool{}
	for page := 0; page < 10000; page++ {
		body := map[string]any{"jql": jql, "fields": []string{"summary", "status", "issuetype", "updated", "created", "creator", "reporter", "assignee", "priority", "issuelinks", "customfield_10019", "customfield_10185"}, "maxResults": 100}
		if pageToken != "" {
			body["nextPageToken"] = pageToken
		}
		payload, _ := json.Marshal(body)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, x.JiraBaseURL+"/rest/api/3/search/jql", strings.NewReader(string(payload)))
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(x.JiraEmail+":"+x.JiraToken)))
		var result jiraPage
		if err := x.do(req, &result); err != nil {
			return err
		}
		for _, issue := range result.Issues {
			if issue.ID == "" {
				return errors.New("JIRA_ID_MISSING")
			}
			if err := visit(issue); err != nil {
				return err
			}
		}
		if result.IsLast {
			return nil
		}
		if result.NextPageToken == "" {
			return errors.New("JIRA_CURSOR_MISSING")
		}
		if seen[result.NextPageToken] {
			return errors.New("JIRA_CURSOR_LOOP")
		}
		seen[result.NextPageToken] = true
		pageToken = result.NextPageToken
	}
	return errors.New("JIRA_PAGE_LIMIT")
}

// qaPortfolioJQL lists every INIT issue that has a QA assigned, in any status:
// the QA portfolio is derived from it (Total = all of a QA's INITs, Active =
// those whose Jira status isn't Cancel/Done/Postponed/Backlog). The QAs
// field's JSON key isn't known statically (it's workspace-specific), so it's
// injected via JIRA_QAS_FIELD_ID.
const qaPortfolioJQL = `project = INIT AND "QAs[User Picker (multiple users)]" IS NOT EMPTY ORDER BY created DESC`

type jiraQAUser struct {
	AccountID    string `json:"accountId"`
	DisplayName  string `json:"displayName"`
	EmailAddress string `json:"emailAddress"`
}

type jiraIssueWithQAs struct {
	Key     string
	Summary string
	Status  string
	QAs     []jiraQAUser
}

// JiraInitsWithQAs lists every INIT issue that has a QA assigned and hands
// back its key, summary, Jira status and QAs custom field. It reads the field
// ID from JIRA_QAS_FIELD_ID at call time and no-ops cleanly while it's unset.
func (x Connector) JiraInitsWithQAs(ctx context.Context, visit func(jiraIssueWithQAs) error) error {
	fieldID := os.Getenv("JIRA_QAS_FIELD_ID")
	if fieldID == "" {
		return nil
	}
	if x.JiraBaseURL == "" || x.JiraEmail == "" || x.JiraToken == "" {
		return errors.New("JIRA_CONFIG_MISSING")
	}
	pageToken := ""
	seen := map[string]bool{}
	for page := 0; page < 10000; page++ {
		body := map[string]any{"jql": qaPortfolioJQL, "fields": []string{"summary", "status", fieldID}, "maxResults": 100}
		if pageToken != "" {
			body["nextPageToken"] = pageToken
		}
		payload, _ := json.Marshal(body)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, x.JiraBaseURL+"/rest/api/3/search/jql", strings.NewReader(string(payload)))
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(x.JiraEmail+":"+x.JiraToken)))
		var raw struct {
			Issues []struct {
				Key    string                     `json:"key"`
				Fields map[string]json.RawMessage `json:"fields"`
			} `json:"issues"`
			NextPageToken string `json:"nextPageToken"`
			IsLast        bool   `json:"isLast"`
		}
		if err := x.do(req, &raw); err != nil {
			return err
		}
		for _, issue := range raw.Issues {
			if issue.Key == "" {
				return errors.New("JIRA_ID_MISSING")
			}
			var qas []jiraQAUser
			if b := issue.Fields[fieldID]; len(b) > 0 {
				_ = json.Unmarshal(b, &qas)
			}
			var summary string
			_ = json.Unmarshal(issue.Fields["summary"], &summary)
			var status struct {
				Name string `json:"name"`
			}
			_ = json.Unmarshal(issue.Fields["status"], &status)
			if err := visit(jiraIssueWithQAs{Key: issue.Key, Summary: summary, Status: status.Name, QAs: qas}); err != nil {
				return err
			}
		}
		if raw.IsLast {
			return nil
		}
		if raw.NextPageToken == "" {
			return errors.New("JIRA_CURSOR_MISSING")
		}
		if seen[raw.NextPageToken] {
			return errors.New("JIRA_CURSOR_LOOP")
		}
		seen[raw.NextPageToken] = true
		pageToken = raw.NextPageToken
	}
	return errors.New("JIRA_PAGE_LIMIT")
}

func (x Connector) JiraIssue(ctx context.Context, key string) (jiraIssue, error) {
	if x.JiraBaseURL == "" || x.JiraEmail == "" || x.JiraToken == "" || key == "" {
		return jiraIssue{}, errors.New("JIRA_CONFIG_MISSING")
	}
	endpoint := fmt.Sprintf("%s/rest/api/3/issue/%s?fields=summary,status,issuetype,assignee,reporter,creator,priority,created,customfield_10185", x.JiraBaseURL, url.PathEscape(key))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return jiraIssue{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(x.JiraEmail+":"+x.JiraToken)))
	var issue jiraIssue
	if err := x.do(req, &issue); err != nil {
		// x.do maps 404 into the same generic UPSTREAM_BAD_RESPONSE as other
		// non-2xx statuses; a plain single-issue GET has no other 4xx cause
		// worth distinguishing here, so treat any failure as "not found".
		return jiraIssue{}, errors.New("JIRA_ISSUE_NOT_FOUND")
	}
	if issue.ID == "" {
		return jiraIssue{}, errors.New("JIRA_ISSUE_NOT_FOUND")
	}
	return issue, nil
}

func (x Connector) QaseProject(ctx context.Context, code string) (json.RawMessage, error) {
	if x.QaseToken == "" || code == "" {
		return nil, errors.New("QASE_CONFIG_MISSING")
	}
	endpoint := fmt.Sprintf("%s/v1/project/%s", x.QaseBaseURL, url.PathEscape(code))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Token", x.QaseToken)
	var env qaseEnvelope
	if err := x.do(req, &env); err != nil {
		return nil, errors.New("QASE_PROJECT_NOT_FOUND")
	}
	if !env.Status || len(env.Result) == 0 || strings.TrimSpace(string(env.Result)) == "null" {
		return nil, errors.New("QASE_PROJECT_NOT_FOUND")
	}
	return env.Result, nil
}

type qaseEnvelope struct {
	Status bool            `json:"status"`
	Result json.RawMessage `json:"result"`
}
type qaseList struct {
	Total    int               `json:"total"`
	Entities []json.RawMessage `json:"entities"`
}

func (x Connector) QasePages(ctx context.Context, resource, projectCode string, visit func(json.RawMessage) error) error {
	return x.QasePagesForRun(ctx, resource, projectCode, 0, visit)
}

func (x Connector) QasePagesForRun(ctx context.Context, resource, projectCode string, runID int64, visit func(json.RawMessage) error) error {
	if x.QaseToken == "" || projectCode == "" {
		return errors.New("QASE_CONFIG_MISSING")
	}
	for offset := 0; offset <= 100000; offset += 100 {
		endpoint := fmt.Sprintf("%s/v1/%s/%s?limit=100&offset=%d", x.QaseBaseURL, resource, url.PathEscape(projectCode), offset)
		if resource == "result" && runID > 0 {
			endpoint += "&run=" + strconv.FormatInt(runID, 10)
		}
		var list qaseList
		for attempt := 0; ; attempt++ {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
			if err != nil {
				return err
			}
			req.Header.Set("Accept", "application/json")
			req.Header.Set("Token", x.QaseToken)
			var env qaseEnvelope
			if err := x.do(req, &env); err != nil {
				return err
			}
			if !env.Status {
				return errors.New("QASE_REJECTED")
			}
			if err := json.Unmarshal(env.Result, &list); err != nil {
				return errors.New("QASE_RESPONSE_INVALID")
			}
			// Qase's reported total can lag or over-count filtered items, making a
			// genuine last page look short. Retry a few times for transient lag,
			// then accept the page as final rather than failing the whole sync.
			if list.Total > offset+len(list.Entities) && len(list.Entities) < 100 && attempt < 3 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(time.Duration(attempt+1) * 300 * time.Millisecond):
				}
				continue
			}
			break
		}
		for _, item := range list.Entities {
			if err := visit(item); err != nil {
				return err
			}
		}
		if len(list.Entities) < 100 || (list.Total > 0 && offset+len(list.Entities) >= list.Total) {
			return nil
		}
	}
	return errors.New("QASE_OFFSET_LIMIT")
}

type qaseFieldOption struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

// qaseFieldOptions unwraps a quirk in Qase's own API: for selectbox/multiselect
// custom fields, "value" comes back as a JSON string containing the array
// (double-encoded) instead of a native array. Unmarshaling that directly into
// []qaseFieldOption fails silently upstream, which is why tester-name
// resolution was always empty — this handles both shapes.
type qaseFieldOptions []qaseFieldOption

func (o *qaseFieldOptions) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		*o = nil
		return nil
	}
	if strings.HasPrefix(trimmed, `"`) {
		var inner string
		if err := json.Unmarshal(data, &inner); err != nil {
			return err
		}
		data = []byte(inner)
	}
	var opts []qaseFieldOption
	if err := json.Unmarshal(data, &opts); err != nil {
		return err
	}
	*o = opts
	return nil
}

type qaseCustomField struct {
	ID     int64            `json:"id"`
	Title  string           `json:"title"`
	Entity string           `json:"entity"`
	Value  qaseFieldOptions `json:"value"`
}

func (x Connector) QaseCustomFields(ctx context.Context) ([]qaseCustomField, error) {
	if x.QaseToken == "" {
		return nil, errors.New("QASE_CONFIG_MISSING")
	}
	var out []qaseCustomField
	for offset := 0; offset <= 100000; offset += 100 {
		endpoint := fmt.Sprintf("%s/v1/custom_field?limit=100&offset=%d", x.QaseBaseURL, offset)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Token", x.QaseToken)
		var env qaseEnvelope
		if err := x.do(req, &env); err != nil {
			return nil, err
		}
		if !env.Status {
			return nil, errors.New("QASE_REJECTED")
		}
		var list struct {
			Total    int               `json:"total"`
			Entities []qaseCustomField `json:"entities"`
		}
		if err := json.Unmarshal(env.Result, &list); err != nil {
			return nil, errors.New("QASE_RESPONSE_INVALID")
		}
		out = append(out, list.Entities...)
		if len(list.Entities) < 100 || (list.Total > 0 && offset+len(list.Entities) >= list.Total) {
			return out, nil
		}
	}
	return nil, errors.New("QASE_OFFSET_LIMIT")
}

func (x Connector) JiraUserByEmail(ctx context.Context, email string) (string, error) {
	if x.JiraBaseURL == "" || x.JiraEmail == "" || x.JiraToken == "" || email == "" {
		return "", errors.New("JIRA_CONFIG_MISSING")
	}
	endpoint := fmt.Sprintf("%s/rest/api/3/user/search?query=%s", x.JiraBaseURL, url.QueryEscape(email))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(x.JiraEmail+":"+x.JiraToken)))
	var users []struct {
		AccountID string `json:"accountId"`
	}
	if err := x.do(req, &users); err != nil {
		return "", err
	}
	if len(users) == 0 {
		return "", errors.New("JIRA_USER_NOT_FOUND")
	}
	return users[0].AccountID, nil
}

func (x Connector) QaseRun(ctx context.Context, projectCode string, runID int64) (json.RawMessage, error) {
	if x.QaseToken == "" || x.QaseBaseURL == "" || projectCode == "" || runID <= 0 {
		return nil, errors.New("QASE_CONFIG_MISSING")
	}
	endpoint := fmt.Sprintf("%s/v1/run/%s/%d?include=cases", x.QaseBaseURL, url.PathEscape(projectCode), runID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Token", x.QaseToken)
	var env qaseEnvelope
	if err := x.do(req, &env); err != nil {
		return nil, err
	}
	if !env.Status {
		return nil, errors.New("QASE_REJECTED")
	}
	if len(env.Result) == 0 || strings.TrimSpace(string(env.Result)) == "null" {
		return nil, errors.New("QASE_RESPONSE_INVALID")
	}
	return env.Result, nil
}

// QaseRunIDs lists every run for a project, in-progress or finished.
// ponytail: Qase's own "status=active" filter means "in progress", which
// drops runs that already finished (e.g. a completed [STG] cycle) — we need
// those too since project progress is summed across all tracked runs, not
// just the ones still running.
func (x Connector) QaseRunIDs(ctx context.Context, projectCode string) ([]int64, error) {
	if x.QaseToken == "" || projectCode == "" {
		return nil, errors.New("QASE_CONFIG_MISSING")
	}
	var ids []int64
	for offset := 0; offset <= 100000; offset += 100 {
		endpoint := fmt.Sprintf("%s/v1/run/%s?limit=100&offset=%d", x.QaseBaseURL, url.PathEscape(projectCode), offset)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Token", x.QaseToken)
		var env qaseEnvelope
		if err := x.do(req, &env); err != nil {
			return nil, err
		}
		if !env.Status {
			return nil, errors.New("QASE_REJECTED")
		}
		var list qaseList
		if err := json.Unmarshal(env.Result, &list); err != nil {
			return nil, errors.New("QASE_RESPONSE_INVALID")
		}
		for _, item := range list.Entities {
			var run struct {
				ID int64 `json:"id"`
			}
			if err := json.Unmarshal(item, &run); err != nil {
				return nil, errors.New("QASE_RESPONSE_INVALID")
			}
			ids = append(ids, run.ID)
		}
		if len(list.Entities) < 100 || (list.Total > 0 && offset+len(list.Entities) >= list.Total) {
			return ids, nil
		}
	}
	return nil, errors.New("QASE_OFFSET_LIMIT")
}

type qaseDefect struct {
	ID           int64    `json:"id"`
	Title        string   `json:"title"`
	Status       string   `json:"status"`
	Severity     string   `json:"severity"`
	ExternalData string   `json:"external_data"`
	Runs         []int64  `json:"runs"`
	Results      []string `json:"results"`
	// ponytail: Qase's own docs aren't consistent on the field name for a
	// defect's creation timestamp — accept either, worker.go tries Created
	// first then CreatedAt.
	Created   string `json:"created"`
	CreatedAt string `json:"created_at"`
}

func (x Connector) QaseDefects(ctx context.Context, projectCode string) ([]qaseDefect, error) {
	if x.QaseToken == "" || projectCode == "" {
		return nil, errors.New("QASE_CONFIG_MISSING")
	}
	var out []qaseDefect
	for offset := 0; offset <= 100000; offset += 100 {
		endpoint := fmt.Sprintf("%s/v1/defect/%s?limit=100&offset=%d", x.QaseBaseURL, url.PathEscape(projectCode), offset)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Token", x.QaseToken)
		var env qaseEnvelope
		if err := x.do(req, &env); err != nil {
			return nil, err
		}
		if !env.Status {
			return nil, errors.New("QASE_REJECTED")
		}
		var list struct {
			Total    int          `json:"total"`
			Entities []qaseDefect `json:"entities"`
		}
		if err := json.Unmarshal(env.Result, &list); err != nil {
			return nil, errors.New("QASE_RESPONSE_INVALID")
		}
		out = append(out, list.Entities...)
		if len(list.Entities) < 100 || (list.Total > 0 && offset+len(list.Entities) >= list.Total) {
			return out, nil
		}
	}
	return nil, errors.New("QASE_OFFSET_LIMIT")
}

func (x Connector) QaseRunCases(ctx context.Context, projectCode string, runID int64) ([]int64, error) {
	raw, err := x.QaseRun(ctx, projectCode, runID)
	if err != nil {
		return nil, err
	}
	var result struct {
		Cases json.RawMessage `json:"cases"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, errors.New("QASE_RESPONSE_INVALID")
	}
	return qaseCaseIDs(result.Cases)
}
func qaseCaseIDs(raw json.RawMessage) ([]int64, error) {
	var ids []int64
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return nil, errors.New("QASE_RUN_CASES_MISSING")
	}
	if json.Unmarshal(raw, &ids) == nil {
		for _, id := range ids {
			if id <= 0 {
				return nil, errors.New("QASE_RUN_CASES_INVALID")
			}
		}
		return ids, nil
	}
	ids = nil
	var cases []struct {
		ID int64 `json:"id"`
	}
	if json.Unmarshal(raw, &cases) != nil {
		return nil, errors.New("QASE_RUN_CASES_INVALID")
	}
	for _, c := range cases {
		if c.ID <= 0 {
			return nil, errors.New("QASE_RUN_CASES_INVALID")
		}
		ids = append(ids, c.ID)
	}
	return ids, nil
}
func (x Connector) do(req *http.Request, dest any) error {
	client := x.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 && req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return errors.New("UPSTREAM_REQUEST_INVALID")
			}
			req.Body = body
		}
		resp, err := client.Do(req)
		if err != nil {
			if attempt == 3 {
				return errors.New("UPSTREAM_UNAVAILABLE")
			}
			if err := pause(req.Context(), time.Duration(1<<attempt)*time.Second); err != nil {
				return err
			}
			continue
		}
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			wait := time.Duration(1<<attempt) * time.Second
			if v := resp.Header.Get("Retry-After"); v != "" {
				if n, e := strconv.Atoi(v); e == nil && n >= 0 && n <= 60 {
					wait = time.Duration(n) * time.Second
				}
			}
			io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			if attempt == 3 {
				return errors.New("UPSTREAM_RETRY_EXHAUSTED")
			}
			if err := pause(req.Context(), wait); err != nil {
				return err
			}
			continue
		}
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			resp.Body.Close()
			return errors.New("UPSTREAM_ACCESS_DENIED")
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			resp.Body.Close()
			return errors.New("UPSTREAM_BAD_RESPONSE")
		}
		defer resp.Body.Close()
		if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(dest); err != nil {
			return errors.New("UPSTREAM_JSON_INVALID")
		}
		return nil
	}
	return errors.New("UPSTREAM_RETRY_EXHAUSTED")
}
func pause(ctx context.Context, d time.Duration) error {
	d += time.Duration(rand.Intn(250)) * time.Millisecond
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
