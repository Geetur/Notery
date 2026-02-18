// subnoteries.ts — Subnotery API service.
import { apiGet, apiPost } from "@/lib/api-client";
import type {
    NotesListResponse,
    PaginationParams,
    SubnoteryDetail,
    SubnoteryListResponse,
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

/** GET /subnoteries/:id/notes — List approved notes in a subnotery (paginated). */
export function getSubnoteryNotes(
    subnoteryId: number,
    params?: PaginationParams
): Promise<NotesListResponse> {
    const query = new URLSearchParams();
    if (params?.page) query.set("page", String(params.page));
    if (params?.limit) query.set("limit", String(params.limit));
    const qs = query.toString();
    return apiGet(`/subnoteries/${subnoteryId}/notes${qs ? `?${qs}` : ""}`);
}

/** POST /subnoteries/:id/join — Join a subnotery. */
export function joinSubnotery(
    subnoteryId: number
): Promise<{ message: string }> {
    return apiPost(`/subnoteries/${subnoteryId}/join`);
}

/** POST /subnoteries/:id/admins — Add admin to subnotery (admin only). */
export function addAdminToSubnotery(
    subnoteryId: number,
    userId: number
): Promise<{ message: string }> {
    return apiPost(`/subnoteries/${subnoteryId}/admins`, { user_id: userId });
}
