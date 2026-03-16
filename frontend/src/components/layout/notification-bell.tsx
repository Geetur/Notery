// notification-bell.tsx — Reddit-style notification bell with dropdown.
// Shows unread count badge and a popover with recent notifications.
// Actionable notifications (admin invites) have accept/deny buttons.
"use client";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
    Popover,
    PopoverContent,
    PopoverTrigger,
} from "@/components/ui/popover";
import { useToast } from "@/hooks/use-toast";
import { timeAgo } from "@/lib/format";
import {
    acceptNotification,
    denyNotification,
    getNotifications,
    getUnreadCount,
    markAllNotificationsRead,
    markNotificationRead,
} from "@/services/notifications";
import type { NotificationItem } from "@/types";
import {
    Bell,
    Check,
    CheckCircle,
    DollarSign,
    MessageSquare,
    Reply,
    Shield,
    TrendingUp,
    X,
} from "lucide-react";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useRef, useState } from "react";

export function NotificationBell() {
    const router = useRouter();
    const { toast } = useToast();
    const [open, setOpen] = useState(false);
    const [unreadCount, setUnreadCount] = useState(0);
    const [notifications, setNotifications] = useState<NotificationItem[]>([]);
    const [loading, setLoading] = useState(false);
    const [actioning, setActioning] = useState<number | null>(null);
    const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

    // Poll unread count every 30 seconds
    const fetchUnreadCount = useCallback(async () => {
        try {
            const res = await getUnreadCount();
            setUnreadCount(res.unread_count);
        } catch {
            // Silently fail — user may not be authenticated
        }
    }, []);

    useEffect(() => {
        fetchUnreadCount();
        pollRef.current = setInterval(fetchUnreadCount, 30000);
        return () => {
            if (pollRef.current) clearInterval(pollRef.current);
        };
    }, [fetchUnreadCount]);

    // Load notifications when popover opens
    const handleOpenChange = async (isOpen: boolean) => {
        setOpen(isOpen);
        if (isOpen) {
            setLoading(true);
            try {
                const res = await getNotifications({ limit: 20 });
                setNotifications(res.notifications ?? []);
            } catch {
                // ignore
            } finally {
                setLoading(false);
            }
        }
    };

    const handleMarkAllRead = async () => {
        try {
            await markAllNotificationsRead();
            setNotifications((prev) =>
                prev.map((n) => ({ ...n, is_read: true }))
            );
            setUnreadCount(0);
        } catch {
            toast({
                title: "Error",
                description: "Failed to mark all as read",
                variant: "destructive",
            });
        }
    };

    const handleNotificationClick = async (notif: NotificationItem) => {
        // Mark as read
        if (!notif.is_read) {
            try {
                await markNotificationRead(notif.id);
                setNotifications((prev) =>
                    prev.map((n) =>
                        n.id === notif.id ? { ...n, is_read: true } : n
                    )
                );
                setUnreadCount((c) => Math.max(0, c - 1));
            } catch {
                // ignore
            }
        }

        // Navigate based on notification type
        if (notif.type === "upvote_milestone" || notif.type === "purchase" || notif.type === "comment") {
            if (notif.reference_type === "note") {
                setOpen(false);
                router.push(`/notes/${notif.reference_id}`);
            } else if (notif.reference_type === "comment") {
                setOpen(false);
                router.push(`/notes/${notif.reference_id}`);
            }
        } else if (notif.type === "reply") {
            // Reply notifications: metadata contains note_id for navigation
            setOpen(false);
            try {
                const meta = JSON.parse(notif.metadata || "{}");
                if (meta.note_id) {
                    router.push(`/notes/${meta.note_id}`);
                }
            } catch {
                // fallback — no navigation
            }
        } else if (notif.type === "admin_invite") {
            if (notif.action_status === "pending") {
                // Don't navigate — show accept/deny inline
                return;
            }
            setOpen(false);
            router.push(`/communities/${notif.reference_id}`);
        }
    };

    const handleAccept = async (notif: NotificationItem) => {
        setActioning(notif.id);
        try {
            await acceptNotification(notif.id);
            setNotifications((prev) =>
                prev.map((n) =>
                    n.id === notif.id
                        ? { ...n, action_status: "accepted", is_read: true }
                        : n
                )
            );
            setUnreadCount((c) => Math.max(0, c - 1));
            toast({
                title: "Accepted!",
                description: "You are now an admin of this community.",
            });
        } catch (err: unknown) {
            const msg =
                err instanceof Error ? err.message : "Failed to accept";
            toast({
                title: "Error",
                description: msg,
                variant: "destructive",
            });
        } finally {
            setActioning(null);
        }
    };

    const handleDeny = async (notif: NotificationItem) => {
        setActioning(notif.id);
        try {
            await denyNotification(notif.id);
            setNotifications((prev) =>
                prev.map((n) =>
                    n.id === notif.id
                        ? { ...n, action_status: "denied", is_read: true }
                        : n
                )
            );
            setUnreadCount((c) => Math.max(0, c - 1));
            toast({ title: "Declined", description: "Invite declined." });
        } catch (err: unknown) {
            const msg = err instanceof Error ? err.message : "Failed to deny";
            toast({
                title: "Error",
                description: msg,
                variant: "destructive",
            });
        } finally {
            setActioning(null);
        }
    };

    const getNotificationIcon = (notif: NotificationItem) => {
        switch (notif.type) {
            case "admin_invite":
                return <Shield className="h-4 w-4 text-blue-500" />;
            case "upvote_milestone":
                return <TrendingUp className="h-4 w-4 text-orange-500" />;
            case "purchase":
                return <DollarSign className="h-4 w-4 text-green-500" />;
            case "comment":
                return <MessageSquare className="h-4 w-4 text-purple-500" />;
            case "reply":
                return <Reply className="h-4 w-4 text-cyan-500" />;
            default:
                return <Bell className="h-4 w-4 text-muted-foreground" />;
        }
    };

    return (
        <Popover open={open} onOpenChange={handleOpenChange}>
            <PopoverTrigger asChild>
                <Button variant="ghost" size="icon" className="h-9 w-9 relative">
                    <Bell className="h-4 w-4" />
                    {unreadCount > 0 && (
                        <span className="absolute -top-0.5 -right-0.5 flex h-4 min-w-[16px] items-center justify-center rounded-full bg-red-500 px-1 text-[10px] font-bold text-white">
                            {unreadCount > 99 ? "99+" : unreadCount}
                        </span>
                    )}
                </Button>
            </PopoverTrigger>
            <PopoverContent
                className="w-96 p-0"
                align="end"
                sideOffset={8}
            >
                {/* Header */}
                <div className="flex items-center justify-between px-4 py-3 border-b">
                    <h3 className="font-semibold text-sm">Notifications</h3>
                    {unreadCount > 0 && (
                        <Button
                            variant="ghost"
                            size="sm"
                            className="h-7 text-xs text-muted-foreground"
                            onClick={handleMarkAllRead}
                        >
                            <Check className="h-3 w-3 mr-1" />
                            Mark all read
                        </Button>
                    )}
                </div>

                {/* Notification list */}
                <div className="max-h-96 overflow-y-auto">
                    {loading ? (
                        <div className="p-4 text-center text-sm text-muted-foreground">
                            Loading...
                        </div>
                    ) : notifications.length === 0 ? (
                        <div className="p-8 text-center text-sm text-muted-foreground">
                            <Bell className="h-8 w-8 mx-auto mb-2 opacity-30" />
                            No notifications yet
                        </div>
                    ) : (
                        notifications.map((notif) => (
                            <div
                                key={notif.id}
                                className={`px-4 py-3 border-b last:border-b-0 hover:bg-muted/50 transition-colors cursor-pointer ${!notif.is_read
                                    ? "bg-blue-50/50 dark:bg-blue-950/20"
                                    : ""
                                    }`}
                                onClick={() => handleNotificationClick(notif)}
                            >
                                <div className="flex gap-3">
                                    {/* Icon */}
                                    <div className="mt-0.5 shrink-0">
                                        {getNotificationIcon(notif)}
                                    </div>

                                    {/* Content */}
                                    <div className="flex-1 min-w-0">
                                        <p className="text-sm font-medium leading-tight">
                                            {notif.title}
                                        </p>
                                        <p className="text-xs text-muted-foreground mt-0.5 line-clamp-2">
                                            {notif.message}
                                        </p>
                                        <p className="text-xs text-muted-foreground mt-1">
                                            {timeAgo(notif.created_at)}
                                        </p>

                                        {/* Actionable: admin invite with accept/deny */}
                                        {notif.type === "admin_invite" &&
                                            notif.action_status ===
                                            "pending" && (
                                                <div className="flex gap-2 mt-2">
                                                    <Button
                                                        size="sm"
                                                        className="h-7 text-xs"
                                                        disabled={
                                                            actioning ===
                                                            notif.id
                                                        }
                                                        onClick={(e) => {
                                                            e.stopPropagation();
                                                            handleAccept(
                                                                notif
                                                            );
                                                        }}
                                                    >
                                                        <CheckCircle className="h-3 w-3 mr-1" />
                                                        Accept
                                                    </Button>
                                                    <Button
                                                        size="sm"
                                                        variant="outline"
                                                        className="h-7 text-xs"
                                                        disabled={
                                                            actioning ===
                                                            notif.id
                                                        }
                                                        onClick={(e) => {
                                                            e.stopPropagation();
                                                            handleDeny(notif);
                                                        }}
                                                    >
                                                        <X className="h-3 w-3 mr-1" />
                                                        Decline
                                                    </Button>
                                                </div>
                                            )}

                                        {/* Resolved status badge */}
                                        {notif.type === "admin_invite" &&
                                            notif.action_status ===
                                            "accepted" && (
                                                <Badge
                                                    variant="secondary"
                                                    className="mt-1 text-xs text-green-600"
                                                >
                                                    Accepted
                                                </Badge>
                                            )}
                                        {notif.type === "admin_invite" &&
                                            notif.action_status ===
                                            "denied" && (
                                                <Badge
                                                    variant="secondary"
                                                    className="mt-1 text-xs text-red-600"
                                                >
                                                    Declined
                                                </Badge>
                                            )}
                                    </div>

                                    {/* Unread indicator */}
                                    {!notif.is_read && (
                                        <div className="mt-1.5 shrink-0">
                                            <div className="h-2 w-2 rounded-full bg-blue-500" />
                                        </div>
                                    )}
                                </div>
                            </div>
                        ))
                    )}
                </div>
            </PopoverContent>
        </Popover>
    );
}
