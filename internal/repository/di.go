package repository

import "go.uber.org/dig"

type Dependency struct {
	dig.In
}

func Register(container *dig.Container) error {

	return nil
}
