package delivery

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/Beyondtech-ID/ms-monitoring-qa-be/internal/shared/dto"
	"github.com/Beyondtech-ID/ms-monitoring-qa-be/internal/shared/errors"
	"github.com/labstack/echo/v4"
)

func ResponseData(c echo.Context, data []byte, contentType string) error {
	c.Response().Header().Set(echo.HeaderContentType, contentType)
	return c.Blob(http.StatusOK, contentType, data)
}

func ResponseWithCode(
	c echo.Context, resp any, message string, httpCode int, serviceCode string, caseCode string,
) error {
	return c.JSON(
		httpCode, dto.BaseResponse{
			ResponseCode:    fmt.Sprintf("%v%s%s", httpCode, serviceCode, caseCode),
			ResponseMessage: message,
			ResponseData:    resp,
		},
	)
}

// ResponseError renders err through the shared error contract (errors.ExtractSnapError),
// so every failure path returns the same response-code/message shape.
func ResponseError(c echo.Context, err error) error {
	httpCode, serviceCode, caseCode, message := errors.ExtractSnapError(err)
	return c.JSON(
		httpCode, dto.BaseResponse{
			ResponseCode:    fmt.Sprintf("%v%s%s", httpCode, serviceCode, caseCode),
			ResponseMessage: message,
			ResponseData:    nil,
		},
	)
}

// ResponseBindError renders an Echo bind/validation error, extracting the
// offending field name when possible so clients get an actionable message.
func ResponseBindError(c echo.Context, err error, serviceCode string, caseCode string) error {
	var fieldName string
	var message = "invalid request format"

	if he, ok := err.(*echo.HTTPError); ok {
		msgStr := fmt.Sprintf("%v", he.Message)
		if he.Internal != nil {
			msgStr = he.Internal.Error()
		}

		if strings.Contains(msgStr, "field=") {
			parts := strings.Split(msgStr, " ")
			for _, part := range parts {
				if strings.HasPrefix(part, "field=") {
					fieldName = strings.TrimPrefix(strings.TrimSuffix(part, ","), "field=")
					message = fmt.Sprintf("invalid property '%s'", fieldName)
					break
				}
			}
		} else if strings.Contains(msgStr, "json: cannot unmarshal") {
			fieldName, message = fieldFromUnmarshalError(msgStr)
		} else {
			message = msgStr
		}
	} else {
		msgStr := err.Error()
		if strings.Contains(msgStr, "json: cannot unmarshal") {
			fieldName, message = fieldFromUnmarshalError(msgStr)
		} else {
			message = msgStr
		}
	}

	var responseData any
	if fieldName != "" {
		responseData = map[string]string{"field": fieldName}
	}

	return c.JSON(
		http.StatusBadRequest, dto.BaseResponse{
			ResponseCode:    fmt.Sprintf("%d%s%s", http.StatusBadRequest, serviceCode, caseCode),
			ResponseMessage: message,
			ResponseData:    responseData,
		},
	)
}

// fieldFromUnmarshalError extracts the field name from a Go
// "json: cannot unmarshal ... into Go struct field X.field of type Y" message.
func fieldFromUnmarshalError(msgStr string) (fieldName string, message string) {
	message = msgStr
	parts := strings.Split(msgStr, " ")
	for i, part := range parts {
		if part == "field" && i+1 < len(parts) {
			fieldPath := parts[i+1]
			fieldParts := strings.Split(fieldPath, ".")
			fieldName = fieldParts[len(fieldParts)-1]
			message = fmt.Sprintf("invalid property '%s'", fieldName)
			break
		}
	}
	return fieldName, message
}
