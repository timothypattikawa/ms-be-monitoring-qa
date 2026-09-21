package di

import (
	"github.com/Beyondtech-ID/ms-monitoring-qa-be/configs"
	"github.com/Beyondtech-ID/ms-monitoring-qa-be/configs/db"
)

func NewDatabase(cfg *configs.Config) (*db.Database, error) {

	var (
		opts = []db.Option{
			db.WithMaxIdleConnection(cfg.DBConfig.MaxIdleConnection),
			db.WithMaxOpenConnection(cfg.DBConfig.MaxOpenConnection),
			db.WithMaxConnectionLifeTime(cfg.DBConfig.MaxConnLifeTime),
		}
	)

	return db.NewPostgreDB(*cfg, false, opts...)
}
