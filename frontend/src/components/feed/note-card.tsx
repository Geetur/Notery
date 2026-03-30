// note-card.tsx — Reddit-style expanded post card for the feed.
// Unified card layout matching the note detail page style: vote buttons on left,
// meta line with subnotery avatar, title, badges (price, PDF, status),
// description, and large thumbnail.
// Admins see a three-dot menu to delete notes from a subnotery.
"use client";

import { SubnoteryAvatar } from "@/components/subnotery-avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useToast } from "@/hooks/use-toast";
import { formatFileSize, formatPrice, thumbnailUrl, timeAgo } from "@/lib/format";
import { addBookmark, removeBookmark } from "@/services/bookmarks";
import { lockNote, unlockNote } from "@/services/notes";
import { useAuthStore } from "@/stores/auth-store";
import type { Note, ViewMode } from "@/types";
import { Bookmark, BookmarkCheck, FileText, Lock, LockOpen, MessageSquare, MoreVertical, Trash2 } from "lucide-react";
import Link from "next/link";
import { useState } from "react";
import { VoteButtons } from "./vote-buttons";

interface NoteCardProps {
    note: Note;
    /** Kept for API compatibility — layout is always expanded. */
    viewMode?: ViewMode;
    purchased?: boolean;
    bookmarked?: boolean;
    /** If true, shows admin controls (delete). */
    isAdmin?: boolean;
    /** Called when admin deletes the note. */
    onDelete?: (noteId: number) => void;
}

export function NoteCard({ note, bookmarked: initialBookmarked, isAdmin, onDelete }: NoteCardProps) {
    const { isAuthenticated } = useAuthStore();
    const { toast } = useToast();
    const [bookmarked, setBookmarked] = useState(initialBookmarked ?? false);
    const [locked, setLocked] = useState(note.is_locked ?? false);

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
                        initialUserVote={note.user_vote}
                        noteStatus={note.status}
                        orientation="vertical"
                    />
                </div>

                {/* Card content */}
                <div className="flex-1 min-w-0 overflow-hidden p-3 pl-2">
                    {/* Meta line */}
                    <div className="flex items-center gap-1.5 text-xs text-muted-foreground mb-1">
                        <div className="flex-1 flex items-center gap-1.5 min-w-0">
                            <SubnoteryAvatar
                                subnoteryId={note.subnotery_id}
                                profilePictureUrl={note.subnotery_profile_picture_url}
                                name={note.subnotery_name}
                                size="sm"
                            />
                            <Link
                                href={`/communities/${note.subnotery_id}`}
                                className="font-semibold text-foreground hover:underline"
                            >
                                n/{note.subnotery_name || note.subnotery_id}
                            </Link>
                            <span>•</span>
                            <span>{timeAgo(note.created_at)}</span>
                        </div>
                        {isAdmin && onDelete && (
                            <DropdownMenu>
                                <DropdownMenuTrigger asChild>
                                    <Button variant="ghost" size="sm" className="h-6 w-6 p-0 shrink-0">
                                        <MoreVertical className="h-3.5 w-3.5" />
                                    </Button>
                                </DropdownMenuTrigger>
                                <DropdownMenuContent align="end">
                                    <DropdownMenuItem
                                        onClick={async (e) => {
                                            e.preventDefault();
                                            e.stopPropagation();
                                            try {
                                                if (locked) {
                                                    await unlockNote(note.id);
                                                    setLocked(false);
                                                    toast({ title: "Unlocked", description: "Comments re-enabled." });
                                                } else {
                                                    await lockNote(note.id);
                                                    setLocked(true);
                                                    toast({ title: "Locked", description: "Comments disabled." });
                                                }
                                            } catch {
                                                toast({ title: "Error", description: "Failed to toggle lock.", variant: "destructive" });
                                            }
                                        }}
                                    >
                                        {locked ? (
                                            <><LockOpen className="h-3.5 w-3.5 mr-2" /> Unlock Comments</>
                                        ) : (
                                            <><Lock className="h-3.5 w-3.5 mr-2" /> Lock Comments</>
                                        )}
                                    </DropdownMenuItem>
                                    <DropdownMenuItem
                                        className="text-destructive focus:text-destructive"
                                        onClick={(e) => {
                                            e.preventDefault();
                                            e.stopPropagation();
                                            onDelete(note.id);
                                        }}
                                    >
                                        <Trash2 className="h-3.5 w-3.5 mr-2" />
                                        Delete Note
                                    </DropdownMenuItem>
                                </DropdownMenuContent>
                            </DropdownMenu>
                        )}
                    </div>

                    {/* Title */}
                    <Link href={`/notes/${note.id}`} className="block group/title">
                        <h3 className="text-lg font-bold text-foreground group-hover/title:text-primary transition-colors leading-snug mb-2">
                            {note.title}
                        </h3>
                    </Link>



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
                            {note.comment_count > 0 && note.comment_count}
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
                        </button>

                        <div className="ml-auto flex items-center gap-2">
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
                        </div>
                    </div>
                </div>
            </div>
        </Card>
    );
}
