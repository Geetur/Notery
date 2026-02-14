// k6 Load Test — Purchase & Order Lifecycle
//
// Tests the checkout flow under concurrent load:
// signup → login → add to cart → checkout → poll order status
//
// Run: k6 run scripts/k6/purchase-load.js
//
// Prerequisites:
// - API running on localhost:8080
// - At least one approved note with a price set
// - Stripe in test mode (or mock payment service)

import { check, group, sleep } from 'k6';
import http from 'k6/http';
import { Rate, Trend } from 'k6/metrics';

const BASE_URL = __ENV.API_URL || 'http://localhost:8080/api/v1';

// Custom metrics
const checkoutErrors = new Rate('checkout_errors');
const checkoutDuration = new Trend('checkout_duration', true);
const orderPollDuration = new Trend('order_poll_duration', true);

export const options = {
    scenarios: {
        purchase_flow: {
            executor: 'ramping-vus',
            startVUs: 1,
            stages: [
                { duration: '10s', target: 5 },
                { duration: '20s', target: 15 },
                { duration: '10s', target: 0 },
            ],
        },
    },
    thresholds: {
        http_req_duration: ['p(95)<1000'],
        checkout_errors: ['rate<0.1'],
        checkout_duration: ['p(95)<800'],
    },
};

export default function () {
    const uniqueId = `${__VU}-${__ITER}-${Date.now()}`;
    const email = `buyer-${uniqueId}@test.com`;
    const password = 'BuyerTest123!';

    // 1. Signup + Login
    http.post(`${BASE_URL}/signup`, JSON.stringify({
        email, password, username: `buyer-${uniqueId}`,
    }), { headers: { 'Content-Type': 'application/json' } });

    const loginRes = http.post(`${BASE_URL}/login`, JSON.stringify({
        email, password,
    }), { headers: { 'Content-Type': 'application/json' } });

    if (loginRes.status !== 200) {
        checkoutErrors.add(true);
        return;
    }

    const accessToken = JSON.parse(loginRes.body).access_token;
    const authHeaders = {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${accessToken}`,
    };

    group('Purchase Flow', () => {
        // 2. Get approved notes
        const notesRes = http.get(`${BASE_URL}/notes/approved`, { headers: authHeaders });
        if (notesRes.status !== 200) {
            checkoutErrors.add(true);
            return;
        }

        const notes = JSON.parse(notesRes.body).notes || JSON.parse(notesRes.body).data || [];
        if (notes.length === 0) {
            sleep(1);
            return;
        }

        const noteId = notes[0].id || notes[0].ID;

        // 3. Add to cart
        const cartRes = http.post(`${BASE_URL}/cart`, JSON.stringify({
            note_id: noteId,
        }), { headers: authHeaders });

        check(cartRes, {
            'added to cart': (r) => r.status === 200 || r.status === 201,
        });

        sleep(0.1);

        // 4. Checkout
        const checkoutStart = Date.now();
        const checkoutRes = http.post(`${BASE_URL}/checkout`, null, { headers: authHeaders });
        checkoutDuration.add(Date.now() - checkoutStart);

        const checkoutOk = check(checkoutRes, {
            'checkout succeeds': (r) => r.status === 200 || r.status === 201,
        });
        checkoutErrors.add(!checkoutOk);

        if (checkoutOk) {
            const checkoutBody = JSON.parse(checkoutRes.body);
            const orderId = checkoutBody.order_id || checkoutBody.order?.id;

            if (orderId) {
                // 5. Poll order status (simulates frontend polling)
                for (let i = 0; i < 5; i++) {
                    const pollStart = Date.now();
                    const orderRes = http.get(`${BASE_URL}/orders/${orderId}`, { headers: authHeaders });
                    orderPollDuration.add(Date.now() - pollStart);

                    check(orderRes, {
                        'order status returned': (r) => r.status === 200,
                    });

                    const orderBody = JSON.parse(orderRes.body);
                    if (orderBody.status === 'completed' || orderBody.status === 'fulfilled') {
                        break;
                    }

                    sleep(0.5); // Poll interval
                }
            }
        }

        // 6. Check purchase status
        const purchaseRes = http.get(`${BASE_URL}/notes/${noteId}/purchased`, { headers: authHeaders });
        check(purchaseRes, {
            'purchase status returned': (r) => r.status === 200,
        });
    });

    sleep(1);
}
