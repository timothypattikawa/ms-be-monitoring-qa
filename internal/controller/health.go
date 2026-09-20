package controller

import (
	"net/http"

	"github.com/Beyondtech-ID/boiler-plate-be-api/internal/shared"
	"github.com/Beyondtech-ID/boiler-plate-be-api/internal/shared/delivery"
	"github.com/Beyondtech-ID/boiler-plate-be-api/internal/usecase"
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
	dbSQL, err := h.deps.ORM.DB.DB()
	if err != nil {
		return delivery.ResponseWithCode(c, "Failed to get database instance", "Internal Server Error", http.StatusInternalServerError, HealthCheckService, "01")
	}

	if err := dbSQL.Ping(); err != nil {
		return delivery.ResponseWithCode(c, "Database is unreachable", "Internal Server Error", http.StatusInternalServerError, HealthCheckService, "02")
	}

	return delivery.ResponseWithCode(c, "It's OK", "OK", http.StatusOK, HealthCheckService, "00")
}

func NewHealthCheckHandler(usecase usecase.Dependency, deps shared.Dependency) HealthCheckHandler {
	return &healthCheckHandler{
		usecase: usecase,
		deps:    deps,
	}
}
