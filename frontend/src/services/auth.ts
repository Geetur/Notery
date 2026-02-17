// auth.ts — Auth API service. Wraps all /auth/* endpoints.
import { apiPost, clearTokens, getRefreshToken, setTokens } from "@/lib/api-client";
import type {
    AuthRequest,
    AuthResponse,
    ChangePasswordRequest,
    ForgotPasswordRequest,
    ResetPasswordRequest,
} from "@/types";

/** POST /auth/signup — Register a new account. */
export async function signup(
    email: string,
    password: string,
    username: string
): Promise<AuthResponse> {
    const data = await apiPost<AuthResponse>("/auth/signup", {
        email,
        password,
        username,
    } satisfies AuthRequest);
    setTokens(data.access_token, data.refresh_token);
    return data;
}

/** POST /auth/login — Authenticate and receive tokens. */
export async function login(
    email: string,
    password: string
): Promise<AuthResponse> {
    const data = await apiPost<AuthResponse>("/auth/login", {
        email,
        password,
    } satisfies AuthRequest);
    setTokens(data.access_token, data.refresh_token);
    return data;
}

/** POST /auth/refresh — Rotate refresh token. Called automatically by api-client. */
export async function refreshTokens(): Promise<AuthResponse> {
    const refreshToken = getRefreshToken();
    if (!refreshToken) throw new Error("No refresh token available");
    const data = await apiPost<AuthResponse>("/auth/refresh", {
        refresh_token: refreshToken,
    });
    setTokens(data.access_token, data.refresh_token);
    return data;
}

/** POST /auth/logout — Revoke current refresh token. */
export async function logout(): Promise<void> {
    const refreshToken = getRefreshToken();
    try {
        if (refreshToken) {
            await apiPost("/auth/logout", { refresh_token: refreshToken });
        }
    } finally {
        clearTokens();
    }
}

/** POST /auth/logout-all — Revoke all refresh tokens for this user. */
export async function logoutAll(): Promise<void> {
    try {
        await apiPost("/auth/logout-all");
    } finally {
        clearTokens();
    }
}

/** POST /auth/forgot-password — Request a password reset email. */
export function forgotPassword(email: string): Promise<{ message: string }> {
    return apiPost("/auth/forgot-password", {
        email,
    } satisfies ForgotPasswordRequest);
}

/** POST /auth/reset-password — Reset password with token. */
export function resetPassword(
    token: string,
    newPassword: string
): Promise<{ message: string }> {
    return apiPost("/auth/reset-password", {
        token,
        new_password: newPassword,
    } satisfies ResetPasswordRequest);
}

/** POST /auth/change-password — Change password (requires auth). */
export function changePassword(
    currentPassword: string,
    newPassword: string
): Promise<{ message: string }> {
    return apiPost("/auth/change-password", {
        current_password: currentPassword,
        new_password: newPassword,
    } satisfies ChangePasswordRequest);
}

/** POST /auth/resend-verification — Resend email verification link. */
export function resendVerification(): Promise<{ message: string }> {
    return apiPost("/auth/resend-verification");
}
