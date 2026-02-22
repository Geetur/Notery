// comment-section.tsx — Full comment section with sort tabs, composer, and threaded display.
"use client";

import { Button } from "@/components/ui/button";
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import { useToast } from "@/hooks/use-toast";
import { createComment, getNoteComments } from "@/services/comments";
import { useAuthStore } from "@/stores/auth-store";
import type { CommentSortOrder } from "@/types";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2, MessageSquare } from "lucide-react";
import { useState } from "react";
import { CommentThread } from "./comment-thread";

interface CommentSectionProps {
    noteId: number;
    isAdmin?: boolean;
}

const SORT_OPTIONS: { value: CommentSortOrder; label: string }[] = [
    { value: "hot", label: "Hot" },
    { value: "new", label: "New" },
    { value: "top", label: "Top" },
    { value: "controversial", label: "Controversial" },
];

export function CommentSection({ noteId, isAdmin }: CommentSectionProps) {
    const { isAuthenticated } = useAuthStore();
    const { toast } = useToast();
    const queryClient = useQueryClient();
    const [sort, setSort] = useState<CommentSortOrder>("hot");
    const [newComment, setNewComment] = useState("");
    const [submitting, setSubmitting] = useState(false);
    const [page, setPage] = useState(1);

    const { data, isLoading, isError } = useQuery({
        queryKey: ["comments", noteId, sort, page],
        queryFn: () => getNoteComments(noteId, { sort, page, limit: 25 }),
    });

    const handleSubmit = async () => {
        if (!newComment.trim()) return;
        setSubmitting(true);
        try {
            await createComment(noteId, newComment.trim());
            setNewComment("");
            queryClient.invalidateQueries({ queryKey: ["comments", noteId] });
            toast({ title: "Comment posted" });
        } catch {
            toast({ title: "Failed to post comment", variant: "destructive" });
        } finally {
            setSubmitting(false);
        }
    };

    const handleCommentUpdated = () => {
        queryClient.invalidateQueries({ queryKey: ["comments", noteId] });
    };

    const totalComments = data?.total ?? 0;

    return (
        <div className="mt-4">
            {/* Comment composer */}
            {isAuthenticated && (
                <div className="mb-4">
                    <p className="text-xs text-muted-foreground mb-1.5">
                        Comment as{" "}
                        <span className="text-foreground font-medium">
                            {useAuthStore.getState().user?.username}
                        </span>
                    </p>
                    <Textarea
                        placeholder="What are your thoughts?"
                        value={newComment}
                        onChange={(e) => setNewComment(e.target.value)}
                        className="min-h-[100px] text-sm resize-none"
                    />
                    <div className="flex justify-end mt-2">
                        <Button
                            size="sm"
                            onClick={handleSubmit}
                            disabled={submitting || !newComment.trim()}
                        >
                            {submitting ? (
                                <Loader2 className="h-4 w-4 animate-spin mr-1" />
                            ) : (
                                <MessageSquare className="h-4 w-4 mr-1" />
                            )}
                            Comment
                        </Button>
                    </div>
                </div>
            )}

            {/* Sort + count header */}
            <div className="flex items-center gap-3 border-b border-border pb-2 mb-3">
                <span className="text-sm font-medium text-foreground">
                    {totalComments} Comment{totalComments !== 1 ? "s" : ""}
                </span>
                <Select
                    value={sort}
                    onValueChange={(v) => {
                        setSort(v as CommentSortOrder);
                        setPage(1);
                    }}
                >
                    <SelectTrigger className="h-7 w-[130px] text-xs">
                        <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                        {SORT_OPTIONS.map((opt) => (
                            <SelectItem
                                key={opt.value}
                                value={opt.value}
                                className="text-xs"
                            >
                                {opt.label}
                            </SelectItem>
                        ))}
                    </SelectContent>
                </Select>
            </div>

            {/* Comments */}
            {isLoading ? (
                <div className="space-y-3">
                    {Array.from({ length: 3 }).map((_, i) => (
                        <div key={i} className="space-y-2">
                            <Skeleton className="h-4 w-32" />
                            <Skeleton className="h-12 w-full" />
                            <Skeleton className="h-4 w-48" />
                        </div>
                    ))}
                </div>
            ) : isError ? (
                <p className="text-sm text-destructive py-4">
                    Failed to load comments.
                </p>
            ) : (data?.comments?.length ?? 0) === 0 ? (
                <p className="text-sm text-muted-foreground py-6 text-center">
                    No comments yet. Be the first to share your thoughts!
                </p>
            ) : (
                <>
                    <div className="space-y-0">
                        {data!.comments.map((comment) => (
                            <CommentThread
                                key={comment.id}
                                comment={comment}
                                noteId={noteId}
                                isAdmin={isAdmin}
                                onCommentUpdated={handleCommentUpdated}
                            />
                        ))}
                    </div>

                    {/* Pagination */}
                    {data && data.total > data.limit && (
                        <div className="flex items-center justify-center gap-2 mt-4">
                            <Button
                                variant="outline"
                                size="sm"
                                disabled={page <= 1}
                                onClick={() => setPage((p) => p - 1)}
                            >
                                Previous
                            </Button>
                            <span className="text-xs text-muted-foreground">
                                Page {page} of {Math.ceil(data.total / data.limit)}
                            </span>
                            <Button
                                variant="outline"
                                size="sm"
                                disabled={page >= Math.ceil(data.total / data.limit)}
                                onClick={() => setPage((p) => p + 1)}
                            >
                                Next
                            </Button>
                        </div>
                    )}
                </>
            )}
        </div>
    );
}
