// k6/load-test.js — Sustained load test simulating realistic traffic patterns.
// Run: k6 run scripts/k6/load-test.js
//
// Scenario: Ramp up to 50 VUs over 1 minute, hold for 3 minutes, ramp down.
// Each VU cycles through read-heavy operations (feed, search, notes).
//
// After the run, prints a per-endpoint bottleneck analysis showing avg / p95 / p99
// latencies and classifying each endpoint by its dominant resource dependency.
import { check, sleep } from "k6";
import http from "k6/http";
import { Rate, Trend } from "k6/metrics";
import { BASE_URL, jsonHeaders, login, signup, buildBottleneckReport } from "./common.js";

const errorRate = new Rate("errors");

// --- Per-endpoint duration trends ---
const durFeed         = new Trend("dur_hot_feed", true);
const durSearchNotes  = new Trend("dur_search_notes", true);
const durSearchSubs   = new Trend("dur_search_subnoteries", true);
const durSearchUsers  = new Trend("dur_search_users", true);
const durSearchComm   = new Trend("dur_search_comments", true);
const durApproved     = new Trend("dur_approved_notes", true);
const durProfile      = new Trend("dur_profile", true);
const durSubnoteries  = new Trend("dur_subnoteries_list", true);
const durBookmarks    = new Trend("dur_bookmarks", true);

export const options = {
    stages: [
        { duration: "30s", target: 10 },
        { duration: "1m", target: 50 },
        { duration: "3m", target: 50 },
        { duration: "30s", target: 0 },
    ],
    thresholds: {
        http_req_duration: ["p(95)<500"],
        http_req_failed: ["rate<0.05"],
        errors: ["rate<0.05"],
    },
};

export function setup() {
    // Create a test user for authenticated requests.
    const ts = Date.now();
    const email = `k6load${ts}@test.local`;
    const password = "Passw0rd!Load";
    const username = `k6load${ts}`;
    signup(email, password, username);
    const loginData = login(email, password);
    return { token: loginData.access_token };
}

export default function (data) {
    const token = data.token;

    // --- Read-heavy mix ---

    // 60% hot feed
    if (Math.random() < 0.6) {
        const page = Math.floor(Math.random() * 5) + 1;
        const res = http.get(`${BASE_URL}/feed/hot?page=${page}&limit=20`);
        durFeed.add(res.timings.duration);
        const ok = check(res, { "feed 200": (r) => r.status === 200 });
        errorRate.add(!ok);
    }

    // 20% search (random type)
    if (Math.random() < 0.2) {
        const types = ["notes", "subnoteries", "users", "comments"];
        const t = types[Math.floor(Math.random() * types.length)];
        const sorts = ["hot", "new", "top", "controversial"];
        const s = sorts[Math.floor(Math.random() * sorts.length)];
        const res = http.get(`${BASE_URL}/search?q=test&type=${t}&sort=${s}`);

        // Track by search type for granular analysis
        switch (t) {
            case "notes":       durSearchNotes.add(res.timings.duration); break;
            case "subnoteries": durSearchSubs.add(res.timings.duration);  break;
            case "users":       durSearchUsers.add(res.timings.duration); break;
            case "comments":    durSearchComm.add(res.timings.duration);  break;
        }

        const ok = check(res, { "search 200": (r) => r.status === 200 });
        errorRate.add(!ok);
    }

    // 10% approved notes list
    if (Math.random() < 0.1) {
        const res = http.get(`${BASE_URL}/notes/approved`, jsonHeaders(token));
        durApproved.add(res.timings.duration);
        const ok = check(res, { "approved 200": (r) => r.status === 200 });
        errorRate.add(!ok);
    }

    // 5% profile
    if (Math.random() < 0.05) {
        const res = http.get(`${BASE_URL}/me/profile`, jsonHeaders(token));
        durProfile.add(res.timings.duration);
        const ok = check(res, { "profile 200": (r) => r.status === 200 });
        errorRate.add(!ok);
    }

    // 5% subnoteries list
    if (Math.random() < 0.05) {
        const res = http.get(`${BASE_URL}/subnoteries`);
        durSubnoteries.add(res.timings.duration);
        const ok = check(res, { "subnoteries 200": (r) => r.status === 200 });
        errorRate.add(!ok);
    }

    // 5% bookmarks
    if (Math.random() < 0.05) {
        const res = http.get(`${BASE_URL}/bookmarks`, jsonHeaders(token));
        durBookmarks.add(res.timings.duration);
        const ok = check(res, { "bookmarks 200": (r) => r.status === 200 });
        errorRate.add(!ok);
    }

    sleep(Math.random() * 2 + 0.5); // 0.5–2.5s think time
}

// --- Bottleneck analysis report ---
export function handleSummary(data) {
    return buildBottleneckReport(data, "LOAD TEST (50 VUs, 5 min)");
}
