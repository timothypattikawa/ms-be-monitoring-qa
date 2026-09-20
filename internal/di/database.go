package di

import (
	"github.com/Beyondtech-ID/boiler-plate-be-api/configs"
	"github.com/Beyondtech-ID/boiler-plate-be-api/configs/db"
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
