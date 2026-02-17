// profile.ts — Profile API service.
import { apiGet, apiPatch } from "@/lib/api-client";
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
