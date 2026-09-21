package redis

import (
	"context"
	"crypto/tls"
	"time"

	"github.com/Beyondtech-ID/ms-monitoring-qa-be/internal/shared/cache"
	"github.com/Beyondtech-ID/ms-monitoring-qa-be/internal/shared/errors"
	redis "github.com/go-redis/redis/v8"
)

type (
	rCache struct {
		client redis.UniversalClient
	}

	Option struct {
		Address, UserName, Password                        string
		DB, PoolSize, MinIdleConn                          int
		DialTimeout, ReadTimeout, WriteTimeout, MaxConnAge time.Duration
		TlsConfig                                          tls.Config
	}
)

func (r *rCache) SetNX(ctx context.Context, key string, value interface{}, exp time.Duration) (bool, error) {
	isExist, err := r.client.SetNX(ctx, key, value, exp).Result()
	if err != nil {
		return isExist, errors.ErrSetCache.WithUnderlyingErrors(err)
	}

	return isExist, nil
}

func (r *rCache) Set(ctx context.Context, key string, value interface{}) error {
	return r.SetWithExp(ctx, key, value, 0)
}

func (r *rCache) SetWithExp(ctx context.Context, key string, value interface{}, exp time.Duration) error {
	err := r.client.Set(ctx, key, value, exp).Err()
	if err != nil {
		return errors.ErrSetCache.WithUnderlyingErrors(err)
	}

	return err
}

func (r *rCache) Get(ctx context.Context, key string, object interface{}) error {
	err := r.client.Get(ctx, key).Scan(object)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return errors.ErrCacheNotFound.WithUnderlyingErrors(err)
		}
		return errors.ErrGetCache.WithUnderlyingErrors(err)
	}

	return nil
}

func (r *rCache) GetBytes(ctx context.Context, key string) ([]byte, error) {
	bytes, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return bytes, errors.ErrCacheNotFound.WithUnderlyingErrors(err)
		}
		return bytes, err
	}

	return bytes, nil
}

func (r *rCache) Delete(ctx context.Context, keys ...string) (int64, error) {
	isSuccess, err := r.client.Del(ctx, keys...).Result()
	if err != nil {
		return isSuccess, errors.ErrDelCache.WithUnderlyingErrors(err)
	}

	return isSuccess, nil
}

func (r *rCache) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

func (r *rCache) Close() error {
	return r.client.Close()
}

func New(client *redis.Client) (cache.Cache, error) {
	return &rCache{client}, nil
}
