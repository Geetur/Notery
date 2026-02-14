// k6 Load Test — Auth & Session Lifecycle
//
// Tests: signup → login → refresh → logout flow under load.
// Run: k6 run scripts/k6/auth-load.js
//
// Prerequisites:
// - API running on localhost:8080
// - k6 installed (https://k6.io/)

import { check, group, sleep } from 'k6';
import http from 'k6/http';
import { Rate, Trend } from 'k6/metrics';

const BASE_URL = __ENV.API_URL || 'http://localhost:8080/api/v1';

// Custom metrics
const authErrors = new Rate('auth_errors');
const loginDuration = new Trend('login_duration', true);
const refreshDuration = new Trend('refresh_duration', true);

export const options = {
    scenarios: {
        // Ramp-up scenario: gradual increase → steady → ramp-down
        auth_flow: {
            executor: 'ramping-vus',
            startVUs: 1,
            stages: [
                { duration: '15s', target: 10 },   // warm up
                { duration: '30s', target: 25 },   // steady state
                { duration: '30s', target: 50 },   // peak load
                { duration: '15s', target: 0 },    // cool down
            ],
        },
    },
    thresholds: {
        http_req_duration: ['p(95)<500', 'p(99)<1000'],
        auth_errors: ['rate<0.05'],
        login_duration: ['p(95)<400'],
        refresh_duration: ['p(95)<200'],
    },
};

export default function () {
    const uniqueId = `${__VU}-${__ITER}-${Date.now()}`;
    const email = `loadtest-${uniqueId}@test.com`;
    const password = 'LoadTest123!';

    group('Auth Lifecycle', () => {
        // 1. Signup
        const signupRes = http.post(`${BASE_URL}/signup`, JSON.stringify({
            email: email,
            password: password,
            username: `user-${uniqueId}`,
        }), { headers: { 'Content-Type': 'application/json' } });

        const signupOk = check(signupRes, {
            'signup returns 201': (r) => r.status === 201,
            'signup has user_id': (r) => JSON.parse(r.body).user_id > 0,
        });
        authErrors.add(!signupOk);

        sleep(0.1);

        // 2. Login
        const loginStart = Date.now();
        const loginRes = http.post(`${BASE_URL}/login`, JSON.stringify({
            email: email,
            password: password,
        }), { headers: { 'Content-Type': 'application/json' } });
        loginDuration.add(Date.now() - loginStart);

        const loginOk = check(loginRes, {
            'login returns 200': (r) => r.status === 200,
            'login has access_token': (r) => JSON.parse(r.body).access_token !== undefined,
            'login has refresh_token': (r) => JSON.parse(r.body).refresh_token !== undefined,
        });
        authErrors.add(!loginOk);

        if (!loginOk) return;

        const loginBody = JSON.parse(loginRes.body);
        const accessToken = loginBody.access_token;
        let refreshToken = loginBody.refresh_token;

        sleep(0.1);

        // 3. Refresh token (rotate 3 times)
        for (let i = 0; i < 3; i++) {
            const refreshStart = Date.now();
            const refreshRes = http.post(`${BASE_URL}/auth/refresh`, JSON.stringify({
                refresh_token: refreshToken,
            }), { headers: { 'Content-Type': 'application/json' } });
            refreshDuration.add(Date.now() - refreshStart);

            const refreshOk = check(refreshRes, {
                [`refresh ${i + 1} returns 200`]: (r) => r.status === 200,
                [`refresh ${i + 1} has new tokens`]: (r) => {
                    const body = JSON.parse(r.body);
                    return body.access_token && body.refresh_token;
                },
            });
            authErrors.add(!refreshOk);

            if (refreshOk) {
                refreshToken = JSON.parse(refreshRes.body).refresh_token;
            } else {
                break;
            }

            sleep(0.05);
        }

        // 4. Logout
        const logoutRes = http.post(`${BASE_URL}/auth/logout`, JSON.stringify({
            refresh_token: refreshToken,
        }), { headers: { 'Content-Type': 'application/json' } });

        check(logoutRes, {
            'logout returns 200': (r) => r.status === 200,
        });

        // 5. Verify old token is revoked
        const reuseRes = http.post(`${BASE_URL}/auth/refresh`, JSON.stringify({
            refresh_token: refreshToken,
        }), { headers: { 'Content-Type': 'application/json' } });

        check(reuseRes, {
            'revoked token returns 401': (r) => r.status === 401,
        });
    });

    sleep(0.5);
}
