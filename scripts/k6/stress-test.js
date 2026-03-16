// k6/stress-test.js — Stress test to find breaking points.
// Run: k6 run scripts/k6/stress-test.js
//
// Scenario: Aggressively ramp to 200 VUs to find where the API degrades.
// This is the most important test for identifying bottlenecks because it
// pushes each resource (CPU, Postgres connections, Redis, Meilisearch) to
// saturation. The bottleneck report at the end shows exactly which endpoints
// degrade first and why.
import { check, sleep } from "k6";
import http from "k6/http";
import { Rate, Trend } from "k6/metrics";
import { BASE_URL, jsonHeaders, login, signup, buildBottleneckReport } from "./common.js";

const errorRate = new Rate("errors");

// --- Per-endpoint duration trends ---
const durFeed        = new Trend("dur_hot_feed", true);
const durSearchNotes = new Trend("dur_search_notes", true);
const durSearchSubs  = new Trend("dur_search_subnoteries", true);
const durSearchUsers = new Trend("dur_search_users", true);
const durSearchComm  = new Trend("dur_search_comments", true);
const durSubnoteries = new Trend("dur_subnoteries_list", true);
const durBookmarks   = new Trend("dur_bookmarks", true);
const durApproved    = new Trend("dur_approved_notes", true);
const durProfile     = new Trend("dur_profile", true);

export const options = {
    stages: [
        { duration: "30s", target: 20 },
        { duration: "30s", target: 50 },
        { duration: "1m", target: 100 },
        { duration: "1m", target: 200 },
        { duration: "2m", target: 200 },
        { duration: "30s", target: 0 },
    ],
    thresholds: {
        http_req_duration: ["p(99)<2000"],
        errors: ["rate<0.20"],
    },
};

export function setup() {
    const ts = Date.now();
    const email = `k6stress${ts}@test.local`;
    const password = "Passw0rd!Stress";
    const username = `k6stress${ts}`;
    signup(email, password, username);
    const loginData = login(email, password);
    return { token: loginData.access_token };
}

export default function (data) {
    const token = data.token;

    // Stress mix — weighted to hammer the critical read paths
    const r = Math.random();

    if (r < 0.40) {
        // 40% hot feed — Redis ZREVRANGE + Postgres batch fetch
        const page = Math.floor(Math.random() * 3) + 1;
        const res = http.get(`${BASE_URL}/feed/hot?page=${page}&limit=25`);
        durFeed.add(res.timings.duration);
        const ok = check(res, { "feed ok": (r) => r.status === 200 });
        errorRate.add(!ok);
    } else if (r < 0.65) {
        // 25% search (random type for coverage)
        const types = ["notes", "subnoteries", "users", "comments"];
        const t = types[Math.floor(Math.random() * types.length)];
        const sorts = ["hot", "new", "top", "controversial"];
        const s = sorts[Math.floor(Math.random() * sorts.length)];
        const res = http.get(`${BASE_URL}/search?q=test&type=${t}&sort=${s}`);

        switch (t) {
            case "notes":       durSearchNotes.add(res.timings.duration); break;
            case "subnoteries": durSearchSubs.add(res.timings.duration);  break;
            case "users":       durSearchUsers.add(res.timings.duration); break;
            case "comments":    durSearchComm.add(res.timings.duration);  break;
        }

        const ok = check(res, { "search ok": (r) => r.status === 200 });
        errorRate.add(!ok);
    } else if (r < 0.75) {
        // 10% subnoteries list
        const res = http.get(`${BASE_URL}/subnoteries?page=1&limit=20`);
        durSubnoteries.add(res.timings.duration);
        const ok = check(res, { "subnoteries ok": (r) => r.status === 200 });
        errorRate.add(!ok);
    } else if (r < 0.85) {
        // 10% bookmarks (authenticated, Redis cart check + Postgres join)
        const res = http.get(`${BASE_URL}/bookmarks`, jsonHeaders(token));
        durBookmarks.add(res.timings.duration);
        const ok = check(res, { "bookmarks ok": (r) => r.status === 200 });
        errorRate.add(!ok);
    } else if (r < 0.93) {
        // 8% approved notes list
        const res = http.get(`${BASE_URL}/notes/approved?page=1&limit=20`, jsonHeaders(token));
        durApproved.add(res.timings.duration);
        const ok = check(res, { "approved ok": (r) => r.status === 200 });
        errorRate.add(!ok);
    } else {
        // 7% profile (fast PK lookup — acts as a low-latency baseline)
        const res = http.get(`${BASE_URL}/me/profile`, jsonHeaders(token));
        durProfile.add(res.timings.duration);
        const ok = check(res, { "profile ok": (r) => r.status === 200 });
        errorRate.add(!ok);
    }

    sleep(Math.random() * 0.5); // Minimal think time for stress
}

// --- Bottleneck analysis report ---
export function handleSummary(data) {
    return buildBottleneckReport(data, "STRESS TEST (200 VUs, 5m 30s)");
}
