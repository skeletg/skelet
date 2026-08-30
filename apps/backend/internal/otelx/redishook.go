package otelx

import (
	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
)

// A no-op reporting no error when OTel is off - a silently-untraced Redis is a smaller incident than a server that refuses to start
func InstrumentRedis(client *redis.Client) error {
	if !enabled() {
		return nil
	}
	if err := redisotel.InstrumentTracing(client); err != nil {
		return err
	}
	if err := redisotel.InstrumentMetrics(client); err != nil {
		return err
	}
	return nil
}
