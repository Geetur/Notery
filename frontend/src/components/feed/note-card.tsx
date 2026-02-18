// note-card.tsx — Reddit-style post card for the feed.
// Supports both "card" (expanded) and "compact" (dense list) view modes.
// Shows optional thumbnail when available.
"use client";

import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { useToast } from "@/hooks/use-toast";
import { formatPrice, thumbnailUrl, timeAgo } from "@/lib/format";
import { cn } from "@/lib/utils";
import { addBookmark, removeBookmark } from "@/services/bookmarks";
import { useAuthStore } from "@/stores/auth-store";
import type { Note, ViewMode } from "@/types";
import { Bookmark, BookmarkCheck, CheckCircle, FileText, Lock, MessageSquare } from "lucide-react";
import Link from "next/link";
import { useState } from "react";
import { VoteButtons } from "./vote-buttons";

interface NoteCardProps {
    note: Note;
    viewMode: ViewMode;
    purchased?: boolean;
    bookmarked?: boolean;
}

export function NoteCard({ note, viewMode, purchased, bookmarked: initialBookmarked }: NoteCardProps) {
    const isCompact = viewMode === "compact";
    const { isAuthenticated } = useAuthStore();
    const { toast } = useToast();
    const [bookmarked, setBookmarked] = useState(initialBookmarked ?? false);

    const toggleBookmark = async (e: React.MouseEvent) => {
        e.preventDefault();
        e.stopPropagation();
        if (!isAuthenticated) {
            toast({ title: "Sign in", description: "Log in to bookmark notes.", variant: "destructive" });
            return;
        }
        try {
            if (bookmarked) {
                await removeBookmark(note.id);
                setBookmarked(false);
                toast({ title: "Removed", description: "Bookmark removed." });
            } else {
                await addBookmark(note.id);
                setBookmarked(true);
                toast({ title: "Saved", description: "Note bookmarked." });
            }
        } catch {
            toast({ title: "Error", description: "Failed to update bookmark.", variant: "destructive" });
        }
    };

    return (
        <Card
            className={cn(
                "group border-border hover:border-primary/30 transition-colors",
                isCompact ? "rounded-none border-x-0 first:rounded-t last:rounded-b" : ""
            )}
        >
            <div className={cn("flex", isCompact ? "items-center gap-2 px-2 py-1.5" : "p-0")}>
                {/* Vote buttons */}
                <div className={cn(isCompact ? "" : "p-2 pr-0")}>
                    <VoteButtons
                        noteId={note.id}
                        upvotes={note.upvotes}
                        downvotes={note.downvotes}
                        orientation={isCompact ? "horizontal" : "vertical"}
                    />
                </div>

                {/* Card content */}
                <div className={cn("flex-1 min-w-0", isCompact ? "" : "p-2 pl-1")}>
                    {/* Meta line */}
                    <div className="flex items-center gap-1.5 text-xs text-muted-foreground mb-0.5">
                        <Link
                            href={`/communities/${note.subnotery_id}`}
                            className="font-semibold text-foreground hover:underline"
                        >
                            n/{note.subnotery_name || note.subnotery_id}
                        </Link>
                        <span>•</span>
                        <span>Posted by</span>
                        <Link
                            href={`/user/${note.creator_id}`}
                            className="hover:underline"
                        >
                            u/{note.author}
                        </Link>
                        <span>{timeAgo(note.created_at)}</span>
                    </div>

                    {/* Title */}
                    <Link href={`/notes/${note.id}`} className="block group/title">
                        <h3
                            className={cn(
                                "font-medium text-foreground group-hover/title:text-primary transition-colors",
                                isCompact ? "text-sm truncate" : "text-base leading-snug mb-1"
                            )}
                        >
                            {note.title}
                        </h3>
                    </Link>

                    {/* Thumbnail preview (card mode only) */}
                    {!isCompact && note.has_thumbnail && note.thumbnail_url && (
                        <Link href={`/notes/${note.id}`} className="block mt-2">
                            {/* eslint-disable-next-line @next/next/no-img-element */}
                            <img
                                src={thumbnailUrl(note.id, note.thumbnail_url)}
                                alt={`Thumbnail for ${note.title}`}
                                className="w-full max-h-48 object-cover rounded-md border border-border"
                            />
                        </Link>
                    )}

                    {/* Description snippet (card mode only) */}
                    {!isCompact && note.description && (
                        <p className="text-xs text-muted-foreground mt-1 line-clamp-2">
                            {note.description}
                        </p>
                    )}

                    {/* Card mode extras */}
                    {!isCompact && (
                        <div className="flex items-center gap-3 mt-2">
                            {/* Price badge */}
                            <Badge
                                variant={note.price === 0 ? "secondary" : "default"}
                                className="text-xs"
                            >
                                {formatPrice(note.price)}
                            </Badge>

                            {/* PDF indicator */}
                            {note.has_pdf && (
                                <span className="flex items-center gap-1 text-xs text-muted-foreground">
                                    <FileText className="h-3 w-3" />
                                    PDF
                                </span>
                            )}

                            {/* Purchase state */}
                            {purchased ? (
                                <span className="flex items-center gap-1 text-xs text-green-500">
                                    <CheckCircle className="h-3 w-3" />
                                    Owned
                                </span>
                            ) : (
                                note.price > 0 && (
                                    <span className="flex items-center gap-1 text-xs text-muted-foreground">
                                        <Lock className="h-3 w-3" />
                                        Locked
                                    </span>
                                )
                            )}
                        </div>
                    )}

                    {/* Bottom action bar */}
                    <div
                        className={cn(
                            "flex items-center gap-4 text-xs text-muted-foreground",
                            isCompact ? "" : "mt-2"
                        )}
                    >
                        <Link
                            href={`/notes/${note.id}`}
                            className="flex items-center gap-1 hover:text-foreground transition-colors"
                        >
                            <MessageSquare className="h-3.5 w-3.5" />
                            Comments
                        </Link>

                        <button
                            onClick={toggleBookmark}
                            className="flex items-center gap-1 hover:text-foreground transition-colors"
                            title={bookmarked ? "Remove bookmark" : "Bookmark"}
                        >
                            {bookmarked ? (
                                <BookmarkCheck className="h-3.5 w-3.5 text-primary" />
                            ) : (
                                <Bookmark className="h-3.5 w-3.5" />
                            )}
                            {bookmarked ? "Saved" : "Save"}
                        </button>

                        {/* Price in compact mode */}
                        {isCompact && (
                            <Badge
                                variant={note.price === 0 ? "secondary" : "default"}
                                className="text-[10px] h-5 px-1.5"
                            >
                                {formatPrice(note.price)}
                            </Badge>
                        )}
                    </div>
                </div>
            </div>
        </Card>
    );
}
