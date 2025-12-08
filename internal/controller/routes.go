package controller

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func (ctrl *Dependency) SetupEchoRoutes(e *echo.Echo) {
	e.Use(middleware.Recover())
	e.Use(middleware.Logger())

	e.GET("/health", ctrl.HealthCheckHandler.HealthCheck)

	// admin := e.Group("/admin")
	// admin.Use(ctrl.Middleware.Authorize)

}
