// auth.test.ts — Tests for auth service functions.

// Mock the api-client module
jest.mock("@/lib/api-client", () => ({
  apiPost: jest.fn(),
  apiGet: jest.fn(),
  setTokens: jest.fn(),
  clearTokens: jest.fn(),
  getRefreshToken: jest.fn(),
}));

import {
  login,
  signup,
  forgotPassword,
  resetPassword,
  changePassword,
} from "@/services/auth";
import { apiPost } from "@/lib/api-client";

const mockApiPost = apiPost as jest.MockedFunction<typeof apiPost>;

beforeEach(() => {
  jest.clearAllMocks();
});

describe("login", () => {
  it("calls POST /auth/login with credentials", async () => {
    const mockResponse = {
      access_token: "access123",
      refresh_token: "refresh456",
    };
    mockApiPost.mockResolvedValue(mockResponse);

    const result = await login("test@example.com", "password123");
    expect(mockApiPost).toHaveBeenCalledWith("/auth/login", {
      email: "test@example.com",
      password: "password123",
    });
    expect(result).toEqual(mockResponse);
  });
});

describe("signup", () => {
  it("calls POST /auth/signup with user data", async () => {
    const mockResponse = {
      access_token: "access123",
      refresh_token: "refresh456",
    };
    mockApiPost.mockResolvedValue(mockResponse);

    const result = await signup("user@test.com", "pass1234", "user1");
    expect(mockApiPost).toHaveBeenCalledWith("/auth/signup", {
      email: "user@test.com",
      password: "pass1234",
      username: "user1",
    });
    expect(result).toEqual(mockResponse);
  });
});

describe("forgotPassword", () => {
  it("calls POST /auth/forgot-password", async () => {
    mockApiPost.mockResolvedValue({ message: "ok" });

    await forgotPassword("test@example.com");
    expect(mockApiPost).toHaveBeenCalledWith("/auth/forgot-password", {
      email: "test@example.com",
    });
  });
});

describe("resetPassword", () => {
  it("calls POST /auth/reset-password with token and new password", async () => {
    mockApiPost.mockResolvedValue({ message: "ok" });

    await resetPassword("token123", "newpassword");
    expect(mockApiPost).toHaveBeenCalledWith("/auth/reset-password", {
      token: "token123",
      new_password: "newpassword",
    });
  });
});

describe("changePassword", () => {
  it("calls POST /auth/change-password", async () => {
    mockApiPost.mockResolvedValue({ message: "ok" });

    await changePassword("oldpass", "newpass");
    expect(mockApiPost).toHaveBeenCalledWith("/auth/change-password", {
      current_password: "oldpass",
      new_password: "newpass",
    });
  });
});
