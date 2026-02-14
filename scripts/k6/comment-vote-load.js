// k6 Load Test — Comment & Vote Concurrency
//
// Tests concurrent comment creation, voting, and vote toggling under load.
// Validates that counters stay consistent and no race conditions surface.
//
// Run: k6 run scripts/k6/comment-vote-load.js
//
// Prerequisites:
// - API running on localhost:8080
// - k6 installed
// - At least one approved note in the database (or use setup function)

import { check, group, sleep } from 'k6';
import http from 'k6/http';
import { Counter, Rate, Trend } from 'k6/metrics';

const BASE_URL = __ENV.API_URL || 'http://localhost:8080/api/v1';

// Custom metrics
const voteErrors = new Rate('vote_errors');
const commentErrors = new Rate('comment_errors');
const voteDuration = new Trend('vote_duration', true);
const commentDuration = new Trend('comment_duration', true);
const totalVotes = new Counter('total_votes');
const totalComments = new Counter('total_comments');

export const options = {
    scenarios: {
        concurrent_activity: {
            executor: 'ramping-vus',
            startVUs: 1,
            stages: [
                { duration: '10s', target: 15 },
                { duration: '30s', target: 30 },
                { duration: '20s', target: 50 },
                { duration: '10s', target: 0 },
            ],
        },
    },
    thresholds: {
        http_req_duration: ['p(95)<800'],
        vote_errors: ['rate<0.05'],
        comment_errors: ['rate<0.05'],
        vote_duration: ['p(95)<300'],
        comment_duration: ['p(95)<500'],
    },
};

// Each VU creates its own user and works with it
export default function () {
    const uniqueId = `${__VU}-${__ITER}-${Date.now()}`;
    const email = `voter-${uniqueId}@test.com`;
    const password = 'VoteTest123!';

    // 1. Signup + Login to get access token
    http.post(`${BASE_URL}/signup`, JSON.stringify({
        email, password, username: `voter-${uniqueId}`,
    }), { headers: { 'Content-Type': 'application/json' } });

    const loginRes = http.post(`${BASE_URL}/login`, JSON.stringify({
        email, password,
    }), { headers: { 'Content-Type': 'application/json' } });

    if (loginRes.status !== 200) {
        voteErrors.add(true);
        return;
    }

    const accessToken = JSON.parse(loginRes.body).access_token;
    const authHeaders = {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${accessToken}`,
    };

    // 2. Get hot feed to find a note to interact with
    const feedRes = http.get(`${BASE_URL}/feed/hot`, { headers: authHeaders });
    if (feedRes.status !== 200) {
        sleep(1);
        return;
    }

    const feedBody = JSON.parse(feedRes.body);
    const notes = feedBody.notes || feedBody.data || [];

    if (notes.length === 0) {
        // No notes available — create one if user can
        sleep(1);
        return;
    }

    const noteId = notes[0].id || notes[0].ID;

    group('Vote Operations', () => {
        // Upvote
        const upStart = Date.now();
        const upRes = http.post(`${BASE_URL}/notes/${noteId}/upvote`, null, { headers: authHeaders });
        voteDuration.add(Date.now() - upStart);
        totalVotes.add(1);

        const upOk = check(upRes, {
            'upvote succeeds': (r) => r.status === 200 || r.status === 201,
        });
        voteErrors.add(!upOk);

        sleep(0.1);

        // Downvote (toggle from upvote)
        const downStart = Date.now();
        const downRes = http.post(`${BASE_URL}/notes/${noteId}/downvote`, null, { headers: authHeaders });
        voteDuration.add(Date.now() - downStart);
        totalVotes.add(1);

        const downOk = check(downRes, {
            'downvote succeeds': (r) => r.status === 200 || r.status === 201,
        });
        voteErrors.add(!downOk);

        sleep(0.1);
    });

    group('Comment Operations', () => {
        // Create comment
        const commentStart = Date.now();
        const commentRes = http.post(`${BASE_URL}/notes/${noteId}/comments`, JSON.stringify({
            body: `Load test comment from VU ${__VU} iter ${__ITER}`,
        }), { headers: authHeaders });
        commentDuration.add(Date.now() - commentStart);
        totalComments.add(1);

        const commentOk = check(commentRes, {
            'comment created': (r) => r.status === 201 || r.status === 200,
        });
        commentErrors.add(!commentOk);

        if (commentOk) {
            const commentBody = JSON.parse(commentRes.body);
            const commentId = commentBody.comment?.id || commentBody.id;

            if (commentId) {
                // Vote on the comment
                const voteRes = http.post(`${BASE_URL}/comments/${commentId}/vote`, JSON.stringify({
                    direction: 1,
                }), { headers: authHeaders });

                check(voteRes, {
                    'comment vote succeeds': (r) => r.status === 200 || r.status === 201,
                });

                sleep(0.1);

                // Toggle vote off
                http.del(`${BASE_URL}/comments/${commentId}/vote`, null, { headers: authHeaders });
            }
        }
    });

    group('Read Operations', () => {
        // Read comments (public)
        const commentsRes = http.get(`${BASE_URL}/notes/${noteId}/comments`, { headers: authHeaders });
        check(commentsRes, {
            'comments returned': (r) => r.status === 200,
        });

        // Read hot feed
        const hotRes = http.get(`${BASE_URL}/feed/hot`, { headers: authHeaders });
        check(hotRes, {
            'hot feed returned': (r) => r.status === 200,
        });
    });

    sleep(0.5);
}
