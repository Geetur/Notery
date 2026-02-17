# Notery

A Go REST API for a Reddit-like note marketplace — users sign up, creators publish notes (with PDF content), admins approve them, buyers purchase, and everyone votes, comments, and browses a hot feed.

<img width="1024" height="1024" alt="Scribbled N logo in ink" src="https://github.com/user-attachments/assets/b7ec42b5-f272-4b4a-ac7d-d0b6e6a99394" />

## Features

- **Authentication** — JWT (HS256) with refresh token rotation; 15-min access tokens, 30-day refresh tokens with family-based theft detection
- **Email Verification** — Token-based email verification with configurable SMTP (SMTP/Log/Mock mailer)
- **Password Management** — Secure forgot-password / reset-password flows with single-use tokens; change-password with session revocation
- **User Profiles** — Display name, bio, avatar URL, public/private visibility; PATCH partial updates with regex validation
- **Notes** — CRUD with admin approval lifecycle (Pending → Approved → Rejected)
- **PDF Content** — Secure upload to Cloudflare R2, proxy-only viewing, magic-byte (`%PDF-`) validation
- **Subnoteries** — Community-based note organisation with scoped admin controls
- **Shopping Cart** — Redis-backed cart for multi-note checkout
- **Purchases & Orders** — State machine (pending → paid → fulfilled), idempotency keys, Stripe PaymentIntent integration
- **Voting & Hot Feed** — Reddit-style hotness algorithm; DB-authoritative votes with Redis cache
- **Comments** — Threaded tree with Wilson score ranking, soft-delete, depth caps, per-comment voting
- **Search** — Reddit-style multi-type search (notes via Meilisearch, subnoteries/users/comments via DB)
- **Rate Limiting** — Redis sliding-window per-user; three tiers: auth (5/min), write (60/min), read (120/min)
- **Security Headers** — X-Content-Type-Options, X-Frame-Options, Referrer-Policy, Permissions-Policy
- **CORS** — Configurable origins via `CORS_ORIGINS` env var
- **Role-Based Access** — Global admins, subnotery-scoped admins, creators, purchasers

## Tech Stack

