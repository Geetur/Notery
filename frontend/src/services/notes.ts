// notes.ts — Notes API service. Wraps all note-related endpoints.
import {
    apiDelete,
    apiGet,
    apiPatch,
    apiPost,
    getAccessToken,
} from "@/lib/api-client";
import { API_V1 } from "@/lib/config";
import type {
    CreateNoteRequest,
    FeedResponse,
    Note,
    NotesListResponse,
    NoteStatus,
    PaginationParams,
    VoteResponse,
} from "@/types";

/** GET /feed/hot — Hot feed with optional pagination. */
export function getHotFeed(params?: PaginationParams): Promise<FeedResponse> {
    const query = new URLSearchParams();
    if (params?.page) query.set("page", String(params.page));
    if (params?.limit) query.set("limit", String(params.limit));
    const qs = query.toString();
    return apiGet(`/feed/hot${qs ? `?${qs}` : ""}`);
}

/** GET /notes/approved — Paginated list of approved notes. */
export function getApprovedNotes(
    params?: PaginationParams
): Promise<NotesListResponse> {
    const query = new URLSearchParams();
    if (params?.page) query.set("page", String(params.page));
    if (params?.limit) query.set("limit", String(params.limit));
    const qs = query.toString();
    return apiGet(`/notes/approved${qs ? `?${qs}` : ""}`);
}

/** GET /notes/:id — Get a single note by ID. */
export function getNoteById(id: number): Promise<Note> {
    return apiGet(`/notes/${id}`);
}

/** POST /notes — Create a new note (JSON body). */
export function createNote(data: CreateNoteRequest): Promise<Note> {
    return apiPost("/notes", data);
}

/** POST /notes/:id/content — Upload a PDF for a pending note. */
export async function uploadNotePDF(
    noteId: number,
    file: File
): Promise<{ message: string }> {
    const formData = new FormData();
    formData.append("pdf", file);
    const token = getAccessToken();
    const res = await fetch(`${API_V1}/notes/${noteId}/content`, {
        method: "POST",
        headers: token ? { Authorization: `Bearer ${token}` } : {},
        body: formData,
    });
    if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        throw new Error(body.error || "PDF upload failed");
    }
    return res.json();
}

/** POST /notes/:id/upvote — Upvote a note. */
export function upvoteNote(id: number): Promise<VoteResponse> {
    return apiPost(`/notes/${id}/upvote`);
}

/** POST /notes/:id/downvote — Downvote a note. */
export function downvoteNote(id: number): Promise<VoteResponse> {
    return apiPost(`/notes/${id}/downvote`);
}

/** DELETE /notes/:id — Delete a note (admin only). */
export function deleteNote(id: number): Promise<{ message: string }> {
    return apiDelete(`/notes/${id}`);
}

/** GET /notes/pending — List pending notes (admin only, paginated, optionally scoped by subnotery). */
export function getPendingNotes(
    params?: PaginationParams & { subnotery_id?: number }
): Promise<NotesListResponse> {
    const query = new URLSearchParams();
    if (params?.page) query.set("page", String(params.page));
    if (params?.limit) query.set("limit", String(params.limit));
    if (params?.subnotery_id) query.set("subnotery_id", String(params.subnotery_id));
    const qs = query.toString();
    return apiGet(`/notes/pending${qs ? `?${qs}` : ""}`);
}

/** GET /me/notes — List notes created by the authenticated user (with optional status filter). */
export function getMyNotes(
    params?: PaginationParams & { status?: NoteStatus }
): Promise<NotesListResponse> {
    const query = new URLSearchParams();
    if (params?.page) query.set("page", String(params.page));
    if (params?.limit) query.set("limit", String(params.limit));
    if (params?.status) query.set("status", params.status);
    const qs = query.toString();
    return apiGet(`/me/notes${qs ? `?${qs}` : ""}`);
}

/** PATCH /notes/:id/approve — Approve a pending note (admin only). */
export function approveNote(id: number): Promise<{ message: string }> {
    return apiPatch(`/notes/${id}/approve`);
}

/** PATCH /notes/:id/reject — Reject a note (admin only). */
export function rejectNote(id: number): Promise<{ message: string }> {
    return apiPatch(`/notes/${id}/reject`);
}

/** POST /notes/:id/thumbnail — Upload a thumbnail image for a note. */
export async function uploadNoteThumbnail(
    noteId: number,
    file: File
): Promise<{ message: string; thumbnail_url: string }> {
    const formData = new FormData();
    formData.append("thumbnail", file);
    const token = getAccessToken();
    const res = await fetch(`${API_V1}/notes/${noteId}/thumbnail`, {
        method: "POST",
        headers: token ? { Authorization: `Bearer ${token}` } : {},
        body: formData,
    });
    if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        throw new Error(body.error || "Thumbnail upload failed");
    }
    return res.json();
}

/** DELETE /notes/:id/thumbnail — Delete a note's thumbnail. */
export function deleteNoteThumbnail(id: number): Promise<{ message: string }> {
    return apiDelete(`/notes/${id}/thumbnail`);
}

/** GET /users/:id/notes — List a user's approved notes (public, paginated). */
export function getUserNotes(
    userId: number,
    params?: PaginationParams
): Promise<NotesListResponse> {
    const query = new URLSearchParams();
    if (params?.page) query.set("page", String(params.page));
    if (params?.limit) query.set("limit", String(params.limit));
    const qs = query.toString();
    return apiGet(`/users/${userId}/notes${qs ? `?${qs}` : ""}`);
}
