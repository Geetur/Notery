// k6/smoke-test.js — Quick smoke test hitting all major endpoints.
// Run: k6 run scripts/k6/smoke-test.js
import { check, sleep } from "k6";
import http from "k6/http";
import { BASE_URL, jsonHeaders } from "./common.js";

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
    check(health, { "health 200": (r) => r.status === 200 });

    // 2. Signup + Login
    const ts = Date.now();
    const email = `k6smoke${ts}@test.local`;
    const password = "Passw0rd!Smoke";
    const username = `k6smoke${ts}`;

    const signupRes = http.post(
        `${BASE_URL}/signup`,
        JSON.stringify({ email, password, username }),
        jsonHeaders()
    );
    check(signupRes, { "signup 2xx": (r) => r.status >= 200 && r.status < 300 });

    const loginRes = http.post(
        `${BASE_URL}/login`,
        JSON.stringify({ email, password }),
        jsonHeaders()
    );
    check(loginRes, { "login 200": (r) => r.status === 200 });
    const token = loginRes.json("access_token");

    // 3. Public endpoints
    const feed = http.get(`${BASE_URL}/feed/hot`);
    check(feed, { "hot feed 200": (r) => r.status === 200 });

    const searchRes = http.get(`${BASE_URL}/search?q=test&type=notes`);
    check(searchRes, { "search 200": (r) => r.status === 200 });

    const subnoteries = http.get(`${BASE_URL}/subnoteries`);
    check(subnoteries, { "subnoteries 200": (r) => r.status === 200 });

    // 4. Authenticated endpoints
    const profile = http.get(`${BASE_URL}/me/profile`, jsonHeaders(token));
    check(profile, { "profile 200": (r) => r.status === 200 });

    const approved = http.get(`${BASE_URL}/notes/approved`, jsonHeaders(token));
    check(approved, { "approved notes 200": (r) => r.status === 200 });

    const cart = http.get(`${BASE_URL}/cart`, jsonHeaders(token));
    check(cart, { "cart 200": (r) => r.status === 200 });

    const bookmarks = http.get(`${BASE_URL}/bookmarks`, jsonHeaders(token));
    check(bookmarks, { "bookmarks 200": (r) => r.status === 200 });

    const myNotes = http.get(`${BASE_URL}/me/notes`, jsonHeaders(token));
    check(myNotes, { "my notes 200": (r) => r.status === 200 });

    sleep(0.5);
}
