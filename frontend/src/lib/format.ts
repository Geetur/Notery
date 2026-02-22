// format.ts — Formatting utilities for the Notery frontend.
// Prices, dates, vote counts, file sizes, etc.

import { format, formatDistanceToNow } from "date-fns";
import { API_V1 } from "./config";

/**
 * Format cents to a display price string.
 * 499 → "$4.99", 0 → "Free"
 */
export function formatPrice(cents: number): string {
    if (cents === 0) return "Free";
    return `$${(cents / 100).toFixed(2)}`;
}

/**
 * Format a date string to a relative time (e.g., "3 hours ago").
 */
export function timeAgo(dateStr: string): string {
    try {
        return formatDistanceToNow(new Date(dateStr), { addSuffix: true });
    } catch {
        return dateStr;
    }
}

/**
 * Format a date string to a full readable date.
 */
export function formatDate(dateStr: string): string {
    try {
        return format(new Date(dateStr), "MMM d, yyyy");
    } catch {
        return dateStr;
    }
}

/**
 * Format a date string to date + time.
 */
export function formatDateTime(dateStr: string): string {
    try {
        return format(new Date(dateStr), "MMM d, yyyy 'at' h:mm a");
    } catch {
        return dateStr;
    }
}

/**
 * Format vote count for display. Compact format for large numbers.
 * 1234 → "1.2k", 999 → "999"
 */
export function formatVotes(count: number): string {
    if (count >= 100000) return `${(count / 1000).toFixed(0)}k`;
    if (count >= 10000) return `${(count / 1000).toFixed(1)}k`;
    if (count >= 1000) return `${(count / 1000).toFixed(1)}k`;
    return String(count);
}

/**
 * Format file size in bytes to human-readable.
 * 1048576 → "1.0 MB"
 */
export function formatFileSize(bytes: number): string {
    if (bytes === 0) return "0 B";
    const units = ["B", "KB", "MB", "GB"];
    const i = Math.floor(Math.log(bytes) / Math.log(1024));
    const size = bytes / Math.pow(1024, i);
    return `${size.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

/**
 * Net vote score (upvotes - downvotes).
 */
export function netScore(upvotes: number, downvotes: number): number {
    return upvotes - downvotes;
}

/**
 * Build the avatar proxy URL for a user.
 * Returns undefined if the user has no avatar, so <AvatarImage> shows fallback.
 * Appends a cache-bust `v` param to force reload after upload.
 */
export function avatarUrl(
    userId: number,
    avatarKey: string | undefined | null
): string | undefined {
    if (!avatarKey) return undefined;
    return `${API_V1}/users/${userId}/avatar?v=${encodeURIComponent(avatarKey)}`;
}

/**
 * Build the thumbnail proxy URL for a note.
 * Returns undefined if the note has no thumbnail.
 * Appends a cache-bust `v` param to force reload after upload.
 */
export function thumbnailUrl(
    noteId: number,
    thumbnailKey: string | undefined | null
): string | undefined {
    if (!thumbnailKey) return undefined;
    return `${API_V1}/notes/${noteId}/thumbnail?v=${encodeURIComponent(thumbnailKey)}`;
}

/**
 * Build the banner proxy URL for a user.
 * Returns undefined if the user has no banner.
 * Appends a cache-bust `v` param to force reload after upload.
 */
export function userBannerUrl(
    userId: number,
    bannerKey: string | undefined | null
): string | undefined {
    if (!bannerKey) return undefined;
    return `${API_V1}/users/${userId}/banner?v=${encodeURIComponent(bannerKey)}`;
}
