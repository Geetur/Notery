// k6/common.js — Shared helpers for k6 load tests.
import http from "k6/http";

// Base URL for the API.
export const BASE_URL = __ENV.BASE_URL || "http://localhost:8080/api/v1";

// Default request headers.
export function jsonHeaders(token) {
    const h = { "Content-Type": "application/json" };
    if (token) h["Authorization"] = `Bearer ${token}`;
    return { headers: h };
}

// Signup a user and return { access_token, refresh_token }.
export function signup(email, password, username) {
    const res = http.post(
        `${BASE_URL}/auth/signup`,
        JSON.stringify({ email, password, username }),
        jsonHeaders()
    );
    return res.json();
}

// Login and return { access_token, refresh_token }.
export function login(email, password) {
    const res = http.post(
        `${BASE_URL}/auth/login`,
        JSON.stringify({ email, password }),
        jsonHeaders()
    );
    return res.json();
}

// Check response status and tag errors.
export function check200(res, name) {
    if (res.status !== 200 && res.status !== 201) {
        console.error(`${name} failed: ${res.status} ${res.body}`);
        return false;
    }
    return true;
}

// ---------------------------------------------------------------------------
// Bottleneck classification helpers
// ---------------------------------------------------------------------------
//
// Each endpoint in Notery has a known set of backend operations. By mapping
// endpoint names to their dominant resource dependency we can explain *why*
// latency increases as RPS grows:
//
//   CPU-bound        — bcrypt password hashing, HMAC-SHA256 JWT signing/verify
//   Postgres-bound   — sequential scans (ILIKE), row-level locks (votes),
//                      transaction contention (vote + karma ledger)
//   Redis-bound      — ZUNIONSTORE (personalised feed), sorted-set reads
//   Meilisearch-bound— full-text index search latency
//   Mixed            — multiple backends contribute roughly equally
//
// The metadata object below is consumed by handleSummary in each test to
// print a human-readable bottleneck report.

