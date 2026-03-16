// subnoteries.ts — Subnotery API service.
import { apiDelete, apiGet, apiPatch, apiPost, getAccessToken } from "@/lib/api-client";
import { API_V1 } from "@/lib/config";
import type {
    NotesListResponse,
    PaginationParams,
    SubnoteryDetail,
    SubnoteryListResponse,
    SubnoteryMembersResponse,
} from "@/types";

/** GET /subnoteries — List all subnoteries (paginated). */
export function listSubnoteries(
    params?: PaginationParams
): Promise<SubnoteryListResponse> {
    const query = new URLSearchParams();
    if (params?.page) query.set("page", String(params.page));
    if (params?.limit) query.set("limit", String(params.limit));
    const qs = query.toString();
    return apiGet(`/subnoteries${qs ? `?${qs}` : ""}`);
}

/** GET /subnoteries/:id — Get subnotery details (admins, member count). */
export function getSubnotery(subnoteryId: number): Promise<SubnoteryDetail> {
    return apiGet(`/subnoteries/${subnoteryId}`);
}

/** GET /subnoteries/:id/notes — List approved notes in a subnotery (paginated, sortable). */
export function getSubnoteryNotes(
    subnoteryId: number,
    params?: PaginationParams & { sort?: string; time?: string }
): Promise<NotesListResponse> {
    const query = new URLSearchParams();
    if (params?.page) query.set("page", String(params.page));
    if (params?.limit) query.set("limit", String(params.limit));
    if (params?.sort) query.set("sort", params.sort);
    if (params?.time) query.set("time", params.time);
    const qs = query.toString();
    return apiGet(`/subnoteries/${subnoteryId}/notes${qs ? `?${qs}` : ""}`);
}

/** POST /subnoteries/:id/join — Join a subnotery. */
export function joinSubnotery(
    subnoteryId: number
): Promise<{ message: string }> {
    return apiPost(`/subnoteries/${subnoteryId}/join`);
}

/** POST /subnoteries/:id/leave — Leave a subnotery. */
export function leaveSubnotery(
    subnoteryId: number
): Promise<{ message: string }> {
    return apiPost(`/subnoteries/${subnoteryId}/leave`);
}

/** POST /subnoteries/:id/admins — Add admin to subnotery (admin only). */
export function addAdminToSubnotery(
    subnoteryId: number,
    userId: number
): Promise<{ message: string }> {
    return apiPost(`/subnoteries/${subnoteryId}/admins`, { user_id: userId });
}

/** PATCH /subnoteries/:id/settings — Update subnotery settings (admin only). */
export function updateSubnoterySettings(
    subnoteryId: number,
    settings: { description?: string; content_type?: string; rules?: string; background_color?: string; min_post_notoriety?: number; min_comment_notoriety?: number }
): Promise<{ message: string }> {
    return apiPatch(`/subnoteries/${subnoteryId}/settings`, settings);
}

/** DELETE /subnoteries/:id/admins/:uid — Remove admin from subnotery. */
export function removeAdminFromSubnotery(
    subnoteryId: number,
    userId: number
): Promise<{ message: string }> {
    return apiDelete(`/subnoteries/${subnoteryId}/admins/${userId}`);
}

/** POST /subnoteries/:id/banner — Upload banner image (multipart). */
export async function uploadSubnoteryBanner(
    subnoteryId: number,
    file: File
): Promise<{ message: string; banner_url: string }> {
    const formData = new FormData();
    formData.append("banner", file);
    const token = getAccessToken();
    const res = await fetch(`${API_V1}/subnoteries/${subnoteryId}/banner`, {
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

/** DELETE /subnoteries/:id/banner — Delete banner image. */
export function deleteSubnoteryBanner(
    subnoteryId: number
): Promise<{ message: string }> {
    return apiDelete(`/subnoteries/${subnoteryId}/banner`);
}

/** GET /subnoteries/:id/members — List members (paginated, includes admin flag). */
export function getSubnoteryMembers(
    subnoteryId: number,
    params?: PaginationParams
): Promise<SubnoteryMembersResponse> {
    const query = new URLSearchParams();
    if (params?.page) query.set("page", String(params.page));
    if (params?.limit) query.set("limit", String(params.limit));
    const qs = query.toString();
    return apiGet(`/subnoteries/${subnoteryId}/members${qs ? `?${qs}` : ""}`);
}

/** DELETE /subnoteries/:id/members/:uid — Remove a member from subnotery (admin action). */
export function removeMemberFromSubnotery(
    subnoteryId: number,
    userId: number
): Promise<{ message: string }> {
    return apiDelete(`/subnoteries/${subnoteryId}/members/${userId}`);
}
