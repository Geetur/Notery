// auth-store.ts — Zustand store for authentication state.
// Manages user session, tokens, and auth UI state.
import { create } from "zustand";
import type { SelfProfile } from "@/types";
import { getAccessToken, clearTokens } from "@/lib/api-client";
import { getMyProfile } from "@/services/profile";
import { logout as apiLogout, logoutAll as apiLogoutAll } from "@/services/auth";

interface AuthState {
  /** Current authenticated user profile, null if not logged in. */
  user: SelfProfile | null;
  /** Whether auth state is being loaded (initial check / refresh). */
  loading: boolean;
  /** Whether user is authenticated. Derived from user != null. */
  isAuthenticated: boolean;

  // Actions
  /** Load user profile from API using stored token. Called on app mount. */
  initialize: () => Promise<void>;
  /** Set user after login/signup (called from auth pages). */
  setUser: (user: SelfProfile) => void;
  /** Logout: revoke token, clear storage, reset state. */
  logout: () => Promise<void>;
  /** Logout all sessions. */
  logoutAll: () => Promise<void>;
  /** Clear auth state without API call (e.g., on 401). */
  clearAuth: () => void;
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  loading: true,
  isAuthenticated: false,

  initialize: async () => {
    const token = getAccessToken();
    if (!token) {
      set({ user: null, loading: false, isAuthenticated: false });
      return;
    }

    try {
      const profile = await getMyProfile();
      set({ user: profile, loading: false, isAuthenticated: true });
    } catch {
      // Token expired or invalid — clear and continue as guest
      clearTokens();
      set({ user: null, loading: false, isAuthenticated: false });
    }
  },

  setUser: (user) => {
    set({ user, isAuthenticated: true, loading: false });
  },

  logout: async () => {
    try {
      await apiLogout();
    } catch {
      // Best-effort — clear local state regardless
    }
    set({ user: null, isAuthenticated: false, loading: false });
  },

  logoutAll: async () => {
    try {
      await apiLogoutAll();
    } catch {
      // Best-effort
    }
    set({ user: null, isAuthenticated: false, loading: false });
  },

  clearAuth: () => {
    clearTokens();
    set({ user: null, isAuthenticated: false, loading: false });
  },
}));