export const ENDPOINT_META = {
    // -------- Auth (CPU-bound: bcrypt + JWT) --------
    signup: {
        bottleneck: "CPU-bound",
        why: "bcrypt hash (cost 10) dominates — ~80-120 ms per call. " +
             "Also: INSERT user row (Postgres), INSERT refresh_token row, " +
             "JWT HMAC-SHA256 signing. Under load, bcrypt saturates CPU cores " +
             "because it is intentionally slow. Latency grows linearly with " +
             "concurrency once CPU is fully utilised.",
    },
    login: {
        bottleneck: "CPU-bound",
        why: "bcrypt.CompareHashAndPassword is the same cost as hashing. " +
             "SELECT user by email (indexed, fast), then bcrypt verify (~80-120 ms). " +
             "INSERT refresh_token row + JWT sign are negligible. Under high RPS " +
             "the bcrypt calls queue behind each other on available CPU cores.",
    },
    refresh: {
        bottleneck: "Postgres-bound (row locking)",
        why: "SHA-256 hash of token (fast), then SELECT refresh_token by hash (indexed). " +
             "UPDATE to revoke old token + INSERT new token — both touch the same " +
             "token-family rows. Under concurrency, row-level locks on refresh_tokens " +
             "cause queuing. Theft-detection scan (WHERE family_id = ?) adds latency " +
             "if families grow large.",
    },
    logout: {
        bottleneck: "Postgres-bound",
        why: "SELECT + UPDATE refresh_token by hash. Minimal contention unless " +
             "many sessions for the same user are being revoked concurrently.",
    },

    // -------- Feed (Redis + Postgres) --------
    hot_feed: {
        bottleneck: "Redis-bound → Postgres-bound",
        why: "Anonymous: Redis ZREVRANGE on global sorted set (fast, O(log N + M)). " +
             "Authenticated: ZUNIONSTORE merges subscribed subnotery feeds with global " +
             "(O(N) in number of subscriptions × set sizes), then ZREVRANGE. " +
             "After Redis returns note IDs: SELECT notes WHERE id IN (...) from Postgres, " +
             "plus batch SELECTs for subnotery names, comment counts, and user votes. " +
             "Under high RPS the ZUNIONSTORE temporary keys and Postgres IN-clause queries " +
             "become the bottleneck. ZUNIONSTORE is single-threaded in Redis.",
    },

    // -------- Search (Meilisearch / Postgres) --------
    search_notes: {
        bottleneck: "Meilisearch-bound",
        why: "Full-text search delegated to Meilisearch (offset/limit pagination). " +
             "Meilisearch is single-node; under load, query queuing occurs. " +
             "Fallback path (DB): ILIKE '%term%' on title/author — sequential scan, " +
             "no index can accelerate leading-wildcard patterns. Very slow at scale.",
    },
    search_subnoteries: {
        bottleneck: "Postgres-bound (sequential scan)",
        why: "ILIKE '%term%' on subnotery name — Postgres cannot use B-tree indexes " +
             "for leading-wildcard patterns. Must seq-scan entire subnoteries table. " +
             "COUNT(*) + SELECT with OFFSET pagination. Hot/top sort adds a correlated " +
             "subquery (COUNT members) per row. Latency grows with table size and RPS.",
    },
    search_users: {
        bottleneck: "Postgres-bound (sequential scan)",
        why: "ILIKE '%term%' on username/display_name — same leading-wildcard issue. " +
             "Seq-scans the users table. Linear degradation with table size.",
    },
    search_comments: {
        bottleneck: "Postgres-bound (sequential scan + join)",
        why: "ILIKE '%term%' on comment body joined with approved notes. " +
             "Leading-wildcard forces seq-scan on comments table. Join filter on " +
             "notes.status adds index lookup but the ILIKE dominates.",
    },

    // -------- Notes --------
    approved_notes: {
        bottleneck: "Postgres-bound",
        why: "SELECT notes WHERE status='approved' ORDER BY ... OFFSET/LIMIT. " +
             "If the status column is indexed, the index narrows the scan. " +
             "Batch comment-count subquery (GROUP BY note_id) adds overhead. " +
             "Under load, shared buffer contention and OFFSET-based pagination " +
             "(Postgres must skip N rows) increase latency for later pages.",
    },
    my_notes: {
        bottleneck: "Postgres-bound",
        why: "SELECT notes WHERE creator_id = ? — indexed lookup, fast. " +
             "Minimal contention. Latency stays low unless the creator has many notes.",
    },

    // -------- Profile --------
    profile: {
        bottleneck: "Postgres-bound",
        why: "SELECT user WHERE id = ? — primary-key lookup, very fast. " +
             "Minimal resource usage. Unlikely to be a bottleneck.",
    },

    // -------- Bookmarks --------
    bookmarks: {
        bottleneck: "Postgres-bound",
        why: "SELECT bookmarks JOIN notes WHERE user_id = ?. Indexed on user_id. " +
             "Note data is fetched via JOIN. Pagination keeps result sets bounded.",
    },

    // -------- Subnoteries --------
    subnoteries_list: {
        bottleneck: "Postgres-bound",
        why: "SELECT subnoteries with OFFSET/LIMIT. Small table, fast. " +
             "Member-count sort adds a correlated subquery per row.",
    },

    // -------- Voting (write-heavy, high contention) --------
    upvote: {
        bottleneck: "Postgres-bound (row locking + transaction)",
        why: "DB transaction: SELECT vote (FOR UPDATE implicit via GORM), " +
             "INSERT/UPDATE/DELETE vote row, UPDATE note counters (upvotes/downvotes), " +
             "INSERT karma_ledger, UPDATE user.post_karma. All within one transaction. " +
             "The note row is locked for the duration (row-level lock). " +
             "Concurrent votes on the SAME note serialize at the row lock. " +
             "After commit: re-SELECT note, UPDATE note.hotness (Postgres), " +
             "ZADD to Redis (global + subnotery feed). " +
             "This is the highest-contention endpoint — latency spikes " +
             "proportional to concurrent votes on popular notes.",
    },
    downvote: {
        bottleneck: "Postgres-bound (row locking + transaction)",
        why: "Identical to upvote — same transaction, same row locks, same contention.",
    },

    // -------- Cart (Redis) --------
    cart: {
        bottleneck: "Redis-bound",
        why: "Cart is stored in Redis (HGETALL on cart:{user_id} key). " +
             "Very fast. Unlikely to be a bottleneck unless Redis is saturated.",
    },

    // -------- Health --------
    health: {
        bottleneck: "None (in-memory)",
        why: "Returns 200 with no backend calls. Measures pure HTTP + Go overhead.",
    },
};

// ---------------------------------------------------------------------------
// Summary report generator
// ---------------------------------------------------------------------------
//
// Call this from handleSummary(data) in each test file. It reads the custom
// Trend metrics (named like "dur_<endpoint>") and prints:
//   1. Per-endpoint avg / p50 / p95 / p99 / max
//   2. Bottleneck classification + explanation
//   3. Overall analysis of where latency is coming from

