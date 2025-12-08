package di

import (
	"github.com/Beyondtech-ID/ms-backbone-emoney/internal/repository"
	"github.com/Beyondtech-ID/ms-backbone-emoney/internal/usecase"
	"go.uber.org/dig"
)

func Register(cont *dig.Container) error {
	if err := RegisterDependency(cont); err != nil {
		return err
	}

	if err := repository.Register(cont); err != nil {
		return err
	}

	if err := usecase.Register(cont); err != nil {
		return err
	}

	return nil
}
