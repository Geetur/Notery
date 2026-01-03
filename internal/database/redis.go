package database

import (
	"log"
	"context"

	"github.com/redis/go-redis/v9"
)

var RedisClient *redis.Client

func InitRedis() {

	RedisClient = redis.NewClient(&redis.Options{
		// same as docker compose file
		Addr:     "localhost:6379",
		// no password set
		Password: "",
		// use default DB
		DB:       0, 
	})
}

// TestRedisConnection pings the Redis server to ensure connectivity
// using background context because this is happening at startup
func TestRedisConnection() error{
	log.Println("Testing redis connection...")
	ctx := context.Background()
	pong, err := RedisClient.Ping(ctx).Result()
	if err != nil {
		log.Printf("Failed to connect to Redis: %v", err)
	}
	log.Printf("Redis connection successful: %s", pong)
	return err
}