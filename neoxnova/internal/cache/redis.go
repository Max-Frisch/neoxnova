package cache

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

func NewRedis(ctx context.Context, addr, password string, db int) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, err
	}
	return client, nil
}

func FleetEventsKey(universeID string) string {
	return fmt.Sprintf("universe:%s:fleet_events", universeID)
}
