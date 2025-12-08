package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/Beyondtech-ID/ms-backbone-emoney/configs"
	"github.com/Beyondtech-ID/ms-backbone-emoney/internal/controller"
	"github.com/Beyondtech-ID/ms-backbone-emoney/internal/di"
	"github.com/Beyondtech-ID/ms-backbone-emoney/internal/shared"
	"github.com/Beyondtech-ID/ms-backbone-emoney/internal/usecase"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/saucon/errcntrct"
)

func main() {
	var err error

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

			sig := make(chan os.Signal, 1)
			signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

			if err := errcntrct.InitContract("errorContract.json"); err != nil {
				return err
			}

			fmt.Println("here")

			e := echo.New()
			e.HideBanner = true
			e.HidePort = true

			e.Use(middleware.Recover())
			e.Use(middleware.Logger())

			ctrl.SetupEchoRoutes(e)

			go func() {
				addr := cfg.AppConfig.Host + ":" + cfg.AppConfig.Port
				if err := e.Start(addr); err != nil {
					if !errors.Is(err, http.ErrServerClosed) {
						deps.Logger.WithError(err).Fatal("failed running echo server")
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
