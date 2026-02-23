# Contributing to Notery

Thanks for your interest in contributing to Notery! This guide covers everything you need to get started.

---

## Table of Contents

- [Prerequisites](#prerequisites)
- [Local Setup](#local-setup)
- [Repo Structure](#repo-structure)
- [Development Workflow](#development-workflow)
- [Coding Conventions](#coding-conventions)
- [Adding a Feature](#adding-a-feature)
- [Testing](#testing)
- [Pull Request Process](#pull-request-process)
- [Common Playbooks](#common-playbooks)

---

## Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.25+ | Backend API |
| Node.js | 18+ | Frontend build + tests |
| Docker | Latest | PostgreSQL, Redis, Meilisearch, MinIO |
| Git | Latest | Version control |

---

## Local Setup

```bash
# Clone
git clone https://github.com/Geetur/Notery.git
cd Notery

# Start infrastructure
docker-compose up -d

# Backend
go run cmd/api/main.go
# → http://localhost:8080/health

# Frontend (new terminal)
cd frontend
npm install
npm run dev
# → http://localhost:3000
```

Copy `.env.example` to `.env` and fill in values. When SMTP and Stripe are omitted, the API runs in dev mode (auto-verify emails, auto-fulfil payments).

---

## Repo Structure

| Directory | What lives here |
|-----------|----------------|
| `cmd/api/main.go` | App boot, dependency init, route wiring, graceful shutdown |
| `internal/config/` | Centralised env loading |
| `internal/handlers/` | HTTP handlers — one file per domain (auth, note, comment, ...) |
| `internal/models/` | GORM models, domain constants, algorithms |
| `internal/database/` | DB/Redis/Meili/R2 setup + migrations |
| `internal/middleware/` | Auth, admin, rate-limit, security, verified middleware |
| `internal/helpers/` | Shared utilities (pagination, logging, validation, fetch helpers) |
| `internal/email/` | Mailer interface + email templates |
| `internal/payment/` | Payment service interface (Stripe + mock) |
| `frontend/src/app/` | Next.js pages (App Router) |
| `frontend/src/components/` | React components (feed, comments, layout, ui) |
| `frontend/src/services/` | API service layer — one file per domain |
| `frontend/src/types/` | TypeScript types mirroring Go API models |
| `frontend/src/stores/` | Zustand state stores |
| `scripts/` | Test scripts + k6 load tests |
| `docs/` | Architecture documentation |

---

## Development Workflow

1. Create a feature branch from `main`
2. Make changes following the conventions below
3. Run the full test suite (both backend and frontend)
4. Submit a PR with a clear description

### Quick validation loop

```bash
# Backend compile check
go build ./...

# Backend tests (fast)
go test ./internal/models -count=1
go test ./internal/handlers -run YourTest -count=1

# Full backend suite
go test ./... -count=1 -timeout 60s
go vet ./...

# Frontend
cd frontend
npm run build
npm test
```

---

## Coding Conventions

### Go Backend

**File headers**: Every file uses a standardised header comment:
- Primary file per package: `// Package X provides ...`
- All other files: `// filename.go — Brief description.`

**Code style**:
- Keep handlers practical — validation, authz, response formatting
- Put reusable logic in helpers/models when truly shared
- Use typed constants/enums for domain state
- Return clear HTTP errors (400/401/403/404/500) with stable JSON keys
- Favor explicit checks over implicit behavior
- Don't leak sensitive fields (email, hash, secrets) in public responses
- Use `errors.Is()` for sentinel error comparisons
- Use domain-specific loggers from `helpers` (e.g., `NoteLog`, `CommentLog`)

**Naming**:
- Handlers: `func (app *App) VerbNoun(c *gin.Context)` — e.g., `CreateNote`, `GetMyProfile`
- Tests: `TestVerbNoun_Scenario` — e.g., `TestCreateNote_MissingTitle`
- Models: singular nouns — `Note`, `User`, `Subnotery`

### TypeScript Frontend

- Components: PascalCase files — `NoteCard.tsx`, `SortTabs.tsx`
- Pages: `page.tsx` in App Router directories
- Services: one file per domain — `notes.ts`, `comments.ts`, `profile.ts`
- Types: mirror Go API shapes in `types/api.ts`
- State: Zustand stores in `stores/`
- Style: Tailwind utility classes, shadcn/ui components

---

## Adding a Feature

### Backend endpoint

1. **Handler** — Add method in `internal/handlers/*.go`
2. **Route** — Register in `cmd/api/main.go` with correct middleware group:
   - `public` — No auth needed
   - `optional` — OptionalAuth middleware
   - `read` — RequireAuth (read-only, no email verification needed)
   - `write` — RequireAuth + RequireVerified (email must be verified)
   - `admin` — RequireAuth + RequireAdmin
3. **Model** — If new table, add to `internal/models/` and `database.go` AutoMigrate
4. **Tests** — Add tests in the corresponding `*_test.go` file
5. **Docs** — Update README endpoint table

### Frontend feature

1. **Types** — Add/update types in `types/api.ts`
2. **Service** — Add API calls in `services/*.ts`
3. **Component** — Create or update components
4. **Page** — Wire into a page in `app/`
5. **Tests** — Add tests if business logic is involved

### Full-stack feature

Every backend endpoint **must** be wired into the frontend. No orphaned API endpoints.

---

## Testing

### Required before every PR

```bash
# Backend
go test ./... -count=1 -timeout 60s
go vet ./...

# Frontend
cd frontend && npm run build && npm test
```

### Test organisation

| Location | What |
|----------|------|
| `internal/handlers/*_test.go` | Handler integration tests (mock DB, real handler logic) |
| `internal/models/*_test.go` | Model unit tests (algorithms, validation) |
| `internal/email/email_test.go` | Email template + mailer tests |
| `internal/middleware/auth_test.go` | Auth middleware tests |
| `internal/payment/payment_test.go` | Payment service tests |
| `frontend/src/**/__tests__/` | Frontend unit tests (Jest + Testing Library) |

### Writing tests

- **Cover the happy path** and at least one error case per endpoint
- **Use table-driven tests** for Go functions with multiple input scenarios
- **Mock external dependencies** (DB, Redis, R2) — don't require running infrastructure for unit tests
- **Test the unhappy path**: empty inputs, max-length inputs, auth bypass, missing dependencies

---

## Pull Request Process

1. **Title**: `feat(domain): short summary` or `fix(domain): short summary`
2. **Description**: What changed, why, and how to test
3. **Checklist**:
   - [ ] `go build ./...` succeeds
   - [ ] `go test ./...` passes (all packages, not just changed files)
   - [ ] `go vet ./...` passes
   - [ ] `npm run build` succeeds (if frontend touched)
   - [ ] `npm test` passes (if frontend touched)
   - [ ] New endpoints have tests
   - [ ] README/docs updated for API changes
   - [ ] No `TODO`/`FIXME` comments without a linked issue

---

## Common Playbooks

### A) Add a new API endpoint

1. Add handler method in `internal/handlers/*.go`
2. Register route in `cmd/api/main.go`
3. Apply correct middleware (public/optional/protected/admin)
4. Add tests in handler test file
5. Add frontend service call + component
6. Update README endpoint table

### B) Modify a database model

1. Update model fields/tags in `internal/models/`
2. Ensure migration support in `internal/database/database.go` (AutoMigrate)
3. Update handler logic + tests
4. Update TypeScript types in `types/api.ts`
5. Consider backfill/migration safety for existing data

### C) Touch auth or rate limiting

1. Validate security behavior first (deny-by-default mindset)
2. Add tests for negative/abuse paths
3. Keep logs useful but avoid sensitive token/body leakage

### D) Touch vote counters or karma

1. All changes must happen inside a DB transaction
2. Re-read from DB after transaction for accurate counts
3. Redis cache updates are best-effort (guarded with `if app.RDB != nil`)
4. KarmaLedger entries must be created/reversed atomically with the vote

---

## High-Risk Areas (extra care)

These areas require particular attention and thorough testing:

- **Vote counters and karma** — Race risk; re-read from DB after transaction
- **Admin scope resolution** — Privilege escalation risk
- **Comment tree queries** — Performance risk with deep trees
- **PDF access checks** — Content access risk; magic-byte validation required
- **Avatar/thumbnail uploads** — File type spoofing risk; magic bytes must match Content-Type
- **Rate limiting** — Abuse risk; `ExpireNX` for correct sliding window
- **Notoriety enforcement** — Must skip for admins, handle new subnoteries correctly

---

