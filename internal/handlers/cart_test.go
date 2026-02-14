package handlers

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	redisLib "github.com/redis/go-redis/v9"
)

// ===== CART TESTS =====

func TestAddToCart_HappyPath(t *testing.T) {
	app := testAppWithRedis(t)
	if app == nil {
		t.Skip("Redis not available")
	}
	uid := seedUser(t, app.DB, "cartuser")
	noteID := seedApprovedNote(t, app.DB, uid)

	w := serve("POST", "/cart", "/cart",
		jsonBody(map[string]string{
			"item_id": fmt.Sprintf("%d", noteID),
		}), app.AddToCart, authMW(uid))
	assertStatus(t, w, http.StatusOK)
}

func TestAddToCart_MissingItemID(t *testing.T) {
	app := testAppWithRedis(t)
	if app == nil {
		t.Skip("Redis not available")
	}
	uid := seedUser(t, app.DB, "emptycart")

	w := serve("POST", "/cart", "/cart",
		jsonBody(map[string]string{}), app.AddToCart, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

func TestAddToCart_NonExistentNote(t *testing.T) {
	app := testAppWithRedis(t)
	if app == nil {
		t.Skip("Redis not available")
	}
	uid := seedUser(t, app.DB, "ghostcart")

	w := serve("POST", "/cart", "/cart",
		jsonBody(map[string]string{
			"item_id": "99999",
		}), app.AddToCart, authMW(uid))
	assertStatus(t, w, http.StatusNotFound)
}

func TestAddToCart_PendingNote(t *testing.T) {
	app := testAppWithRedis(t)
	if app == nil {
		t.Skip("Redis not available")
	}
	uid := seedUser(t, app.DB, "pendcart")
	noteID := seedPendingNote(t, app.DB, uid)

	w := serve("POST", "/cart", "/cart",
		jsonBody(map[string]string{
			"item_id": fmt.Sprintf("%d", noteID),
		}), app.AddToCart, authMW(uid))
	assertStatus(t, w, http.StatusForbidden)
}

func TestGetCart_Empty(t *testing.T) {
	app := testAppWithRedis(t)
	if app == nil {
		t.Skip("Redis not available")
	}
	uid := seedUser(t, app.DB, "getempty")

	w := serve("GET", "/cart", "/cart",
		nil, app.GetCart, authMW(uid))
	assertStatus(t, w, http.StatusOK)
}

func TestRemoveFromCart_HappyPath(t *testing.T) {
	app := testAppWithRedis(t)
	if app == nil {
		t.Skip("Redis not available")
	}
	uid := seedUser(t, app.DB, "remover")
	noteID := seedApprovedNote(t, app.DB, uid)
	itemID := fmt.Sprintf("%d", noteID)

	// Add to cart first
	serve("POST", "/cart", "/cart",
		jsonBody(map[string]string{"item_id": itemID}),
		app.AddToCart, authMW(uid))

	// Remove from cart
	w := serve("DELETE", "/cart/:item_id", "/cart/"+itemID,
		nil, app.RemoveFromCart, authMW(uid))
	assertStatus(t, w, http.StatusOK)
}

// testAppWithRedis creates a test app with Redis if available.
// Returns nil if Redis is not available (tests should skip).
func testAppWithRedis(t *testing.T) *App {
	t.Helper()
	app := testApp(t)

	// Try to connect to Redis — skip tests if not available
	rdb, err := tryRedisConnect()
	if err != nil {
		return nil
	}
	app.RDB = rdb
	// Use a dedicated test DB to avoid collisions
	return app
}

func tryRedisConnect() (*redisLib.Client, error) {
	client := redisLib.NewClient(&redisLib.Options{
		Addr: "localhost:6379",
		DB:   15, // Use DB 15 for tests
	})
	_, err := client.Ping(context.Background()).Result()
	if err != nil {
		return nil, err
	}
	return client, nil
}
