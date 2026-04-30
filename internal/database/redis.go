// redis.go — Redis client initialization and connection testing.
package database

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"strings"

	"github.com/redis/go-redis/v9"

	"github.com/Geetur/Notery/internal/config"
)

// InitRedis initializes a Redis client with configuration from environment variables.
// It supports both a full Redis URL (REDIS_URL or a rediss:// / redis:// REDIS_ADDR)
// and individual components (REDIS_ADDR host:port, REDIS_PASSWORD, REDIS_DB).
// It returns the client or an error if the connection test fails.
func InitRedis() (*redis.Client, error) {
	// NOTE: godotenv.Load() is called once in config.Load() at startup.

	var opts *redis.Options

	// Prefer REDIS_URL (Railway/Render/Upstash convention), then fall back to
	// REDIS_ADDR. If either looks like a full URL, parse it with ParseURL so
	// scheme, host, port, password, and TLS are all set correctly.
	redisURL := getenv("REDIS_URL", "")
	if redisURL == "" {
		redisURL = getenv("REDIS_ADDR", "")
	}

	if strings.HasPrefix(redisURL, "redis://") || strings.HasPrefix(redisURL, "rediss://") {
		parsed, err := redis.ParseURL(redisURL)
		if err != nil {
			return nil, fmt.Errorf("invalid Redis URL: %w", err)
		}
		opts = parsed
	} else {
		// Plain host:port form — build options from individual env vars.
		addr := redisURL
		if addr == "" {
			addr = "localhost:6379"
		}
		opts = &redis.Options{
			Addr:     addr,
			Password: getenv("REDIS_PASSWORD", ""),
			DB:       getenvInt("REDIS_DB", 0),
		}
	}

	// Override TLS if explicitly requested (rediss:// already enables it via ParseURL).
	if strings.EqualFold(getenv("REDIS_TLS_ENABLED", ""), "true") && opts.TLSConfig == nil {
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
