package repository

import (
	"github.com/Beyondtech-ID/ms-monitoring-qa-be/configs/db"
	"go.uber.org/dig"
)

type Dependency struct {
	dig.In
	Monitoring *Monitoring
}

func NewMonitoring(database *db.Database) *Monitoring { return &Monitoring{db: database.DB} }

func Register(container *dig.Container) error { return container.Provide(NewMonitoring) }
