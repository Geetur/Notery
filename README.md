# Notery

A full-stack Reddit-like note marketplace built with **Go** (API) and **Next.js** (frontend). Users sign up, creators publish notes with PDF content, admins approve them, buyers purchase, and everyone votes, comments, and browses a hot feed.

<img width="1024" height="1024" alt="Scribbled N logo in ink" src="https://github.com/user-attachments/assets/b7ec42b5-f272-4b4a-ac7d-d0b6e6a99394" />

## Features

### Backend (Go + Gin API)

- **Authentication** — JWT HS256 with family-based refresh token rotation + theft detection; 15-min access tokens, 30-day refresh tokens
- **Email Verification** — Token-based email verification with configurable mailer (SMTP / Log / Mock)
- **Password Management** — Forgot-password / reset-password with single-use tokens (1h TTL); change-password revokes all sessions; anti-enumeration (always returns 200)
- **User Profiles** — Display name, bio, avatar URL, public/private visibility; partial PATCH updates with regex validation
- **Avatar System** — Upload (JPEG/PNG/WebP/GIF, max 5 MB), magic-byte validation, Cloudflare R2 storage, 24h cached public proxy
- **Notes** — CRUD with admin approval lifecycle (Pending → Approved → Rejected)
- **PDF Content** — Secure upload to Cloudflare R2, proxy-only viewing, magic-byte (`%PDF-`) validation
- **Subnoteries** — Reddit-style community containers with scoped admin controls
- **Shopping Cart** — Redis-backed cart for multi-note checkout
- **Purchases & Orders** — State machine (pending → paid → fulfilled), idempotency keys, Stripe PaymentIntent integration
- **Voting & Hot Feed** — Reddit-style hotness algorithm; DB-authoritative votes with Redis best-effort cache
- **Comments** — Threaded tree with Wilson score ranking, soft-delete, depth caps, per-comment voting
- **Search** — Multi-type search: notes (Meilisearch full-text), subnoteries/users/comments (DB ILIKE)
- **Rate Limiting** — Redis sliding-window; three tiers: auth (5/min), write (60/min), read (120/min)
- **Security Headers** — X-Content-Type-Options, X-Frame-Options, Referrer-Policy, X-XSS-Protection, Permissions-Policy
- **CORS** — Configurable origins via `CORS_ORIGINS` env var (defaults to localhost:3000, localhost:5173)
- **Role-Based Access** — Global admins, subnotery-scoped admins, creators, purchasers

### Frontend (Next.js 14)

- **Reddit-style feed** — Hot, New, Top sorting
- **Note marketplace** — Browse, purchase, download PDFs
- **Voting** — Reddit-style upvote/downvote on notes and comments
- **Threaded comments** — Nested threads with collapse, reply, edit, delete
- **User profiles** — Public profiles, settings, email verification indicators
- **Shopping cart** — Add notes, checkout, purchase history
- **Multi-type search** — Notes, communities, users, comments
- **Dark mode** — Full light/dark theme support via next-themes
- **Responsive** — Mobile-first 3-column layout
- **Auth** — JWT-based with automatic 401 token refresh

## Tech Stack

