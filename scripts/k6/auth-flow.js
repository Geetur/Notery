// k6/auth-flow.js — Auth-focused load test covering signup, login, refresh, and profile.
// Run: k6 run scripts/k6/auth-flow.js
import { check, sleep } from "k6";
import http from "k6/http";
import { Rate } from "k6/metrics";
import { BASE_URL, jsonHeaders } from "./common.js";

const errorRate = new Rate("errors");

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

    // 1. Signup
    const signupRes = http.post(
        `${BASE_URL}/auth/signup`,
        JSON.stringify({ email, password, username }),
        jsonHeaders()
    );
    const signupOk = check(signupRes, {
        "signup 2xx": (r) => r.status >= 200 && r.status < 300,
    });
    errorRate.add(!signupOk);
    if (!signupOk) {
        sleep(1);
        return;
    }
    const refreshToken = signupRes.json("refresh_token");

    // 2. Login
    const loginRes = http.post(
        `${BASE_URL}/auth/login`,
        JSON.stringify({ email, password }),
        jsonHeaders()
    );
    const loginOk = check(loginRes, { "login 200": (r) => r.status === 200 });
    errorRate.add(!loginOk);
    const token = loginRes.json("access_token");

    // 3. Get profile
    const profileRes = http.get(`${BASE_URL}/me/profile`, jsonHeaders(token));
    const profileOk = check(profileRes, {
        "profile 200": (r) => r.status === 200,
    });
    errorRate.add(!profileOk);

    // 4. Refresh token
    if (refreshToken) {
        const refreshRes = http.post(
            `${BASE_URL}/auth/refresh`,
            JSON.stringify({ refresh_token: refreshToken }),
            jsonHeaders()
        );
        check(refreshRes, { "refresh 200": (r) => r.status === 200 });
    }

    // 5. Logout
    if (refreshToken) {
        http.post(
            `${BASE_URL}/auth/logout`,
            JSON.stringify({ refresh_token: refreshToken }),
            jsonHeaders(token)
        );
    }

    sleep(1);
}
