// search-autocomplete.tsx — Dropdown below search bar showing top note and subnotery matches while typing.
"use client";

import { searchNotes, searchSubnoteries } from "@/services/search";
import type { Note, Subnotery } from "@/types";
import { FileText, Hash, Loader2 } from "lucide-react";
import Link from "next/link";
import { useEffect, useRef, useState } from "react";

interface SearchAutocompleteProps {
    query: string;
    visible: boolean;
    onClose: () => void;
}

export function SearchAutocomplete({ query, visible, onClose }: SearchAutocompleteProps) {
    const [notes, setNotes] = useState<Note[]>([]);
    const [subnoteries, setSubnoteries] = useState<Subnotery[]>([]);
    const [loading, setLoading] = useState(false);
    const containerRef = useRef<HTMLDivElement>(null);
    const abortRef = useRef<AbortController | null>(null);

    // Debounced fetch
    useEffect(() => {
        if (!query.trim() || !visible) {
            setNotes([]);
            setSubnoteries([]);
            return;
        }

        const timeout = setTimeout(async () => {
            // Cancel any in-flight requests
            abortRef.current?.abort();
            const controller = new AbortController();
            abortRef.current = controller;

            setLoading(true);
            try {
                const [noteRes, subRes] = await Promise.all([
                    searchNotes(query, { limit: 4 }),
                    searchSubnoteries(query, { limit: 5 }),
                ]);
                if (!controller.signal.aborted) {
                    setNotes(noteRes.results ?? []);
                    setSubnoteries(subRes.results ?? []);
                }
            } catch {
                if (!controller.signal.aborted) {
                    setNotes([]);
                    setSubnoteries([]);
                }
            } finally {
                if (!controller.signal.aborted) setLoading(false);
            }
        }, 250);

        return () => {
            clearTimeout(timeout);
            abortRef.current?.abort();
        };
    }, [query, visible]);

    // Close on click outside
    useEffect(() => {
        if (!visible) return;
        const handleClick = (e: MouseEvent) => {
            if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
                onClose();
            }
        };
        document.addEventListener("mousedown", handleClick);
        return () => document.removeEventListener("mousedown", handleClick);
    }, [visible, onClose]);

    if (!visible || !query.trim()) return null;

    const hasResults = notes.length > 0 || subnoteries.length > 0;

    return (
        <div
            ref={containerRef}
            className="absolute top-full left-0 right-0 mt-1 bg-popover border border-border rounded-lg shadow-lg z-50 overflow-hidden"
        >
            {loading && !hasResults ? (
                <div className="flex items-center justify-center py-4">
                    <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
                </div>
            ) : !hasResults ? (
                <div className="py-3 px-4 text-sm text-muted-foreground">
                    No results found
                </div>
            ) : (
                <>
                    {/* Notes section */}
                    {notes.length > 0 && (
                        <div>
                            <div className="px-3 py-1.5 text-xs font-semibold text-muted-foreground uppercase tracking-wider bg-muted/50">
                                Notes
                            </div>
                            {notes.map((note) => (
                                <Link
                                    key={note.id}
                                    href={`/notes/${note.id}`}
                                    onClick={onClose}
                                    className="flex items-center gap-2.5 px-3 py-2 hover:bg-accent transition-colors"
                                >
                                    <FileText className="h-4 w-4 text-muted-foreground shrink-0" />
                                    <div className="min-w-0 flex-1">
                                        <p className="text-sm font-medium truncate">{note.title}</p>
                                        <p className="text-xs text-muted-foreground truncate">
                                            n/{note.subnotery_name || note.subnotery_id}
                                        </p>
                                    </div>
                                </Link>
                            ))}
                        </div>
                    )}

                    {/* Subnoteries section */}
                    {subnoteries.length > 0 && (
                        <div>
                            <div className="px-3 py-1.5 text-xs font-semibold text-muted-foreground uppercase tracking-wider bg-muted/50">
                                Communities
                            </div>
                            {subnoteries.map((sub) => (
                                <Link
                                    key={sub.id}
                                    href={`/communities/${sub.id}`}
                                    onClick={onClose}
                                    className="flex items-center gap-2.5 px-3 py-2 hover:bg-accent transition-colors"
                                >
                                    <Hash className="h-4 w-4 text-muted-foreground shrink-0" />
                                    <div className="min-w-0 flex-1">
                                        <p className="text-sm font-medium">n/{sub.name}</p>
                                        {sub.member_count !== undefined && (
                                            <p className="text-xs text-muted-foreground">
                                                {sub.member_count} member{sub.member_count !== 1 ? "s" : ""}
                                            </p>
                                        )}
                                    </div>
                                </Link>
                            ))}
                        </div>
                    )}
                </>
            )}
        </div>
    );
}
