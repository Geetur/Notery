// profile.ts — Profile API service.
import { apiDelete, apiGet, apiPatch, getAccessToken } from "@/lib/api-client";
import { API_V1 } from "@/lib/config";
import type { PublicProfile, SelfProfile, UpdateProfileRequest } from "@/types";

/** GET /me/profile — Get own full profile (authenticated). */
export function getMyProfile(): Promise<SelfProfile> {
    return apiGet("/me/profile");
}

/** PATCH /me/profile — Partial update own profile. */
export function updateMyProfile(
    data: UpdateProfileRequest
): Promise<SelfProfile> {
    return apiPatch("/me/profile", data);
}

/** GET /users/:id/profile — Get public profile. */
export function getUserProfile(userId: number): Promise<PublicProfile> {
    return apiGet(`/users/${userId}/profile`);
}

/** POST /me/avatar — Upload avatar (multipart form data). */
export async function uploadAvatar(
    file: File
): Promise<{ message: string; avatar_url: string }> {
    const formData = new FormData();
    formData.append("avatar", file);
    const token = getAccessToken();
    const res = await fetch(`${API_V1}/me/avatar`, {
        method: "POST",
        headers: token ? { Authorization: `Bearer ${token}` } : {},
        body: formData,
    });
    if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        throw new Error(body.error || "Avatar upload failed");
    }
    return res.json();
}

/** DELETE /me/avatar — Delete current avatar. */
export function deleteAvatar(): Promise<{ message: string }> {
    return apiDelete("/me/avatar");
}

/** POST /me/banner — Upload profile banner (multipart form data). */
export async function uploadBanner(
    file: File
): Promise<{ message: string; banner_url: string }> {
    const formData = new FormData();
    formData.append("banner", file);
    const token = getAccessToken();
    const res = await fetch(`${API_V1}/me/banner`, {
        method: "POST",
        headers: token ? { Authorization: `Bearer ${token}` } : {},
        body: formData,
    });
    if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        throw new Error(body.error || "Banner upload failed");
    }
    return res.json();
}

/** DELETE /me/banner — Delete current profile banner. */
export function deleteBanner(): Promise<{ message: string }> {
    return apiDelete("/me/banner");
}
