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

## Features

| Category | Highlights |
|----------|-----------|
| **Auth** | JWT access/refresh tokens, family-based rotation with theft detection, Google + GitHub OAuth, email verification, password reset |
| **Notes** | Admin approval lifecycle, in-app PDF viewing (react-pdf), magic-byte validation, preview truncation, thumbnails |
| **Communities** | Subnoteries with scoped admin controls, configurable rules, minimum notoriety thresholds, banners |
| **Commerce** | Stripe PaymentIntents, Redis cart, multi-note checkout, Stripe Connect creator payouts |
| **Social** | Reddit-style voting, hot feed with time decay, threaded comments (Wilson score), multi-type search |
| **Karma** | Notoriety system with logarithmic diminishing returns, confidence gating, exact-reversal ledger |
| **Frontend** | Three-column layout, dark mode, optimistic vote UI, in-app PDF viewer, autocomplete search |
| **Security** | Rate limiting (3 tiers), bcrypt passwords, security headers, CORS, input validation, ban system |

---

## Quick Start

```bash
# 1. Clone
git clone https://github.com/Geetur/Notery.git && cd Notery

# 2. Start infrastructure
docker-compose up -d

# 3. Configure environment
cp .env.example .env   # edit as needed

# 4. Start API
go run cmd/api/main.go  # → http://localhost:8080/health

# 5. Start frontend (new terminal)
cd frontend && npm install && npm run dev  # → http://localhost:3000
```

**Prerequisites:** Go 1.25+, Node.js 18+, Docker

---

## Architecture

```
Browser (Next.js :3000)
   ↓ HTTP / REST
Go API (Gin :8080)
   ↓
┌──────────────┬────────────┬──────────────┬────────────┐
│ PostgreSQL   │   Redis    │ Meilisearch  │ R2 / MinIO │
│ (data)       │ (cache,    │ (full-text)  │ (PDFs,     │
│              │  rate lim) │              │  media)    │
└──────────────┴────────────┴──────────────┴────────────┘
                     │
                  Stripe (payments)
```

All dependencies are injected via a single `App` struct at startup (`cmd/api/main.go`). Handlers, middleware, and models live under `internal/`.

**Key decisions:** prices in cents (int64), DB-authoritative votes (Redis is cache), family-based refresh tokens, configurable payment service (Stripe / auto-fulfil / mock).

---

## Environment Variables

Copy `.env.example` and configure. Key groups:

| Group | Variables | Notes |
|-------|----------|-------|
| **PostgreSQL** | `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_SSLMODE` | `DB_SSLMODE=require` in production |
| **Redis** | `REDIS_ADDR`, `REDIS_PASSWORD`, `REDIS_DB` | `REDIS_TLS_ENABLED=true` for managed Redis |
| **Meilisearch** | `MEILISEARCH_HOST`, `MEILISEARCH_MASTER_KEY` | |
| **Object Storage** | `R2_ACCOUNT_ID`, `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`, `R2_BUCKET_NAME` | R2 in prod, MinIO locally |
| **JWT** | `JWT_SECRET` | Min 32 chars, random |
| **CORS** | `CORS_ORIGINS` | Comma-separated origins |
| **SMTP** | `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASS`, `SMTP_FROM` | Omit for dev auto-verify |
| **Stripe** | `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET` | Omit for dev auto-fulfil |
| **OAuth** | `GOOGLE_CLIENT_ID/SECRET`, `GITHUB_CLIENT_ID/SECRET` | Optional |
| **Server** | `PORT`, `TRUSTED_PROXIES`, `BASE_URL`, `FRONTEND_URL` | Port defaults to 8080 |

### Email (SMTP) Setup

Notery uses SMTP for email verification and password reset. When SMTP is not configured, users are auto-verified on signup (dev mode).

**Using [Resend](https://resend.com):** (recommended for production)

```env
SMTP_HOST=smtp.resend.com
SMTP_PORT=465
SMTP_USER=resend
SMTP_PASS=re_your_api_key
SMTP_FROM=noreply@yourdomain.com
```

Resend requires a verified domain. Add the DNS records Resend provides, then use the API key as `SMTP_PASS`.

---

## Deployment

Recommended stack for scale-to-zero with minimal cost:

| Service | Provider | Why |
|---------|----------|-----|
| **Go API** | [Railway](https://railway.app) | Scale-to-zero, $5 hobby plan, deploy from GitHub |
| **Frontend** | [Vercel](https://vercel.com) | Native Next.js support, generous free tier |
| **PostgreSQL** | [Neon](https://neon.tech) | Serverless Postgres, scale-to-zero, free tier |
| **Redis** | [Upstash](https://upstash.com) | Serverless Redis, pay-per-request, free tier |
| **Search** | [Meilisearch Cloud](https://www.meilisearch.com/cloud) | Managed, free dev tier |
| **Object Storage** | [Cloudflare R2](https://developers.cloudflare.com/r2/) | No egress fees, S3-compatible |
| **Email** | [Resend](https://resend.com) | 3k emails/month free, simple SMTP |
| **DNS / CDN** | [Cloudflare](https://cloudflare.com) | Free plan, DDoS protection, caching |

### Production checklist

- Set `GIN_MODE=release`
- Set `DB_SSLMODE=require` and `REDIS_TLS_ENABLED=true`
- Use strong `JWT_SECRET` (32+ random chars)
- Configure `CORS_ORIGINS`, `BASE_URL`, `FRONTEND_URL` (no localhost)
- Set `TRUSTED_PROXIES` to your reverse proxy IPs
- Configure Stripe webhook endpoint + secret
- Verify domain with Resend for SMTP

---

## Testing

```bash
# Backend
go test ./... -count=1 -timeout 60s
go test -race ./...
go vet ./...

# Frontend
cd frontend && npm test

# Unified runner
.\scripts\test-all.ps1          # Go + frontend
.\scripts\test-all.ps1 -E2E     # + end-to-end scripts
.\scripts\test-all.ps1 -K6      # + k6 load tests
```

| Package | Tests |
|---------|-------|
| `internal/handlers` | 190+ |
| `internal/models` | 70+ |
| `internal/email` | 20 |
| `internal/middleware` | 13 |
| `internal/payment` | 8 |

---

## Documentation

| Document | Description |
|----------|-------------|
| [docs/API.md](docs/API.md) | Full API reference (73 endpoints) |
| [docs/PERFORMANCE.md](docs/PERFORMANCE.md) | Load test results + bottleneck analysis |
| [docs/DEEP_ANALYSIS.md](docs/DEEP_ANALYSIS.md) | Architecture deep-dive |
| [docs/PAYMENT_SYSTEM.md](docs/PAYMENT_SYSTEM.md) | Payment flow documentation |
| [docs/PDF_CONTENT_SYSTEM.md](docs/PDF_CONTENT_SYSTEM.md) | PDF access control documentation |
| [AGENTS.md](AGENTS.md) | AI contributor guide |
| [CONTRIBUTOR.md](CONTRIBUTOR.md) | Human contributor guide |
| [TERMS.md](TERMS.md) | Terms of Service |

---

## Contributing

See [CONTRIBUTOR.md](CONTRIBUTOR.md) for setup, conventions, and workflow guidelines.

---

## License

Copyright © 2026 Jeter Pontes. All rights reserved.

This repository is proprietary. No permission is granted to use, copy, modify, or distribute this code without explicit written permission.
