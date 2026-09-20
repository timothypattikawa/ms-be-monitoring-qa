package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Beyondtech-ID/boiler-plate-be-api/configs"
	"github.com/Beyondtech-ID/boiler-plate-be-api/internal/controller"
	"github.com/Beyondtech-ID/boiler-plate-be-api/internal/di"
	"github.com/Beyondtech-ID/boiler-plate-be-api/internal/shared"
	otelshared "github.com/Beyondtech-ID/boiler-plate-be-api/internal/shared/otel"
	"github.com/Beyondtech-ID/boiler-plate-be-api/internal/usecase"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/saucon/errcntrct"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
)

// excludedOTelPaths are infra/health-check routes excluded from both tracing
// and metrics, so infra traffic never pollutes them.
var excludedOTelPaths = map[string]bool{
	"/health":  true,
	"/metrics": true,
	"/ready":   true,
}

func otelSkipper(c echo.Context) bool {
	path := c.Path()
	if excludedOTelPaths[path] {
		return true
	}
	return strings.HasPrefix(path, "/health") || strings.HasPrefix(path, "/metrics") || strings.HasPrefix(path, "/ready")
}

func main() {
	var err error

	ctx := context.Background()

	container := di.GetContainer()

	if err = di.Register(container); err != nil {
		panic(err)
	}

	if err = controller.RegisterController(container); err != nil {
		panic(err)
	}

	err = container.Invoke(
		func(
			deps shared.Dependency,
			ctrl controller.Dependency,
			uc usecase.Dependency,
		) error {
			cfg := configs.GetConfig()

			collectorAddr := cfg.OTel.CollectorAddr
			if collectorAddr == "" {
				collectorAddr = "localhost:4317"
			}

			otelShutdown, err := otelshared.SetupOTelSDK(ctx, otelshared.ServiceName, collectorAddr)
			if err != nil {
				panic(err)
			}
			defer func() {
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := otelShutdown(shutdownCtx); err != nil {
					// Best-effort: process is exiting either way.
					_ = err
				}
			}()

			sig := make(chan os.Signal, 1)
			signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

			if err := errcntrct.InitContract("errorContract.json"); err != nil {
				return err
			}

			otelMetrics, err := otelshared.NewMetrics()
			if err != nil {
				return err
			}

			e := echo.New()
			e.HideBanner = true
			e.HidePort = true

			e.Use(middleware.Recover())
			e.Use(middleware.Logger())
			e.Use(otelecho.Middleware(otelshared.ServiceName, otelecho.WithSkipper(otelSkipper)))
			e.Use(otelshared.NewMetricsMiddleware(otelMetrics, otelSkipper))

			ctrl.SetupEchoRoutes(e)

			go func() {
				addr := cfg.AppConfig.Host + ":" + cfg.AppConfig.Port
				if err := e.Start(addr); err != nil {
					if !errors.Is(err, http.ErrServerClosed) {
						deps.Logger.WithError(err).Error("failed running echo server")
					}
					sig <- syscall.SIGTERM
				}
			}()

			<-sig

			_ = deps.Close()

			if err := e.Shutdown(context.Background()); err != nil {
				deps.Logger.WithError(err).Error("failed to shutdown echo")
			}

			return nil
		},
	)

	if err != nil {
		panic(err)
	}
}
