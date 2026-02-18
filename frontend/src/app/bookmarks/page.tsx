// page.tsx — Bookmarks page: shows all saved notes.
"use client";

import { NoteCard } from "@/components/feed/note-card";
import { RightSidebar } from "@/components/layout/right-sidebar";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { getBookmarks } from "@/services/bookmarks";
import { useAuthStore } from "@/stores/auth-store";
import { useFeedStore } from "@/stores/feed-store";
import type { Note } from "@/types";
import { useQuery } from "@tanstack/react-query";
import { Bookmark, ChevronLeft, ChevronRight } from "lucide-react";
import { useRouter } from "next/navigation";
import { useState } from "react";

const PAGE_SIZE = 20;

export default function BookmarksPage() {
    const { isAuthenticated } = useAuthStore();
    const { viewMode } = useFeedStore();
    const router = useRouter();
    const [page, setPage] = useState(1);

    const { data, isLoading, isError } = useQuery({
        queryKey: ["bookmarks", page],
        queryFn: () => getBookmarks({ page, limit: PAGE_SIZE }),
        enabled: isAuthenticated,
    });

    if (!isAuthenticated) {
        return (
            <div className="flex">
                <main className="flex-1 min-w-0 px-4 py-4">
                    <Card className="p-8 text-center max-w-xl mx-auto">
                        <Bookmark className="h-12 w-12 text-muted-foreground mx-auto mb-4" />
                        <h2 className="text-lg font-semibold mb-2">
                            Sign in to see bookmarks
                        </h2>
                        <p className="text-sm text-muted-foreground mb-4">
                            Save notes to come back to them later.
                        </p>
                        <Button onClick={() => router.push("/login")}>
                            Sign In
                        </Button>
                    </Card>
                </main>
            </div>
        );
    }

    const notes: Note[] = data?.notes ?? [];
    const total = data?.total ?? 0;
    const totalPages = Math.ceil(total / PAGE_SIZE);

    return (
        <div className="flex">
            <main className="flex-1 min-w-0 px-4 py-4">
                <div className="max-w-3xl mx-auto">
                    <div className="flex items-center gap-2 mb-4">
                        <Bookmark className="h-5 w-5" />
                        <h1 className="text-xl font-bold">Bookmarks</h1>
                        {total > 0 && (
                            <span className="text-sm text-muted-foreground">
                                ({total})
                            </span>
                        )}
                    </div>

                    {isLoading ? (
                        <div className="space-y-3">
                            {Array.from({ length: 5 }).map((_, i) => (
                                <Skeleton key={i} className="h-24 w-full" />
                            ))}
                        </div>
                    ) : isError ? (
                        <Card className="p-6 text-center text-destructive">
                            Failed to load bookmarks.
                        </Card>
                    ) : notes.length === 0 ? (
                        <Card className="p-8 text-center">
                            <Bookmark className="h-12 w-12 text-muted-foreground mx-auto mb-4" />
                            <h2 className="text-lg font-semibold mb-2">
                                No bookmarks yet
                            </h2>
                            <p className="text-sm text-muted-foreground">
                                Save notes from the feed to find them here later.
                            </p>
                        </Card>
                    ) : (
                        <>
                            <div className="space-y-2">
                                {notes.map((note) => (
                                    <NoteCard
                                        key={note.id}
                                        note={note}
                                        viewMode={viewMode}
                                        bookmarked
                                    />
                                ))}
                            </div>

                            {totalPages > 1 && (
                                <div className="flex items-center justify-center gap-4 mt-6">
                                    <Button
                                        variant="outline"
                                        size="sm"
                                        disabled={page <= 1}
                                        onClick={() =>
                                            setPage((p) => Math.max(1, p - 1))
                                        }
                                    >
                                        <ChevronLeft className="h-4 w-4 mr-1" />
                                        Previous
                                    </Button>
                                    <span className="text-sm text-muted-foreground">
                                        Page {page} of {totalPages}
                                    </span>
                                    <Button
                                        variant="outline"
                                        size="sm"
                                        disabled={page >= totalPages}
                                        onClick={() => setPage((p) => p + 1)}
                                    >
                                        Next
                                        <ChevronRight className="h-4 w-4 ml-1" />
                                    </Button>
                                </div>
                            )}
                        </>
                    )}
                </div>
            </main>
            <RightSidebar />
        </div>
    );
}
