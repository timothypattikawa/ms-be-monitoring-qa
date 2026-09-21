package shared

import (
	"github.com/Beyondtech-ID/ms-monitoring-qa-be/configs"
	"github.com/Beyondtech-ID/ms-monitoring-qa-be/configs/db"
	"github.com/Beyondtech-ID/ms-monitoring-qa-be/internal/shared/log"
	"go.uber.org/dig"
)

type Dependency struct {
	dig.In

	Logger log.Logger
	Config *configs.Config
	ORM    *db.Database
}

func (d Dependency) Close() error {
	if sql, err := d.ORM.DB.DB(); err == nil {
		_ = sql.Close()
	}

	return nil
}
