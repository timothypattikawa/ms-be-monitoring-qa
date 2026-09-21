package errors

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode"

	"github.com/Beyondtech-ID/ms-monitoring-qa-be/configs"
	"github.com/joomcode/errorx"
)

var (
	ErrNamespace        = errorx.NewNamespace("ms-monitoring-qa-be")
	ErrCodeProperty     = errorx.RegisterProperty("code")
	ErrHttpCodeProperty = errorx.RegisterProperty("httpcode")
	ErrCause            = errorx.RegisterProperty("cause")
	ErrBase             = errorx.NewType(ErrNamespace, configs.GetConfig().AppConfig.Name)
)

func New(httpCode int, code string, msg string, args ...any) *errorx.Error {
	return ErrBase.New(msg, args...).
		WithProperty(ErrCodeProperty, code).
		WithProperty(ErrHttpCodeProperty, httpCode)
}

func NewNative(msg string, args ...any) error {
	if len(args) > 0 {
		return fmt.Errorf(msg, args...)
	}
	return errors.New(msg)
}

// Wrap will replace message shown to user but will still store actual error
func Wrap(err error, msg string, args ...any) error {
	return errorx.Decorate(err, msg, args...)
}

func GetCause(err error) error {
	cause := errorx.Cast(err).Cause()
	if cause == nil {
		return err
	}

	return cause
}

func GetInnerCause(err error) error {
	exErr := errorx.Cast(err)
	for exErr != nil && exErr.Cause() != nil {
		exErr = errorx.Cast(exErr.Cause())
	}
	if exErr != nil {
		return exErr // The deepest error message
	}
	return err // Fallback to the main error message
}

// Extracts both the outer message (excluding hidden) and the innermost message
func ExtractErrorMessages(err error) (outerMessage, hiddenMessage string) {
	errMsg := err.Error()

	if idx := strings.LastIndex(errMsg, "(hidden: "); idx != -1 {
		outerMessage = strings.TrimSpace(errMsg[:idx])                             // Everything before (hidden)
		hiddenMessage = strings.TrimSuffix(strings.TrimSpace(errMsg[idx+9:]), ")") // Extract inside (hidden: ...)
	} else {
		outerMessage = errMsg
		hiddenMessage = ""
	}

	if idx := strings.Index(outerMessage, ": "); idx != -1 {
		outerMessage = strings.TrimSpace(outerMessage[idx+2:])
	}

	return outerMessage, hiddenMessage

}

// isNumeric checks if a string contains only numeric characters
func isNumeric(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// ExtractSnapError extracts structured info for Snap response format
func ExtractSnapError(err error) (httpCode int, serviceCode, caseCode, message string) {
	if err == nil {
		return http.StatusOK, "00", "000", "success"
	}

	exErr := errorx.Cast(err)
	if exErr == nil {
		// native error (non-errorx)
		return http.StatusInternalServerError, "XX", "999", err.Error()
	}

	// HTTP code
	httpCode = http.StatusInternalServerError
	if val, ok := exErr.Property(ErrHttpCodeProperty); ok {
		httpCode = val.(int)
	}

	codeProp, ok := exErr.Property(ErrCodeProperty)
	if !ok {
		// Extract just the message part, removing namespace prefix
		errMsg := exErr.Error()
		if idx := strings.LastIndex(errMsg, ": "); idx != -1 {
			message = strings.TrimSpace(errMsg[idx+2:])
		} else {
			message = errMsg
		}
		return httpCode, "XX", "000", message
	}

	codeStr := fmt.Sprintf("%v", codeProp)

	if parts := strings.SplitN(codeStr, "-", 2); len(parts) == 2 {
		serviceCode = parts[0]
		caseCode = parts[1]
	} else if len(codeStr) == 4 && isNumeric(codeStr) {
		serviceCode = codeStr[:2]
		caseCode = codeStr[2:]
	} else {
		serviceCode = "00"
		caseCode = codeStr
	}

	errMsg := exErr.Error()
	if idx := strings.LastIndex(errMsg, ": "); idx != -1 {
		message = strings.TrimSpace(errMsg[idx+2:])
	} else {
		message = errMsg
	}
	return
}

// WithUnderlyingMsg will add hidden error message for better traceability on backend side
func WithUnderlyingMsg(err error, msg string, args ...any) error {
	if erx, ok := err.(*errorx.Error); ok {
		return erx.WithUnderlyingErrors(NewNative(msg, args...))
	}
	return Wrap(err, msg, args...)
}

var (
	ErrBadRequest = New(http.StatusBadRequest, "400", "invalid request")
	ErrNotFound   = New(http.StatusNotFound, "404", "data not found")
	ErrDateOnly   = New(http.StatusBadRequest, "400", "time format should YYYY-MM-DD")
	ErrForbidden  = New(http.StatusForbidden, "403", "Access Denied – You don’t have permission to access")
)

var (
	ErrSetCache      = New(http.StatusInternalServerError, "CAC-10001", "failed to set data")
	ErrGetCache      = New(http.StatusInternalServerError, "CAC-10002", "failed to get data")
	ErrDelCache      = New(http.StatusInternalServerError, "CAC-10003", "failed to delete data")
	ErrCacheNotFound = New(http.StatusNotFound, "CAC-10004", "failed to get data, not found")
	ErrSetExpCache   = New(http.StatusInternalServerError, "CAC-10005", "failed to set exp")
)

var (
	ErrWebhookCall = New(http.StatusBadRequest, "WEBHOOK-10001", "failed call discord webhook")
)

func Is(err, target error) bool {
	if !errors.Is(err, target) {
		return false
	}
	return isErrorCodeSame(err, target)
}

func isErrorCodeSame(err, target error) bool {
	var errx *errorx.Error
	errxOk := errors.As(err, &errx)
	var targetx *errorx.Error
	targetxOk := errors.As(target, &targetx)

	if !errxOk || !targetxOk {
		return true
	}

	targetCode, targetOk := targetx.Property(ErrCodeProperty)
	errCode, errOk := errx.Property(ErrCodeProperty)
	if !targetOk || !errOk {
		return false
	}
	return targetCode == errCode
}

// GetStatusCodeAndMessage extracts the HTTP status code and human-readable message from an error
func GetStatusCodeAndMessage(err error) (int, string) {
	if err == nil {
		return http.StatusOK, ""
	}

	exErr := errorx.Cast(err)
	if exErr == nil {
		return http.StatusInternalServerError, err.Error()
	}

	httpCode, hasHttpCode := exErr.Property(ErrHttpCodeProperty)
	if hasHttpCode {
		return httpCode.(int), exErr.Error()
	}

	return http.StatusInternalServerError, exErr.Error()
}
