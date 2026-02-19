// note-feed.tsx — The main feed component that loads and renders note cards.
// Supports infinite scroll pagination.
"use client";

import { Button } from "@/components/ui/button";
import { getApprovedNotes, getHotFeed } from "@/services/notes";
import { useFeedStore } from "@/stores/feed-store";
import { useInfiniteQuery } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { useCallback, useEffect, useRef } from "react";
import { FeedSkeleton } from "./feed-skeleton";
import { NoteCard } from "./note-card";
import { SortTabs } from "./sort-tabs";

import type { FeedSort } from "@/types";

interface NoteFeedProps {
    initialSort?: FeedSort;
}

export function NoteFeed({ initialSort }: NoteFeedProps = {}) {
    const { sort, timeFilter, setSort } = useFeedStore();
    const loadMoreRef = useRef<HTMLDivElement>(null);

    // Sync initial sort from route if provided
    useEffect(() => {
        if (initialSort && initialSort !== sort) {
            setSort(initialSort);
        }
        // Only on mount
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    const {
        data,
        fetchNextPage,
        hasNextPage,
        isFetchingNextPage,
        isLoading,
        isError,
        error,
    } = useInfiniteQuery({
        queryKey: ["feed", sort, (sort === "top" || sort === "controversial") ? timeFilter : null],
        queryFn: async ({ pageParam = 1 }) => {
            if (sort === "hot") {
                return getHotFeed({ page: pageParam, limit: 25 });
            }
            // For "new", "top", "controversial": use the approved notes endpoint.
            // Time filter applies to "top" and "controversial".
            return getApprovedNotes({
                page: pageParam,
                limit: 25,
                sort: sort,
                time: (sort === "top" || sort === "controversial") ? timeFilter : undefined,
            });
        },
        getNextPageParam: (lastPage, allPages) => {
            // Hot feed doesn't return total, so check if we got a full page
            const notes = lastPage.notes || [];
            if (notes.length < 25) return undefined;
            return allPages.length + 1;
        },
        initialPageParam: 1,
    });

    // Intersection Observer for infinite scroll
    const handleObserver = useCallback(
        (entries: IntersectionObserverEntry[]) => {
            const [entry] = entries;
            if (entry.isIntersecting && hasNextPage && !isFetchingNextPage) {
                fetchNextPage();
            }
        },
        [fetchNextPage, hasNextPage, isFetchingNextPage]
    );

    useEffect(() => {
        const el = loadMoreRef.current;
        if (!el) return;

        const observer = new IntersectionObserver(handleObserver, {
            threshold: 0.1,
        });
        observer.observe(el);
        return () => observer.disconnect();
    }, [handleObserver]);

    const allNotes = data?.pages.flatMap((p) => p.notes || []) ?? [];

    return (
        <div>
            <SortTabs />

            {isLoading ? (
                <FeedSkeleton />
            ) : isError ? (
                <div className="text-center py-8">
                    <p className="text-sm text-destructive">
                        Failed to load feed: {(error as Error).message}
                    </p>
                    <Button
                        variant="outline"
                        size="sm"
                        className="mt-2"
                        onClick={() => window.location.reload()}
                    >
                        Retry
                    </Button>
                </div>
            ) : allNotes.length === 0 ? (
                <div className="text-center py-12">
                    <p className="text-muted-foreground">No notes yet. Be the first to post!</p>
                </div>
            ) : (
                <div className="space-y-4">
                    {allNotes.map((note) => (
                        <NoteCard
                            key={note.id}
                            note={note}
                        />
                    ))}
                </div>
            )}

            {/* Load more trigger */}
            <div ref={loadMoreRef} className="h-10 flex items-center justify-center mt-4">
                {isFetchingNextPage && (
                    <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
                )}
            </div>
        </div>
    );
}
