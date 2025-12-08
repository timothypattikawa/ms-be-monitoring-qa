package cache

import (
	"context"
	"time"
)

type (
	Cache interface {
		Set(ctx context.Context, key string, value interface{}) error
		SetNX(ctx context.Context, key string, value interface{}, exp time.Duration) (bool, error)
		SetWithExp(ctx context.Context, key string, value interface{}, exp time.Duration) error
		Get(ctx context.Context, key string, object interface{}) error
		GetBytes(ctx context.Context, key string) ([]byte, error)
		Delete(ctx context.Context, keys ...string) (int64, error)

		Hash

		Ping(ctx context.Context) error
		Close() error
	}

	Hash interface {
		HSet(ctx context.Context, key string, field string, value interface{}, ttl ...time.Duration) error
		HGet(ctx context.Context, key string, field string, value interface{}) error
		HGetBytes(ctx context.Context, key string, field string) ([]byte, error)
		HDel(ctx context.Context, key string, field string) error
	}
)
