# Notery — Performance & Bottleneck Analysis

## Load Test Results

Benchmarked on localhost (Go + Gin, PostgreSQL, Redis, Meilisearch). Single API instance.

| Test | VUs | Duration | Throughput | p95 Latency | p99 Latency | Error Rate |
|------|-----|----------|------------|-------------|-------------|------------|
| **Smoke** | 1 | 1 iter | 15 req/s | 63 ms | 68 ms | 0.00% |
| **Auth Flow** | 20 | 1m 45s | 15 req/s | 71 ms | 79 ms | 0.00% |
| **Load** | 50 | 5m | 28 req/s | 4.7 ms | 53 ms | 2.35% |
| **Stress** | 200 | 5m 30s | 170 req/s | 947 ms | 1.04 s | 1.17% |

**Key takeaways:**

- **170 req/s sustained** at 200 concurrent users with sub-second p95 latency.
- **Feed & search endpoints** handle the bulk of traffic (read-heavy mix: 50% feed, 30% search, 10% subnoteries, 10% auth).
- **Auth endpoints** complete in under 80 ms at p95 with 20 concurrent users.
- **Error budget** stays well under 5% even at 200 VUs — failures are primarily rate-limited search requests.

> Rate limits are env-configurable via `RATE_LIMIT_AUTH`, `RATE_LIMIT_WRITE`, `RATE_LIMIT_READ`, `RATE_LIMIT_OAUTH`.

---

## Running Load Tests

```bash
k6 run scripts/k6/smoke-test.js
k6 run scripts/k6/load-test.js
k6 run scripts/k6/auth-flow.js
k6 run scripts/k6/stress-test.js
```

Each test prints a bottleneck analysis report showing per-endpoint latency percentiles, bottleneck classification, and root-cause explanations.

---

## Bottleneck Classification by Endpoint

| Endpoint | Type | Dominant Bottleneck | Why |
|----------|------|--------------------|----|
| `POST /auth/signup` | Write | **CPU (bcrypt)** | `bcrypt.GenerateFromPassword` cost=10 takes ~80-120 ms per call. Each request pins one CPU core. |
| `POST /auth/login` | Read+CPU | **CPU (bcrypt)** | `bcrypt.CompareHashAndPassword` has the same cost as hashing. |
| `POST /auth/refresh` | Write | **Postgres (row lock)** | SHA-256 hash + SELECT/UPDATE/INSERT on `refresh_tokens`. Theft-detection scan adds latency for large families. |
| `GET /feed/hot` | Read | **Redis → Postgres** | Anonymous: `ZREVRANGE` (fast). Authenticated: `ZUNIONSTORE` merges subscribed feeds. Then batch SELECT from Postgres. |
| `GET /search?type=notes` | Read | **Meilisearch** | Full-text search on single node. Under load, query queuing occurs. |
| `GET /search?type=subnoteries\|users\|comments` | Read | **Postgres (seq scan)** | `ILIKE '%term%'` cannot use B-tree indexes. |
| `POST /notes/:id/upvote` | Write | **Postgres (row lock + tx)** | Highest contention. Transaction locks note row for vote + karma + hotness. |
| `GET /cart` | Read | **Redis** | `HGETALL` — very fast, unlikely bottleneck. |

---

## The Five Causes of Latency Growth

| # | Cause | Endpoints Affected | Mechanism |
|---|-------|-------------------|-----------|
| 1 | **CPU saturation** (bcrypt) | signup, login | ~100 ms/call, caps at N×10 req/s with N cores |
| 2 | **Row-level locks** | vote endpoints | Concurrent votes on same note serialize (~5-15 ms/tx) |
| 3 | **Sequential scans** (ILIKE) | search by DB | Full table scan, buffer thrashing under concurrency |
| 4 | **Redis queuing** | feed assembly | ZUNIONSTORE is O(N×M), blocks single-threaded event loop |
| 5 | **Connection pool exhaustion** | all Postgres endpoints | When pool is full, requests wait for a free connection |

---

## Mitigation Strategies

| Bottleneck | Fix | Difficulty |
|-----------|-----|-----------|
| CPU (bcrypt) | Add CPU cores / horizontal scaling. Consider Argon2id. | Medium |
| Row locks (votes) | Decouple hotness from vote tx. Async karma workers. | Hard |
| Sequential scans | Add `pg_trgm` GIN indexes. Migrate all search to Meilisearch. | Easy |
| Redis queuing | Cache personalised feed with short TTL. Pre-compute on write. | Medium |
| Connection pool | Tune `MaxOpenConns`. Add PgBouncer. Read replicas. | Easy |
| OFFSET pagination | Switch to keyset/cursor pagination. | Medium |
