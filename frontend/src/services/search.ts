// search.ts — Search API service.
import { apiGet } from "@/lib/api-client";
import type {
    CommentSearchResult,
    Note,
    PaginationParams,
    PublicProfile,
    SearchResponse,
    SearchSort,
    SearchType,
    Subnotery,
} from "@/types";

interface SearchParams extends PaginationParams {
    q: string;
    type: SearchType;
    sort?: SearchSort;
}

/** GET /search — Multi-type search across notes, subnoteries, users, comments. */
export function search<T = unknown>(params: SearchParams): Promise<SearchResponse<T>> {
    const query = new URLSearchParams();
    query.set("q", params.q);
    query.set("type", params.type);
    if (params.sort) query.set("sort", params.sort);
    if (params.page) query.set("page", String(params.page));
    if (params.limit) query.set("limit", String(params.limit));
    return apiGet(`/search?${query.toString()}`);
}

/** Typed search helpers for each search type. */
export const searchNotes = (q: string, params?: PaginationParams & { sort?: SearchSort }) =>
    search<Note>({ q, type: "notes", ...params });

export const searchSubnoteries = (q: string, params?: PaginationParams & { sort?: SearchSort }) =>
    search<Subnotery>({ q, type: "subnoteries", ...params });

export const searchUsers = (q: string, params?: PaginationParams & { sort?: SearchSort }) =>
    search<PublicProfile>({ q, type: "users", ...params });

export const searchComments = (q: string, params?: PaginationParams & { sort?: SearchSort }) =>
    search<CommentSearchResult>({ q, type: "comments", ...params });
