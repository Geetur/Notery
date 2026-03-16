// notifications.ts — Notification API service.
import { apiGet, apiPatch, apiPost } from "@/lib/api-client";
import type {
    NotificationsResponse,
    PaginationParams,
    UnreadCountResponse,
} from "@/types";

/** GET /notifications — List notifications (paginated, optional unread_only filter). */
export function getNotifications(
    params?: PaginationParams & { unread_only?: boolean }
): Promise<NotificationsResponse> {
    const query = new URLSearchParams();
    if (params?.page) query.set("page", String(params.page));
    if (params?.limit) query.set("limit", String(params.limit));
    if (params?.unread_only) query.set("unread_only", "true");
    const qs = query.toString();
    return apiGet(`/notifications${qs ? `?${qs}` : ""}`);
}

/** GET /notifications/unread-count — Get unread notification count. */
export function getUnreadCount(): Promise<UnreadCountResponse> {
    return apiGet("/notifications/unread-count");
}

/** PATCH /notifications/:id/read — Mark a single notification as read. */
export function markNotificationRead(
    notifId: number
): Promise<{ message: string }> {
    return apiPatch(`/notifications/${notifId}/read`);
}

/** POST /notifications/read-all — Mark all notifications as read. */
export function markAllNotificationsRead(): Promise<{ message: string }> {
    return apiPost("/notifications/read-all");
}

/** POST /notifications/:id/accept — Accept an actionable notification. */
export function acceptNotification(
    notifId: number
): Promise<{ message: string }> {
    return apiPost(`/notifications/${notifId}/accept`);
}

/** POST /notifications/:id/deny — Deny an actionable notification. */
export function denyNotification(
    notifId: number
): Promise<{ message: string }> {
    return apiPost(`/notifications/${notifId}/deny`);
}

/** POST /subnoteries/:id/invite-admin — Send admin invite to a user by username. */
export function inviteAdmin(
    subnoteryId: number,
    username: string
): Promise<{ message: string }> {
    return apiPost(`/subnoteries/${subnoteryId}/invite-admin`, { username });
}
