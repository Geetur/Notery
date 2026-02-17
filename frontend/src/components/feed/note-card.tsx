// note-card.tsx — Reddit-style post card for the feed.
// Supports both "card" (expanded) and "compact" (dense list) view modes.
"use client";

import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { formatPrice, timeAgo } from "@/lib/format";
import { cn } from "@/lib/utils";
import type { Note, ViewMode } from "@/types";
import { Download, FileText, Lock, MessageSquare } from "lucide-react";
import Link from "next/link";
import { VoteButtons } from "./vote-buttons";

interface NoteCardProps {
    note: Note;
    viewMode: ViewMode;
    purchased?: boolean;
}

export function NoteCard({ note, viewMode, purchased }: NoteCardProps) {
    const isCompact = viewMode === "compact";

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
                            href={`/n/${note.subnotery_id}`}
                            className="font-semibold text-foreground hover:underline"
                        >
                            n/{note.subnotery_id}
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
                                    <Download className="h-3 w-3" />
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
