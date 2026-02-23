# Notery

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="frontend/public/notery-logo.png" />
  <source media="(prefers-color-scheme: light)" srcset="frontend/public/notery-logo.png" />
  <img alt="Notery — scribbled N logo" src="frontend/public/notery-logo.png" width="128" />
</picture>

A full-stack Reddit-style note marketplace. Creators publish study notes with PDF content, admins curate communities, buyers purchase, and everyone votes, comments, and discovers content through a ranked hot feed.

**Backend:** Go · Gin · PostgreSQL · Redis · Meilisearch · Cloudflare R2 · Stripe
**Frontend:** Next.js 15 · TypeScript · Tailwind CSS · shadcn/ui · Zustand · React Query

---

## Table of Contents

- [Features](#features)
- [Tech Stack](#tech-stack)
- [Architecture](#architecture)
- [Project Structure](#project-structure)
- [Getting Started](#getting-started)
- [Environment Variables](#environment-variables)
- [API Reference](#api-reference)
- [Data Models](#data-models)
- [Notoriety System](#notoriety-system)
- [Testing](#testing)
- [Contributing](#contributing)
- [License](#license)

---

## Features

### Core Platform

| Feature | Description |
|---------|-------------|
| **Authentication** | JWT HS256 access tokens (15 min) + family-based refresh token rotation (30 days) with theft detection |
| **OAuth2** | Google and GitHub social login with account linking and auto-verification |
| **Email Verification** | Token-based verification with configurable mailer (SMTP / Log / Mock); dev-mode auto-verify |
| **Password Reset** | Forgot-password + reset-password with single-use tokens (1h TTL); anti-enumeration (always 200) |
| **Notes Marketplace** | CRUD with admin approval lifecycle (Pending → Approved → Rejected) |
| **PDF System** | Secure R2 upload, proxy-only in-app viewing via react-pdf, magic-byte validation, preview truncation |
| **Subnoteries** | Reddit-style community containers with scoped admin controls, settings, and rules |
| **Notoriety (Karma)** | Post and comment karma with logarithmic diminishing returns and confidence gating |
| **Voting** | Reddit-style upvote/downvote on notes and comments; DB-authoritative with Redis cache |
| **Hot Feed** | Reddit-style hotness ranking with time decay |
| **Threaded Comments** | Tree with Wilson score ranking, materialized paths, soft-delete, depth caps |
| **Search** | Multi-type: notes (Meilisearch full-text), subnoteries/users/comments (DB ILIKE) |
| **Shopping Cart** | Redis-backed cart for multi-note checkout |
| **Purchases & Orders** | State machine (pending → paid → fulfilled), idempotency keys, Stripe PaymentIntents |
| **Avatars** | Upload (JPEG/PNG/WebP/GIF ≤ 5 MB), magic-byte validation, R2 storage, 24h cached proxy |
| **Thumbnails** | Optional note thumbnail images with the same validation pipeline as avatars |
| **Rate Limiting** | Redis sliding-window; three tiers: auth (5/min), write (60/min), read (120/min) |

### Frontend

| Feature | Description |
|---------|-------------|
| **Reddit-style layout** | Three-column responsive layout with left nav, content feed, right sidebar |
| **Feed sorting** | Hot, New, Top, Controversial with time filtering |
| **In-app PDF viewer** | react-pdf based viewer with page navigation; preview mode (1 page per 5 total) |
| **Threaded comments** | Nested threads with collapse, reply, edit, delete, and per-comment voting |
| **User profiles** | Public profiles with post/comment notoriety stats; settings, avatar upload |
| **Community pages** | Subnotery browsing, admin settings (description, rules, min notoriety), pending notes panel |
| **Dark mode** | Full light/dark theme support |
| **Auth flows** | Login, signup, forgot password, OAuth (Google/GitHub), email verification banners |
| **Search** | Multi-type search with autocomplete |

---

## Tech Stack

| Layer | Technology | Purpose |
|-------|-----------|---------|
| **Backend** | [Go](https://go.dev/) 1.25 | API language |
| **HTTP Framework** | [Gin](https://github.com/gin-gonic/gin) 1.11 | Router + middleware |
| **ORM** | [GORM](https://gorm.io/) 1.26 | Database access + migrations |
| **Database** | [PostgreSQL](https://www.postgresql.org/) 16 | Primary data store |
| **Cache** | [Redis](https://redis.io/) 7 | Cart, rate limiting, hot feed, vote cache |
| **Search** | [Meilisearch](https://www.meilisearch.com/) | Full-text note search |
| **Object Storage** | [Cloudflare R2](https://developers.cloudflare.com/r2/) / [MinIO](https://min.io/) (dev) | PDF + avatar + thumbnail storage |
| **Payments** | [Stripe](https://stripe.com/) | PaymentIntent API with webhook fulfilment |
| **Frontend** | [Next.js](https://nextjs.org/) 15 (App Router) | React framework |
| **Language** | [TypeScript](https://www.typescriptlang.org/) 5 | Frontend type safety |
| **Styling** | [Tailwind CSS](https://tailwindcss.com/) 3.4 | Utility-first CSS |
| **Components** | [shadcn/ui](https://ui.shadcn.com/) | 20+ accessible UI components |
| **State** | [Zustand](https://github.com/pmndrs/zustand) 5 | Client state management |
| **Server State** | [React Query](https://tanstack.com/query) 5 | Data fetching + caching |
| **PDF** | [react-pdf](https://github.com/wojtekmaj/react-pdf) | In-app PDF rendering |
| **Testing** | Go `testing`, [Jest](https://jestjs.io/), [k6](https://k6.io/) | Unit, integration, load tests |

---

## Architecture

```
Browser (Next.js :3000)
   ↓ HTTP / REST
Go API (Gin :8080)
   ↓
┌──────────────┬────────────┬──────────────┬────────────┐
│ PostgreSQL   │   Redis    │ Meilisearch  │ R2 / MinIO │
│ (data +      │ (cache,    │ (full-text   │ (PDF,      │
│  migrations) │  cart,     │  search)     │  avatars,  │
│              │  rate      │              │  thumbs)   │
│              │  limits)   │              │            │
└──────────────┴────────────┴──────────────┴────────────┘
                     │
                  Stripe (payments)
```

All handler methods live on a single `App` struct that holds every dependency (DB, Redis, R2, Meili, JWT secret, payment service, mailer). Dependencies are injected once at startup via `cmd/api/main.go`.

### Key Design Decisions

- **Prices in cents** — All monetary values stored as `int64` cents (499 = $4.99) to avoid floating-point errors
- **DB-authoritative votes** — The `votes` table is the single source of truth; Redis is best-effort cache
- **Family-based refresh tokens** — Reusing a revoked token revokes the entire family (theft detection)
- **Configurable payments** — `payment.Service` interface: Stripe in prod, auto-fulfil in dev, mock in tests
- **Notoriety ledger** — Every vote's karma delta is stored for exact reversal on toggle/switch
- **O(n) comment trees** — Two-phase read model: paginated roots → descendant fetch → in-memory assembly

---

## Project Structure

```
Notery/
├── cmd/api/
│   └── main.go                    # Entrypoint: config, init, routes, graceful shutdown
├── internal/
│   ├── config/config.go           # Centralised env loading (JWT, Stripe, CORS, OAuth, SMTP)
│   ├── database/
│   │   ├── database.go            # PostgreSQL init + auto-migrations (13 models)
│   │   ├── redis.go               # Redis connection
│   │   ├── meilisearch.go         # Meilisearch client + index setup
│   │   └── r2.go                  # R2/MinIO S3 client (PDF, avatar, thumbnail storage)
│   ├── email/
│   │   ├── email.go               # Mailer interface (SMTP / LogMailer / MockMailer) + templates
│   │   └── email_test.go
│   ├── handlers/                  # HTTP handlers (one file per domain)
│   │   ├── app.go                 # App struct with all dependencies
│   │   ├── auth.go                # Signup, login, refresh, logout, verify, reset, OAuth
│   │   ├── avatar.go              # Avatar upload, proxy, delete
│   │   ├── cart.go                # Redis cart CRUD
│   │   ├── comment.go             # Threaded comments, voting, tree assembly
│   │   ├── content.go             # PDF upload, view, preview, access control
│   │   ├── feed.go                # Hot feed, note voting, hotness scoring
│   │   ├── note.go                # Note CRUD, approve/reject, Meili indexing
│   │   ├── oauth.go               # Google + GitHub OAuth handlers
│   │   ├── profile.go             # User profile management
│   │   ├── purchase.go            # Checkout, orders, purchase history
│   │   ├── search.go              # Multi-type search
│   │   ├── subnotery.go           # Community browse, join, admin, settings
│   │   ├── thumbnail.go           # Note thumbnail upload + proxy
│   │   ├── webhook.go             # Stripe webhook (signature-verified)
│   │   └── *_test.go              # 190+ handler tests
│   ├── helpers/                   # Shared utilities
│   │   ├── helpers.go             # Pagination, logging, JSON binding, auth context
│   │   ├── validation.go          # Username/display name regex validation
│   │   ├── cart.go, comment.go    # Domain-specific parse/fetch helpers
│   │   ├── note.go, subnotery.go  # Domain-specific parse/fetch helpers
│   │   └── user.go                # User fetch helpers
│   ├── middleware/
│   │   ├── auth.go                # RequireAuth / OptionalAuth (shared JWT parsing, ?token= fallback)
│   │   ├── admin.go               # RequireAdmin (global or subnotery-scoped)
│   │   ├── ratelimit.go           # Redis sliding-window rate limiter (3 tiers)
│   │   ├── security.go            # Security response headers
│   │   ├── verified.go            # Email verification enforcement
│   │   └── auth_test.go
│   ├── models/                    # Domain models + algorithms
│   │   ├── user.go                # User + bcrypt + profile DTOs + notoriety fields
│   │   ├── karma.go               # KarmaLedger model + karma calculation algorithms
│   │   ├── session.go             # RefreshToken, EmailVerification, PasswordReset
│   │   ├── note.go                # Note + NoteStatus enum
│   │   ├── comment.go             # Comment + CommentVote + Wilson score
│   │   ├── vote.go                # Vote + VoteDirection enum
│   │   ├── order.go               # Order + OrderItem + state machine
│   │   ├── purchase.go            # Purchase record
│   │   ├── bookmark.go            # Bookmark model
│   │   ├── subnotery.go           # Subnotery + min notoriety settings
│   │   └── *_test.go              # 70+ model tests
│   └── payment/
│       ├── payment.go             # Service interface + types
│       ├── stripe.go              # Stripe implementation
│       ├── mock.go                # Test double with call tracking
│       └── payment_test.go
├── frontend/                      # Next.js 15 App Router frontend
│   ├── src/
│   │   ├── app/                   # 15+ pages (home, hot, new, login, signup, submit, ...)
│   │   ├── components/            # feed/, comments/, layout/, ui/, pdf-viewer
│   │   ├── hooks/                 # use-toast
│   │   ├── lib/                   # api-client, config, format, utils
│   │   ├── services/              # API service layer (8 domain files)
│   │   ├── stores/                # Zustand stores (auth, feed, sidebar)
│   │   └── types/                 # TypeScript API types
│   ├── public/                    # Static assets (logo, PDF.js worker)
│   └── package.json
├── scripts/
│   ├── test-all.ps1               # Unified test runner (Go + frontend + optional E2E + k6)
│   ├── test-comments.ps1          # Comment system E2E
│   ├── test-hot-feed.ps1          # Hot feed E2E
│   ├── test-pdf-workflow.ps1      # PDF upload/purchase E2E
│   └── k6/                        # Load tests (smoke, load, auth-flow, stress)
├── docs/
│   ├── DEEP_ANALYSIS.md           # Architecture deep-dive
│   ├── PAYMENT_SYSTEM.md          # Payment flow documentation
│   └── PDF_CONTENT_SYSTEM.md      # PDF access control documentation
├── .github/workflows/
│   └── ci.yml                     # CI/CD pipeline (lint, test, build)
├── docker-compose.yml             # Postgres + Redis + Meilisearch + MinIO (dev)
├── AGENTS.md                      # AI contributor onboarding guide
├── CONTRIBUTOR.md                 # Human contributor guide
└── go.mod
```

---

## Getting Started

### Prerequisites

- **Go 1.25+**
- **Node.js 18+** (with npm)
- **Docker & Docker Compose**

### Quick Start

```bash
# 1. Clone the repository
git clone https://github.com/Geetur/Notery.git
cd Notery

# 2. Start infrastructure (Postgres, Redis, Meilisearch, MinIO)
docker-compose up -d

# 3. Copy and configure environment variables
cp .env.example .env  # then edit as needed

# 4. Start the Go API
go run cmd/api/main.go
# → http://localhost:8080/health

# 5. Start the frontend (new terminal)
cd frontend
npm install
npm run dev
# → http://localhost:3000
```

### Health Check

```bash
curl http://localhost:8080/health
# → {"status": "ok"}
```

---

## Environment Variables

Create a `.env` file in the project root:

```env
# ──── PostgreSQL ────
DB_HOST=localhost
DB_PORT=5432
DB_USER=admin
DB_PASSWORD=yourpassword
DB_NAME=notery_db
DB_SSLMODE=disable
DB_TIMEZONE=UTC

# ──── Redis ────
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=yourredispassword
REDIS_DB=0

# ──── Meilisearch ────
MEILISEARCH_HOST=http://localhost:7700
MEILISEARCH_MASTER_KEY=yourmeilisearchkey
MEILISEARCH_INDEX=notes

# ──── Object Storage (R2 or MinIO) ────
R2_ACCOUNT_ID=your_cloudflare_account_id
R2_ACCESS_KEY_ID=your_r2_access_key
R2_SECRET_ACCESS_KEY=your_r2_secret_key
R2_BUCKET_NAME=notery-pdfs

# ──── JWT ────
JWT_SECRET=your-super-secret-key

# ──── CORS ────
CORS_ORIGINS=http://localhost:3000,http://localhost:5173

# ──── Base URL ────
BASE_URL=http://localhost:8080

# ──── SMTP (optional — omit for log-to-stdout dev mode) ────
SMTP_HOST=smtp.example.com
SMTP_PORT=587
SMTP_USER=your_smtp_user
SMTP_PASS=your_smtp_password
SMTP_FROM=noreply@notery.app

# ──── Stripe (optional — omit for auto-fulfil dev mode) ────
STRIPE_SECRET_KEY=sk_test_xxx
STRIPE_WEBHOOK_SECRET=whsec_xxx

# ──── OAuth (optional) ────
GOOGLE_CLIENT_ID=xxx.apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=xxx
GITHUB_CLIENT_ID=xxx
GITHUB_CLIENT_SECRET=xxx
```

When SMTP is not configured, users are **auto-verified on signup** (dev mode).
When Stripe is not configured, orders **auto-fulfil** (dev mode).

---

## API Reference

### Public (14 routes)

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/health` | Health check |
| `POST` | `/api/v1/auth/signup` | Register + issue tokens |
| `POST` | `/api/v1/auth/login` | Authenticate + issue tokens |
| `POST` | `/api/v1/auth/refresh` | Rotate refresh token |
| `POST` | `/api/v1/auth/logout` | Revoke refresh token |
| `POST` | `/api/v1/auth/forgot-password` | Request password reset email |
| `POST` | `/api/v1/auth/reset-password` | Reset password with token |
| `GET` | `/api/v1/auth/verify-email` | Verify email via token |
| `GET` | `/api/v1/auth/oauth/providers` | List configured OAuth providers |
| `GET` | `/api/v1/auth/oauth/google` | Start Google OAuth flow |
| `GET` | `/api/v1/auth/oauth/google/callback` | Google OAuth callback |
| `GET` | `/api/v1/auth/oauth/github` | Start GitHub OAuth flow |
| `GET` | `/api/v1/auth/oauth/github/callback` | GitHub OAuth callback |
| `POST` | `/api/v1/webhooks/stripe` | Stripe webhook (signature-verified) |

### Public with Optional Auth (11 routes)

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/feed/hot` | Hot feed (personalised votes if authenticated) |
| `GET` | `/api/v1/notes/:id/comments` | Threaded comments tree |
| `GET` | `/api/v1/comments/:comment_id` | Single comment subtree |
| `GET` | `/api/v1/users/:id/profile` | Public user profile (includes notoriety) |
| `GET` | `/api/v1/users/:id/avatar` | Public avatar proxy (24h cache) |
| `GET` | `/api/v1/search` | Multi-type search (`?q=&type=notes\|subnoteries\|users\|comments`) |
| `GET` | `/api/v1/subnoteries` | List all subnoteries (paginated) |
| `GET` | `/api/v1/subnoteries/:id` | Subnotery detail (admins, member count, min notoriety) |
| `GET` | `/api/v1/subnoteries/:id/notes` | Approved notes in subnotery (paginated, sortable) |
| `GET` | `/api/v1/notes/:id/thumbnail` | Note thumbnail image (24h cache) |
| `GET` | `/api/v1/subnoteries/:id/banner` | Subnotery banner image (24h cache) |

### Auth-Only (14 routes)

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/v1/auth/logout-all` | Revoke all refresh tokens |
| `POST` | `/api/v1/auth/resend-verification` | Resend verification email |
| `GET` | `/api/v1/notes/:id` | Get note by ID |
| `GET` | `/api/v1/notes/approved` | List approved notes (paginated) |
| `GET` | `/api/v1/notes/:id/content` | View/stream full PDF |
| `GET` | `/api/v1/notes/:id/preview` | Preview PDF (truncated: 1 page per 5 total) |
| `GET` | `/api/v1/cart` | View cart |
| `GET` | `/api/v1/notes/:id/purchased` | Check purchase status |
| `GET` | `/api/v1/me/purchases` | My purchased notes |
| `GET` | `/api/v1/me/purchases/history` | Purchase history (paginated) |
| `GET` | `/api/v1/me/profile` | Own profile (includes notoriety) |
| `GET` | `/api/v1/me/notes` | Own notes (filterable by status) |
| `GET` | `/api/v1/me/comments` | Own comments (flat, paginated) |
| `GET` | `/api/v1/orders/:order_id` | Order status |

### Verified — Requires JWT + Verified Email (25 routes)

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/v1/notes` | Create note (respects min post notoriety) |
| `POST` | `/api/v1/notes/:id/content` | Upload PDF |
| `POST` | `/api/v1/notes/:id/thumbnail` | Upload thumbnail image |
| `DELETE` | `/api/v1/notes/:id/thumbnail` | Delete thumbnail |
| `POST` | `/api/v1/notes/:id/upvote` | Upvote note |
| `POST` | `/api/v1/notes/:id/downvote` | Downvote note |
| `POST` | `/api/v1/cart` | Add to cart |
| `DELETE` | `/api/v1/cart/:item_id` | Remove from cart |
| `POST` | `/api/v1/checkout` | Checkout cart |
| `POST` | `/api/v1/notes/:id/purchase` | Direct purchase |
| `PATCH` | `/api/v1/me/profile` | Update own profile |
| `POST` | `/api/v1/me/avatar` | Upload avatar (≤ 5 MB) |
| `DELETE` | `/api/v1/me/avatar` | Delete avatar |
| `POST` | `/api/v1/notes/:id/comments` | Create comment (respects min comment notoriety) |
| `PUT` | `/api/v1/comments/:comment_id` | Edit comment |
| `DELETE` | `/api/v1/comments/:comment_id` | Soft-delete comment |
| `POST` | `/api/v1/comments/:comment_id/vote` | Vote on comment |
| `DELETE` | `/api/v1/comments/:comment_id/vote` | Remove comment vote |
| `POST` | `/api/v1/orders/:order_id/confirm` | Manual order reconciliation |
| `POST` | `/api/v1/subnoteries/:id/join` | Join subnotery |
| `POST` | `/api/v1/subnoteries/:id/leave` | Leave subnotery (admin succession) |
| `PATCH` | `/api/v1/subnoteries/:id/settings` | Update subnotery settings (admin only) |
| `POST` | `/api/v1/subnoteries/:id/banner` | Upload subnotery banner (admin only, ≤ 5 MB) |
| `DELETE` | `/api/v1/subnoteries/:id/banner` | Delete subnotery banner (admin only) |
| `DELETE` | `/api/v1/subnoteries/:id/admins/:uid` | Remove admin (hierarchy-based) |

### Admin (9 routes)

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/notes/pending` | List pending notes (paginated) |
| `PATCH` | `/api/v1/notes/:id/approve` | Approve note |
| `PATCH` | `/api/v1/notes/:id/reject` | Reject note |
| `PATCH` | `/api/v1/notes/:id/lock` | Lock note (disables comments) |
| `PATCH` | `/api/v1/notes/:id/unlock` | Unlock note |
| `DELETE` | `/api/v1/notes/:id` | Delete note |
| `GET` | `/api/v1/admin/notes/:id/preview` | Preview PDF during approval |
| `DELETE` | `/api/v1/admin/notes/:id/content` | Delete PDF content |
| `POST` | `/api/v1/subnoteries/:id/admins` | Add subnotery admin |

---

## Data Models

13 models auto-migrated by GORM at startup:

| Model | Table | Description |
|-------|-------|-------------|
| `User` | `users` | Accounts, bcrypt passwords, profile, notoriety fields, OAuth |
| `Note` | `notes` | Notes with approval status, pricing, vote counts, hotness |
| `Subnotery` | `subnoteries` | Communities with settings (description, rules, min notoriety) |
| `Vote` | `votes` | User votes on notes (up/down) |
| `Comment` | `comments` | Threaded comments with materialized paths + depth |
| `CommentVote` | `comment_votes` | User votes on comments |
| `KarmaLedger` | `karma_ledgers` | Per-vote karma delta records for exact reversal |
| `Order` | `orders` | Payment sessions (pending → paid → fulfilled) |
| `OrderItem` | `order_items` | Individual notes within an order |
| `Purchase` | `purchases` | Completed purchase records |
| `RefreshToken` | `refresh_tokens` | JWT refresh tokens with family-based rotation |
| `EmailVerification` | `email_verifications` | Single-use email verification tokens (24h TTL) |
| `PasswordReset` | `password_resets` | Single-use password reset tokens (1h TTL) |

---

## Notoriety System

Notery uses a karma system called **Notoriety** that rewards quality contributions with logarithmic diminishing returns and a confidence gate.

### How It Works

Every upvote/downvote on a note or comment creates a `KarmaLedger` entry recording the exact delta applied to the author's notoriety. When a vote is toggled off or switched direction, the original delta is reversed exactly.

### Algorithm

For each vote, the delta is calculated as:

```
δ = v × base × conf
```

Where:
- `v` = vote direction (+1 or -1)
- `base = K / (K + max(0, S))` where `S = upvotes - downvotes` (diminishing returns)
- `conf = min(1, ln(1 + N) / ln(1 + N₀))` where `N = upvotes + downvotes` (confidence gate)

### Parameters

| Parameter | Post Karma | Comment Karma |
|-----------|-----------|--------------|
| K (max delta) | 20 | 10 |
| N₀ (confidence threshold) | 25 | 40 |

### Effects

- **Diminishing returns**: High-scoring content gives progressively smaller karma per additional vote
- **Confidence gate**: New content with few votes has reduced karma impact until voting confidence grows
- **Exact reversal**: Every delta is ledgered for precise undo when votes change

### Subnotery Minimum Notoriety

Admins can set **minimum post notoriety** and **minimum comment notoriety** thresholds per subnotery. Users below the threshold are prevented from posting/commenting (admins are exempt).

---

## Testing

### Backend (Go)

```bash
# All tests
go test ./... -count=1 -timeout 60s

# With race detection
go test -race ./...

# Vet (static analysis)
go vet ./...

# Specific package
go test ./internal/handlers -count=1
go test ./internal/models -count=1
```

| Package | Tests |
|---------|-------|
| `internal/handlers` | 190+ (auth, cart, comment, concurrency, feed, note, oauth, profile, purchase, subnotery) |
| `internal/models` | 70+ (user, note, comment, vote, order, session, karma) |
| `internal/email` | 20 |
| `internal/middleware` | 13 |
| `internal/payment` | 8 |

### Frontend (Jest)

```bash
cd frontend

npm test              # All tests
npm run test:watch    # Watch mode
npm run test:coverage # Coverage report
```

### Unified Test Runner

```powershell
.\scripts\test-all.ps1          # Go + frontend
.\scripts\test-all.ps1 -E2E     # + end-to-end scripts (requires running server)
.\scripts\test-all.ps1 -K6      # + k6 load tests
.\scripts\test-all.ps1 -Verbose # Verbose output
```

### E2E Scripts (require running server + Docker)

```powershell
.\scripts\test-hot-feed.ps1
.\scripts\test-pdf-workflow.ps1
.\scripts\test-comments.ps1
```

### Load Tests (k6)

```bash
k6 run scripts/k6/smoke-test.js
k6 run scripts/k6/load-test.js
k6 run scripts/k6/auth-flow.js
k6 run scripts/k6/stress-test.js
```

### Performance Results

Benchmarked on localhost (Go + Gin, PostgreSQL, Redis, Meilisearch). All tests run against a single API instance.

| Test | VUs | Duration | Throughput | p95 Latency | p99 Latency | Error Rate |
|------|-----|----------|------------|-------------|-------------|------------|
| **Smoke** | 1 | 1 iter | 15 req/s | 63 ms | 68 ms | 0.00% |
| **Auth Flow** | 20 | 1m 45s | 15 req/s | 71 ms | 79 ms | 0.00% |
| **Load** | 50 | 5m | 28 req/s | 4.7 ms | 53 ms | 2.35% |
| **Stress** | 200 | 5m 30s | 170 req/s | 947 ms | 1.04 s | 1.17% |

**Key takeaways:**

- **170 req/s sustained** at 200 concurrent users with sub-second p95 latency.
- **Feed & search endpoints** handle the bulk of traffic (read-heavy mix: 50% feed, 30% search, 10% subnoteries, 10% auth).
- **Auth endpoints** (signup, login, refresh, logout) complete in under 80 ms at p95 with 20 concurrent users.
- **Error budget** stays well under 5% even at 200 VUs — failures are primarily rate-limited search requests, not server errors.

> Rate limits are env-configurable via `RATE_LIMIT_AUTH`, `RATE_LIMIT_WRITE`, `RATE_LIMIT_READ`, `RATE_LIMIT_OAUTH` (requests per minute).

---

## Contributing

See [CONTRIBUTOR.md](CONTRIBUTOR.md) for setup, conventions, and workflow guidelines.

---

## License

Copyright © 2026 Jeter Pontes. All rights reserved.

This repository is proprietary. No permission is granted to use, copy, modify, or distribute this code without explicit written permission.
