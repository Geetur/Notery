// auth-store.test.ts — Tests for the Zustand auth store.
import { useAuthStore } from "@/stores/auth-store";

// Mock the api-client module
jest.mock("@/lib/api-client", () => ({
  getAccessToken: jest.fn(() => null),
  clearTokens: jest.fn(),
}));

jest.mock("@/services/profile", () => ({
  getMyProfile: jest.fn(),
}));

jest.mock("@/services/auth", () => ({
  logout: jest.fn(),
  logoutAll: jest.fn(),
}));

describe("useAuthStore", () => {
  beforeEach(() => {
    // Reset Zustand store state between tests
    useAuthStore.setState({
      user: null,
      loading: true,
      isAuthenticated: false,
    });
  });

  it("starts with no user and loading=true", () => {
    const state = useAuthStore.getState();
    expect(state.user).toBeNull();
    expect(state.loading).toBe(true);
    expect(state.isAuthenticated).toBe(false);
  });

  it("setUser sets user and marks authenticated", () => {
    const mockUser = {
      id: 1,
      username: "testuser",
      email: "test@example.com",
      display_name: "Test User",
      bio: "",
      avatar_url: "",
      is_admin: false,
      profile_visibility: "public" as const,
      email_verified: true,
      created_at: "2024-01-01T00:00:00Z",
      updated_at: "2024-01-01T00:00:00Z",
    };

    useAuthStore.getState().setUser(mockUser);
    const state = useAuthStore.getState();
    expect(state.user).toEqual(mockUser);
    expect(state.isAuthenticated).toBe(true);
    expect(state.loading).toBe(false);
  });

  it("clearAuth resets state", () => {
    // First set a user
    useAuthStore.getState().setUser({
      id: 1,
      username: "test",
      email: "t@t.com",
      display_name: "T",
      bio: "",
      avatar_url: "",
      is_admin: false,
      profile_visibility: "public" as const,
      email_verified: true,
      created_at: "",
      updated_at: "",
    });

    useAuthStore.getState().clearAuth();
    const state = useAuthStore.getState();
    expect(state.user).toBeNull();
    expect(state.isAuthenticated).toBe(false);
    expect(state.loading).toBe(false);
  });

  it("initialize sets loading=false when no token", async () => {
    const { getAccessToken } = require("@/lib/api-client");
    (getAccessToken as jest.Mock).mockReturnValue(null);

    await useAuthStore.getState().initialize();
    const state = useAuthStore.getState();
    expect(state.user).toBeNull();
    expect(state.loading).toBe(false);
    expect(state.isAuthenticated).toBe(false);
  });

  it("initialize fetches profile when token exists", async () => {
    const { getAccessToken } = require("@/lib/api-client");
    const { getMyProfile } = require("@/services/profile");

    const mockProfile = {
      id: 1,
      username: "testuser",
      email: "test@example.com",
      display_name: "Test",
      bio: "",
      avatar_url: "",
      is_admin: false,
      profile_visibility: "public" as const,
      email_verified: true,
      created_at: "",
      updated_at: "",
    };

    (getAccessToken as jest.Mock).mockReturnValue("valid_token");
    (getMyProfile as jest.Mock).mockResolvedValue(mockProfile);

    await useAuthStore.getState().initialize();
    const state = useAuthStore.getState();
    expect(state.user).toEqual(mockProfile);
    expect(state.isAuthenticated).toBe(true);
    expect(state.loading).toBe(false);
  });
});
