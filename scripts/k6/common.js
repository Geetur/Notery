// k6/common.js — Shared helpers for k6 load tests.
import http from "k6/http";

// Base URL for the API.
export const BASE_URL = __ENV.BASE_URL || "http://localhost:8080/api/v1";

// Default request headers.
export function jsonHeaders(token) {
    const h = { "Content-Type": "application/json" };
    if (token) h["Authorization"] = `Bearer ${token}`;
    return { headers: h };
}

// Signup a user and return { access_token, refresh_token }.
export function signup(email, password, username) {
    const res = http.post(
        `${BASE_URL}/signup`,
        JSON.stringify({ email, password, username }),
        jsonHeaders()
    );
    return res.json();
}

// Login and return { access_token, refresh_token }.
export function login(email, password) {
    const res = http.post(
        `${BASE_URL}/login`,
        JSON.stringify({ email, password }),
        jsonHeaders()
    );
    return res.json();
}

// Check response status and tag errors.
export function check200(res, name) {
    if (res.status !== 200 && res.status !== 201) {
        console.error(`${name} failed: ${res.status} ${res.body}`);
        return false;
    }
    return true;
}
