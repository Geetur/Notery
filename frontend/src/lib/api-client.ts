// api-client.ts — Centralised HTTP client for the Notery API.
// Handles auth token injection, automatic refresh on 401, and error normalisation.
// All API calls go through this module.

import { API_V1, TOKEN_KEY, REFRESH_TOKEN_KEY } from "./config";
import type { ApiError, AuthResponse } from "@/types";

// ─── Token Management ─────────────────────────────────────────────────────────

/** Get the current access token from localStorage. */
export function getAccessToken(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem(TOKEN_KEY);
}

/** Get the current refresh token from localStorage. */
export function getRefreshToken(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem(REFRESH_TOKEN_KEY);
}

/** Store both tokens after login/signup/refresh. */
export function setTokens(accessToken: string, refreshToken: string): void {
  if (typeof window === "undefined") return;
  localStorage.setItem(TOKEN_KEY, accessToken);
  localStorage.setItem(REFRESH_TOKEN_KEY, refreshToken);
}

/** Clear all tokens (logout). */
export function clearTokens(): void {
  if (typeof window === "undefined") return;
  localStorage.removeItem(TOKEN_KEY);
  localStorage.removeItem(REFRESH_TOKEN_KEY);
}

// ─── Error Handling ───────────────────────────────────────────────────────────

/** Custom error class for API errors with status code and parsed body. */
export class ApiRequestError extends Error {
  status: number;
  body: ApiError;

  constructor(status: number, body: ApiError) {
    super(body.error || `API error ${status}`);
    this.name = "ApiRequestError";
    this.status = status;
    this.body = body;
  }
}

// ─── Token Refresh ────────────────────────────────────────────────────────────

let refreshPromise: Promise<boolean> | null = null;

/**
 * Attempt to refresh the access token using the stored refresh token.
 * Uses a singleton promise to prevent concurrent refresh attempts.
 * Returns true if refresh succeeded, false otherwise.
 */
async function attemptTokenRefresh(): Promise<boolean> {
  // Deduplicate concurrent refresh calls
  if (refreshPromise) return refreshPromise;

  refreshPromise = (async () => {
    const refreshToken = getRefreshToken();
    if (!refreshToken) return false;

    try {
      const res = await fetch(`${API_V1}/auth/refresh`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ refresh_token: refreshToken }),
      });

      if (!res.ok) {
        clearTokens();
        return false;
      }

      const data: AuthResponse = await res.json();
      setTokens(data.access_token, data.refresh_token);
      return true;
    } catch {
      clearTokens();
      return false;
    }
  })();

  try {
    return await refreshPromise;
  } finally {
    refreshPromise = null;
  }
}

// ─── Core Fetch ───────────────────────────────────────────────────────────────

/**
 * Make an authenticated API request. Automatically:
 * 1. Attaches Bearer token if available
 * 2. Retries once on 401 after refreshing token
 * 3. Throws ApiRequestError for non-2xx responses
 *
 * @param path - API path relative to /api/v1 (e.g., "/feed/hot")
 * @param options - Standard fetch options
 * @returns Parsed JSON response
 */
export async function apiFetch<T>(
  path: string,
  options: RequestInit = {}
): Promise<T> {
  const url = `${API_V1}${path}`;

  const makeRequest = async (): Promise<Response> => {
    const token = getAccessToken();
    const headers: Record<string, string> = {
      ...(options.headers as Record<string, string>),
    };

    if (token) {
      headers["Authorization"] = `Bearer ${token}`;
    }

    // Only set Content-Type for JSON bodies (not FormData uploads)
    if (options.body && !(options.body instanceof FormData)) {
      headers["Content-Type"] = "application/json";
    }

    return fetch(url, { ...options, headers });
  };

  let response = await makeRequest();

  // Auto-refresh on 401
  if (response.status === 401 && getRefreshToken()) {
    const refreshed = await attemptTokenRefresh();
    if (refreshed) {
      response = await makeRequest();
    }
  }

  // Handle non-2xx responses
  if (!response.ok) {
    let body: ApiError;
    try {
      body = await response.json();
    } catch {
      body = { error: `HTTP ${response.status}: ${response.statusText}` };
    }
    throw new ApiRequestError(response.status, body);
  }

  // Handle 204 No Content
  if (response.status === 204) {
    return {} as T;
  }

  return response.json();
}

// ─── Convenience Methods ──────────────────────────────────────────────────────

/** GET request. */
export function apiGet<T>(path: string): Promise<T> {
  return apiFetch<T>(path, { method: "GET" });
}

/** POST request with JSON body. */
export function apiPost<T>(path: string, body?: unknown): Promise<T> {
  return apiFetch<T>(path, {
    method: "POST",
    body: body ? JSON.stringify(body) : undefined,
  });
}

/** PUT request with JSON body. */
export function apiPut<T>(path: string, body?: unknown): Promise<T> {
  return apiFetch<T>(path, {
    method: "PUT",
    body: body ? JSON.stringify(body) : undefined,
  });
}

/** PATCH request with JSON body. */
export function apiPatch<T>(path: string, body?: unknown): Promise<T> {
  return apiFetch<T>(path, {
    method: "PATCH",
    body: body ? JSON.stringify(body) : undefined,
  });
}

/** DELETE request. */
export function apiDelete<T>(path: string): Promise<T> {
  return apiFetch<T>(path, { method: "DELETE" });
}
