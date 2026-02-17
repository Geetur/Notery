// comment-thread.tsx — Reddit-style threaded comment display with collapse/expand and indentation.
"use client";

import { Button } from "@/components/ui/button";
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Textarea } from "@/components/ui/textarea";
import { useToast } from "@/hooks/use-toast";
import { cn } from "@/lib/utils";
import {
    createComment,
    deleteComment,
    editComment,
    removeCommentVote,
    voteComment,
} from "@/services/comments";
import { useAuthStore } from "@/stores/auth-store";
import type { CommentResponse } from "@/types";
import { formatDistanceToNow } from "date-fns";
import {
    ArrowBigDown,
    ArrowBigUp,
    ChevronDown,
    ChevronUp,
    Edit2,
    MessageSquare,
    MoreHorizontal,
    Trash2,
} from "lucide-react";
import { useState } from "react";

interface CommentThreadProps {
    comment: CommentResponse;
    noteId: number;
    onCommentUpdated?: () => void;
}

export function CommentThread({
    comment,
    noteId,
    onCommentUpdated,
}: CommentThreadProps) {
    const { user, isAuthenticated } = useAuthStore();
    const { toast } = useToast();
    const [collapsed, setCollapsed] = useState(false);
    const [replying, setReplying] = useState(false);
    const [editing, setEditing] = useState(false);
    const [replyText, setReplyText] = useState("");
    const [editText, setEditText] = useState(comment.body);
    const [currentVote, setCurrentVote] = useState(comment.user_vote);
    const [upvotes, setUpvotes] = useState(comment.upvotes);
    const [downvotes, setDownvotes] = useState(comment.downvotes);
    const [submitting, setSubmitting] = useState(false);
    const [isDeleted, setIsDeleted] = useState(comment.is_deleted);
    const [body, setBody] = useState(comment.body);

    const isOwnComment = user?.id === comment.user_id;
    const netScore = upvotes - downvotes;

    const handleVote = async (value: 1 | -1) => {
        if (!isAuthenticated) {
            toast({ title: "Login required", variant: "destructive" });
            return;
        }
        try {
            if (currentVote === value) {
                await removeCommentVote(comment.id);
                setCurrentVote(0);
                setUpvotes(value === 1 ? upvotes - 1 : upvotes);
                setDownvotes(value === -1 ? downvotes - 1 : downvotes);
            } else {
                const res = await voteComment(comment.id, value);
                setCurrentVote(res.user_vote);
                setUpvotes(res.upvotes);
                setDownvotes(res.downvotes);
            }
        } catch {
            toast({ title: "Vote failed", variant: "destructive" });
        }
    };

    const handleReply = async () => {
        if (!replyText.trim()) return;
        setSubmitting(true);
        try {
            await createComment(noteId, replyText.trim(), comment.id);
            setReplyText("");
            setReplying(false);
            onCommentUpdated?.();
            toast({ title: "Reply posted" });
        } catch {
            toast({ title: "Failed to post reply", variant: "destructive" });
        } finally {
            setSubmitting(false);
        }
    };

    const handleEdit = async () => {
        if (!editText.trim()) return;
        setSubmitting(true);
        try {
            const res = await editComment(comment.id, editText.trim());
            setBody(res.body);
            setEditing(false);
            toast({ title: "Comment updated" });
        } catch {
            toast({ title: "Failed to edit comment", variant: "destructive" });
        } finally {
            setSubmitting(false);
        }
    };

    const handleDelete = async () => {
        setSubmitting(true);
        try {
            await deleteComment(comment.id);
            setIsDeleted(true);
            setBody("[deleted]");
            toast({ title: "Comment deleted" });
        } catch {
            toast({ title: "Failed to delete comment", variant: "destructive" });
        } finally {
            setSubmitting(false);
        }
    };

    return (
        <div
            className={cn(
                "relative",
                comment.depth > 0 && "ml-4 pl-3 border-l-2 border-border hover:border-primary/30"
            )}
        >
            {/* Comment header */}
            <div className="flex items-center gap-1.5 text-xs py-1">
                <button
                    onClick={() => setCollapsed(!collapsed)}
                    className="text-muted-foreground hover:text-foreground"
                >
                    {collapsed ? (
                        <ChevronDown className="h-3.5 w-3.5" />
                    ) : (
                        <ChevronUp className="h-3.5 w-3.5" />
                    )}
                </button>

                <span className="font-semibold text-foreground">
                    {isDeleted ? "[deleted]" : comment.username}
                </span>
                <span className="text-muted-foreground">•</span>
                <span className="text-muted-foreground">
                    {formatDistanceToNow(new Date(comment.created_at), {
                        addSuffix: true,
                    })}
                </span>
                {comment.is_edited && (
                    <span className="text-muted-foreground italic">(edited)</span>
                )}
                {collapsed && (
                    <span className="text-muted-foreground">
                        ({comment.children?.length || 0} children)
                    </span>
                )}
            </div>

            {!collapsed && (
                <>
                    {/* Comment body */}
                    {editing ? (
                        <div className="mt-1 mb-2">
                            <Textarea
                                value={editText}
                                onChange={(e) => setEditText(e.target.value)}
                                className="min-h-[80px] text-sm"
                            />
                            <div className="flex gap-2 mt-1.5">
                                <Button
                                    size="sm"
                                    className="h-7 text-xs"
                                    onClick={handleEdit}
                                    disabled={submitting}
                                >
                                    Save
                                </Button>
                                <Button
                                    variant="ghost"
                                    size="sm"
                                    className="h-7 text-xs"
                                    onClick={() => {
                                        setEditing(false);
                                        setEditText(body);
                                    }}
                                >
                                    Cancel
                                </Button>
                            </div>
                        </div>
                    ) : (
                        <div className="text-sm text-foreground leading-relaxed py-1 whitespace-pre-wrap">
                            {body}
                        </div>
                    )}

                    {/* Action bar */}
                    {!isDeleted && (
                        <div className="flex items-center gap-1 -ml-1 mb-1">
                            {/* Vote buttons */}
                            <button
                                onClick={() => handleVote(1)}
                                className={cn(
                                    "p-1 rounded hover:bg-accent",
                                    currentVote === 1
                                        ? "text-orange-500"
                                        : "text-muted-foreground hover:text-foreground"
                                )}
                            >
                                <ArrowBigUp
                                    className={cn(
                                        "h-4 w-4",
                                        currentVote === 1 && "fill-current"
                                    )}
                                />
                            </button>
                            <span
                                className={cn(
                                    "text-xs font-bold",
                                    currentVote === 1 && "text-orange-500",
                                    currentVote === -1 && "text-blue-500"
                                )}
                            >
                                {netScore}
                            </span>
                            <button
                                onClick={() => handleVote(-1)}
                                className={cn(
                                    "p-1 rounded hover:bg-accent",
                                    currentVote === -1
                                        ? "text-blue-500"
                                        : "text-muted-foreground hover:text-foreground"
                                )}
                            >
                                <ArrowBigDown
                                    className={cn(
                                        "h-4 w-4",
                                        currentVote === -1 && "fill-current"
                                    )}
                                />
                            </button>

                            {/* Reply button */}
                            {isAuthenticated && (
                                <Button
                                    variant="ghost"
                                    size="sm"
                                    className="h-7 text-xs text-muted-foreground hover:text-foreground gap-1"
                                    onClick={() => setReplying(!replying)}
                                >
                                    <MessageSquare className="h-3.5 w-3.5" />
                                    Reply
                                </Button>
                            )}

                            {/* More options */}
                            {isOwnComment && (
                                <DropdownMenu>
                                    <DropdownMenuTrigger asChild>
                                        <Button
                                            variant="ghost"
                                            size="sm"
                                            className="h-7 w-7 p-0 text-muted-foreground"
                                        >
                                            <MoreHorizontal className="h-3.5 w-3.5" />
                                        </Button>
                                    </DropdownMenuTrigger>
                                    <DropdownMenuContent align="start">
                                        <DropdownMenuItem
                                            onClick={() => {
                                                setEditing(true);
                                                setEditText(body);
                                            }}
                                        >
                                            <Edit2 className="h-3.5 w-3.5 mr-2" />
                                            Edit
                                        </DropdownMenuItem>
                                        <DropdownMenuItem
                                            onClick={handleDelete}
                                            className="text-destructive"
                                        >
                                            <Trash2 className="h-3.5 w-3.5 mr-2" />
                                            Delete
                                        </DropdownMenuItem>
                                    </DropdownMenuContent>
                                </DropdownMenu>
                            )}
                        </div>
                    )}

                    {/* Reply editor */}
                    {replying && (
                        <div className="ml-1 mb-2">
                            <Textarea
                                placeholder="What are your thoughts?"
                                value={replyText}
                                onChange={(e) => setReplyText(e.target.value)}
                                className="min-h-[80px] text-sm"
                            />
                            <div className="flex gap-2 mt-1.5">
                                <Button
                                    size="sm"
                                    className="h-7 text-xs"
                                    onClick={handleReply}
                                    disabled={submitting || !replyText.trim()}
                                >
                                    Reply
                                </Button>
                                <Button
                                    variant="ghost"
                                    size="sm"
                                    className="h-7 text-xs"
                                    onClick={() => {
                                        setReplying(false);
                                        setReplyText("");
                                    }}
                                >
                                    Cancel
                                </Button>
                            </div>
                        </div>
                    )}

                    {/* Children */}
                    {comment.children?.length > 0 && (
                        <div>
                            {comment.children.map((child) => (
                                <CommentThread
                                    key={child.id}
                                    comment={child}
                                    noteId={noteId}
                                    onCommentUpdated={onCommentUpdated}
                                />
                            ))}
                        </div>
                    )}

                    {/* More replies indicator */}
                    {comment.has_more_replies && (
                        <div className="ml-4 py-1">
                            <Button variant="link" size="sm" className="h-6 text-xs p-0 text-primary">
                                Continue this thread →
                            </Button>
                        </div>
                    )}
                </>
            )}
        </div>
    );
}
