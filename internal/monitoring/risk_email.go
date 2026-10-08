package monitoring

import (
	"errors"
	"mime"
	"net/http"
	"net/mail"
	"net/smtp"
	"os"
	"strings"

	"github.com/Beyondtech-ID/ms-monitoring-qa-be/internal/repository"
	"github.com/labstack/echo/v4"
)

const maxRiskEmailCC = 10

// smtpSend is swapped in tests.
var smtpSend = smtp.SendMail

type riskEmailRequest struct {
	To      string   `json:"to"`
	CC      []string `json:"cc"`
	Subject string   `json:"subject"`
	Body    string   `json:"body"`
}

// validate trims the request in place and returns bare addresses.
func (r *riskEmailRequest) validate() (to string, cc []string, err error) {
	r.Subject, r.Body = strings.TrimSpace(r.Subject), strings.TrimSpace(r.Body)
	if r.Subject == "" || r.Body == "" || len(r.CC) > maxRiskEmailCC {
		return "", nil, errors.New("invalid")
	}
	if strings.ContainsAny(r.Subject, "\r\n") {
		return "", nil, errors.New("invalid subject")
	}
	if to, err = bareAddress(r.To); err != nil {
		return "", nil, err
	}
	for _, c := range r.CC {
		addr, err := bareAddress(c)
		if err != nil {
			return "", nil, err
		}
		cc = append(cc, addr)
	}
	return to, cc, nil
}

func bareAddress(s string) (string, error) {
	a, err := mail.ParseAddress(strings.TrimSpace(s))
	if err != nil || a.Address != strings.TrimSpace(s) {
		return "", errors.New("invalid email")
	}
	return a.Address, nil
}

func (a API) sendRiskEmail(c echo.Context) error {
	var req riskEmailRequest
	if err := c.Bind(&req); err != nil {
		return invalidRiskEmail(c)
	}
	to, cc, err := req.validate()
	if err != nil {
		return invalidRiskEmail(c)
	}
	if _, err := a.Repo.Project(c.Param("id")); errors.Is(err, repository.ErrNotFound) {
		return c.JSON(http.StatusNotFound, map[string]string{"code": "NOT_FOUND", "message": "project not found"})
	} else if err != nil {
		return safeError(c, err)
	}
	host, addr, sender, pass := os.Getenv("MAILER_HOST"), os.Getenv("MAILER_ADDRESS"), os.Getenv("MAILER_SENDER"), os.Getenv("MAILER_PASS")
	if host == "" || addr == "" || sender == "" || pass == "" {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"code": "MAILER_CONFIG_MISSING", "message": "Email is not configured"})
	}
	from := mail.Address{Name: os.Getenv("MAILER_SENDERNAME"), Address: sender}
	msg := buildRiskEmail(from, to, cc, req.Subject, req.Body)
	rcpts := append([]string{to}, cc...)
	if err := smtpSend(addr, smtp.PlainAuth("", sender, pass, host), sender, rcpts, msg); err != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{"code": "MAIL_SEND_FAILED", "message": "failed to send email"})
	}
	return c.JSON(http.StatusOK, map[string]any{"sent": true, "to": to, "cc": cc})
}

func invalidRiskEmail(c echo.Context) error {
	return c.JSON(http.StatusBadRequest, map[string]string{"code": "INVALID_EMAIL", "message": "valid To address, subject and body are required; at most 10 valid CC addresses"})
}

func buildRiskEmail(from mail.Address, to string, cc []string, subject, body string) []byte {
	h := []string{
		"From: " + from.String(),
		"To: " + to,
	}
	if len(cc) > 0 {
		h = append(h, "Cc: "+strings.Join(cc, ", "))
	}
	h = append(h,
		"Subject: "+mime.QEncoding.Encode("utf-8", subject),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
		"Content-Transfer-Encoding: 8bit",
	)
	return []byte(strings.Join(h, "\r\n") + "\r\n\r\n" + strings.ReplaceAll(body, "\n", "\r\n") + "\r\n")
}
