package monitoring

import (
	"context"
	"net/http"
	"net/mail"
	"os"
	"strings"

	"github.com/Beyondtech-ID/ms-monitoring-qa-be/internal/repository"
	"github.com/labstack/echo/v4"
)

type AlertEmailSender interface {
	Send(context.Context, []string, string, string) error
}

type projectQaAlertEmailRequest struct {
	Recipients []string `json:"recipients"`
	Body       string   `json:"body"`
}

func (a API) qaAlertEmailStatus(c echo.Context) error {
	configured := a.AlertEmailSender != nil && strings.TrimSpace(os.Getenv("MONITOR_MANAGER_API_KEY")) != ""
	return c.JSON(http.StatusOK, map[string]bool{"configured": configured})
}

func (a API) sendProjectQaAlertEmail(c echo.Context) error {
	if a.AlertEmailSender == nil || strings.TrimSpace(os.Getenv("MONITOR_MANAGER_API_KEY")) == "" {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"code": "EMAIL_NOT_CONFIGURED", "message": "Email delivery is disabled until the approved provider and manager authorization are configured"})
	}
	var req projectQaAlertEmailRequest
	if err := c.Bind(&req); err != nil {
		return invalidQaAlertEmail(c)
	}
	recipients, err := validateAlertEmailRecipients(req.Recipients)
	if err != nil || strings.TrimSpace(req.Body) == "" || len(req.Body) > 20000 {
		return invalidQaAlertEmail(c)
	}
	project, err := a.Repo.Project(c.Param("id"))
	if err != nil {
		if err == repository.ErrNotFound {
			return c.JSON(http.StatusNotFound, map[string]string{"code": "NOT_FOUND", "message": "project not found"})
		}
		return safeError(c, err)
	}
	subject := "QA ALERT: " + project.JiraInitKey + " - " + project.Name
	if err := a.AlertEmailSender.Send(c.Request().Context(), recipients, subject, strings.TrimSpace(req.Body)); err != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{"code": "EMAIL_SEND_FAILED", "message": "Email provider could not send the alert"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": "sent", "recipients": recipients})
}

func validateAlertEmailRecipients(values []string) ([]string, error) {
	if len(values) == 0 || len(values) > 20 {
		return nil, http.ErrNotSupported
	}
	seen := make(map[string]bool, len(values))
	recipients := make([]string, 0, len(values))
	for _, value := range values {
		address := strings.TrimSpace(value)
		parsed, err := mail.ParseAddress(address)
		if err != nil || parsed.Address != address || len(address) > 254 {
			return nil, http.ErrNotSupported
		}
		key := strings.ToLower(address)
		if !seen[key] {
			seen[key] = true
			recipients = append(recipients, address)
		}
	}
	if len(recipients) == 0 {
		return nil, http.ErrNotSupported
	}
	return recipients, nil
}

func invalidQaAlertEmail(c echo.Context) error {
	return c.JSON(http.StatusBadRequest, map[string]string{"code": "INVALID_QA_ALERT_EMAIL", "message": "provide 1 to 20 valid email addresses and an alert body no longer than 20000 bytes"})
}
