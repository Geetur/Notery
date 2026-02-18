// bookmarks.ts — Bookmark API service. Add, remove, list, and check bookmarks.
import { apiDelete, apiGet, apiPost } from "@/lib/api-client";
import type { NotesListResponse, PaginationParams } from "@/types";

/** POST /bookmarks/:noteId — Add a note to bookmarks. */
export function addBookmark(
    noteId: number
): Promise<{ message: string; bookmarked: boolean }> {
    return apiPost(`/bookmarks/${noteId}`);
}

/** DELETE /bookmarks/:noteId — Remove a note from bookmarks. */
export function removeBookmark(
    noteId: number
): Promise<{ message: string; bookmarked: boolean }> {
    return apiDelete(`/bookmarks/${noteId}`);
}

/** GET /bookmarks — List bookmarked notes (paginated). */
export function getBookmarks(
    params?: PaginationParams
): Promise<NotesListResponse> {
    const query = new URLSearchParams();
    if (params?.page) query.set("page", String(params.page));
    if (params?.limit) query.set("limit", String(params.limit));
    const qs = query.toString();
    return apiGet(`/bookmarks${qs ? `?${qs}` : ""}`);
}

/** GET /bookmarks/:noteId — Check if a note is bookmarked. */
export function checkBookmark(
    noteId: number
): Promise<{ bookmarked: boolean }> {
    return apiGet(`/bookmarks/${noteId}`);
}
