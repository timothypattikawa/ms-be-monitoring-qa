package di

import (
	"crypto/tls"
	"crypto/x509"
	"os"

	"github.com/Beyondtech-ID/ms-monitoring-qa-be/configs"
	"github.com/Beyondtech-ID/ms-monitoring-qa-be/internal/shared/cache"
	rediscache "github.com/Beyondtech-ID/ms-monitoring-qa-be/internal/shared/cache/redis"
	"github.com/go-redis/redis/v8"
)

func NewRedisClient() (*redis.Client, error) {
	conf := configs.GetConfig()

	redisOptions := &redis.Options{
		Addr:         conf.Cache.Address,
		Username:     conf.Cache.UserName,
		Password:     conf.Cache.Password,
		DB:           conf.Cache.DB,
		DialTimeout:  conf.Cache.DialTimeout,
		ReadTimeout:  conf.Cache.ReadTimeout,
		WriteTimeout: conf.Cache.WriteTimeout,
		PoolSize:     conf.Cache.PoolSize,
		MinIdleConns: conf.Cache.MinIdleConn,
		MaxConnAge:   conf.Cache.MaxConnAge,
	}

	if conf.Cache.UsingTls {
		caCert, err := os.ReadFile(conf.Cache.TlsCaCert)
		if err != nil {
			return nil, err
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)

		cert, err := tls.LoadX509KeyPair(conf.Cache.TlsCert, conf.Cache.TlsKey)
		if err != nil {
			return nil, err
		}

		redisOptions.TLSConfig = &tls.Config{
			RootCAs:      caCertPool,
			Certificates: []tls.Certificate{cert},
		}
	}

	client := redis.NewClient(redisOptions)
	return client, nil
}

func NewCacheRedis(redisclient *redis.Client) (cache.Cache, error) {
	return rediscache.New(redisclient)
}
