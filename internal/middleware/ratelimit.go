// ratelimit.go — Per-user API rate limiting using Redis sliding windows.
//
// DESIGN:
//
//	Uses a sliding-window counter stored in Redis. Each user gets a key like
//	"ratelimit:<user_id>" with a TTL equal to the window duration. Requests
//	increment the counter atomically via INCR; if the counter exceeds the max
//	allowed requests the middleware returns 429 Too Many Requests.
//
//	Anonymous (unauthenticated) requests are rate-limited by IP address.
//
//	The middleware is intentionally simple: a single global limit.
package middleware

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/Geetur/Notery/internal/helpers"
)

// RateLimitConfig holds the parameters for the rate limiter.
type RateLimitConfig struct {
	// MaxRequests is the maximum number of requests allowed per Window.
	MaxRequests int64
	// Window is the sliding window duration for rate counting.
	Window time.Duration
}

// DefaultWriteRateLimit is the default limit for mutating endpoints
// (comment, post, vote, etc.): 60 write operations per minute.
var DefaultWriteRateLimit = RateLimitConfig{
	MaxRequests: 60,
	Window:      1 * time.Minute,
}

// DefaultAuthRateLimit is the limit for authentication endpoints
// (login, signup, password reset): 5 requests per minute to slow brute-force.
var DefaultAuthRateLimit = RateLimitConfig{
	MaxRequests: 5,
	Window:      1 * time.Minute,
}

// DefaultReadRateLimit is the limit for public read endpoints
// (feed, search, profiles): 120 requests per minute.
var DefaultReadRateLimit = RateLimitConfig{
	MaxRequests: 120,
	Window:      1 * time.Minute,
}

// DefaultOAuthRateLimit is the limit for OAuth redirect/callback endpoints:
// 30 requests per minute. Higher than auth because each OAuth flow consumes
// two requests (redirect + callback) and users may retry after failures.
var DefaultOAuthRateLimit = RateLimitConfig{
	MaxRequests: 30,
	Window:      1 * time.Minute,
}

// RateLimit returns a Gin middleware that enforces per-user rate limiting.
// It requires a Redis client and a configuration. The keyPrefix is prepended to
// the rate-limit key so different route groups can have independent counters
// (e.g., "write:" vs "read:").
func RateLimit(rdb *redis.Client, cfg RateLimitConfig, keyPrefix string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Identify the caller: authenticated user ID or client IP.
		var identifier string
		if userID, exists := c.Get("user_id"); exists {
			identifier = "user:" + strconv.FormatUint(userID.(uint64), 10)
		} else {
			identifier = "ip:" + c.ClientIP()
		}

		key := "ratelimit:" + keyPrefix + identifier
		ctx := context.Background()

		// Atomic increment + conditional TTL set via pipeline for efficiency.
		// ExpireNX only sets TTL if the key has no existing expiry,
		// preventing window reset on every request (true sliding window).
		pipe := rdb.Pipeline()
		incrCmd := pipe.Incr(ctx, key)
		pipe.ExpireNX(ctx, key, cfg.Window)

		if _, err := pipe.Exec(ctx); err != nil {
			// Redis down: fail open — don't block the user, just log.
			mwLog.Log("RATE", "Redis pipeline error, failing open", "key", key, "error", err)
			c.Next()
			return
		}

		count := incrCmd.Val()
		remaining := cfg.MaxRequests - count
		if remaining < 0 {
			remaining = 0
		}

		// Set standard rate-limit headers so clients can self-throttle.
		c.Header("X-RateLimit-Limit", strconv.FormatInt(cfg.MaxRequests, 10))
		c.Header("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))

		if count > cfg.MaxRequests {
			retryAfter := int(cfg.Window.Seconds())
			c.Header("Retry-After", strconv.Itoa(retryAfter))

			helpers.MiddlewareLog.Log("RATE", "rate limit exceeded",
				"identifier", identifier, "count", count, "limit", cfg.MaxRequests)

			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":       "Rate limit exceeded. Please slow down.",
				"retry_after": retryAfter,
			})
			return
		}

		c.Next()
	}
}
