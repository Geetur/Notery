// comments.ts — Comments API service. Wraps all comment-related endpoints.
import { apiGet, apiPost, apiPut, apiDelete } from "@/lib/api-client";
import type {
  CommentsListResponse,
  CommentResponse,
  CommentVoteResponse,
  CommentSortOrder,
  PaginationParams,
} from "@/types";

interface GetCommentsParams extends PaginationParams {
  sort?: CommentSortOrder;
  max_depth?: number;
}

/** GET /notes/:id/comments — Threaded comments for a note. */
export function getNoteComments(
  noteId: number,
  params?: GetCommentsParams
): Promise<CommentsListResponse> {
  const query = new URLSearchParams();
  if (params?.page) query.set("page", String(params.page));
  if (params?.limit) query.set("limit", String(params.limit));
  if (params?.sort) query.set("sort", params.sort);
  if (params?.max_depth) query.set("max_depth", String(params.max_depth));
  const qs = query.toString();
  return apiGet(`/notes/${noteId}/comments${qs ? `?${qs}` : ""}`);
}

/** GET /comments/:id — Single comment with subtree. */
export function getComment(commentId: number): Promise<CommentResponse> {
  return apiGet(`/comments/${commentId}`);
}

/** POST /notes/:id/comments — Create a comment or reply. */
export function createComment(
  noteId: number,
  body: string,
  parentId?: number
): Promise<CommentResponse> {
  const payload: { body: string; parent_id?: number } = { body };
  if (parentId) payload.parent_id = parentId;
  return apiPost(`/notes/${noteId}/comments`, payload);
}

/** PUT /comments/:id — Edit own comment. */
export function editComment(
  commentId: number,
  body: string
): Promise<{ comment_id: number; body: string; is_edited: boolean }> {
  return apiPut(`/comments/${commentId}`, { body });
}

/** DELETE /comments/:id — Soft-delete own comment. */
export function deleteComment(
  commentId: number
): Promise<{ message: string }> {
  return apiDelete(`/comments/${commentId}`);
}

/** POST /comments/:id/vote — Vote on a comment. */
export function voteComment(
  commentId: number,
  value: 1 | -1
): Promise<CommentVoteResponse> {
  return apiPost(`/comments/${commentId}/vote`, { value });
}

/** DELETE /comments/:id/vote — Remove vote from a comment. */
export function removeCommentVote(
  commentId: number
): Promise<{ message: string }> {
  return apiDelete(`/comments/${commentId}/vote`);
}