| Component | Technology | Version |
|-----------|------------|---------|
| Backend Language | Go | 1.25.5 |
| Backend Framework | [Gin](https://github.com/gin-gonic/gin) | 1.11.0 |
| Database | PostgreSQL (via GORM) | 16-alpine |
| Cache | Redis | 7-alpine |
| Search | [Meilisearch](https://www.meilisearch.com/) | 1.0.0 |
| File Storage | Cloudflare R2 (S3-compatible) | — |
| Payments | [Stripe](https://stripe.com/) PaymentIntent API | — |
| Auth | JWT HS256 | — |
| Frontend Framework | [Next.js](https://nextjs.org/) (App Router) | 14.2.35 |
| Frontend Language | TypeScript | 5.x |
| Styling | TailwindCSS | 3.4.x |
| UI Components | [shadcn/ui](https://ui.shadcn.com/) (20 components) | — |
| State Management | [Zustand](https://github.com/pmndrs/zustand) | 5.x |
| Server State | [@tanstack/react-query](https://tanstack.com/query) | 5.x |
| Frontend Testing | Jest + Testing Library | 30.x |

## Architecture

```
Browser (Next.js :3000)
   ↓ HTTP
Go API (Gin :8080)
   ↓
┌──────────────┬────────────┬──────────────┬────────────┐
│ PostgreSQL   │   Redis    │ Meilisearch  │ Cloudflare │
│ (data)       │ (cache/    │ (full-text   │ R2 (PDF +  │
│              │  cart/rate) │  search)     │  avatars)  │
└──────────────┴────────────┴──────────────┴────────────┘
                     │
                  Stripe (payments)
```

All handler methods live on a single `App` struct that holds every dependency (DB, Redis, R2, Meili, JWT secret, payment service, mailer). Dependencies are injected once at startup.

## Project Structure

```
Notery/
├── cmd/api/
│   └── main.go                    # Entrypoint: config, init, routes, graceful shutdown
├── internal/
│   ├── config/
│   │   └── config.go              # Centralised env loading (JWT, Stripe, CORS, SMTP)
│   ├── database/
│   │   ├── database.go            # PostgreSQL init + migrations
│   │   ├── redis.go               # Redis init
│   │   ├── meilisearch.go         # Meilisearch init
│   │   └── r2.go                  # Cloudflare R2 client (PDF + avatar storage)
│   ├── email/
│   │   ├── email.go               # Mailer interface (SMTP/Log/Mock) + templates
│   │   └── email_test.go          # 20 tests
│   ├── handlers/
│   │   ├── app.go                 # App struct + constructor
│   │   ├── auth.go                # Auth: signup, login, refresh, logout, verify, reset
│   │   ├── avatar.go              # Avatar upload, proxy, delete (R2-backed)
│   │   ├── cart.go                # Cart CRUD (Redis)
│   │   ├── comment.go             # Threaded comments, voting, tree assembly
│   │   ├── content.go             # PDF upload / view / delete + access control
│   │   ├── feed.go                # Hot feed, note voting
│   │   ├── note.go                # Note CRUD, approve/reject, Meili indexing
│   │   ├── profile.go             # User profile management
│   │   ├── purchase.go            # Checkout, orders, purchase history
│   │   ├── search.go              # Multi-type search
│   │   ├── subnotery.go           # Join subnotery, add admin
│   │   ├── webhook.go             # Stripe webhook (signature-verified)
│   │   └── *_test.go              # 191 tests across 9 test files
│   ├── helpers/
│   │   ├── helpers.go             # Pagination, logging, binding, auth context
│   │   ├── cart.go                # Cart Redis key builder
│   │   ├── comment.go             # Comment ID parsing
│   │   ├── note.go                # Note fetch/parse helpers
│   │   ├── subnotery.go           # Subnotery fetch/parse helpers
│   │   ├── user.go                # User fetch helpers
│   │   └── validation.go          # Username/display name validation
│   ├── middleware/
│   │   ├── auth.go                # RequireAuth / OptionalAuth (shared JWT parsing)
│   │   ├── admin.go               # RequireAdmin (global or subnotery-scoped)
│   │   ├── ratelimit.go           # Redis sliding-window rate limiter
│   │   ├── security.go            # Security response headers
│   │   └── auth_test.go           # 13 tests
│   ├── models/
│   │   ├── user.go                # User + bcrypt + profile DTOs
│   │   ├── session.go             # RefreshToken, EmailVerification, PasswordReset
│   │   ├── note.go                # Note + NoteStatus enum
│   │   ├── comment.go             # Comment + CommentVote + Wilson score
│   │   ├── order.go               # Order + OrderItem + state machine
│   │   ├── purchase.go            # Purchase record
│   │   ├── subnotery.go           # Subnotery model
│   │   ├── vote.go                # Vote + VoteDirection enum
│   │   └── *_test.go              # 62 tests across 6 test files
│   └── payment/
│       ├── payment.go             # Service interface + types
│       ├── stripe.go              # Stripe implementation
│       ├── mock.go                # Test double with call tracking
│       └── payment_test.go        # 8 tests
├── frontend/                      # Next.js 14 frontend (see below)
├── docs/
│   ├── DEEP_ANALYSIS.md           # Architecture deep-dive
│   ├── PAYMENT_SYSTEM.md          # Payment architecture
│   └── PDF_CONTENT_SYSTEM.md      # PDF architecture
├── scripts/
│   ├── test-hot-feed.ps1          # Hot feed E2E test
│   ├── test-pdf-workflow.ps1      # PDF upload/purchase workflow
│   └── test-comments.ps1          # Comment system E2E test
├── docker-compose.yml             # Postgres + Redis + Meilisearch
├── go.mod
├── AGENTS.md                      # AI contributor onboarding guide
└── README.md
```

### Frontend Structure

```
frontend/
├── src/
│   ├── app/                       # 13 pages (Next.js App Router)
│   │   ├── page.tsx               # Home (hot feed)
│   │   ├── hot/page.tsx           # Hot feed alias
│   │   ├── new/page.tsx           # New/latest feed
│   │   ├── login/page.tsx         # Login
│   │   ├── signup/page.tsx        # Signup
│   │   ├── forgot-password/       # Password reset request
│   │   ├── submit/page.tsx        # Create note
│   │   ├── notes/[id]/page.tsx    # Note detail + comments
│   │   ├── search/page.tsx        # Multi-type search
│   │   ├── cart/page.tsx          # Shopping cart
│   │   ├── purchases/page.tsx     # Purchase history
│   │   ├── profile/page.tsx       # Own profile settings
│   │   └── user/[id]/page.tsx     # Public user profile
│   ├── components/
│   │   ├── feed/                  # note-card, note-feed, vote-buttons, sort-tabs
│   │   ├── comments/              # comment-section, comment-thread
│   │   ├── layout/                # top-nav, left-sidebar, right-sidebar
│   │   ├── ui/                    # 20 shadcn/ui components
│   │   ├── providers.tsx          # App providers (Query, Theme, Auth)
│   │   └── theme-provider.tsx     # Dark mode wrapper
│   ├── hooks/
│   │   └── use-toast.ts           # Toast notification hook
│   ├── lib/
│   │   ├── api-client.ts          # HTTP client with auto 401 refresh
│   │   ├── config.ts              # Environment configuration
│   │   ├── format.ts              # Price, date, vote formatting
│   │   └── utils.ts               # Tailwind class merger
│   ├── services/                  # API service layer (7 domain files)
│   │   ├── auth.ts                # Login, signup, refresh, password reset
│   │   ├── notes.ts               # CRUD, voting, PDF upload
│   │   ├── comments.ts            # Threaded comments, voting
│   │   ├── profile.ts             # User profiles
│   │   ├── purchases.ts           # Cart, checkout, purchase history
│   │   ├── search.ts              # Multi-type search
│   │   └── subnoteries.ts         # Community operations
│   ├── stores/                    # Zustand state management
│   │   ├── auth-store.ts          # User session state
│   │   └── feed-store.ts          # Feed preferences (sort, view mode)
│   └── types/
│       ├── api.ts                 # TypeScript types mirroring Go API models
│       └── index.ts               # Barrel exports
├── jest.config.js
├── jest.setup.ts
├── package.json
├── tailwind.config.ts
├── tsconfig.json
└── next.config.mjs
```

## Getting Started

### Prerequisites

- **Go 1.25+**
- **Node.js 18+**
- **Docker & Docker Compose**

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
# 1. Start infrastructure (Postgres, Redis, Meilisearch)
docker-compose up -d

# 2. Start the Go API (terminal 1)
go run cmd/api/main.go
# → http://localhost:8080/health

# 3. Start the frontend (terminal 2)
cd frontend
npm install
npm run dev
# → http://localhost:3000
```

## API Endpoints (54 routes)

### Public (11 routes)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check |
| POST | `/api/v1/auth/signup` | Register + issue tokens + send verification email |
| POST | `/api/v1/auth/login` | Authenticate + issue tokens |
| POST | `/api/v1/auth/refresh` | Rotate refresh token |
| POST | `/api/v1/auth/logout` | Revoke refresh token |
| POST | `/api/v1/auth/forgot-password` | Request password reset email |
| POST | `/api/v1/auth/reset-password` | Reset password with token |
| GET | `/api/v1/auth/verify-email` | Verify email via token |
| POST | `/api/v1/signup` | Register (legacy) |
| POST | `/api/v1/login` | Authenticate (legacy) |
| POST | `/api/v1/webhooks/stripe` | Stripe webhook (signature-verified) |

### Public with Optional Auth (6 routes)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v1/feed/hot` | Hot feed (personalised if authenticated) |
| GET | `/api/v1/notes/:id/comments` | Threaded comments (user_vote if logged in) |
| GET | `/api/v1/comments/:comment_id` | Single comment subtree |
| GET | `/api/v1/users/:id/profile` | Public user profile |
| GET | `/api/v1/users/:id/avatar` | Public avatar proxy (24h cache) |
| GET | `/api/v1/search?q=&type=` | Multi-type search (notes/subnoteries/users/comments) |

### Protected — Requires JWT (30 routes)

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/v1/auth/logout-all` | Revoke all refresh tokens |
| POST | `/api/v1/auth/resend-verification` | Resend verification email |
| POST | `/api/v1/auth/change-password` | Change password (revokes all sessions) |
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
| GET | `/api/v1/notes/:id/purchased` | Check purchase status |
| GET | `/api/v1/orders/:order_id` | Order status |
| POST | `/api/v1/orders/:order_id/confirm` | Manual order reconciliation |
| GET | `/api/v1/me/purchases` | My purchased notes |
| GET | `/api/v1/me/purchases/history` | Purchase history (paginated) |
| GET | `/api/v1/me/profile` | Own profile (full) |
| PATCH | `/api/v1/me/profile` | Update own profile |
| POST | `/api/v1/me/avatar` | Upload avatar (multipart, max 5 MB) |
| DELETE | `/api/v1/me/avatar` | Delete own avatar |
| POST | `/api/v1/notes/:id/comments` | Create comment |
| PUT | `/api/v1/comments/:comment_id` | Edit comment |
| DELETE | `/api/v1/comments/:comment_id` | Soft-delete comment |
| POST | `/api/v1/comments/:comment_id/vote` | Vote on comment |
| DELETE | `/api/v1/comments/:comment_id/vote` | Remove comment vote |
| POST | `/api/v1/subnoteries/:id/join` | Join subnotery |

### Admin — Global or Subnotery-Scoped (7 routes)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v1/notes/pending` | List pending notes (paginated) |
| PATCH | `/api/v1/notes/:id/approve` | Approve note |
| PATCH | `/api/v1/notes/:id/reject` | Reject note |
| DELETE | `/api/v1/notes/:id` | Delete note |
| GET | `/api/v1/admin/notes/:id/preview` | Preview PDF during approval |
| DELETE | `/api/v1/admin/notes/:id/content` | Delete PDF content |
| POST | `/api/v1/subnoteries/:id/admins` | Add subnotery admin |

## Key Design Decisions

### Prices in Cents
All monetary values stored as `int64` cents (499 = $4.99) to avoid floating-point errors.

### DB-Authoritative Votes
The `votes` table is the single source of truth. Each vote runs in a DB transaction that atomically updates the vote row and note counters. Redis is a best-effort cache.

### Unified Vote Handler
`Upvote()` and `Downvote()` are thin wrappers around a single `voteNote(direction)` method, eliminating duplicated logic.

### JWT Middleware
`RequireAuth` and `OptionalAuth` share a common `parseJWTUserID()` helper — no duplicated token parsing.

### Refresh Token Rotation
Family-based rotation with theft detection. Reusing a revoked token revokes the entire token family. See `models/session.go`.

### Email Verification & Password Reset
Configurable Mailer interface (SMTP prod, LogMailer dev, MockMailer tests). Anti-enumeration on forgot-password (always returns 200). Single-use reset tokens (1h TTL). Password changes revoke all sessions.

### Payment Integration
`payment.Service` interface backed by Stripe in production, configurable mock in tests. Webhook-authoritative fulfilment. When Stripe is not configured, orders auto-fulfil. See [docs/PAYMENT_SYSTEM.md](docs/PAYMENT_SYSTEM.md).

### Comment System
Tree assembled in O(n) per request. Wilson score ranking (no time decay). Atomic votes with `SELECT ... FOR UPDATE`. Soft-delete preserves tree shape. Write depth cap = 15, read depth cap = 10 with `has_more_replies`.

### Search
Reddit-style multi-type search: `?q=world+news&type=notes|subnoteries|users|comments`. Notes via Meilisearch full-text; other types via DB ILIKE. All responses paginated.

### Avatar System
Uploads validated with both Content-Type header and magic-byte signatures. Stored in R2 at `avatars/{user_id}/avatar.{ext}`. Served via public proxy with 24h cache.

## Testing

**348 tests total** — 294 Go backend + 54 frontend.

### Backend (Go) — 294 tests across 18 test files

```bash
# All backend tests
go test ./...

# With race detection
go test -race ./...

# Specific package
go test ./internal/handlers -count=1
go test ./internal/models -count=1

# Specific test
go test ./internal/handlers -run TestSignup_HappyPath -count=1

# Verbose
go test -v ./...
```

| Package | Test Files | Tests |
|---------|-----------|-------|
| `internal/handlers` | 9 (auth, cart, comment, concurrency, feed, note, profile, purchase, subnotery) | 191 |
| `internal/models` | 6 (user, note, comment, vote, order, session) | 62 |
| `internal/email` | 1 | 20 |
| `internal/middleware` | 1 | 13 |
| `internal/payment` | 1 | 8 |

### Frontend (Jest) — 54 tests across 8 suites

```bash
cd frontend

# All tests
npm test

# Watch mode
npm run test:watch

# Coverage report
npm run test:coverage
```

| Suite | Tests |
|-------|-------|
| `lib/api-client` | 5 |
| `lib/config` | 4 |
| `lib/format` | 12 |
| `stores/auth-store` | 5 |
| `stores/feed-store` | 4 |
| `services/auth` | 5 |
| `services/notes` | 9 |
| `services/purchases` | 10 |

### E2E Scripts (require running server + Docker)

```powershell
.\scripts\test-hot-feed.ps1
.\scripts\test-pdf-workflow.ps1
.\scripts\test-comments.ps1
```

## Data Models (12 auto-migrated)

| Model | Description |
|-------|-------------|
| `User` | Accounts with bcrypt passwords, profile fields, admin flag |
| `Note` | Notes with approval status, pricing, vote counts, hotness |
| `Subnotery` | Reddit-style communities containing notes |
| `Vote` | User votes on notes (up/down) |
| `Comment` | Threaded comments with parent pointers + depth |
| `CommentVote` | User votes on comments |
| `Order` | Payment sessions (pending → paid → fulfilled) |
| `OrderItem` | Individual notes within an order |
| `Purchase` | Completed purchase records |
| `RefreshToken` | JWT refresh tokens with family-based rotation |
| `EmailVerification` | Single-use email verification tokens (24h TTL) |
| `PasswordReset` | Single-use password reset tokens (1h TTL) |

## Roadmap

### Near-term
1. **Subnotery browsing** — List, detail, leave (only join + add-admin exist)
2. **Bookmark system** — Save approved notes for later
3. **Karma system** — Note + comment karma on user profiles
4. **"My Notes" endpoint** — Creator's own notes with status filtering
5. **Notification system** — Comment replies, purchase confirmations

### Medium-term
6. **CI/CD pipeline** — GitHub Actions: lint, test, race, build
7. **Pagination on frontend** — Infinite scroll / load-more for all list views
8. **Note preview images** — Thumbnail generation from PDF first page
9. **Reporting system** — Flag notes/comments for admin review
10. **User following** — Follow creators, personalised feed

### Long-term
11. **Real-time updates** — WebSocket/SSE for live comment updates and notifications
12. **Analytics dashboard** — Creator earnings, view counts, engagement metrics
13. **Mobile app** — React Native or Flutter companion app
14. **Content recommendations** — ML-based note suggestions
15. **Deployment** — Docker production images, Kubernetes manifests, CDN integration

## License

Copyright (c) 2026 Jeter Pontes. All rights reserved.

This repository is proprietary. No permission is granted to use, copy, modify, or distribute this code without explicit written permission.
