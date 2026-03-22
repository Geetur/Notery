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
	"os"
	"strconv"
	"sync"
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
// (login, signup, password reset): 15 requests per minute to slow brute-force.
var DefaultAuthRateLimit = RateLimitConfig{
	MaxRequests: 15,
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

// LoadRateLimitOverrides reads optional RATE_LIMIT_* env vars and applies them.
// Must be called after godotenv.Load() so .env values are available.
// Supported: RATE_LIMIT_AUTH, RATE_LIMIT_WRITE, RATE_LIMIT_READ, RATE_LIMIT_OAUTH.
func LoadRateLimitOverrides() {
	if v := os.Getenv("RATE_LIMIT_AUTH"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			DefaultAuthRateLimit.MaxRequests = n
		}
	}
	if v := os.Getenv("RATE_LIMIT_WRITE"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			DefaultWriteRateLimit.MaxRequests = n
		}
	}
	if v := os.Getenv("RATE_LIMIT_READ"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			DefaultReadRateLimit.MaxRequests = n
		}
	}
	if v := os.Getenv("RATE_LIMIT_OAUTH"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			DefaultOAuthRateLimit.MaxRequests = n
		}
	}
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
			// Redis down: fall back to in-memory rate limiter rather than
			// allowing unlimited requests (prevents brute-force when Redis crashes).
			mwLog.Log("RATE", "Redis pipeline error, falling back to in-memory limiter", "key", key, "error", err)
			count := memLimiter.Increment(key, cfg.Window)
			if count > cfg.MaxRequests {
				retryAfter := int(cfg.Window.Seconds())
				c.Header("Retry-After", strconv.Itoa(retryAfter))
				c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
					"error":       "Rate limit exceeded. Please slow down.",
					"retry_after": retryAfter,
				})
				return
			}
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

// ── In-memory fallback rate limiter ────────────────────────────────────────
// Used when Redis is unavailable. Simple per-key counter with expiry.
// Not shared across instances — acceptable for degraded-mode protection.

type memEntry struct {
	count   int64
	expires time.Time
}

type inMemoryLimiter struct {
	mu      sync.Mutex
	entries map[string]*memEntry
}

var memLimiter = &inMemoryLimiter{entries: make(map[string]*memEntry)}

// Increment atomically increments the counter for a key, auto-expiring stale entries.
func (l *inMemoryLimiter) Increment(key string, window time.Duration) int64 {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	e, ok := l.entries[key]
	if !ok || now.After(e.expires) {
		l.entries[key] = &memEntry{count: 1, expires: now.Add(window)}
		return 1
	}
	e.count++
	return e.count
}
