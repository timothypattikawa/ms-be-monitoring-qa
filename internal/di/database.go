package di

import (
	"github.com/Beyondtech-ID/ms-backbone-emoney/configs"
	"github.com/Beyondtech-ID/ms-backbone-emoney/configs/db"
)

func NewDatabase(cfg *configs.Config) (*db.Database, error) {

	var (
		opts = []db.Option{
			db.WithMaxIdleConnection(cfg.DBConfig.MaxIdleConnection),
			db.WithMaxOpenConnection(cfg.DBConfig.MaxOpenConnection),
			db.WithMaxConnectionLifeTime(cfg.DBConfig.MaxConnLifeTime),
		}
	)

	database := db.NewPostgreDB(*cfg, false, opts...)

	return database, nil
}
