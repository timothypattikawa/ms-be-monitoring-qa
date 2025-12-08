package redis

import (
	"context"
	"time"

	"github.com/Beyondtech-ID/ms-backbone-emoney/internal/shared/errors"
	"github.com/go-redis/redis/v8"
)

func (r *rCache) HSet(ctx context.Context, key string, field string, value interface{}, ttl ...time.Duration) error {
	var (
		dur = time.Duration(0)
		err error
	)

	err = r.client.HSet(ctx, key, field, value).Err()
	if err != nil {
		return errors.ErrSetCache.WithUnderlyingErrors(err)
	}

	if len(ttl) > 0 {
		dur = ttl[0]
		err = r.client.Expire(ctx, key, dur).Err()
		if err != nil {
			return errors.ErrSetExpCache.WithUnderlyingErrors(err)
		}
	}

	return nil
}

func (r *rCache) HGet(ctx context.Context, key string, field string, value interface{}) error {
	var (
		err error
	)

	err = r.client.HGet(ctx, key, field).Scan(value)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return errors.ErrCacheNotFound.WithUnderlyingErrors(err)
		}
		return errors.ErrGetCache.WithUnderlyingErrors(err)
	}

	return nil
}

func (r *rCache) HGetBytes(ctx context.Context, key string, field string) ([]byte, error) {
	var (
		err   error
		bytes []byte
	)

	bytes, err = r.client.HGet(ctx, key, field).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, errors.ErrCacheNotFound.WithUnderlyingErrors(err)
		}
		return nil, errors.ErrGetCache.WithUnderlyingErrors(err)
	}

	return bytes, nil
}

// HDel will remove data from hash based on given key and field
func (r *rCache) HDel(ctx context.Context, key string, field string) error {
	var err error

	err = r.client.HDel(ctx, key, field).Err()
	if err != nil {
		return errors.ErrDelCache.WithUnderlyingErrors(err)
	}

	return nil
}
