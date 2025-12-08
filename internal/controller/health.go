package controller

import (
	"net/http"

	"github.com/Beyondtech-ID/ms-backbone-emoney/internal/shared"
	"github.com/Beyondtech-ID/ms-backbone-emoney/internal/shared/delivery"
	"github.com/Beyondtech-ID/ms-backbone-emoney/internal/usecase"
	"github.com/labstack/echo/v4"
)

const (
	HealthCheckService string = "00"
)

type (
	HealthCheckHandler interface {
		HealthCheck(c echo.Context) error
	}

	healthCheckHandler struct {
		usecase usecase.Dependency
		deps    shared.Dependency
	}
)

// HealthCheck implements HealthCheckHandler.
func (h *healthCheckHandler) HealthCheck(c echo.Context) error {
	return delivery.ResponseWithCode(c, "It's OK", "OK", http.StatusOK, HealthCheckService, "00")
}

func NewHealthCheckHandler(usecase usecase.Dependency, deps shared.Dependency) HealthCheckHandler {
	return &healthCheckHandler{
		usecase: usecase,
		deps:    deps,
	}
}
