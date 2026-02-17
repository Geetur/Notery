// search.ts — Search API service.
import { apiGet } from "@/lib/api-client";
import type {
  SearchResponse,
  SearchType,
  Note,
  Subnotery,
  PublicProfile,
  CommentSearchResult,
  PaginationParams,
} from "@/types";

interface SearchParams extends PaginationParams {
  q: string;
  type: SearchType;
}

/** GET /search — Multi-type search across notes, subnoteries, users, comments. */
export function search<T = unknown>(params: SearchParams): Promise<SearchResponse<T>> {
  const query = new URLSearchParams();
  query.set("q", params.q);
  query.set("type", params.type);
  if (params.page) query.set("page", String(params.page));
  if (params.limit) query.set("limit", String(params.limit));
  return apiGet(`/search?${query.toString()}`);
}

/** Typed search helpers for each search type. */
export const searchNotes = (q: string, params?: PaginationParams) =>
  search<Note>({ q, type: "notes", ...params });

export const searchSubnoteries = (q: string, params?: PaginationParams) =>
  search<Subnotery>({ q, type: "subnoteries", ...params });

export const searchUsers = (q: string, params?: PaginationParams) =>
  search<PublicProfile>({ q, type: "users", ...params });

export const searchComments = (q: string, params?: PaginationParams) =>
  search<CommentSearchResult>({ q, type: "comments", ...params });
