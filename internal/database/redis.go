// redis.go — Redis client initialization and connection testing.
package database

import (
	"context"
	"crypto/tls"
	"log"
	"strings"

	"github.com/redis/go-redis/v9"

	"github.com/Geetur/Notery/internal/config"
)

// InitRedis initializes a Redis client with configuration from environment variables.
// It returns the client or an error if the connection test fails.
func InitRedis() (*redis.Client, error) {
	// NOTE: godotenv.Load() is called once in config.Load() at startup.

	opts := &redis.Options{
		Addr:     getenv("REDIS_ADDR", "localhost:6379"),
		Password: getenv("REDIS_PASSWORD", ""),
		DB:       getenvInt("REDIS_DB", 0),
	}

	// Enable TLS if configured (required for most managed Redis providers)
	if strings.EqualFold(getenv("REDIS_TLS_ENABLED", ""), "true") {
		opts.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	if config.IsProduction() && opts.Password == "" {
		log.Println("WARNING: REDIS_PASSWORD is empty in production")
	}

	client := redis.NewClient(opts)
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
