// Package middleware/security.go provides global HTTP security headers.
//
// DESIGN:
//
//	These headers are applied to ALL responses and follow OWASP best practices
//	for defense-in-depth against common web vulnerabilities.
//
// HEADERS SET:
//
//	X-Content-Type-Options: nosniff    — prevents MIME-type sniffing
//	X-Frame-Options: DENY              — prevents clickjacking via iframes
//	Referrer-Policy: strict-origin-when-cross-origin — limits referrer leakage
//	X-XSS-Protection: 0               — disabled (CSP is the modern replacement)
//	Permissions-Policy: (restrictive)  — disables unnecessary browser APIs
//
//	HSTS is NOT set here because it should only be enabled when TLS is guaranteed
//	(i.e., behind a reverse proxy that terminates TLS). The reverse proxy or
//	load balancer should set HSTS in production.
//
// NOTE ON CSP:
//
//	Content-Security-Policy is not set globally because the API serves both
//	JSON responses and proxied PDF/image content, which have different CSP needs.
//	PDF-serving endpoints set their own content-specific headers (see content.go).
package middleware

import "github.com/gin-gonic/gin"

// SecurityHeaders returns a Gin middleware that sets global security headers
// on every response. These are defense-in-depth headers that don't depend on
// TLS configuration or content type.
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Prevent MIME-type sniffing — browsers must respect Content-Type
		c.Header("X-Content-Type-Options", "nosniff")

		// Prevent the page from being embedded in iframes (anti-clickjacking)
		c.Header("X-Frame-Options", "DENY")

		// Control how much referrer info is sent with outgoing requests
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")

		// Disable legacy XSS filter (CSP replaces it; the filter can introduce vulnerabilities)
		c.Header("X-XSS-Protection", "0")

		// Restrict unnecessary browser APIs
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")

		c.Next()
	}
}
