package shared

import (
	"github.com/Beyondtech-ID/ms-backbone-emoney/configs"
	"github.com/Beyondtech-ID/ms-backbone-emoney/configs/db"
	"github.com/Beyondtech-ID/ms-backbone-emoney/internal/shared/log"
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
