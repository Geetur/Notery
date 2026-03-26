// config.test.ts — Tests for configuration module.
import { API_BASE_URL, API_V1, DEFAULT_PAGE_SIZE, TOKEN_KEY } from "@/lib/config";

describe("config", () => {
    it("has a default API base URL", () => {
        expect(API_BASE_URL).toBeDefined();
        expect(typeof API_BASE_URL).toBe("string");
    });

    it("builds API_V1 from base URL", () => {
        expect(API_V1).toBe(`${API_BASE_URL}/api/v1`);
    });

    it("has token storage key", () => {
        expect(TOKEN_KEY).toBe("notery_access_token");
    });

    it("has sensible pagination defaults", () => {
        expect(DEFAULT_PAGE_SIZE).toBeGreaterThan(0);
        expect(DEFAULT_PAGE_SIZE).toBeLessThanOrEqual(100);
    });
});
