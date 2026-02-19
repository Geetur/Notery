// note-card.tsx — Reddit-style expanded post card for the feed.
// Unified card layout matching the note detail page style: vote buttons on left,
// meta line, title, badges (price, PDF, status), description, and large thumbnail.
"use client";

import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { useToast } from "@/hooks/use-toast";
import { formatFileSize, formatPrice, thumbnailUrl, timeAgo } from "@/lib/format";
import { addBookmark, removeBookmark } from "@/services/bookmarks";
import { useAuthStore } from "@/stores/auth-store";
import type { Note, ViewMode } from "@/types";
import { Bookmark, BookmarkCheck, CheckCircle, FileText, Lock, MessageSquare } from "lucide-react";
import Link from "next/link";
import { useState } from "react";
import { VoteButtons } from "./vote-buttons";

interface NoteCardProps {
    note: Note;
    /** Kept for API compatibility — layout is always expanded. */
    viewMode?: ViewMode;
    purchased?: boolean;
    bookmarked?: boolean;
}

export function NoteCard({ note, purchased, bookmarked: initialBookmarked }: NoteCardProps) {
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
        <Card className="group border-border hover:border-primary/30 transition-colors">
            <div className="flex p-0">
                {/* Vote buttons */}
                <div className="p-3 pr-0">
                    <VoteButtons
                        noteId={note.id}
                        upvotes={note.upvotes}
                        downvotes={note.downvotes}
                        orientation="vertical"
                    />
                </div>

                {/* Card content */}
                <div className="flex-1 min-w-0 p-3 pl-2">
                    {/* Meta line */}
                    <div className="flex items-center gap-1.5 text-xs text-muted-foreground mb-1">
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
                        <span>•</span>
                        <span>{timeAgo(note.created_at)}</span>
                    </div>

                    {/* Title */}
                    <Link href={`/notes/${note.id}`} className="block group/title">
                        <h3 className="text-lg font-bold text-foreground group-hover/title:text-primary transition-colors leading-snug mb-2">
                            {note.title}
                        </h3>
                    </Link>

                    {/* Badges row — price, PDF info, status */}
                    <div className="flex items-center gap-2 mb-3">
                        <Badge
                            variant={note.price === 0 ? "secondary" : "default"}
                            className="text-xs"
                        >
                            {formatPrice(note.price)}
                        </Badge>

                        {note.has_pdf && (
                            <Badge variant="outline" className="text-xs">
                                <FileText className="h-3 w-3 mr-1" />
                                PDF — {formatFileSize(note.pdf_size)}
                            </Badge>
                        )}

                        {note.status && (
                            <Badge
                                variant="outline"
                                className={
                                    note.status === "Approved"
                                        ? "text-xs border-green-500/50 text-green-500"
                                        : note.status === "Pending"
                                            ? "text-xs border-yellow-500/50 text-yellow-500"
                                            : "text-xs border-red-500/50 text-red-500"
                                }
                            >
                                {note.status}
                            </Badge>
                        )}

                        {/* Purchase state */}
                        {purchased ? (
                            <span className="flex items-center gap-1 text-xs text-green-500">
                                <CheckCircle className="h-3 w-3" />
                                Owned
                            </span>
                        ) : (
                            note.price > 0 && note.status === "Approved" && (
                                <span className="flex items-center gap-1 text-xs text-muted-foreground">
                                    <Lock className="h-3 w-3" />
                                    Locked
                                </span>
                            )
                        )}
                    </div>

                    {/* Description */}
                    {note.description && (
                        <p className="text-sm text-muted-foreground mb-3 line-clamp-3 whitespace-pre-wrap">
                            {note.description}
                        </p>
                    )}

                    {/* Thumbnail — large, full width */}
                    {note.has_thumbnail && note.thumbnail_url && (
                        <Link href={`/notes/${note.id}`} className="block mb-3">
                            {/* eslint-disable-next-line @next/next/no-img-element */}
                            <img
                                src={thumbnailUrl(note.id, note.thumbnail_url)}
                                alt={`Thumbnail for ${note.title}`}
                                className="w-full max-h-[400px] object-contain rounded-md border border-border bg-muted/30"
                            />
                        </Link>
                    )}

                    {/* Bottom action bar */}
                    <div className="flex items-center gap-4 text-xs text-muted-foreground mt-1">
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
                    </div>
                </div>
            </div>
        </Card>
    );
}
