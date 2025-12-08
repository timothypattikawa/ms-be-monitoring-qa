package delivery

import (
	"fmt"
	"net/http"

	"github.com/Beyondtech-ID/ms-backbone-emoney/internal/shared/dto"
	"github.com/labstack/echo/v4"
)

func ResponseData(c echo.Context, data []byte, contentType string) error {
	c.Response().Header().Set(echo.HeaderContentType, contentType)
	return c.Blob(http.StatusOK, contentType, data)
}

//func ResponseWithMetadata(c echo.Context, data any, metadata any) error {
//	r := dto.BaseResponse{}
//	if metadata != nil {
//		r.Metadata = metadata
//	}
//	return c.JSON(http.StatusOK, r)
//}

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
