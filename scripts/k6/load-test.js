// k6/load-test.js — Sustained load test simulating realistic traffic patterns.
// Run: k6 run scripts/k6/load-test.js
//
// Scenario: Ramp up to 50 VUs over 1 minute, hold for 3 minutes, ramp down.
// Each VU cycles through read-heavy operations (feed, search, notes).
import { check, sleep } from "k6";
import http from "k6/http";
import { Rate, Trend } from "k6/metrics";
import { BASE_URL, jsonHeaders, login, signup } from "./common.js";

const errorRate = new Rate("errors");
const feedDuration = new Trend("feed_duration", true);
const searchDuration = new Trend("search_duration", true);

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
        feedDuration.add(res.timings.duration);
        const ok = check(res, { "feed 200": (r) => r.status === 200 });
        errorRate.add(!ok);
    }

    // 20% search
    if (Math.random() < 0.2) {
        const types = ["notes", "subnoteries", "users", "comments"];
        const t = types[Math.floor(Math.random() * types.length)];
        const sorts = ["relevance", "hot", "new", "top"];
        const s = sorts[Math.floor(Math.random() * sorts.length)];
        const res = http.get(`${BASE_URL}/search?q=test&type=${t}&sort=${s}`);
        searchDuration.add(res.timings.duration);
        const ok = check(res, { "search 200": (r) => r.status === 200 });
        errorRate.add(!ok);
    }

    // 10% approved notes list
    if (Math.random() < 0.1) {
        const res = http.get(`${BASE_URL}/notes/approved`, jsonHeaders(token));
        const ok = check(res, { "approved 200": (r) => r.status === 200 });
        errorRate.add(!ok);
    }

    // 5% profile
    if (Math.random() < 0.05) {
        const res = http.get(`${BASE_URL}/me/profile`, jsonHeaders(token));
        const ok = check(res, { "profile 200": (r) => r.status === 200 });
        errorRate.add(!ok);
    }

    // 5% subnoteries list
    if (Math.random() < 0.05) {
        const res = http.get(`${BASE_URL}/subnoteries`);
        const ok = check(res, { "subnoteries 200": (r) => r.status === 200 });
        errorRate.add(!ok);
    }

    // 5% bookmarks
    if (Math.random() < 0.05) {
        const res = http.get(`${BASE_URL}/bookmarks`, jsonHeaders(token));
        const ok = check(res, { "bookmarks 200": (r) => r.status === 200 });
        errorRate.add(!ok);
    }

    sleep(Math.random() * 2 + 0.5); // 0.5–2.5s think time
}
