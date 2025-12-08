package di

import (
	"fmt"
	"sync"

	"github.com/Beyondtech-ID/ms-backbone-emoney/configs"
	"go.uber.org/dig"
)

var (
	container *dig.Container
	once      sync.Once
)

func GetContainer() *dig.Container {
	once.Do(
		func() {
			container = dig.New()
		},
	)

	return container
}

func RegisterDependency(container *dig.Container) error {

	if err := container.Provide(configs.GetConfig); err != nil {
		return err
	}

	fmt.Println("oke sampe sini")

	if err := container.Provide(NewDatabase); err != nil {
		return err
	}

	if err := container.Provide(NewLogger); err != nil {
		return err
	}

	return nil
}
