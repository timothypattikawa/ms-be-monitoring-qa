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
	Client                                                                         *http.Client
	JiraBaseURL, JiraEmail, JiraToken, JiraJQL, JiraBugJQL, QaseBaseURL, QaseToken string
}

func NewConnector() Connector {
	return Connector{Client: &http.Client{Timeout: 30 * time.Second}, JiraBaseURL: strings.TrimRight(os.Getenv("JIRA_BASE_URL"), "/"), JiraEmail: os.Getenv("JIRA_EMAIL"), JiraToken: os.Getenv("JIRA_API_TOKEN"), JiraJQL: os.Getenv("JIRA_ACTIVE_JQL"), JiraBugJQL: os.Getenv("JIRA_BUG_JQL"), QaseBaseURL: strings.TrimRight(defaultString(os.Getenv("QASE_BASE_URL"), "https://api.qase.io"), "/"), QaseToken: os.Getenv("QASE_API_TOKEN")}
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
			AccountID string `json:"accountId"`
		} `json:"creator"`
		Reporter struct {
			AccountID string `json:"accountId"`
		} `json:"reporter"`
		Assignee struct {
			AccountID string `json:"accountId"`
		} `json:"assignee"`
		Updated    string `json:"updated"`
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
		body := map[string]any{"jql": jql, "fields": []string{"summary", "status", "issuetype", "updated", "creator", "reporter", "assignee", "issuelinks"}, "maxResults": 100}
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

func (x Connector) JiraIssue(ctx context.Context, key string) (jiraIssue, error) {
	if x.JiraBaseURL == "" || x.JiraEmail == "" || x.JiraToken == "" || key == "" {
		return jiraIssue{}, errors.New("JIRA_CONFIG_MISSING")
	}
	endpoint := fmt.Sprintf("%s/rest/api/3/issue/%s?fields=summary,status,issuetype", x.JiraBaseURL, url.PathEscape(key))
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
		var list qaseList
		if err := json.Unmarshal(env.Result, &list); err != nil {
			return errors.New("QASE_RESPONSE_INVALID")
		}
		for _, item := range list.Entities {
			if err := visit(item); err != nil {
				return err
			}
		}
		if list.Total > offset+len(list.Entities) && len(list.Entities) < 100 {
			return errors.New("QASE_PAGE_INCOMPLETE")
		}
		if len(list.Entities) < 100 || (list.Total > 0 && offset+len(list.Entities) >= list.Total) {
			return nil
		}
	}
	return errors.New("QASE_OFFSET_LIMIT")
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
