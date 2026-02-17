// page.tsx — Search page with Reddit-style multi-type search.
"use client";

import { useState, useEffect, Suspense } from "react";
import { useSearchParams, useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { Search, FileText, Users, MessageSquare, BookOpen } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { NoteCard } from "@/components/feed/note-card";
import { search } from "@/services/search";
import { timeAgo } from "@/lib/format";
import { useFeedStore } from "@/stores/feed-store";
import type { SearchType, Note, PublicProfile, Subnotery, CommentSearchResult } from "@/types";
import Link from "next/link";

function SearchPageContent() {
  const searchParams = useSearchParams();
  const router = useRouter();
  const { viewMode } = useFeedStore();
  const [query, setQuery] = useState(searchParams.get("q") || "");
  const [type, setType] = useState<SearchType>(
    (searchParams.get("type") as SearchType) || "notes"
  );
  const [page, setPage] = useState(1);

  const searchQuery = searchParams.get("q") || "";

  const { data, isLoading, isError } = useQuery({
    queryKey: ["search", searchQuery, type, page],
    queryFn: () => search({ q: searchQuery, type, page, limit: 25 }),
    enabled: !!searchQuery,
  });

  useEffect(() => {
    setPage(1);
  }, [searchQuery, type]);

  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault();
    if (query.trim()) {
      router.push(`/search?q=${encodeURIComponent(query.trim())}&type=${type}`);
    }
  };

  const results = data?.results ?? [];
  const total = data?.total ?? 0;

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

      {/* Type tabs */}
      <Tabs value={type} onValueChange={(v) => setType(v as SearchType)} className="mb-4">
        <TabsList className="w-full justify-start">
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

      {/* Results */}
      {!searchQuery ? (
        <div className="text-center py-12">
          <Search className="h-12 w-12 text-muted-foreground mx-auto mb-4" />
          <p className="text-muted-foreground">Enter a search query to find notes, users, and more.</p>
        </div>
      ) : isLoading ? (
        <div className="space-y-3">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="h-20 w-full" />
          ))}
        </div>
      ) : isError ? (
        <p className="text-sm text-destructive py-4">Search failed. Please try again.</p>
      ) : results.length === 0 ? (
        <div className="text-center py-12">
          <p className="text-muted-foreground">
            No results found for &ldquo;{searchQuery}&rdquo; in {type}.
          </p>
        </div>
      ) : (
        <>
          <p className="text-xs text-muted-foreground mb-3">
            {total} result{total !== 1 ? "s" : ""} for &ldquo;{searchQuery}&rdquo;
          </p>

          <div className="space-y-2">
            {type === "notes" &&
              (results as Note[]).map((note) => (
                <NoteCard key={note.id} note={note} viewMode={viewMode} />
              ))}

            {type === "users" &&
              (results as PublicProfile[]).map((user) => (
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

            {type === "subnoteries" &&
              (results as Subnotery[]).map((sub) => (
                <Card key={sub.id} className="border-border">
                  <CardContent className="flex items-center gap-3 p-3">
                    <div className="h-10 w-10 rounded-full bg-primary/10 flex items-center justify-center">
                      <BookOpen className="h-5 w-5 text-primary" />
                    </div>
                    <div>
                      <Link
                        href={`/n/${sub.id}`}
                        className="font-medium text-sm hover:text-primary"
                      >
                        n/{sub.name}
                      </Link>
                      <p className="text-xs text-muted-foreground">
                        {sub.members?.length ?? 0} members
                      </p>
                    </div>
                  </CardContent>
                </Card>
              ))}

            {type === "comments" &&
              (results as CommentSearchResult[]).map((comment) => (
                <Card key={comment.id} className="border-border">
                  <CardContent className="p-3">
                    <div className="flex items-center gap-1.5 text-xs text-muted-foreground mb-1">
                      <span className="font-semibold text-foreground">
                        {comment.username}
                      </span>
                      <span>•</span>
                      <span>{timeAgo(comment.created_at)}</span>
                      <span>•</span>
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
                      <span>↑ {comment.upvotes}</span>
                      <span>↓ {comment.downvotes}</span>
                    </div>
                  </CardContent>
                </Card>
              ))}
          </div>

          {/* Pagination */}
          {total > 25 && (
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
                Page {page} of {Math.ceil(total / 25)}
              </span>
              <Button
                variant="outline"
                size="sm"
                disabled={page >= Math.ceil(total / 25)}
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

export default function SearchPage() {
  return (
    <Suspense
      fallback={
        <div className="max-w-3xl mx-auto px-4 py-4">
          <div className="h-10 w-full bg-muted animate-pulse rounded mb-4" />
          <div className="space-y-3">
            {Array.from({ length: 5 }).map((_, i) => (
              <div key={i} className="h-20 w-full bg-muted animate-pulse rounded" />
            ))}
          </div>
        </div>
      }
    >
      <SearchPageContent />
    </Suspense>
  );
}
