// Package database/redis.go contains the Redis
// client initialization and connection testing logic
package database

import (
	"context"
	"log"

	"github.com/redis/go-redis/v9"
)

// InitRedis initializes a Redis client with configuration from environment variables.
// It returns the client or an error if the connection test fails.
func InitRedis() (*redis.Client, error) {
	// NOTE: godotenv.Load() is called once in config.Load() at startup.

	client := redis.NewClient(&redis.Options{
		Addr:     getenv("REDIS_ADDR", "localhost:6379"),
		Password: getenv("REDIS_PASSWORD", ""),
		DB:       getenvInt("REDIS_DB", 0),
	})
	if err := TestRedisConnection(client); err != nil {
		return nil, err
	}
	return client, nil
}

// TestRedisConnection pings the Redis server to ensure connectivity
// using background context because this is happening at startup
func TestRedisConnection(client *redis.Client) error {
	log.Println("Testing redis connection...")
	pong, err := client.Ping(context.Background()).Result()
	if err != nil {
		log.Printf("Failed to connect to Redis: %v", err)
		return err
	}
	log.Printf("Redis connection successful: %s", pong)
	return nil
}
