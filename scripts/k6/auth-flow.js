// k6/auth-flow.js — Auth-focused load test covering signup, login, refresh, and profile.
// Run: k6 run scripts/k6/auth-flow.js
//
// This test isolates CPU-bound auth endpoints (bcrypt) and Postgres-bound
// session operations (refresh token rotation, logout). The bottleneck report
// shows exactly where latency spends its time.
import { check, sleep } from "k6";
import http from "k6/http";
import { Rate, Trend } from "k6/metrics";
import { BASE_URL, jsonHeaders, buildBottleneckReport } from "./common.js";

const errorRate = new Rate("errors");

// --- Per-endpoint duration trends ---
const durSignup  = new Trend("dur_signup", true);
const durLogin   = new Trend("dur_login", true);
const durProfile = new Trend("dur_profile", true);
const durRefresh = new Trend("dur_refresh", true);
const durLogout  = new Trend("dur_logout", true);

export const options = {
    stages: [
        { duration: "15s", target: 5 },
        { duration: "1m", target: 20 },
        { duration: "30s", target: 0 },
    ],
    thresholds: {
        http_req_duration: ["p(95)<800"],
        errors: ["rate<0.05"],
    },
};

export default function () {
    const ts = `${Date.now()}${__VU}${__ITER}`;
    const email = `k6auth${ts}@test.local`;
    const password = "Passw0rd!Auth";
    const username = `k6a${ts}`.substring(0, 30);

    // 1. Signup (CPU-bound: bcrypt hash)
    const signupRes = http.post(
        `${BASE_URL}/auth/signup`,
        JSON.stringify({ email, password, username }),
        jsonHeaders()
    );
    durSignup.add(signupRes.timings.duration);
    const signupOk = check(signupRes, {
        "signup 2xx": (r) => r.status >= 200 && r.status < 300,
    });
    errorRate.add(!signupOk);
    if (!signupOk) {
        sleep(1);
        return;
    }
    const refreshToken = signupRes.json("refresh_token");

    // 2. Login (CPU-bound: bcrypt verify)
    const loginRes = http.post(
        `${BASE_URL}/auth/login`,
        JSON.stringify({ email, password }),
        jsonHeaders()
    );
    durLogin.add(loginRes.timings.duration);
    const loginOk = check(loginRes, { "login 200": (r) => r.status === 200 });
    errorRate.add(!loginOk);
    const token = loginRes.json("access_token");

    // 3. Get profile (Postgres PK lookup — fast baseline)
    const profileRes = http.get(`${BASE_URL}/me/profile`, jsonHeaders(token));
    durProfile.add(profileRes.timings.duration);
    const profileOk = check(profileRes, {
        "profile 200": (r) => r.status === 200,
    });
    errorRate.add(!profileOk);

    // 4. Refresh token (Postgres row lock on refresh_tokens table)
    if (refreshToken) {
        const refreshRes = http.post(
            `${BASE_URL}/auth/refresh`,
            JSON.stringify({ refresh_token: refreshToken }),
            jsonHeaders()
        );
        durRefresh.add(refreshRes.timings.duration);
        check(refreshRes, { "refresh 200": (r) => r.status === 200 });
    }

    // 5. Logout (Postgres UPDATE on refresh_tokens)
    if (refreshToken) {
        const logoutRes = http.post(
            `${BASE_URL}/auth/logout`,
            JSON.stringify({ refresh_token: refreshToken }),
            jsonHeaders(token)
        );
        durLogout.add(logoutRes.timings.duration);
        check(logoutRes, { "logout 200": (r) => r.status === 200 });
    }

    sleep(1);
}

// --- Bottleneck analysis report ---
export function handleSummary(data) {
    return buildBottleneckReport(data, "AUTH FLOW (20 VUs, 1m 45s)");
}
