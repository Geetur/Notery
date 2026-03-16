// k6/smoke-test.js — Quick smoke test hitting all major endpoints.
// Run: k6 run scripts/k6/smoke-test.js
//
// Outputs a bottleneck analysis report after the run showing per-endpoint
// latency breakdown and resource classification.
import { check, sleep } from "k6";
import http from "k6/http";
import { Trend } from "k6/metrics";
import { BASE_URL, jsonHeaders, buildBottleneckReport } from "./common.js";

// --- Per-endpoint duration trends ---
const durHealth      = new Trend("dur_health", true);
const durSignup      = new Trend("dur_signup", true);
const durLogin       = new Trend("dur_login", true);
const durHotFeed     = new Trend("dur_hot_feed", true);
const durSearch      = new Trend("dur_search_notes", true);
const durSubnoteries = new Trend("dur_subnoteries_list", true);
const durProfile     = new Trend("dur_profile", true);
const durApproved    = new Trend("dur_approved_notes", true);
const durCart        = new Trend("dur_cart", true);
const durBookmarks   = new Trend("dur_bookmarks", true);
const durMyNotes     = new Trend("dur_my_notes", true);

export const options = {
    vus: 1,
    iterations: 1,
    thresholds: {
        http_req_failed: ["rate<0.01"],
    },
};

export default function () {
    // 1. Health check
    const health = http.get(`${BASE_URL.replace("/api/v1", "")}/health`);
    durHealth.add(health.timings.duration);
    check(health, { "health 200": (r) => r.status === 200 });

    // 2. Signup + Login
    const ts = Date.now();
    const email = `k6smoke${ts}@test.local`;
    const password = "Passw0rd!Smoke";
    const username = `k6smoke${ts}`;

    const signupRes = http.post(
        `${BASE_URL}/auth/signup`,
        JSON.stringify({ email, password, username }),
        jsonHeaders()
    );
    durSignup.add(signupRes.timings.duration);
    check(signupRes, { "signup 2xx": (r) => r.status >= 200 && r.status < 300 });

    const loginRes = http.post(
        `${BASE_URL}/auth/login`,
        JSON.stringify({ email, password }),
        jsonHeaders()
    );
    durLogin.add(loginRes.timings.duration);
    check(loginRes, { "login 200": (r) => r.status === 200 });
    const token = loginRes.json("access_token");

    // 3. Public endpoints
    const feed = http.get(`${BASE_URL}/feed/hot`);
    durHotFeed.add(feed.timings.duration);
    check(feed, { "hot feed 200": (r) => r.status === 200 });

    const searchRes = http.get(`${BASE_URL}/search?q=test&type=notes`);
    durSearch.add(searchRes.timings.duration);
    check(searchRes, { "search 200": (r) => r.status === 200 });

    const subnoteries = http.get(`${BASE_URL}/subnoteries`);
    durSubnoteries.add(subnoteries.timings.duration);
    check(subnoteries, { "subnoteries 200": (r) => r.status === 200 });

    // 4. Authenticated endpoints
    const profile = http.get(`${BASE_URL}/me/profile`, jsonHeaders(token));
    durProfile.add(profile.timings.duration);
    check(profile, { "profile 200": (r) => r.status === 200 });

    const approved = http.get(`${BASE_URL}/notes/approved`, jsonHeaders(token));
    durApproved.add(approved.timings.duration);
    check(approved, { "approved notes 200": (r) => r.status === 200 });

    const cart = http.get(`${BASE_URL}/cart`, jsonHeaders(token));
    durCart.add(cart.timings.duration);
    check(cart, { "cart 200": (r) => r.status === 200 });

    const bookmarks = http.get(`${BASE_URL}/bookmarks`, jsonHeaders(token));
    durBookmarks.add(bookmarks.timings.duration);
    check(bookmarks, { "bookmarks 200": (r) => r.status === 200 });

    const myNotes = http.get(`${BASE_URL}/me/notes`, jsonHeaders(token));
    durMyNotes.add(myNotes.timings.duration);
    check(myNotes, { "my notes 200": (r) => r.status === 200 });

    sleep(0.5);
}

// --- Bottleneck analysis report ---
export function handleSummary(data) {
    return buildBottleneckReport(data, "SMOKE TEST (1 VU, 1 iteration)");
}
