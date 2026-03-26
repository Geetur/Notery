# Notery API — Complete Endpoint Reference

> **84 endpoints** across 14 handler files. Every route, middleware chain, request/response contract, data flow, and error path documented.

---

## Table of Contents

1. [Global Middleware](#global-middleware)
2. [Middleware Reference](#middleware-reference)
3. [Endpoint Groups & Middleware Chains](#endpoint-groups--middleware-chains)
4. [Health Check](#1-health-check)
5. [Authentication (10 endpoints)](#2-authentication-endpoints)
6. [OAuth (5 endpoints)](#3-oauth-endpoints)
7. [Stripe Webhook (1 endpoint)](#4-stripe-webhook)
8. [Public Endpoints (14 endpoints)](#5-public-endpoints)
9. [Authenticated Read-Only (14 endpoints)](#6-authenticated-read-only-endpoints)
10. [Verified Write (31 endpoints)](#7-verified-write-endpoints)
11. [Admin (9 endpoints)](#8-admin-endpoints)
12. [Summary Statistics](#summary-statistics)

---

## Global Middleware

Applied to **every** request before route matching:

| Middleware | File | Description |
|---|---|---|
| `SecurityHeaders()` | `middleware/security.go` | Sets `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: strict-origin-when-cross-origin`, `X-XSS-Protection: 0`, `Permissions-Policy` |
| CORS | `cmd/api/main.go` | Origins from `CORS_ORIGINS` env var (default: `localhost:3000,localhost:5173`). Methods: GET, POST, PUT, PATCH, DELETE, OPTIONS |

---

## Middleware Reference

| Middleware | File | Description |
|---|---|---|
| `RequireAuth(jwtSecret)` | `middleware/auth.go` | Validates JWT from `Authorization: Bearer <token>` header **or** `?token=<jwt>` query param. Sets `user_id` in Gin context. Rejects with **401**. |
| `OptionalAuth(jwtSecret)` | `middleware/auth.go` | Same JWT parsing but allows unauthenticated requests through. Sets `user_id` only if valid token present. |
| `RequireVerified(db)` | `middleware/verified.go` | Runs after RequireAuth. Checks `email_verified` flag on user record. Rejects with **403** + `EMAIL_NOT_VERIFIED` code. |
| `RequireAdmin(db)` | `middleware/admin.go` | Runs after RequireAuth + RequireVerified. Checks global admin flag (`is_admin`), then subnotery admin via `user_admins` table. Resolves subnotery from `:subnotery_id` param, `:id` param (note → subnotery lookup), or `?subnotery_id=` query param. Rejects with **403**. |
| `RateLimit(rdb, cfg, prefix)` | `middleware/ratelimit.go` | Redis sliding-window rate limiter. Keyed by `user:<id>` (authenticated) or `ip:<addr>` (anonymous). Fails **open** if Redis is unreachable. Returns **429** with `Retry-After` header when exceeded. Uses `ExpireNX` for correct sliding window. |

### Rate Limit Tiers

| Tier | Prefix | Limit | Applied To |
|---|---|---|---|
| Auth | `auth:` | 5 req/min | Login, signup, password reset, token refresh |
| OAuth | `oauth:` | 30 req/min | OAuth redirects and callbacks |
| Write | `write:` | 60 req/min | All mutating endpoints |
| Read | `read:` | 120 req/min | Public read endpoints |

---

## Endpoint Groups & Middleware Chains

| Group | Middleware Stack |
|---|---|
| **Auth (public)** | RateLimit(auth: 5/min) |
| **OAuth (public)** | RateLimit(oauth: 30/min) |
| **Auth Protected** | RateLimit(auth: 5/min) + RequireAuth |
| **Public + OptionalAuth** | OptionalAuth |
| **Public (no auth)** | None |
| **Read-Only (authenticated)** | RequireAuth |
| **Write (verified)** | RequireAuth + RequireVerified + RateLimit(write: 60/min) |
| **Admin** | RequireAuth + RequireVerified + RateLimit(write: 60/min) + RequireAdmin |

---

## 1. Health Check

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 1 | `GET` | `/health` | inline lambda | `cmd/api/main.go` | None |

**Response (200):**
```json
{"status": "OK", "message": "Notery API is alive"}
```

No auth. No rate limit. Used for uptime monitoring and load balancer health probes.

---

## 2. Authentication Endpoints

**Base path:** `/api/v1/auth/`  
**File:** `auth.go`

### 2.1 Signup

| # | Method | Path | Handler | Middleware |
|---|--------|------|---------|------------|
| 2 | `POST` | `/api/v1/auth/signup` | `Signup` | RateLimit(auth) |

**Request:**
```json
{"email": "user@example.com", "password": "securepass", "username": "jdoe"}
```
`username` is optional — auto-generated if omitted.

**Flow:**
1. Validate JSON body
2. Enforce password ≥ 8 chars
3. Check username uniqueness (409 if taken)
4. Hash password (bcrypt)
5. INSERT user record
6. Generate access token (JWT HS256, 15 min TTL) + refresh token (opaque, 30 day TTL)
7. INSERT refresh_token record
8. If SMTP configured: generate email verification token → INSERT email_verification → send email
9. If LogMailer (dev mode): auto-set `email_verified = true`

**Response (201):**
```json
{
  "message": "User created successfully",
  "user_id": 1,
  "access_token": "eyJ...",
  "refresh_token": "abc123..."
}
```

**Errors:** `400` (bad JSON, short password) · `409` (username taken) · `500` (DB error, email duplicate)  
**DB:** INSERT users, INSERT refresh_tokens, conditional INSERT email_verifications  
**External:** Email (SMTP or LogMailer)

---

### 2.2 Login

| # | Method | Path | Handler | Middleware |
|---|--------|------|---------|------------|
| 3 | `POST` | `/api/v1/auth/login` | `Login` | RateLimit(auth) |

**Request:**
```json
{"email": "user@example.com", "password": "securepass"}
```

**Flow:**
1. Find user by email
2. Reject if OAuth-only user (no password hash)
3. Compare password hash (bcrypt)
4. Generate access + refresh tokens
5. INSERT refresh_token record

**Response (200):**
```json
{"access_token": "eyJ...", "refresh_token": "abc123..."}
```

**Errors:** `400` (bad JSON) · `401` (invalid credentials)  
**DB:** SELECT users, INSERT refresh_tokens

---

### 2.3 Refresh Token

| # | Method | Path | Handler | Middleware |
|---|--------|------|---------|------------|
| 4 | `POST` | `/api/v1/auth/refresh` | `RefreshAccessToken` | RateLimit(auth) |

**Request:**
```json
{"refresh_token": "abc123..."}
```

**Flow:**
1. Hash incoming token (SHA-256)
2. Look up refresh_tokens by hash
3. **Theft detection:** If token found but revoked → entire family (same `family_id`) is revoked → 401
4. Check expiry
5. Revoke current token
6. Issue new access + refresh token pair (same family)

**Response (200):**
```json
{"access_token": "eyJ...", "refresh_token": "def456..."}
```

**Errors:** `401` (invalid/expired/reused token) · `500` (token generation failure)  
**DB:** SELECT refresh_tokens, UPDATE revoked, INSERT new refresh_token

---

### 2.4 Logout

| # | Method | Path | Handler | Middleware |
|---|--------|------|---------|------------|
| 5 | `POST` | `/api/v1/auth/logout` | `Logout` | RateLimit(auth) |

**Request:**
```json
{"refresh_token": "abc123..."}
```

**Flow:** Hash token → revoke in DB. Always returns 200 (anti-enumeration — doesn't reveal whether token existed).

**Response (200):**
```json
{"message": "Logged out"}
```

**DB:** UPDATE refresh_tokens SET revoked = true

---

### 2.5 Logout All Sessions

| # | Method | Path | Handler | Middleware |
|---|--------|------|---------|------------|
| 6 | `POST` | `/api/v1/auth/logout-all` | `LogoutAll` | RateLimit(auth) + RequireAuth |

**Flow:** Revoke all refresh tokens for the authenticated user.

**Response (200):**
```json
{"message": "All sessions revoked"}
```

**DB:** UPDATE refresh_tokens SET revoked = true WHERE user_id = ?

---

### 2.6 Verify Email

| # | Method | Path | Handler | Middleware |
|---|--------|------|---------|------------|
| 7 | `GET` | `/api/v1/auth/verify-email` | `VerifyEmail` | RateLimit(auth) |

**Query params:** `?token=<verification_token>`

**Flow:**
1. Hash token (SHA-256)
2. Look up email_verifications by hash
3. Check expiry (24h TTL)
4. Set `user.email_verified = true`
5. Delete all verification tokens for user (cleanup)

**Response (200):**
```json
{"message": "Email verified successfully"}
```

**Errors:** `400` (missing/invalid/expired token)  
**DB:** SELECT email_verifications, UPDATE users, DELETE email_verifications

---

### 2.7 Resend Verification

| # | Method | Path | Handler | Middleware |
|---|--------|------|---------|------------|
| 8 | `POST` | `/api/v1/auth/resend-verification` | `ResendVerification` | RateLimit(auth) + RequireAuth |

**Flow:**
1. Fetch authenticated user
2. Check not already verified (400 if verified)
3. Delete old verification tokens
4. Generate + send new verification email

**Response (200):**
```json
{"message": "Verification email sent"}
```

**Errors:** `400` (already verified) · `404` (user not found)  
**DB:** SELECT users, DELETE email_verifications, INSERT email_verifications  
**External:** Email

---

### 2.8 Forgot Password

| # | Method | Path | Handler | Middleware |
|---|--------|------|---------|------------|
| 9 | `POST` | `/api/v1/auth/forgot-password` | `ForgotPassword` | RateLimit(auth) |

**Request:**
```json
{"email": "user@example.com"}
```

**Flow:** **Always returns 200** (anti-enumeration — doesn't reveal whether email exists). If email found: delete old reset tokens → generate new (1h TTL) → send email.

**Response (200):**
```json
{"message": "If that email is registered, a reset link has been sent"}
```

**DB:** SELECT users, DELETE password_resets, INSERT password_resets  
**External:** Email

---

### 2.9 Reset Password

| # | Method | Path | Handler | Middleware |
|---|--------|------|---------|------------|
| 10 | `POST` | `/api/v1/auth/reset-password` | `ResetPassword` | RateLimit(auth) |

**Request:**
```json
{"token": "reset_token_here", "new_password": "newsecurepass"}
```

**Flow:**
1. Hash token → lookup unused password_resets
2. Check expiry (1h)
3. Hash new password (bcrypt)
4. Update user's password hash
5. Mark reset token as used
6. Revoke ALL refresh tokens (forces re-login everywhere)

**Response (200):**
```json
{"message": "Password has been reset. Please log in again."}
```

**Errors:** `400` (invalid/expired token, password < 8 chars)  
**DB:** SELECT password_resets, SELECT users, UPDATE users.hash, UPDATE password_resets.used, UPDATE refresh_tokens.revoked

---

## 3. OAuth Endpoints

**Base path:** `/api/v1/auth/oauth/`  
**File:** `oauth.go`

### 3.1 OAuth Providers

| # | Method | Path | Handler | Middleware |
|---|--------|------|---------|------------|
| 11 | `GET` | `/api/v1/auth/oauth/providers` | `OAuthProviders` | RateLimit(oauth) |

**Response (200):**
```json
{"google": true, "github": false}
```

Returns which OAuth providers are configured (have client ID + secret set).

---

### 3.2 Google OAuth Redirect

| # | Method | Path | Handler | Middleware |
|---|--------|------|---------|------------|
| 12 | `GET` | `/api/v1/auth/oauth/google` | `OAuthGoogle` | RateLimit(oauth) |

**Flow:**
1. Generate random CSRF state token
2. Store state in secure cookie (10 min TTL, HttpOnly, SameSite=Lax)
3. Redirect (307) to Google consent screen with scopes: `openid`, `email`, `profile`

**Response:** `307 Redirect → https://accounts.google.com/o/oauth2/auth?...`

---

### 3.3 Google OAuth Callback

| # | Method | Path | Handler | Middleware |
|---|--------|------|---------|------------|
| 13 | `GET` | `/api/v1/auth/oauth/google/callback` | `OAuthGoogleCallback` | RateLimit(oauth) |

**Query params:** `?code=<auth_code>&state=<csrf_state>` (from Google)

**Flow:**
1. Verify CSRF state matches cookie value
2. Exchange auth code for access token (Google token endpoint)
3. Fetch user info from Google userinfo API
4. Find or create user:
   - If email exists: link account (set `google_id`)
   - If new: create user with auto-generated username (dedup with numeric suffix)
5. Auto-verify email
6. Issue access + refresh tokens
7. Redirect to frontend with tokens in query params

**Response:** `307 Redirect → {frontendURL}/auth/callback?access_token=...&refresh_token=...`

**DB:** SELECT/INSERT users, INSERT refresh_tokens  
**External:** Google OAuth2 token endpoint, Google Userinfo API

---

### 3.4 GitHub OAuth Redirect

| # | Method | Path | Handler | Middleware |
|---|--------|------|---------|------------|
| 14 | `GET` | `/api/v1/auth/oauth/github` | `OAuthGitHub` | RateLimit(oauth) |

**Flow:** Same pattern as Google — CSRF state cookie → redirect to GitHub consent screen.

**Response:** `307 Redirect → https://github.com/login/oauth/authorize?...`

---

### 3.5 GitHub OAuth Callback

| # | Method | Path | Handler | Middleware |
|---|--------|------|---------|------------|
| 15 | `GET` | `/api/v1/auth/oauth/github/callback` | `OAuthGitHubCallback` | RateLimit(oauth) |

**Query params:** `?code=<auth_code>&state=<csrf_state>` (from GitHub)

**Flow:** Same as Google callback. Falls back to `/user/emails` API if primary email not in `/user` response.

**DB:** SELECT/INSERT users, INSERT refresh_tokens  
**External:** GitHub OAuth2 API, GitHub User API, GitHub User/Emails API

---

## 4. Stripe Webhook

**File:** `webhook.go`

| # | Method | Path | Handler | Middleware |
|---|--------|------|---------|------------|
| 16 | `POST` | `/api/v1/webhooks/stripe` | `HandleStripeWebhook` | None (Stripe signature verification) |

**Security:** Verifies `Stripe-Signature` header using webhook secret. No JWT auth — Stripe authenticates via HMAC signature.

**Events handled:**

| Event | Action |
|---|---|
| `payment_intent.succeeded` | Find order by PaymentIntentID → verify amount/currency match → fulfil order (create purchases, clear cart from Redis) |
| `payment_intent.payment_failed` | Mark order as `failed` |
| `payment_intent.canceled` | Mark order as `failed` |

**Idempotency:** Checks order status before transitioning. Already-fulfilled orders acknowledged silently (200).

**DB:** SELECT orders, UPDATE orders, INSERT purchases, DELETE cart items  
**External:** Stripe (signature verification), Redis (cart cleanup)

---

## 5. Public Endpoints

Endpoints accessible without authentication, or with optional auth for personalization.

### 5.1 Hot Feed

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 17 | `GET` | `/api/v1/feed/hot` | `GetHotFeed` | `feed.go` | OptionalAuth |

**Query params:** `page` (default 1), `limit` (default 25, max 100)

**Flow:**
1. If **authenticated**: personalized feed — union of user's subscribed subnotery feeds + global feed. Subscribed subnoteries get 1.1x weight boost
2. If **anonymous**: global hot feed only
3. Fetch note IDs from Redis sorted sets (`ZUNIONSTORE` + `ZREVRANGE`)
4. Batch-fetch notes from DB by IDs
5. Populate subnotery names, comment counts
6. If authenticated: populate user's votes on returned notes

**Response (200):**
```json
{"notes": [...], "page": 1, "limit": 25}
```

**DB:** SELECT user_memberships, SELECT notes, SELECT subnoteries, COUNT comments  
**External:** Redis (ZUNIONSTORE, ZREVRANGE)

---

### 5.2 Note Comments

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 18 | `GET` | `/api/v1/notes/:id/comments` | `GetNoteComments` | `comment.go` | OptionalAuth |

**Query params:** `sort` (best/hot/new/old/top/controversial), `page`, `limit`, `max_depth` (default 10, max 20)

**Flow (two-phase read):**
1. Fetch paginated **root** comments with DB sort
2. Fetch **descendants** for those roots only (prevents unbounded tree traversal)
3. Cap at 500 total nodes
4. Batch-fetch usernames and user votes
5. Build tree structure, wire children, sort recursively, truncate depth

**Response (200):**
```json
{
  "comments": [{"id": 1, "body": "...", "children": [...], ...}],
  "total": 42,
  "page": 1,
  "limit": 25,
  "sort": "best",
  "truncated": false
}
```

**DB:** SELECT notes, COUNT comments (roots), SELECT comments (roots + descendants), SELECT users (usernames), SELECT comment_votes

---

### 5.3 Get Single Comment

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 19 | `GET` | `/api/v1/comments/:comment_id` | `GetComment` | `comment.go` | OptionalAuth |

**Query params:** `sort`, `max_depth`

**Flow:** Fetch target comment → verify note is approved → fetch subtree using materialized paths → build tree → truncate depth

**Response (200):** Full `CommentResponse` tree rooted at the target comment

---

### 5.4 Search

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 20 | `GET` | `/api/v1/search` | `SearchAll` | `search.go` | OptionalAuth |

**Query params:** `q` (required), `type` (notes/subnoteries/users/comments, default: notes), `page`, `limit`, `sort` (relevance/hot/new/top/comments/controversial)

**Flow:** Dispatches to type-specific search:

| Type | Search Method | Data Source |
|---|---|---|
| `notes` | Full-text search (falls back to DB ILIKE if Meilisearch unavailable) | Meilisearch / PostgreSQL |
| `subnoteries` | ILIKE on `name` | PostgreSQL |
| `users` | ILIKE on `username` / `display_name` → returns `PublicProfile` (no email/hash leak) | PostgreSQL |
| `comments` | ILIKE on `body` (approved notes only, non-deleted) | PostgreSQL |

**Response (200):**
```json
{"type": "notes", "results": [...], "total": 42, "page": 1, "limit": 25}
```

**External:** Meilisearch (for notes), PostgreSQL (for all types)

---

### 5.5 Public User Profile

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 21 | `GET` | `/api/v1/users/:id/profile` | `GetUserProfile` | `profile.go` | None |

**Flow:** Fetch user → return public fields only. Respects `profile_visibility` setting.

**Response (200):**
```json
{
  "id": 1,
  "username": "jdoe",
  "display_name": "jdoe",
  "bio": "About me",
  "avatar_url": "avatars/1/avatar.jpg",
  "post_karma": 42,
  "comment_karma": 15,
  "created_at": "2024-01-01T00:00:00Z"
}
```

**DB:** SELECT users

---

### 5.6 User Avatar Proxy

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 22 | `GET` | `/api/v1/users/:id/avatar` | `GetAvatar` | `avatar.go` | None |

**Flow:** Lookup user's `avatar_url` → fetch from R2 → stream to client with 24h cache

**Response:** Image binary with `Cache-Control: public, max-age=86400`  
**Errors:** `404` (no avatar) · `503` (R2 not configured)  
**DB:** SELECT users.avatar_url  
**External:** Cloudflare R2

---

### 5.7 User Banner Proxy

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 23 | `GET` | `/api/v1/users/:id/banner` | `GetUserBanner` | `user_banner.go` | None |

**Flow:** Same as avatar proxy but for user profile banners stored at `user-banners/{id}/banner.{ext}`

**Response:** Image binary with 24h cache  
**External:** Cloudflare R2

---

### 5.8 User Notes (Public)

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 24 | `GET` | `/api/v1/users/:id/notes` | `GetUserNotes` | `note.go` | OptionalAuth |

**Query params:** `page`, `limit`

**Flow:** Only returns **approved** notes by the given user. Populates subnotery names, comment counts. If viewer is authenticated, populates user's votes.

**Response (200):**
```json
{"notes": [...], "total": 10, "page": 1, "limit": 25}
```

---

### 5.9 User Comments (Public)

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 25 | `GET` | `/api/v1/users/:id/comments` | `GetUserComments` | `comment.go` | None |

**Query params:** `page`, `limit`

**Flow:** Flat paginated list of non-deleted comments by the user. Includes note titles for context.

**Response (200):**
```json
{
  "comments": [{"id": 1, "note_id": 5, "note_title": "...", "body": "..."}],
  "total": 3,
  "page": 1,
  "limit": 25
}
```

---

### 5.10 Note Thumbnail Proxy

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 26 | `GET` | `/api/v1/notes/:id/thumbnail` | `GetThumbnail` | `thumbnail.go` | None |

**Flow:** Fetch note → check `has_thumbnail` flag → fetch from R2 → stream with 24h cache

**Response:** Image binary  
**External:** Cloudflare R2

---

### 5.11 List Subnoteries

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 27 | `GET` | `/api/v1/subnoteries` | `ListSubnoteries` | `subnotery.go` | None |

**Query params:** `page`, `limit`

**Response (200):**
```json
{
  "subnoteries": [
    {"id": 1, "name": "math", "admin_count": 2, "member_count": 50, "created_at": "..."}
  ],
  "total": 10,
  "page": 1,
  "limit": 25
}
```

**DB:** COUNT + SELECT subnoteries with Preload Admins + Members

---

### 5.12 Subnotery Detail

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 28 | `GET` | `/api/v1/subnoteries/:subnotery_id` | `GetSubnoteryDetail` | `subnotery.go` | OptionalAuth |

**Flow:** Fetch subnotery with admins/members preloaded. If authenticated, populates `is_member` field.

**Response (200):**
```json
{
  "id": 1,
  "name": "math",
  "description": "...",
  "content_type": "...",
  "rules": "...",
  "banner_url": "...",
  "background_color": "#1a1a2e",
  "min_post_notoriety": 0,
  "min_comment_notoriety": 0,
  "admins": [{"id": 1, "username": "admin_user"}],
  "member_count": 50,
  "is_member": true,
  "created_at": "...",
  "updated_at": "..."
}
```

---

### 5.13 Subnotery Notes

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 29 | `GET` | `/api/v1/subnoteries/:subnotery_id/notes` | `GetSubnoteryNotes` | `subnotery.go` | OptionalAuth |

**Query params:** `page`, `limit`, `sort` (new/top/controversial/hot), `time` (day/week/month/year/all)

**Flow:** Only **approved** notes. Populates subnotery names, comment counts, user votes (if authenticated).

**Response (200):**
```json
{"notes": [...], "total": 25, "page": 1, "limit": 25}
```

---

### 5.14 Subnotery Banner Proxy

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 30 | `GET` | `/api/v1/subnoteries/:subnotery_id/banner` | `GetSubnoteryBanner` | `subnotery.go` | None |

**Flow:** Fetch subnotery → check `banner_url` → fetch from R2 → stream with 24h cache

**External:** Cloudflare R2

---

## 6. Authenticated Read-Only Endpoints

Require `RequireAuth` — user must be logged in but email verification NOT required.

### 6.1 Get Note by ID

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 31 | `GET` | `/api/v1/notes/:id` | `GetNoteByID` | `note.go` | RequireAuth |

**Flow:**
1. Fetch note from DB
2. If **approved**: visible to all authenticated users
3. If **not approved** (Pending/Rejected): visible only to global admins or subnotery admins (403 for others)
4. Populate subnotery name, user's vote, comment count
5. Compute `has_full_access` field via `CheckNoteAccess`

**Access levels** (for `has_full_access`):
- `AccessNone` → `false` (preview only)
- `AccessPurchased` / `AccessCreator` / `AccessSubAdmin` / `AccessGlobalAdmin` → `true` (full PDF)

**Response (200):** Full Note JSON with `has_full_access` boolean field

---

### 6.2 Approved Notes List

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 32 | `GET` | `/api/v1/notes/approved` | `GetApprovedNotes` | `note.go` | RequireAuth |

**Query params:** `page`, `limit`, `sort` (new/top/controversial/hot), `time` (day/week/month/year/all)

**Response (200):**
```json
{"notes": [...], "total": 100, "page": 1, "limit": 25}
```

---

### 6.3 Note PDF Content (Full Access)

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 33 | `GET` | `/api/v1/notes/:id/content` | `GetNotePDFContent` | `content.go` | RequireAuth |

**Access control decision tree:**

```
Note Status?
├── Pending → Admin (global/subnotery) or Creator? → Yes: serve PDF
│                                                  → No: 403
├── Approved → CheckNoteAccess:
│   ├── Creator → serve PDF
│   ├── Global Admin → serve PDF
│   ├── Subnotery Admin → serve PDF
│   ├── Purchased → serve PDF
│   ├── Free (price=0) → serve PDF
│   └── None → 403
└── Rejected → 410 Gone
```

**Response:** PDF binary stream  
**Headers:** `Content-Disposition: inline`, `Content-Type: application/pdf`, `Cache-Control: no-store`, `X-Notery-Access: full`  
**External:** Cloudflare R2

---

### 6.4 Note PDF Preview

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 34 | `GET` | `/api/v1/notes/:id/preview?pages=N` | `GetNotePreview` | `content.go` | RequireAuth |

**Flow:** Only approved notes (admins can preview non-approved). Requires `?pages=N` query parameter. Server extracts the first N pages using pdfcpu and returns a valid truncated PDF. Extracted previews are cached in R2. Returns `X-Total-Pages` header with the full page count. Admins reviewing non-approved notes receive the full PDF (no extraction).

**Response:** PDF binary  
**Headers:** `Content-Disposition: inline`, `X-Notery-Access: preview`, `X-Total-Pages: <count>`, `Cache-Control: public, max-age=3600`  
**External:** Cloudflare R2

---

### 6.5 Get Cart

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 35 | `GET` | `/api/v1/cart` | `GetCart` | `cart.go` | RequireAuth |

**Flow:** Fetch cart contents from Redis set keyed by `cart:{user_id}`

**Response (200):**
```json
{"cart": ["1", "5", "12"]}
```

**External:** Redis SMEMBERS

---

### 6.6 Check Purchase Status

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 36 | `GET` | `/api/v1/notes/:id/purchased` | `CheckPurchaseStatus` | `purchase.go` | RequireAuth |

**Flow:** Query purchases table for user + note combination.

**Response (200):**
```json
{"purchased": true, "purchased_at": "2024-01-15T10:00:00Z", "price_paid": 999}
```
or
```json
{"purchased": false}
```

---

### 6.7 My Purchases

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 37 | `GET` | `/api/v1/me/purchases` | `GetMyPurchases` | `purchase.go` | RequireAuth |

**Flow:** SELECT purchases JOIN notes for authenticated user. Returns purchased note details with purchase metadata.

**Response (200):**
```json
{"purchases": [{"note_id": 5, "title": "...", "price_paid": 500, "purchased_at": "..."}]}
```

**DB:** SELECT purchases JOIN notes

---

### 6.8 Purchase History

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 38 | `GET` | `/api/v1/me/purchases/history` | `GetPurchaseHistory` | `purchase.go` | RequireAuth |

**Query params:** `page`, `limit` (default 20)

**Response (200):**
```json
{
  "purchases": [
    {"purchase_id": 1, "note_id": 5, "note_title": "...", "price_paid": 500, "purchased_at": "...", "has_pdf": true}
  ],
  "page": 1,
  "limit": 20,
  "total": 3
}
```

---

### 6.9 My Profile

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 39 | `GET` | `/api/v1/me/profile` | `GetMyProfile` | `profile.go` | RequireAuth |

**Flow:** Returns **full** user profile including private fields (email, email_verified, profile_visibility, etc.)

**Response (200):**
```json
{
  "id": 1,
  "username": "jdoe",
  "email": "user@example.com",
  "email_verified": true,
  "bio": "...",
  "avatar_url": "...",
  "banner_url": "...",
  "profile_visibility": "public",
  "post_karma": 42,
  "comment_karma": 15,
  "is_admin": false,
  "created_at": "..."
}
```

**DB:** SELECT users

---

### 6.10 Order Status

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 40 | `GET` | `/api/v1/orders/:order_id` | `GetOrderStatus` | `purchase.go` | RequireAuth |

**Flow:** Fetch order with items → verify ownership (**403** if not owner)

**Response (200):**
```json
{
  "order_id": 1,
  "status": "fulfilled",
  "total_cents": 999,
  "items": [{"note_id": 5, "title": "...", "price_cents": 999}],
  "created_at": "...",
  "paid_at": "..."
}
```

---

### 6.11 My Notes (Creator)

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 41 | `GET` | `/api/v1/me/notes` | `GetMyNotes` | `note.go` | RequireAuth |

**Query params:** `page`, `limit`, `status` (optional: Pending/Approved/Rejected)

**Flow:** Returns notes **created by** the authenticated user, with optional status filter. Populates subnotery names, comment counts, user votes.

**Response (200):**
```json
{"notes": [...], "total": 5, "page": 1, "limit": 25}
```

---

### 6.12 My Comments

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 42 | `GET` | `/api/v1/me/comments` | `GetMyComments` | `comment.go` | RequireAuth |

**Query params:** `page`, `limit`

**Flow:** Flat paginated list of own non-deleted comments with note titles for context.

**Response (200):**
```json
{
  "comments": [
    {"id": 1, "note_id": 5, "note_title": "...", "body": "...", "upvotes": 3, "downvotes": 0, "created_at": "..."}
  ],
  "total": 10,
  "page": 1,
  "limit": 25
}
```

---

### 6.13 Get Bookmarks

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 43 | `GET` | `/api/v1/bookmarks` | `GetBookmarks` | `bookmark.go` | RequireAuth |

**Query params:** `page`, `limit`

**Flow:** JOIN bookmarks → notes (approved only), ordered by bookmark creation time DESC

**Response (200):**
```json
{"notes": [...], "total": 5, "page": 1, "limit": 25}
```

---

### 6.14 Check Bookmark

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 44 | `GET` | `/api/v1/bookmarks/:note_id` | `CheckBookmark` | `bookmark.go` | RequireAuth |

**Response (200):**
```json
{"bookmarked": true}
```

---

## 7. Verified Write Endpoints

Require `RequireAuth` + `RequireVerified` + `RateLimit(write: 60/min)`. Email must be verified to perform any write operation.

### Notes

#### 7.1 Create Note

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 45 | `POST` | `/api/v1/notes` | `CreateNote` | `note.go` | Auth + Verified + WriteRL |

**Request:**
```json
{
  "subnotery_name": "math",
  "title": "Calculus I Notes",
  "description": "Full course notes covering limits, derivatives, and integrals",
  "price": 500
}
```

**Flow:**
1. Validate required fields (subnotery_name, title)
2. Find or **auto-create** subnotery (auto-create makes user first admin + member)
3. Check `min_post_notoriety` threshold (admins exempt)
4. Create note in `Pending` status
5. Author auto-set from user's `display_name`

**Response (201):** Full Note JSON

**Errors:** `400` (missing fields, negative price) · `403` (insufficient notoriety) · `500` (DB error)  
**DB (in transaction):** SELECT/INSERT subnotery, SELECT user, INSERT user_admins, INSERT user_memberships, INSERT note

---

#### 7.2 Upload Note PDF

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 46 | `POST` | `/api/v1/notes/:id/content` | `UploadNotePDF` | `content.go` | Auth + Verified + WriteRL |

**Request:** `multipart/form-data` with `pdf` field

**Validation:**
- Only **pending** notes
- Only creator or admin can upload
- Content-Type must be `application/pdf`
- Magic bytes validated (`%PDF-` header)
- Max 50 MB

**Response (200):**
```json
{"message": "PDF uploaded successfully", "pdf_size": 1234567}
```

**External:** Cloudflare R2 (S3 PutObject)

---

#### 7.3 Upload Thumbnail

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 47 | `POST` | `/api/v1/notes/:id/thumbnail` | `UploadThumbnail` | `thumbnail.go` | Auth + Verified + WriteRL |

**Request:** `multipart/form-data` with `thumbnail` field

**Validation:** Creator only. JPEG/PNG/WebP/GIF. Max 5 MB. Magic-byte validation.

**Response (200):**
```json
{"message": "Thumbnail uploaded successfully", "thumbnail_url": "notes/1/thumbnail.jpg"}
```

**External:** Cloudflare R2

---

#### 7.4 Delete Thumbnail

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 48 | `DELETE` | `/api/v1/notes/:id/thumbnail` | `DeleteThumbnail` | `thumbnail.go` | Auth + Verified + WriteRL |

**Validation:** Creator only. 404 if no thumbnail exists.

**Response (200):**
```json
{"message": "Thumbnail deleted successfully"}
```

**External:** Cloudflare R2 (S3 DeleteObject)

---

### Voting

#### 7.5 Upvote Note

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 49 | `POST` | `/api/v1/notes/:id/upvote` | `Upvote` | `feed.go` | Auth + Verified + WriteRL |

**Flow:**
1. Delegates to `voteNote(VoteUp)`
2. **Toggle behavior:** same direction = remove vote, opposite direction = switch
3. DB transaction with atomic counter updates
4. Insert/update/delete karma_ledger entries
5. Re-read note after transaction for accurate counts
6. Recalculate hotness score

**Response (200):**
```json
{"upvotes": 42, "downvotes": 3, "hotness": 1234.56}
```

**DB (in transaction):** SELECT/INSERT/UPDATE/DELETE votes, UPDATE notes counters, INSERT karma_ledgers, UPDATE users.post_karma  
**External:** Redis HSET/HDEL (vote cache), Redis ZADD (hotness feed)

---

#### 7.6 Downvote Note

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 50 | `POST` | `/api/v1/notes/:id/downvote` | `Downvote` | `feed.go` | Auth + Verified + WriteRL |

**Flow:** Same as Upvote but with `VoteDown` direction.

---

### Cart & Checkout

#### 7.7 Add to Cart

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 51 | `POST` | `/api/v1/cart` | `AddToCart` | `cart.go` | Auth + Verified + WriteRL |

**Request:**
```json
{"item_id": "5"}
```

**Validation:** `item_id` must be a positive integer string. Note must exist and be approved. Returns **409 Conflict** if the user has already purchased this note.

**Response (200):**
```json
{"message": "Item added to cart successfully"}
```

**Error (409):**
```json
{"error": "You have already purchased this note"}
```

**DB:** SELECT notes (validation), SELECT purchases (duplicate check)  
**External:** Redis SADD

---

#### 7.8 Remove from Cart

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 52 | `DELETE` | `/api/v1/cart/:item_id` | `RemoveFromCart` | `cart.go` | Auth + Verified + WriteRL |

**Flow:** Idempotent — removing non-existent item silently succeeds.

**Response (200):**
```json
{"message": "Item removed from cart successfully"}
```

**External:** Redis SREM

---

#### 7.9 Checkout Cart

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 53 | `POST` | `/api/v1/checkout` | `CheckoutCart` | `purchase.go` | Auth + Verified + WriteRL |

**Request (optional):**
```json
{"idempotency_key": "uuid-here"}
```

**Flow:**
1. Idempotency check (return cached result if key seen before)
2. Fetch cart contents from Redis
3. Validate each item: approved, has PDF, not already purchased
4. Create Order + OrderItems in DB transaction
5. If **free** or **no payment provider**: auto-fulfil immediately
6. If **paid + Stripe configured**: create Stripe PaymentIntent

**Response (200) — auto-fulfilled:**
```json
{
  "order_id": 1,
  "status": "fulfilled",
  "purchased_count": 3,
  "total_cents": 1500
}
```

**Response (200) — Stripe:**
```json
{
  "order_id": 1,
  "status": "pending",
  "total_cents": 1500,
  "client_secret": "pi_..._secret_...",
  "payment_intent_id": "pi_..."
}
```

**DB:** SELECT notes, INSERT orders, INSERT order_items, INSERT/SELECT purchases  
**External:** Redis SMEMBERS/SREM, Stripe (CreatePaymentIntent)

---

#### 7.10 Purchase Single Note

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 54 | `POST` | `/api/v1/notes/:id/purchase` | `PurchaseSingleNote` | `purchase.go` | Auth + Verified + WriteRL |

**Request (optional):**
```json
{"idempotency_key": "uuid-here"}
```

**Validation:** Note must be approved, have PDF, not already purchased (**409** if duplicate).

**Flow:** Same as cart checkout but for a single item. Creates order → auto-fulfil (if free/no Stripe) or create PaymentIntent.

---

### Profile & Avatar

#### 7.11 Update Profile

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 55 | `PATCH` | `/api/v1/me/profile` | `UpdateMyProfile` | `profile.go` | Auth + Verified + WriteRL |

**Request (all fields optional):**
```json
{
  "username": "new_username",
  "bio": "About me",
  "avatar_url": "https://example.com/img.jpg",
  "profile_visibility": "public"
}
```

**Validation:** Username: regex validated (`helpers.ValidateUsername`). Bio: max length. Avatar URL: HTTPS only, max length. Visibility: `"public"` or `"private"`.

**Response (200):** Updated SelfProfile JSON

**Errors:** `400` (validation) · `409` (username/display_name taken)

---

#### 7.12 Upload Avatar

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 56 | `POST` | `/api/v1/me/avatar` | `UploadAvatar` | `avatar.go` | Auth + Verified + WriteRL |

**Request:** `multipart/form-data` with `avatar` field

**Validation:** Max 5 MB. JPEG/PNG/WebP/GIF. Content-Type + magic-byte dual verification.

**Response (200):**
```json
{"message": "Avatar uploaded successfully", "avatar_url": "avatars/1/avatar.jpg"}
```

**External:** Cloudflare R2

---

#### 7.13 Delete Avatar

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 57 | `DELETE` | `/api/v1/me/avatar` | `DeleteAvatar` | `avatar.go` | Auth + Verified + WriteRL |

**Response (200):**
```json
{"message": "Avatar deleted successfully"}
```

**Errors:** `404` (no avatar) · `503` (R2 not configured)  
**External:** Cloudflare R2

---

#### 7.14 Upload User Banner

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 58 | `POST` | `/api/v1/me/banner` | `UploadUserBanner` | `user_banner.go` | Auth + Verified + WriteRL |

**Request:** `multipart/form-data` with `banner` field

**Validation:** Same as avatar — 5 MB max, JPEG/PNG/WebP/GIF, magic-byte detection.

**Response (200):**
```json
{"message": "Banner uploaded successfully", "banner_url": "user-banners/1/banner.jpg"}
```

**External:** Cloudflare R2

---

#### 7.15 Delete User Banner

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 59 | `DELETE` | `/api/v1/me/banner` | `DeleteUserBanner` | `user_banner.go` | Auth + Verified + WriteRL |

**Response (200):**
```json
{"message": "Banner deleted successfully"}
```

**Errors:** `404` (no banner)  
**External:** Cloudflare R2

---

### Comments

#### 7.16 Create Comment

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 60 | `POST` | `/api/v1/notes/:id/comments` | `CreateComment` | `comment.go` | Auth + Verified + WriteRL |

**Request:**
```json
{"body": "Great notes!", "parent_id": 42}
```
`parent_id` optional — omit for top-level comment.

**Validation:**
- Body non-empty after trim, max 10,000 chars
- Parent must exist, belong to same note, not be deleted
- Max write depth enforced
- **Locked notes** reject new comments with **403**
- `min_comment_notoriety` threshold checked (admins exempt)
- Only approved notes (admins can comment on pending)

**Flow (in transaction):**
1. INSERT comment
2. Compute materialized path (parent path + own ID)
3. UPDATE path on comment

**Response (201):** Full `CommentResponse` JSON

---

#### 7.17 Edit Comment

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 61 | `PUT` | `/api/v1/comments/:comment_id` | `EditComment` | `comment.go` | Auth + Verified + WriteRL |

**Request:**
```json
{"body": "Updated text"}
```

**Validation:** Own comment only. Not deleted. Body non-empty, max length. Edit grace period (~3 min) — edits within grace period don't show "edited" indicator.

**Response (200):**
```json
{"comment_id": 1, "body": "Updated text", "is_edited": true}
```

---

#### 7.18 Delete Comment

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 62 | `DELETE` | `/api/v1/comments/:comment_id` | `DeleteComment` | `comment.go` | Auth + Verified + WriteRL |

**Flow:** Soft-delete — clears body, sets `is_deleted = true`. Tree structure preserved for threading. Owner or admin (global/subnotery) can delete.

**Response (200):**
```json
{"message": "Comment deleted"}
```

---

#### 7.19 Vote on Comment

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 63 | `POST` | `/api/v1/comments/:comment_id/vote` | `VoteComment` | `comment.go` | Auth + Verified + WriteRL |

**Request:**
```json
{"value": 1}
```
Or `{"value": -1}` for downvote.

**Flow:**
1. Reddit-style toggle (same value = remove, opposite = switch)
2. Transaction with `SELECT ... FOR UPDATE` on comment row (prevents race conditions)
3. Update `upvotes` / `downvotes` counters
4. Recalculate Wilson score
5. Insert/update karma_ledger entry for author's `comment_karma`

**Response (200):**
```json
{"comment_id": 1, "upvotes": 5, "downvotes": 1, "score": 0.72, "user_vote": 1}
```

---

#### 7.20 Remove Comment Vote

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 64 | `DELETE` | `/api/v1/comments/:comment_id/vote` | `RemoveCommentVote` | `comment.go` | Auth + Verified + WriteRL |

**Flow:** Idempotent. Transaction with FOR UPDATE. Reverse karma ledger entry. Recalculate Wilson score.

**Response (200):**
```json
{"message": "Vote removed"}
```

---

#### 7.21 Pin Comment

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 65 | `POST` | `/api/v1/comments/:comment_id/pin` | `PinComment` | `comment.go` | Auth + Verified + WriteRL |

**Flow:** Admin only (global or subnotery). Top-level comments only. Max 3 pinned per note. Sets `is_pinned = true`.

**Errors:** `400` (not top-level, deleted) · `403` (not admin) · `409` (max pinned reached)

**Response (200):**
```json
{"message": "Comment pinned"}
```

---

#### 7.22 Unpin Comment

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 66 | `DELETE` | `/api/v1/comments/:comment_id/pin` | `UnpinComment` | `comment.go` | Auth + Verified + WriteRL |

**Flow:** Admin only. Sets `is_pinned = false`.

**Response (200):**
```json
{"message": "Comment unpinned"}
```

---

### Orders

#### 7.23 Confirm Order

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 67 | `POST` | `/api/v1/orders/:order_id/confirm` | `ConfirmOrder` | `purchase.go` | Auth + Verified + WriteRL |

**Flow:** Manual reconciliation when Stripe webhook hasn't arrived. Checks Stripe PaymentIntent status directly. If succeeded: verify amount/currency → fulfil order → clear cart. If canceled: mark failed.

**Errors:** `400` (no payment provider, wrong state) · `403` (not order owner) · `409` (amount/currency mismatch) · `502` (Stripe API error)  
**External:** Stripe (RetrievePaymentIntent)

---

### Subnoteries

#### 7.24 Join Subnotery

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 68 | `POST` | `/api/v1/subnoteries/:subnotery_id/join` | `JoinSubnotery` | `subnotery.go` | Auth + Verified + WriteRL |

**Flow:** Idempotent join. If subnotery has **no admins**, joining user is auto-promoted to admin.

**Response (200):**
```json
{"message": "Joined subnotery successfully"}
```

---

#### 7.25 Leave Subnotery

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 69 | `POST` | `/api/v1/subnoteries/:subnotery_id/leave` | `LeaveSubnotery` | `subnotery.go` | Auth + Verified + WriteRL |

**Flow:**
1. If leaving user is admin:
   - If **last admin**: promote oldest remaining member (lowest `user_id`) → auto-admin succession
   - Remove from admins list
2. Remove from members list

**Response (200):**
```json
{"message": "Left subnotery successfully"}
```

---

#### 7.26 Update Subnotery Settings

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 70 | `PATCH` | `/api/v1/subnoteries/:subnotery_id/settings` | `UpdateSubnoterySettings` | `subnotery.go` | Auth + Verified + WriteRL |

**Request (all fields optional):**
```json
{
  "description": "A community for math notes",
  "content_type": "Academic",
  "rules": "1. Be respectful\n2. PDF required",
  "banner_url": "...",
  "background_color": "#1a1a2e",
  "min_post_notoriety": 5.0,
  "min_comment_notoriety": 2.0
}
```

**Validation:** Admin only (global or subnotery). Self-checks admin permission within handler (not via RequireAdmin middleware chain).

**Response (200):**
```json
{"message": "Settings updated successfully", ...}
```

---

#### 7.27 Upload Subnotery Banner

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 71 | `POST` | `/api/v1/subnoteries/:subnotery_id/banner` | `UploadSubnoteryBanner` | `subnotery.go` | Auth + Verified + WriteRL |

**Request:** `multipart/form-data` with `banner` field

**Validation:** Admin only. Max 5 MB. JPEG/PNG/WebP/GIF. Magic-byte detection.

**Response (200):**
```json
{"message": "Banner uploaded successfully", "banner_url": "banners/1/banner.jpg"}
```

**External:** Cloudflare R2

---

#### 7.28 Delete Subnotery Banner

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 72 | `DELETE` | `/api/v1/subnoteries/:subnotery_id/banner` | `DeleteSubnoteryBanner` | `subnotery.go` | Auth + Verified + WriteRL |

**Flow:** Admin only. Clears `banner_url` in DB. Best-effort R2 cleanup (tries all extensions).

**Response (200):**
```json
{"message": "Banner deleted successfully"}
```

---

#### 7.29 Remove Admin from Subnotery

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 73 | `DELETE` | `/api/v1/subnoteries/:subnotery_id/admins/:uid` | `RemoveAdminFromSubnotery` | `subnotery.go` | Auth + Verified + WriteRL |

**Flow:**
- **Hierarchy enforcement:** Older admins (lower `user_admins.id` row) can remove younger admins
- **Global admins** can remove anyone
- Cannot remove yourself (use Leave instead)

**Errors:** `400` (self-removal) · `403` (not admin, insufficient seniority) · `404` (target not admin)

**Response (200):**
```json
{"message": "Admin removed successfully"}
```

---

### Bookmarks

#### 7.30 Add Bookmark

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 74 | `POST` | `/api/v1/bookmarks/:note_id` | `AddBookmark` | `bookmark.go` | Auth + Verified + WriteRL |

**Flow:** Only approved notes. Idempotent — duplicate returns `200` with "Already bookmarked".

**Response:** `201` `{"message": "Bookmark added", "bookmarked": true}` or `200` if already exists

---

#### 7.31 Remove Bookmark

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 75 | `DELETE` | `/api/v1/bookmarks/:note_id` | `RemoveBookmark` | `bookmark.go` | Auth + Verified + WriteRL |

**Flow:** Idempotent — removing non-existent bookmark silently succeeds.

**Response (200):**
```json
{"message": "Bookmark removed", "bookmarked": false}
```

---

## 8. Admin Endpoints

Require full admin middleware chain: `RequireAuth` + `RequireVerified` + `RateLimit(write: 60/min)` + `RequireAdmin`.

### 8.1 Pending Notes

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 76 | `GET` | `/api/v1/notes/pending` | `GetPendingNotes` | `note.go` | Admin |

**Query params:** `page`, `limit`, `subnotery_id` (optional filter)

**Flow:**
- **Global admins:** see all pending notes across all subnoteries
- **Subnotery admins:** see only pending notes from subnoteries they admin (via JOIN on `user_admins`)

**Response (200):**
```json
{"notes": [...], "total": 5, "page": 1, "limit": 25}
```

---

### 8.2 Approve Note

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 77 | `PATCH` | `/api/v1/notes/:id/approve` | `ApproveNote` | `note.go` | Admin |

**Flow:**
1. Validate note has PDF uploaded
2. Update status to `Approved`
3. Index note in Meilisearch (for full-text search)
4. Add to Redis hot feed (ZADD with hotness score)
5. Delete admin review comments (cleanup)
6. **Rollback** status if Meilisearch indexing fails

**Response (200):**
```json
{"message": "Note approved successfully"}
```

**External:** Meilisearch (AddDocuments), Redis (ZADD)

---

### 8.3 Reject Note

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 78 | `PATCH` | `/api/v1/notes/:id/reject` | `RejectNote` | `note.go` | Admin |

**Flow:**
1. Update status to `Rejected`
2. Remove from Meilisearch + Redis feed (if was previously approved)
3. Delete all comments and votes on the note
4. Delete note record from DB
5. Cleanup PDF and thumbnail from R2

**Response (200):**
```json
{"message": "Note rejected successfully"}
```

---

### 8.4 Delete Note

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 79 | `DELETE` | `/api/v1/notes/:id` | `DeleteNote` | `note.go` | Admin |

**Flow:**
1. Remove from Meilisearch + Redis feed (if approved)
2. Delete note from DB
3. Cleanup PDF and thumbnail from R2
4. Re-indexes on delete failure rollback

**Response (200):**
```json
{"message": "Note deleted successfully"}
```

---

### 8.5 Lock Note

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 80 | `PATCH` | `/api/v1/notes/:id/lock` | `LockNote` | `note.go` | Admin |

**Flow:** Sets `is_locked = true`. Locked notes **reject new comments** with 403.

**Response (200):**
```json
{"message": "Note locked successfully"}
```

---

### 8.6 Unlock Note

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 81 | `PATCH` | `/api/v1/notes/:id/unlock` | `UnlockNote` | `note.go` | Admin |

**Response (200):**
```json
{"message": "Note unlocked successfully"}
```

---

### 8.7 Admin Preview PDF

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 82 | `GET` | `/api/v1/admin/notes/:id/preview` | `AdminPreviewPDF` | `content.go` | Admin |

**Flow:** Alias for `GetNotePDFContent` — same access control applies. Provides clearer admin-specific semantics for reviewing pending notes.

**Response:** PDF binary stream

---

### 8.8 Delete Note PDF

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 83 | `DELETE` | `/api/v1/admin/notes/:id/content` | `DeleteNotePDF` | `content.go` | Admin |

**Flow:** Delete PDF from R2 → update note metadata (`has_pdf = false`, `pdf_size = 0`)

**Response (200):**
```json
{"message": "PDF deleted successfully"}
```

**External:** Cloudflare R2

---

### 8.9 Add Admin to Subnotery

| # | Method | Path | Handler | File | Middleware |
|---|--------|------|---------|------|------------|
| 84 | `POST` | `/api/v1/subnoteries/:subnotery_id/admins` | `AddAdminToSubnotery` | `subnotery.go` | Admin |

**Request:**
```json
{"email": "user@example.com"}
```

**Flow:** Find subnotery → find user by email → append to Admins association

**Response (200):**
```json
{"message": "Admin added to subnotery successfully"}
```

**Errors:** `404` (subnotery or user not found)

---

## Summary Statistics

| Category | Count |
|---|---|
| **Total Endpoints** | **84** |
| Public (no auth) | 12 |
| Public (optional auth) | 7 |
| Authenticated read-only | 14 |
| Verified write | 31 |
| Admin | 9 |
| Auth (rate-limited) | 8 |
| OAuth | 5 |
| Webhook | 1 |

### External Service Dependencies

| Service | Usage |
|---|---|
| **PostgreSQL (GORM)** | All endpoints — models, queries, transactions |
| **Redis** | Cart (SADD/SMEMBERS/SREM), Hot Feed (ZADD/ZREVRANGE/ZUNIONSTORE), Vote Cache (HSET/HDEL), Rate Limiting (INCR/ExpireNX) |
| **Cloudflare R2** | PDF storage/retrieval, Avatar upload/proxy, Thumbnail upload/proxy, User Banner upload/proxy, Subnotery Banner upload/proxy |
| **Meilisearch** | Note indexing on approval, Full-text note search |
| **Stripe** | PaymentIntent create/retrieve, Webhook signature verification |
| **SMTP/Email** | Verification emails, Password reset emails |
| **Google OAuth** | User consent redirect, Token exchange, Userinfo fetch |
| **GitHub OAuth** | User consent redirect, Token exchange, User/Emails fetch |

### Handler File Index

| File | Endpoints | Domain |
|---|---|---|
| `auth.go` | 10 | Signup, login, tokens, email verify, password reset |
| `oauth.go` | 5 | Google + GitHub OAuth flows |
| `webhook.go` | 1 | Stripe payment webhook |
| `feed.go` | 3 | Hot feed, upvote, downvote |
| `note.go` | 9 | CRUD notes, pending/approved lists, lock/unlock |
| `content.go` | 5 | PDF upload/download/preview/delete |
| `thumbnail.go` | 3 | Thumbnail upload/proxy/delete |
| `comment.go` | 10 | CRUD comments, voting, pinning, user comments |
| `purchase.go` | 7 | Checkout, single purchase, history, orders |
| `cart.go` | 3 | Cart add/remove/get |
| `profile.go` | 3 | Profile get/update (self + public) |
| `avatar.go` | 3 | Avatar upload/proxy/delete |
| `user_banner.go` | 3 | User banner upload/proxy/delete |
| `bookmark.go` | 4 | Bookmark add/remove/check/list |
| `subnotery.go` | 11 | Subnotery CRUD, membership, admin management, banners |
| `search.go` | 1 | Multi-type search |
