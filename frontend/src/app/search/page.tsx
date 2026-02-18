// page.tsx — Search page with Reddit-style multi-type search, All tab, and sort.
"use client";

import { NoteCard } from "@/components/feed/note-card";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { timeAgo } from "@/lib/format";
import { search } from "@/services/search";
import { useFeedStore } from "@/stores/feed-store";
import type {
    CommentSearchResult,
    Note,
    PublicProfile,
    SearchSort,
    SearchType,
    Subnotery,
} from "@/types";
import { useQuery } from "@tanstack/react-query";
import {
    BookOpen,
    FileText,
    Layers,
    MessageSquare,
    Search,
    Users,
} from "lucide-react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useEffect, useState } from "react";

const PAGE_SIZE = 25;
const ALL_PREVIEW_LIMIT = 3;

function SearchPageContent() {
    const searchParams = useSearchParams();
    const router = useRouter();
    const { viewMode } = useFeedStore();
    const [query, setQuery] = useState(searchParams.get("q") || "");
    const [type, setType] = useState<SearchType>(
        (searchParams.get("type") as SearchType) || "all"
    );
    const [sort, setSort] = useState<SearchSort>(
        (searchParams.get("sort") as SearchSort) || "relevance"
    );
    const [page, setPage] = useState(1);

    const searchQuery = searchParams.get("q") || "";

    // Single-type query for non-"all" tabs.
    const singleQuery = useQuery({
        queryKey: ["search", searchQuery, type, sort, page],
        queryFn: () =>
            search({ q: searchQuery, type, sort, page, limit: PAGE_SIZE }),
        enabled: !!searchQuery && type !== "all",
    });

    // "All" tab: one query per type, limited preview.
    const allNotes = useQuery({
        queryKey: ["search", searchQuery, "notes", sort, "all-preview"],
        queryFn: () =>
            search<Note>({
                q: searchQuery,
                type: "notes",
                sort,
                page: 1,
                limit: ALL_PREVIEW_LIMIT,
            }),
        enabled: !!searchQuery && type === "all",
    });
    const allSubnoteries = useQuery({
        queryKey: ["search", searchQuery, "subnoteries", sort, "all-preview"],
        queryFn: () =>
            search<Subnotery>({
                q: searchQuery,
                type: "subnoteries",
                sort,
                page: 1,
                limit: ALL_PREVIEW_LIMIT,
            }),
        enabled: !!searchQuery && type === "all",
    });
    const allUsers = useQuery({
        queryKey: ["search", searchQuery, "users", sort, "all-preview"],
        queryFn: () =>
            search<PublicProfile>({
                q: searchQuery,
                type: "users",
                sort,
                page: 1,
                limit: ALL_PREVIEW_LIMIT,
            }),
        enabled: !!searchQuery && type === "all",
    });
    const allComments = useQuery({
        queryKey: ["search", searchQuery, "comments", sort, "all-preview"],
        queryFn: () =>
            search<CommentSearchResult>({
                q: searchQuery,
                type: "comments",
                sort,
                page: 1,
                limit: ALL_PREVIEW_LIMIT,
            }),
        enabled: !!searchQuery && type === "all",
    });

    useEffect(() => {
        setPage(1);
    }, [searchQuery, type, sort]);

    const handleSearch = (e: React.FormEvent) => {
        e.preventDefault();
        if (query.trim()) {
            router.push(
                `/search?q=${encodeURIComponent(query.trim())}&type=${type}&sort=${sort}`
            );
        }
    };

    const results = singleQuery.data?.results ?? [];
    const total = singleQuery.data?.total ?? 0;
    const isLoading =
        type === "all"
            ? allNotes.isLoading ||
            allSubnoteries.isLoading ||
            allUsers.isLoading ||
            allComments.isLoading
            : singleQuery.isLoading;
    const isError =
        type === "all"
            ? allNotes.isError &&
            allSubnoteries.isError &&
            allUsers.isError &&
            allComments.isError
            : singleQuery.isError;

    return (
        <div className="max-w-3xl mx-auto px-4 py-4">
            {/* Search input */}
            <form onSubmit={handleSearch} className="mb-4">
                <div className="relative">
                    <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                    <Input
                        type="search"
                        placeholder="Search notes, users, communities..."
                        className="pl-9"
                        value={query}
                        onChange={(e) => setQuery(e.target.value)}
                        autoFocus
                    />
                </div>
            </form>

            {/* Type tabs + Sort dropdown */}
            <div className="flex items-center gap-3 mb-4">
                <Tabs
                    value={type}
                    onValueChange={(v) => setType(v as SearchType)}
                    className="flex-1 min-w-0"
                >
                    <TabsList className="w-full justify-start">
                        <TabsTrigger value="all" className="gap-1.5">
                            <Layers className="h-3.5 w-3.5" />
                            All
                        </TabsTrigger>
                        <TabsTrigger value="notes" className="gap-1.5">
                            <FileText className="h-3.5 w-3.5" />
                            Notes
                        </TabsTrigger>
                        <TabsTrigger value="subnoteries" className="gap-1.5">
                            <BookOpen className="h-3.5 w-3.5" />
                            Communities
                        </TabsTrigger>
                        <TabsTrigger value="users" className="gap-1.5">
                            <Users className="h-3.5 w-3.5" />
                            Users
                        </TabsTrigger>
                        <TabsTrigger value="comments" className="gap-1.5">
                            <MessageSquare className="h-3.5 w-3.5" />
                            Comments
                        </TabsTrigger>
                    </TabsList>
                </Tabs>

                <Select
                    value={sort}
                    onValueChange={(v) => setSort(v as SearchSort)}
                >
                    <SelectTrigger className="w-[140px] shrink-0">
                        <SelectValue placeholder="Sort" />
                    </SelectTrigger>
                    <SelectContent>
                        <SelectItem value="relevance">Relevance</SelectItem>
                        <SelectItem value="hot">Hotness</SelectItem>
                        <SelectItem value="new">Newest</SelectItem>
                        <SelectItem value="top">Top</SelectItem>
                        <SelectItem value="comments">Comments</SelectItem>
                    </SelectContent>
                </Select>
            </div>

            {/* Results */}
            {!searchQuery ? (
                <div className="text-center py-12">
                    <Search className="h-12 w-12 text-muted-foreground mx-auto mb-4" />
                    <p className="text-muted-foreground">
                        Enter a search query to find notes, users, and more.
                    </p>
                </div>
            ) : isLoading ? (
                <div className="space-y-3">
                    {Array.from({ length: 5 }).map((_, i) => (
                        <Skeleton key={i} className="h-20 w-full" />
                    ))}
                </div>
            ) : isError ? (
                <p className="text-sm text-destructive py-4">
                    Search failed. Please try again.
                </p>
            ) : type === "all" ? (
                <AllResults
                    notes={allNotes.data}
                    subnoteries={allSubnoteries.data}
                    users={allUsers.data}
                    comments={allComments.data}
                    onSwitchTab={setType}
                    viewMode={viewMode}
                />
            ) : results.length === 0 ? (
                <div className="text-center py-12">
                    <p className="text-muted-foreground">
                        No results found for &ldquo;{searchQuery}&rdquo; in{" "}
                        {type}.
                    </p>
                </div>
            ) : (
                <>
                    <p className="text-xs text-muted-foreground mb-3">
                        {total} result{total !== 1 ? "s" : ""} for &ldquo;
                        {searchQuery}&rdquo;
                    </p>

                    <div className="space-y-2">
                        {type === "notes" && <NoteResults notes={results as Note[]} viewMode={viewMode} />}
                        {type === "users" && <UserResults users={results as PublicProfile[]} />}
                        {type === "subnoteries" && <SubnoteryResults subnoteries={results as Subnotery[]} />}
                        {type === "comments" && <CommentResults comments={results as CommentSearchResult[]} />}
                    </div>

                    {/* Pagination */}
                    {total > PAGE_SIZE && (
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
                                Page {page} of {Math.ceil(total / PAGE_SIZE)}
                            </span>
                            <Button
                                variant="outline"
                                size="sm"
                                disabled={page >= Math.ceil(total / PAGE_SIZE)}
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

// ─── Result renderers ─────────────────────────────────────────────────────────

function NoteResults({
    notes,
    viewMode,
}: {
    notes: Note[];
    viewMode: string;
}) {
    return (
        <>
            {notes.map((note) => (
                <NoteCard
                    key={note.id}
                    note={note}
                    viewMode={viewMode as "card" | "compact"}
                />
            ))}
        </>
    );
}

function UserResults({ users }: { users: PublicProfile[] }) {
    return (
        <>
            {users.map((user) => (
                <Card key={user.id} className="border-border">
                    <CardContent className="flex items-center gap-3 p-3">
                        <div className="h-10 w-10 rounded-full bg-primary/10 flex items-center justify-center">
                            <Users className="h-5 w-5 text-primary" />
                        </div>
                        <div>
                            <Link
                                href={`/user/${user.id}`}
                                className="font-medium text-sm hover:text-primary"
                            >
                                u/{user.username}
                            </Link>
                            {user.display_name && (
                                <p className="text-xs text-muted-foreground">
                                    {user.display_name}
                                </p>
                            )}
                        </div>
                    </CardContent>
                </Card>
            ))}
        </>
    );
}

function SubnoteryResults({ subnoteries }: { subnoteries: Subnotery[] }) {
    return (
        <>
            {subnoteries.map((sub) => (
                <Card key={sub.id} className="border-border">
                    <CardContent className="flex items-center gap-3 p-3">
                        <div className="h-10 w-10 rounded-full bg-primary/10 flex items-center justify-center">
                            <BookOpen className="h-5 w-5 text-primary" />
                        </div>
                        <div>
                            <Link
                                href={`/communities/${sub.id}`}
                                className="font-medium text-sm hover:text-primary"
                            >
                                n/{sub.name}
                            </Link>
                            <p className="text-xs text-muted-foreground">
                                {sub.members?.length ?? sub.member_count ?? 0}{" "}
                                members
                            </p>
                        </div>
                    </CardContent>
                </Card>
            ))}
        </>
    );
}

function CommentResults({ comments }: { comments: CommentSearchResult[] }) {
    return (
        <>
            {comments.map((comment) => (
                <Card key={comment.id} className="border-border">
                    <CardContent className="p-3">
                        <div className="flex items-center gap-1.5 text-xs text-muted-foreground mb-1">
                            <span className="font-semibold text-foreground">
                                {comment.username}
                            </span>
                            <span>&bull;</span>
                            <span>{timeAgo(comment.created_at)}</span>
                            <span>&bull;</span>
                            <Link
                                href={`/notes/${comment.note_id}`}
                                className="hover:underline"
                            >
                                in note #{comment.note_id}
                            </Link>
                        </div>
                        <p className="text-sm text-foreground line-clamp-2">
                            {comment.body}
                        </p>
                        <div className="flex gap-2 mt-1 text-xs text-muted-foreground">
                            <span>&uarr; {comment.upvotes}</span>
                            <span>&darr; {comment.downvotes}</span>
                        </div>
                    </CardContent>
                </Card>
            ))}
        </>
    );
}

// ─── "All" tab: shows a preview section per type ──────────────────────────────

interface AllResultsProps {
    notes?: { results: Note[]; total: number };
    subnoteries?: { results: Subnotery[]; total: number };
    users?: { results: PublicProfile[]; total: number };
    comments?: { results: CommentSearchResult[]; total: number };
    onSwitchTab: (type: SearchType) => void;
    viewMode: string;
}

function AllResults({
    notes,
    subnoteries,
    users,
    comments,
    onSwitchTab,
    viewMode,
}: AllResultsProps) {
    const sections: {
        key: Exclude<SearchType, "all">;
        label: string;
        icon: React.ReactNode;
    }[] = [
            {
                key: "notes",
                label: "Notes",
                icon: <FileText className="h-4 w-4" />,
            },
            {
                key: "subnoteries",
                label: "Communities",
                icon: <BookOpen className="h-4 w-4" />,
            },
            {
                key: "users",
                label: "Users",
                icon: <Users className="h-4 w-4" />,
            },
            {
                key: "comments",
                label: "Comments",
                icon: <MessageSquare className="h-4 w-4" />,
            },
        ];

    const dataMap: Record<
        string,
        { results: unknown[]; total: number } | undefined
    > = {
        notes,
        subnoteries,
        users,
        comments,
    };

    const hasAnyResults = Object.values(dataMap).some(
        (d) => d && d.results.length > 0
    );

    if (!hasAnyResults) {
        return (
            <div className="text-center py-12">
                <p className="text-muted-foreground">
                    No results found across any category.
                </p>
            </div>
        );
    }

    return (
        <div className="space-y-6">
            {sections.map(({ key, label, icon }) => {
                const data = dataMap[key];
                if (!data || data.results.length === 0) return null;
                return (
                    <div key={key}>
                        <div className="flex items-center justify-between mb-2">
                            <h3 className="text-sm font-semibold flex items-center gap-1.5">
                                {icon}
                                {label}
                                <span className="text-muted-foreground font-normal">
                                    ({data.total})
                                </span>
                            </h3>
                            {data.total > ALL_PREVIEW_LIMIT && (
                                <Button
                                    variant="ghost"
                                    size="sm"
                                    className="text-xs"
                                    onClick={() => onSwitchTab(key)}
                                >
                                    See all &rarr;
                                </Button>
                            )}
                        </div>
                        <div className="space-y-2">
                            {key === "notes" && (
                                <NoteResults
                                    notes={data.results as Note[]}
                                    viewMode={viewMode}
                                />
                            )}
                            {key === "users" && (
                                <UserResults
                                    users={data.results as PublicProfile[]}
                                />
                            )}
                            {key === "subnoteries" && (
                                <SubnoteryResults
                                    subnoteries={
                                        data.results as Subnotery[]
                                    }
                                />
                            )}
                            {key === "comments" && (
                                <CommentResults
                                    comments={
                                        data.results as CommentSearchResult[]
                                    }
                                />
                            )}
                        </div>
                    </div>
                );
            })}
        </div>
    );
}

export default function SearchPage() {
    return (
        <Suspense
            fallback={
                <div className="max-w-3xl mx-auto px-4 py-4">
                    <div className="h-10 w-full bg-muted animate-pulse rounded mb-4" />
                    <div className="space-y-3">
                        {Array.from({ length: 5 }).map((_, i) => (
                            <div
                                key={i}
                                className="h-20 w-full bg-muted animate-pulse rounded"
                            />
                        ))}
                    </div>
                </div>
            }
        >
            <SearchPageContent />
        </Suspense>
    );
}