export function buildBottleneckReport(data, testName) {
    const lines = [];
    lines.push("=".repeat(80));
    lines.push(`  BOTTLENECK ANALYSIS — ${testName}`);
    lines.push("=".repeat(80));
    lines.push("");

    // Collect all custom duration trends (dur_*)
    const endpoints = [];
    for (const key in data.metrics) {
        if (key.startsWith("dur_") && data.metrics[key].type === "trend") {
            const name = key.replace("dur_", "");
            const m = data.metrics[key].values;
            endpoints.push({
                name,
                count:  m["count"] || 0,
                avg:    m["avg"] || 0,
                med:    m["med"] || 0,
                p90:    m["p(90)"] || 0,
                p95:    m["p(95)"] || 0,
                p99:    m["p(99)"] || 0,
                max:    m["max"] || 0,
                min:    m["min"] || 0,
            });
        }
    }

    if (endpoints.length === 0) {
        lines.push("  No custom endpoint metrics found (dur_* trends).");
        lines.push("");
        return { stdout: lines.join("\n") };
    }

    // Sort by p95 descending (worst offenders first)
    endpoints.sort((a, b) => b.p95 - a.p95);

    // ---- Per-endpoint table ----
    lines.push("  ENDPOINT LATENCY BREAKDOWN (sorted by p95, worst first)");
    lines.push("-".repeat(80));
    lines.push(
        pad("Endpoint", 22) +
        pad("Reqs", 8) +
        pad("Avg", 10) +
        pad("Med", 10) +
        pad("p90", 10) +
        pad("p95", 10) +
        pad("p99", 10) +
        pad("Max", 10)
    );
    lines.push("-".repeat(80));

    for (const ep of endpoints) {
        lines.push(
            pad(ep.name, 22) +
            pad(String(ep.count), 8) +
            pad(ms(ep.avg), 10) +
            pad(ms(ep.med), 10) +
            pad(ms(ep.p90), 10) +
            pad(ms(ep.p95), 10) +
            pad(ms(ep.p99), 10) +
            pad(ms(ep.max), 10)
        );
    }
    lines.push("");

    // ---- Bottleneck annotations ----
    lines.push("  BOTTLENECK CLASSIFICATION PER ENDPOINT");
    lines.push("-".repeat(80));

    for (const ep of endpoints) {
        const meta = ENDPOINT_META[ep.name];
        if (!meta) {
            lines.push(`  ${ep.name}: (no metadata — unknown bottleneck)`);
            lines.push("");
            continue;
        }
        lines.push(`  ${ep.name}`);
        lines.push(`    Bottleneck : ${meta.bottleneck}`);
        lines.push(`    p95 latency: ${ms(ep.p95)}`);
        lines.push(`    Requests   : ${ep.count}`);
        lines.push(`    Why        : ${meta.why}`);
        lines.push("");
    }

    // ---- Overall analysis ----
    lines.push("-".repeat(80));
    lines.push("  WHY DOES LATENCY INCREASE WITH MORE RPS?");
    lines.push("-".repeat(80));
    lines.push("");

    // Categorise endpoints by bottleneck type
    const buckets = {};
    for (const ep of endpoints) {
        const meta = ENDPOINT_META[ep.name];
        const cat = meta ? meta.bottleneck : "Unknown";
        if (!buckets[cat]) buckets[cat] = [];
        buckets[cat].push(ep);
    }

    for (const cat in buckets) {
        const eps = buckets[cat];
        const worst = eps[0];
        lines.push(`  [${cat}]`);
        lines.push(`    Endpoints: ${eps.map(e => e.name).join(", ")}`);
        lines.push(`    Worst p95: ${ms(worst.p95)} (${worst.name})`);

        if (cat.includes("CPU")) {
            lines.push("    Analysis : bcrypt is intentionally slow (~100 ms per hash). Each");
            lines.push("               request consumes a full CPU core for the hash duration.");
            lines.push("               With N CPU cores, throughput caps at ~N * 10 req/s for");
            lines.push("               auth endpoints. Beyond that, requests queue and latency");
            lines.push("               grows linearly. JWT signing is negligible by comparison.");
            lines.push("    Fix      : Increase CPU cores, or offload bcrypt to a dedicated");
            lines.push("               auth service. Consider Argon2id with tuned parallelism.");
        } else if (cat.includes("row lock") || cat.includes("transaction")) {
            lines.push("    Analysis : Vote transactions lock the note row for the entire");
            lines.push("               transaction (SELECT vote → UPDATE counters → INSERT karma");
            lines.push("               → UPDATE user karma). Concurrent votes on the SAME note");
            lines.push("               serialize. With popular notes, vote throughput is limited");
            lines.push("               to ~1 / (transaction duration) per note per second.");
            lines.push("    Fix      : Decouple hotness recalculation from the vote transaction.");
            lines.push("               Use async workers for karma. Consider optimistic locking");
            lines.push("               or counter batching (aggregate votes, flush periodically).");
        } else if (cat.includes("sequential scan")) {
            lines.push("    Analysis : ILIKE '%term%' cannot use B-tree indexes (leading wildcard).");
            lines.push("               Postgres must read every row in the table. As the table");
            lines.push("               grows and RPS increases, shared buffers thrash and disk I/O");
            lines.push("               spikes. Each query holds a snapshot, so many concurrent");
            lines.push("               queries compete for buffer pool pages.");
            lines.push("    Fix      : Use pg_trgm GIN indexes for trigram-based ILIKE acceleration,");
            lines.push("               or migrate all search to Meilisearch / full-text search.");
        } else if (cat.includes("Redis")) {
            lines.push("    Analysis : Redis is single-threaded. ZUNIONSTORE (personalised feed)");
            lines.push("               is O(N*M) where N = keys, M = elements. Under high RPS,");
            lines.push("               these commands queue behind each other. Simple reads");
            lines.push("               (ZREVRANGE, HGETALL) are fast but still serialise.");
            lines.push("    Fix      : Cache personalised feed results with short TTL. Use");
            lines.push("               Redis Cluster for horizontal scaling. Pre-compute feeds.");
        } else if (cat.includes("Meilisearch")) {
            lines.push("    Analysis : Meilisearch is single-node. Query processing is sequential");
            lines.push("               per index. Under load, queries queue. Pagination via");
            lines.push("               offset is O(offset + limit) internally.");
            lines.push("    Fix      : Deploy Meilisearch with more RAM. Consider search result");
            lines.push("               caching. For very high RPS, shard the index.");
        } else if (cat.includes("Postgres")) {
            lines.push("    Analysis : Standard indexed queries. Latency increases from shared");
            lines.push("               buffer contention, connection pool saturation, and");
            lines.push("               OFFSET-pagination overhead (skip N rows per page).");
            lines.push("    Fix      : Use cursor / keyset pagination. Tune max_connections");
            lines.push("               and shared_buffers. Add read replicas for read endpoints.");
        }
        lines.push("");
    }

    // ---- Global summary ----
    const totalReqs = endpoints.reduce((s, e) => s + e.count, 0);
    const globalP95 = data.metrics["http_req_duration"]
        ? data.metrics["http_req_duration"].values["p(95)"] || 0
        : 0;
    const globalAvg = data.metrics["http_req_duration"]
        ? data.metrics["http_req_duration"].values["avg"] || 0
        : 0;

    lines.push("-".repeat(80));
    lines.push("  SUMMARY");
    lines.push("-".repeat(80));
    lines.push(`  Total requests      : ${totalReqs}`);
    lines.push(`  Global avg latency  : ${ms(globalAvg)}`);
    lines.push(`  Global p95 latency  : ${ms(globalP95)}`);
    lines.push(`  Slowest endpoint    : ${endpoints[0].name} (p95 = ${ms(endpoints[0].p95)})`);
    lines.push(`  Fastest endpoint    : ${endpoints[endpoints.length - 1].name} (p95 = ${ms(endpoints[endpoints.length - 1].p95)})`);
    lines.push("");
    lines.push("  LATENCY SCALING RULES OF THUMB:");
    lines.push("    • CPU-bound (bcrypt)       : latency ∝ VUs / CPU_cores");
    lines.push("    • Row-lock contention      : latency ∝ concurrent writes to same row");
    lines.push("    • Sequential scan          : latency ∝ table_size × concurrent_queries");
    lines.push("    • Redis single-thread      : latency ∝ command_complexity × queue_depth");
    lines.push("    • Connection pool exhaustion: latency spikes when pool_size < VUs");
    lines.push("=".repeat(80));
    lines.push("");

    return { stdout: lines.join("\n") };
}

// Format milliseconds nicely.
function ms(val) {
    if (val < 1) return `${val.toFixed(2)} ms`;
    if (val < 1000) return `${val.toFixed(1)} ms`;
    return `${(val / 1000).toFixed(2)} s`;
}

// Right-pad a string to a given width.
function pad(str, width) {
    if (str.length >= width) return str.substring(0, width);
    return str + " ".repeat(width - str.length);
}
