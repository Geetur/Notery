// k6/stress-test.js — Stress test to find breaking points.
// Run: k6 run scripts/k6/stress-test.js
//
// Scenario: Aggressively ramp to 200 VUs to find where the API degrades.
import { check, sleep } from "k6";
import http from "k6/http";
import { Rate } from "k6/metrics";
import { BASE_URL, jsonHeaders, login, signup } from "./common.js";

const errorRate = new Rate("errors");

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

    // Hammer the hot feed and search — the most critical endpoints.
    const r = Math.random();

    if (r < 0.5) {
        const res = http.get(`${BASE_URL}/feed/hot?page=1&limit=25`);
        const ok = check(res, { "feed ok": (r) => r.status === 200 });
        errorRate.add(!ok);
    } else if (r < 0.8) {
        const sorts = ["hot", "new", "top", "controversial"];
        const s = sorts[Math.floor(Math.random() * sorts.length)];
        const res = http.get(
            `${BASE_URL}/search?q=test&type=notes&sort=${s}`
        );
        const ok = check(res, { "search ok": (r) => r.status === 200 });
        errorRate.add(!ok);
    } else if (r < 0.9) {
        const res = http.get(`${BASE_URL}/subnoteries?page=1&limit=20`);
        const ok = check(res, { "subnoteries ok": (r) => r.status === 200 });
        errorRate.add(!ok);
    } else {
        const res = http.get(`${BASE_URL}/bookmarks`, jsonHeaders(token));
        const ok = check(res, { "bookmarks ok": (r) => r.status === 200 });
        errorRate.add(!ok);
    }

    sleep(Math.random() * 0.5); // Minimal think time for stress
}
