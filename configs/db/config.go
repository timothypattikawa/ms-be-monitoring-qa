// This file is generated using ucnbrew tool.
// Check out for more info "https://github.com/saucon/ucnbrew"
package db

import (
	"github.com/Beyondtech-ID/ms-monitoring-qa-be/configs"
	"gorm.io/driver/postgres"

	//nolint:typecheck

	"fmt"
	"time"

	"gorm.io/gorm"
)

type Database struct {
	DB *gorm.DB
}

func NewPostgreDB(conf configs.Config, isDbLog bool, opts ...Option) (*Database, error) {
	var DB *gorm.DB
	var err error
	option := NewDefaultOption()

	for _, apply := range opts {
		apply(option)
	}

	var host, user, password, name, port string

	defer func() {
		if r := recover(); r != nil {
			// Logging
		}
	}()

	// check DB version
	if isDbLog {
		host = conf.DBConfig.Host
		port = conf.DBConfig.Port
		user = conf.DBConfig.User
		password = conf.DBConfig.Pass
		name = conf.DBConfig.Name
	} else {
		host = conf.DBConfig.Host
		port = conf.DBConfig.Port
		user = conf.DBConfig.User
		password = conf.DBConfig.Pass
		name = conf.DBConfig.Name
	}

	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=Asia/Jakarta",
		host, user, password, name, port,
	)

	DB, err = gorm.Open(
		postgres.Open(dsn), &gorm.Config{
			NowFunc: func() time.Time {
				return time.Now().UTC()
			},
		},
	)
	if err != nil {
		return nil, err
	}

	dbSQL, err := DB.DB()
	if err != nil {
		return nil, err
	}

	//Database Connection Pool
	dbSQL.SetMaxIdleConns(option.MaxIdleConnection)
	dbSQL.SetMaxOpenConns(option.MaxOpenConnection)
	dbSQL.SetConnMaxLifetime(option.MaxConnLifeTime)

	err = dbSQL.Ping()
	if err != nil {
		return nil, err
	} else {
		go doEvery(10*time.Minute, pingDb, DB)
		return &Database{
			DB: DB,
		}, nil
	}

	return nil, fmt.Errorf("database unavailable")
}

func doEvery(d time.Duration, f func(*gorm.DB), x *gorm.DB) {
	for range time.Tick(d) {
		f(x)
	}
}

func pingDb(db *gorm.DB) {
	dbSQL, err := db.DB()
	if err != nil {
	}

	err = dbSQL.Ping()
	if err != nil {
	}
}

func (d *Database) AutoMigrate(schemas ...interface{}) {
	for _, schema := range schemas {
		if err := d.DB.AutoMigrate(schema); err != nil {
		}
	}
}

func (db *Database) DropTable(schemas ...interface{}) error {
	for _, schema := range schemas {

		if err := db.DB.Migrator().DropTable(schema); err != nil {
			return err
		}
	}
	return nil
}
