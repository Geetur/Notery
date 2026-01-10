// Package database/redis.go contains the Redis 
// client initialization and connection testing logic
package database

import (
	"context"
	"log"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)
// InitRedis initializes the Redis client with configuration from environment variables
func InitRedis() (*redis.Client, error) {
	
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found (ok):", err)
	}

	client := redis.NewClient(&redis.Options{

		// same as docker compose file
		Addr:    getenv("REDIS_ADDR", "localhost:6379"),
		Password: getenv("REDIS_PASSWORD", ""),
		// no password set
		// use default DB
		DB: getenvInt("REDIS_DB", 0),
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
	ctx := context.Background()
	pong, err := client.Ping(ctx).Result()
	if err != nil {
		log.Printf("Failed to connect to Redis: %v", err)
	}
	log.Printf("Redis connection successful: %s", pong)
	return err
}