| Component | Technology |
|-----------|------------|
| Language | Go |
| Framework | [Gin](https://github.com/gin-gonic/gin) |
| Database | PostgreSQL 16 (via GORM) |
| Cache | Redis 7 |
| Search | [Meilisearch](https://www.meilisearch.com/) |
| Storage | Cloudflare R2 (S3-compatible) |
| Payments | [Stripe](https://stripe.com/) PaymentIntent API |
| Auth | JWT HS256 |

## Architecture

```
Client → Gin Router → Middleware (Auth / Admin / RateLimit / Security) → Handlers → DB / Redis / Meili / R2 / Stripe
```

All handler methods live on a single `App` struct that holds every dependency (DB, Redis, R2, Meili, JWT secret, payment service). Dependencies are injected once at startup.

## Project Structure

```
Notery/
├── cmd/api/
│   └── main.go                    # Entrypoint: config, init, routes, graceful shutdown
├── internal/
│   ├── config/
│   │   └── config.go              # Centralised env loading (JWT, Stripe, CORS)
│   ├── database/
│   │   ├── database.go            # PostgreSQL init + migrations (Package doc)
│   │   ├── redis.go               # Redis init
│   │   ├── meilisearch.go         # Meilisearch init
│   │   └── r2.go                  # Cloudflare R2 client (PDF storage)
│   ├── email/
│   │   └── email.go               # Mailer interface (SMTP/Log/Mock) + templates (Package doc)
│   ├── handlers/
│   │   ├── app.go                 # App struct + constructor (Package doc)
│   │   ├── auth.go                # Auth: signup, login, refresh, logout, verify, reset, change-password
│   │   ├── cart.go                # Cart CRUD (Redis)
│   │   ├── comment.go             # Threaded comments, voting, tree assembly
│   │   ├── content.go             # PDF upload / view / delete + access control
│   │   ├── feed.go                # Hot feed, note voting (unified handler)
│   │   ├── note.go                # Note CRUD, approve/reject, Meili indexing
│   │   ├── profile.go             # User profile management
│   │   ├── purchase.go            # Checkout, orders, purchase history
│   │   ├── search.go              # Multi-type search (notes/subnoteries/users/comments)
│   │   ├── subnotery.go           # Join subnotery, add admin
│   │   └── webhook.go             # Stripe webhook (signature-verified)
│   ├── helpers/
│   │   ├── helpers.go             # Pagination, logging, binding, auth context (Package doc)
│   │   ├── cart.go                # Cart Redis key builder
│   │   ├── comment.go             # Comment ID parsing
│   │   ├── note.go                # Note fetch/parse helpers
│   │   ├── subnotery.go           # Subnotery fetch/parse helpers
│   │   ├── user.go                # User fetch helpers
│   │   └── validation.go          # Username/display name validation, whitespace normalisation
│   ├── middleware/
│   │   ├── auth.go                # RequireAuth / OptionalAuth (shared JWT parsing)
│   │   ├── admin.go               # RequireAdmin (global or subnotery-scoped)
│   │   ├── ratelimit.go           # Redis sliding-window rate limiter
│   │   └── security.go            # Security response headers
│   ├── models/
│   │   ├── user.go                # User + bcrypt + profile DTOs (Package doc)
│   │   ├── session.go             # RefreshToken, EmailVerification, PasswordReset + crypto helpers
│   │   ├── note.go                # Note + NoteStatus enum
│   │   ├── comment.go             # Comment + CommentVote + Wilson score
│   │   ├── order.go               # Order + OrderItem + state machine
│   │   ├── purchase.go            # Purchase record
│   │   ├── subnotery.go           # Subnotery model
│   │   └── vote.go                # Vote + VoteDirection enum
│   └── payment/
│       ├── payment.go             # Service interface + types (Package doc)
│       ├── stripe.go              # Stripe implementation
│       └── mock.go                # Test double with call tracking
├── docs/
│   ├── PAYMENT_SYSTEM.md          # Payment architecture deep-dive
│   └── PDF_CONTENT_SYSTEM.md      # PDF architecture deep-dive
├── scripts/
│   ├── test-hot-feed.ps1          # Hot feed E2E test
│   ├── test-pdf-workflow.ps1      # PDF upload/purchase workflow
│   ├── test-comments.ps1          # Comment system E2E test
│   └── k6/                        # Load test scripts
├── docker-compose.yml
├── go.mod
├── AGENTS.md                      # AI contributor onboarding guide
└── README.md
```

## Getting Started

### Prerequisites

- Go 1.22+
- Docker & Docker Compose

### Environment Variables

Create a `.env` file in the project root:

```env
# PostgreSQL
DB_HOST=localhost
DB_PORT=5432
DB_USER=admin
DB_PASSWORD=yourpassword
DB_NAME=notery_db
DB_SSLMODE=disable
DB_TIMEZONE=UTC

# Redis
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=yourredispassword
REDIS_DB=0

# Meilisearch
MEILISEARCH_HOST=http://localhost:7700
MEILISEARCH_MASTER_KEY=yourmeilisearchkey
MEILISEARCH_INDEX=notes

# Cloudflare R2
R2_ACCOUNT_ID=your_cloudflare_account_id
R2_ACCESS_KEY_ID=your_r2_access_key
R2_SECRET_ACCESS_KEY=your_r2_secret_key
R2_BUCKET_NAME=notery-pdfs

# JWT
JWT_SECRET=your-super-secret-key

# CORS (comma-separated, defaults to localhost:3000,localhost:5173)
CORS_ORIGINS=http://localhost:3000,http://localhost:5173

# Base URL (used in email links, defaults to http://localhost:8080)
BASE_URL=http://localhost:8080

# SMTP (optional — omit for log-to-stdout dev mode)
SMTP_HOST=smtp.example.com
SMTP_PORT=587
SMTP_USER=your_smtp_user
SMTP_PASS=your_smtp_password
SMTP_FROM=noreply@notery.app

# Stripe (optional — omit for auto-fulfil dev mode)
STRIPE_SECRET_KEY=sk_test_xxx
STRIPE_WEBHOOK_SECRET=whsec_xxx
```

### Running Locally

```bash
# Start infrastructure
docker-compose up -d

# Run the API
go run cmd/api/main.go

# Health check
curl http://localhost:8080/health
```

## API Endpoints

### Public

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check |
| POST | `/api/v1/signup` | Register (legacy) |
| POST | `/api/v1/login` | Authenticate (legacy) |
| POST | `/api/v1/auth/signup` | Register + issue tokens + send verification email |
| POST | `/api/v1/auth/login` | Authenticate + issue tokens |
| POST | `/api/v1/auth/refresh` | Rotate refresh token |
| POST | `/api/v1/auth/logout` | Revoke refresh token |
| POST | `/api/v1/auth/forgot-password` | Request password reset email |
| POST | `/api/v1/auth/reset-password` | Reset password with token |
| GET  | `/api/v1/auth/verify-email` | Verify email via token |
| POST | `/api/v1/webhooks/stripe` | Stripe webhook (signature-verified) |

### Public (Optional Auth)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v1/feed/hot` | Hot feed (personalised if authenticated) |
| GET | `/api/v1/notes/:id/comments` | Threaded comments (user_vote if logged in) |
| GET | `/api/v1/comments/:comment_id` | Single comment subtree |
| GET | `/api/v1/users/:id/profile` | Public user profile |
| GET | `/api/v1/search?q=&type=` | Multi-type search (notes/subnoteries/users/comments) |

### Protected (Requires JWT)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v1/notes/:id` | Get note by ID |
| POST | `/api/v1/notes` | Create note |
| GET | `/api/v1/notes/approved` | List approved notes (paginated) |
| POST | `/api/v1/notes/:id/content` | Upload PDF |
| GET | `/api/v1/notes/:id/content` | View/stream PDF |
| POST | `/api/v1/notes/:id/upvote` | Upvote note |
| POST | `/api/v1/notes/:id/downvote` | Downvote note |
| GET | `/api/v1/cart` | View cart |
| POST | `/api/v1/cart` | Add to cart |
| DELETE | `/api/v1/cart/:item_id` | Remove from cart |
| POST | `/api/v1/checkout` | Checkout cart |
| POST | `/api/v1/notes/:id/purchase` | Direct purchase |
| GET | `/api/v1/orders/:order_id` | Order status |
| POST | `/api/v1/orders/:order_id/confirm` | Manual reconciliation |
| GET | `/api/v1/notes/:id/purchased` | Check purchase status |
| GET | `/api/v1/me/purchases` | My purchased notes |
| GET | `/api/v1/me/purchases/history` | Purchase history (paginated) |
| GET | `/api/v1/me/profile` | Own profile |
| PATCH | `/api/v1/me/profile` | Update profile |
| POST | `/api/v1/auth/logout-all` | Revoke all refresh tokens |
| POST | `/api/v1/auth/resend-verification` | Resend verification email |
| POST | `/api/v1/auth/change-password` | Change password (revokes sessions) |
| POST | `/api/v1/subnoteries/:id/join` | Join subnotery |
| POST | `/api/v1/notes/:id/comments` | Create comment |
| PUT | `/api/v1/comments/:comment_id` | Edit comment |
| DELETE | `/api/v1/comments/:comment_id` | Soft-delete comment |
| POST | `/api/v1/comments/:comment_id/vote` | Vote on comment |
| DELETE | `/api/v1/comments/:comment_id/vote` | Remove comment vote |

### Admin (Global or Subnotery-Scoped)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v1/notes/pending` | List pending notes (paginated) |
| PATCH | `/api/v1/notes/:id/approve` | Approve note |
| PATCH | `/api/v1/notes/:id/reject` | Reject note |
| DELETE | `/api/v1/notes/:id` | Delete note |
| GET | `/api/v1/admin/notes/:id/preview` | Preview PDF |
| DELETE | `/api/v1/admin/notes/:id/content` | Delete PDF |
| POST | `/api/v1/subnoteries/:id/admins` | Add admin |

## Key Design Decisions

### Prices in Cents
All monetary values stored as `int64` cents (499 = $4.99) to avoid floating-point errors.

### DB-Authoritative Votes
The `votes` table is the single source of truth. Each vote runs in a DB transaction that atomically updates the vote row and note counters. Redis is a best-effort cache.

### Unified Vote Handler
`Upvote()` and `Downvote()` are thin wrappers around a single `voteNote(direction)` method, eliminating ~90 lines of duplication.

### JWT Middleware
`RequireAuth` and `OptionalAuth` share a common `parseJWTUserID()` helper, eliminating ~40 lines of duplicated token parsing.

### Refresh Token Rotation
Family-based rotation with theft detection. Reusing a revoked token revokes the entire token family (all sessions for that login). See `models/session.go` for crypto helpers.

### Email Verification & Password Reset
Configurable Mailer interface (SMTP for production, LogMailer for dev, MockMailer for tests). Anti-enumeration on forgot-password (always returns 200). Single-use reset tokens with 1-hour TTL. Password changes revoke all active sessions.

### Payment Integration
`payment.Service` interface backed by Stripe in production, configurable mock in tests. Fulfilment is webhook-authoritative. When Stripe is not configured, orders auto-fulfil. See [docs/PAYMENT_SYSTEM.md](docs/PAYMENT_SYSTEM.md).

### Comment System
Tree assembled in O(n) per request. Wilson score ranking (no time decay). Atomic votes with `SELECT ... FOR UPDATE`. Soft-delete preserves tree shape. Write depth cap = 15, read depth cap = 10 with `has_more_replies`. Public reads, no auth required.

### Search
Reddit-style multi-type search: `?q=world+news&type=notes|subnoteries|users|comments`. Notes searched via Meilisearch full-text; other types via DB ILIKE. All responses paginated.

## Testing

```bash
# All tests
go test ./...

# Specific package
go test ./internal/handlers -count=1

# With race detection
go test -race ./...
```

### E2E Scripts (require running server)

```powershell
.\scripts\test-hot-feed.ps1
.\scripts\test-pdf-workflow.ps1
.\scripts\test-comments.ps1
```

## Frontend

The `frontend/` directory contains a production-quality Next.js 14 frontend with:

- Reddit-style feed, voting, threaded comments
- Note marketplace (browse, purchase, download PDFs)
- Dark mode, responsive 3-column layout
- 54 tests, TypeScript, TailwindCSS, shadcn/ui

```bash
cd frontend
npm install
npm run dev    # http://localhost:3000
npm test       # 54 tests
```

See [frontend/README.md](frontend/README.md) for full documentation.

## License

Copyright (c) 2026 Jeter Pontes. All rights reserved.

This repository is proprietary. No permission is granted to use, copy, modify, or distribute this code without explicit written permission.
