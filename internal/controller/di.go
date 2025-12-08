package controller

import "go.uber.org/dig"

type (
	Dependency struct {
		dig.In

		HealthCheckHandler HealthCheckHandler
	}
)

func RegisterController(container *dig.Container) error {

	if err := container.Provide(NewHealthCheckHandler); err != nil {
		return err
	}

	return nil
}
