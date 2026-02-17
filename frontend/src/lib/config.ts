// config.ts — Frontend environment configuration.
// All environment variables are accessed through this module for centralised control.

/** Base URL for the Notery API. Defaults to localhost:8080 in development. */
export const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

/** API v1 prefix. All endpoints are under this path. */
export const API_V1 = `${API_BASE_URL}/api/v1`;

/** LocalStorage keys for auth tokens. */
export const TOKEN_KEY = "notery_access_token";
export const REFRESH_TOKEN_KEY = "notery_refresh_token";

/** Default pagination limit. */
export const DEFAULT_PAGE_SIZE = 25;

/** Max pagination limit (server enforced). */
export const MAX_PAGE_SIZE = 100;
