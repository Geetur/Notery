// cart.go — Cart-related helper utilities (Redis key builders).
package helpers

import "strconv"

// CartKey returns the Redis key for a user's shopping cart.
func CartKey(userID uint64) string {
	return "cart:" + strconv.FormatUint(userID, 10)
}
