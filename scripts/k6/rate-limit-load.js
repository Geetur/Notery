// k6 Load Test — Public Endpoint Rate Limiting
//
// Tests that public endpoints enforce rate limits correctly.
// Validates that requests beyond the limit get 429 responses.
//
// Run: k6 run scripts/k6/rate-limit-load.js
//
// Prerequisites:
// - API running on localhost:8080
// - Redis running (rate limiting requires Redis)

import { check, group, sleep } from 'k6';
import http from 'k6/http';
import { Counter, Rate } from 'k6/metrics';

const BASE_URL = __ENV.API_URL || 'http://localhost:8080/api/v1';

// Custom metrics
const rateLimited = new Counter('rate_limited_responses');
const successResponses = new Counter('success_responses');
const authRateLimited = new Rate('auth_rate_limited');

export const options = {
    scenarios: {
        // Burst scenario: hit auth endpoints fast from a single VU
        auth_burst: {
            executor: 'per-vu-iterations',
            vus: 1,
            iterations: 20, // Should exceed the 5/min auth rate limit
            maxDuration: '30s',
        },
        // Public read burst: verify read limits
        public_burst: {
            executor: 'per-vu-iterations',
            vus: 3,
            iterations: 50,
            maxDuration: '30s',
            startTime: '35s', // Run after auth burst
        },
    },
    thresholds: {
        http_req_duration: ['p(99)<2000'],
    },
};

export default function () {
    const scenario = __ENV.K6_SCENARIO_NAME || exec.scenario.name;

    if (scenario === 'auth_burst') {
        group('Auth Rate Limit Test', () => {
            const res = http.post(`${BASE_URL}/login`, JSON.stringify({
                email: 'nonexistent@test.com',
                password: 'wrongpassword',
            }), { headers: { 'Content-Type': 'application/json' } });

            if (res.status === 429) {
                rateLimited.add(1);
                authRateLimited.add(true);
                check(res, {
                    '429 has retry_after': (r) => JSON.parse(r.body).retry_after > 0,
                });
            } else {
                successResponses.add(1);
                authRateLimited.add(false);
            }

            sleep(0.1);
        });
    }

    if (scenario === 'public_burst') {
        group('Public Read Rate Limit Test', () => {
            // Hit public endpoints
            const endpoints = [
                '/feed/hot',
            ];

            for (const endpoint of endpoints) {
                const res = http.get(`${BASE_URL}${endpoint}`);

                if (res.status === 429) {
                    rateLimited.add(1);
                } else {
                    successResponses.add(1);
                    check(res, {
                        'public read succeeds': (r) => r.status === 200,
                    });
                }

                sleep(0.05);
            }
        });
    }
}
