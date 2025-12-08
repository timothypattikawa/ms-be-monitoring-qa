package db

import "time"

type (
	Option func(option *DBOption)

	DBOption struct {
		MaxIdleConnection int
		MaxOpenConnection int
		MaxConnLifeTime   time.Duration
	}
)

func NewDefaultOption() *DBOption {
	return &DBOption{
		MaxIdleConnection: 10,
		MaxOpenConnection: 100,
		MaxConnLifeTime:   time.Hour,
	}
}

func WithMaxIdleConnection(n int) Option {
	return func(option *DBOption) {
		option.MaxIdleConnection = n
	}
}

func WithMaxOpenConnection(n int) Option {
	return func(option *DBOption) {
		option.MaxOpenConnection = n
	}
}

func WithMaxConnectionLifeTime(maxConnectionLifeTime time.Duration) Option {
	return func(option *DBOption) {
		option.MaxConnLifeTime = maxConnectionLifeTime
	}
}
