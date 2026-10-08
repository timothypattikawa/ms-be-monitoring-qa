package monitoring

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Beyondtech-ID/ms-monitoring-qa-be/internal/repository"
	"github.com/labstack/echo/v4"
)

type recordingAlertEmailSender struct {
	recipients []string
	subject    string
	body       string
}

func (s *recordingAlertEmailSender) Send(_ context.Context, recipients []string, subject, body string) error {
	s.recipients = recipients
	s.subject = subject
	s.body = body
	return nil
}

func TestQaAlertEmailStatusAndDisabledSend(t *testing.T) {
	t.Setenv("MONITOR_MANAGER_API_KEY", "")
	api := API{Repo: repository.NewSQLiteForTest(t)}
	e := echo.New()
	api.Register(e)

	status := httptest.NewRecorder()
	e.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/v1/qa-alert-email/status", nil))
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"configured":false`) {
		t.Fatalf("unexpected status response: %d %s", status.Code, status.Body.String())
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/project-1/qa-alert-email", strings.NewReader(`{"recipients":["qa@example.com"],"body":"QA alert"}`))
	request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), "EMAIL_NOT_CONFIGURED") {
		t.Fatalf("unexpected disabled-send response: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestSendProjectQaAlertEmailValidatesAndSends(t *testing.T) {
	t.Setenv("MONITOR_MANAGER_API_KEY", "manager-key")
	repo := repository.NewSQLiteForTest(t)
	if err := repo.SaveProject(&repository.Project{ID: "project-1", JiraInitKey: "INIT-101", Name: "Revamp"}); err != nil {
		t.Fatal(err)
	}
	sender := &recordingAlertEmailSender{}
	api := API{Repo: repo, AlertEmailSender: sender}
	e := echo.New()
	api.Register(e)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/project-1/qa-alert-email", strings.NewReader(`{"recipients":[" qa@example.com ","qa@example.com","dev@example.com"],"body":"QA alert"}`))
	request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	request.Header.Set("X-Manager-Key", "manager-key")
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected send response: %d %s", recorder.Code, recorder.Body.String())
	}
	if sender.subject != "QA ALERT: INIT-101 - Revamp" || sender.body != "QA alert" || len(sender.recipients) != 2 {
		t.Fatalf("unexpected email contents: %+v", sender)
	}
	var response struct {
		Status     string   `json:"status"`
		Recipients []string `json:"recipients"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Status != "sent" || len(response.Recipients) != 2 {
		t.Fatalf("unexpected response payload: %+v", response)
	}
}

func TestSendProjectQaAlertEmailRejectsInvalidRecipient(t *testing.T) {
	t.Setenv("MONITOR_MANAGER_API_KEY", "manager-key")
	repo := repository.NewSQLiteForTest(t)
	if err := repo.SaveProject(&repository.Project{ID: "project-1", JiraInitKey: "INIT-101", Name: "Revamp"}); err != nil {
		t.Fatal(err)
	}
	sender := &recordingAlertEmailSender{}
	api := API{Repo: repo, AlertEmailSender: sender}
	e := echo.New()
	api.Register(e)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/project-1/qa-alert-email", strings.NewReader(`{"recipients":["not-an-email"],"body":"QA alert"}`))
	request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	request.Header.Set("X-Manager-Key", "manager-key")
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest || sender.body != "" {
		t.Fatalf("invalid recipient must be rejected before sending: %d %+v", recorder.Code, sender)
	}
}
