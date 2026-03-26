// api-client.test.ts — Tests for the API client token management and error handling.

// Mock localStorage
const mockStorage: Record<string, string> = {};
const localStorageMock = {
    getItem: jest.fn((key: string) => mockStorage[key] ?? null),
    setItem: jest.fn((key: string, value: string) => {
        mockStorage[key] = value;
    }),
    removeItem: jest.fn((key: string) => {
        delete mockStorage[key];
    }),
    clear: jest.fn(() => {
        for (const key in mockStorage) delete mockStorage[key];
    }),
    length: 0,
    key: jest.fn(),
};
Object.defineProperty(window, "localStorage", { value: localStorageMock });

import {
    ApiRequestError,
    clearTokens,
    getAccessToken,
    setTokens,
} from "@/lib/api-client";

beforeEach(() => {
    jest.clearAllMocks();
    localStorageMock.clear();
});

describe("Token management", () => {
    it("returns null when no token is stored", () => {
        expect(getAccessToken()).toBeNull();
    });

    it("stores and retrieves access token", () => {
        setTokens("access123");
        expect(localStorageMock.setItem).toHaveBeenCalledWith(
            "notery_access_token",
            "access123"
        );
    });

    it("clears access token and legacy refresh token", () => {
        setTokens("a");
        clearTokens();
        expect(localStorageMock.removeItem).toHaveBeenCalledWith(
            "notery_access_token"
        );
        expect(localStorageMock.removeItem).toHaveBeenCalledWith(
            "notery_refresh_token"
        );
    });
});

describe("ApiRequestError", () => {
    it("creates error with status and body", () => {
        const err = new ApiRequestError(404, { error: "Not found" });
        expect(err.message).toBe("Not found");
        expect(err.status).toBe(404);
        expect(err.body).toEqual({ error: "Not found" });
        expect(err).toBeInstanceOf(Error);
    });

    it("uses status code in message when no error text", () => {
        const err = new ApiRequestError(500, {} as { error: string });
        expect(err.message).toBe("API error 500");
    });
});
