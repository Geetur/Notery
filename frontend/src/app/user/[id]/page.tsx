// page.tsx — Public user profile page: Reddit-style with Overview, Posts, and Comments tabs.
// Shows profile banner, avatar, username, bio, joined date, posted notes, and comments.
// No edit controls — those are only on the own-profile page (/profile).
"use client";

import { NoteCard } from "@/components/feed/note-card";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { avatarUrl, formatDate, userBannerUrl } from "@/lib/format";
import { getUserComments } from "@/services/comments";
import { getUserNotes } from "@/services/notes";
import { getUserProfile } from "@/services/profile";
import { useQuery } from "@tanstack/react-query";
import { FileText, MessageSquare } from "lucide-react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useState } from "react";

type UserTab = "overview" | "posts" | "comments";

export default function UserProfilePage() {
    const params = useParams();
    const userId = Number(params.id);
    const [activeTab, setActiveTab] = useState<UserTab>("overview");
    const [commentsPage, setCommentsPage] = useState(1);

    const {
        data: profile,
        isLoading,
        isError,
    } = useQuery({
        queryKey: ["userProfile", userId],
        queryFn: () => getUserProfile(userId),
        enabled: !!userId,
    });

    const { data: notesData, isLoading: notesLoading } = useQuery({
        queryKey: ["userNotes", userId],
        queryFn: () => getUserNotes(userId, { limit: 25 }),
        enabled: !!userId,
    });

    const { data: commentsData, isLoading: commentsLoading } = useQuery({
        queryKey: ["userComments", userId, commentsPage],
        queryFn: () => getUserComments(userId, { page: commentsPage, limit: 25 }),
        enabled: !!userId && (activeTab === "comments" || activeTab === "overview"),
    });

    if (isLoading) {
        return (
            <div className="flex">
                <main className="flex-1 min-w-0 px-4 py-0">
                    <div className="h-28 bg-gradient-to-r from-primary/20 to-primary/5 -mx-4" />
                    <div className="max-w-3xl mx-auto">
                        <div className="flex items-end gap-4 -mt-10 mb-4 px-2">
                            <Skeleton className="h-20 w-20 rounded-full" />
                            <div className="space-y-2 pb-1">
                                <Skeleton className="h-6 w-40" />
                                <Skeleton className="h-4 w-24" />
                            </div>
                        </div>
                        <Skeleton className="h-10 w-48 mb-4 mx-2" />
                        <div className="space-y-3 px-2">
                            <Skeleton className="h-24 w-full" />
                            <Skeleton className="h-24 w-full" />
                        </div>
                    </div>
                </main>
            </div>
        );
    }

    if (isError || !profile) {
        return (
            <div className="flex">
                <main className="flex-1 min-w-0 px-4 py-8 text-center">
                    <p className="text-destructive">User not found.</p>
                </main>
            </div>
        );
    }

    const notes = notesData?.notes ?? [];

    const tabs: { key: UserTab; label: string; icon: React.ReactNode }[] = [
        { key: "overview", label: "Overview", icon: null },
        { key: "posts", label: "Posts", icon: <FileText className="h-4 w-4" /> },
        { key: "comments", label: "Comments", icon: <MessageSquare className="h-4 w-4" /> },
    ];

    return (
        <div className="flex">
            <main className="flex-1 min-w-0 px-4 py-0">
                {/* Profile banner */}
                <div className="h-28 -mx-4">
                    {profile.banner_url ? (
                        /* eslint-disable-next-line @next/next/no-img-element */
                        <img
                            src={userBannerUrl(profile.id, profile.banner_url)}
                            alt={`${profile.username}'s banner`}
                            className="w-full h-full object-cover"
                        />
                    ) : (
                        <div className="h-full bg-gradient-to-r from-primary/20 via-primary/10 to-primary/5" />
                    )}
                </div>

                <div className="max-w-3xl mx-auto">
                    {/* Avatar + name row */}
                    <div className="flex items-end gap-4 -mt-10 mb-2 px-2">
                        <Avatar className="h-20 w-20 border-4 border-background">
                            <AvatarImage
                                src={avatarUrl(profile.id, profile.avatar_url)}
                            />
                            <AvatarFallback className="text-2xl">
                                {profile.username[0]?.toUpperCase()}
                            </AvatarFallback>
                        </Avatar>
                        <div className="pb-1">
                            <h1 className="text-xl font-bold">
                                {profile.display_name || profile.username}
                            </h1>
                            <p className="text-sm text-muted-foreground">
                                u/{profile.username}
                            </p>
                        </div>
                    </div>

                    {/* Tab navigation — Reddit style */}
                    <div className="flex border-b border-border mb-4 px-2">
                        {tabs.map((tab) => (
                            <button
                                key={tab.key}
                                onClick={() => setActiveTab(tab.key)}
                                className={`flex items-center gap-1.5 px-4 py-2.5 text-sm font-medium border-b-2 transition-colors ${activeTab === tab.key
                                    ? "border-primary text-primary"
                                    : "border-transparent text-muted-foreground hover:text-foreground"
                                    }`}
                            >
                                {tab.icon}
                                {tab.label}
                            </button>
                        ))}
                    </div>

                    {/* ─── Overview Tab ─── */}
                    {activeTab === "overview" && (
                        <div className="space-y-3 px-2">
                            {profile.bio && (
                                <Card className="border-border p-4">
                                    <p className="text-sm">{profile.bio}</p>
                                </Card>
                            )}
                            <h2 className="text-sm font-semibold text-muted-foreground uppercase tracking-wider">
                                Recent Posts
                            </h2>
                            {notesLoading ? (
                                <div className="space-y-2">
                                    {Array.from({ length: 3 }).map((_, i) => (
                                        <Skeleton key={i} className="h-24 w-full" />
                                    ))}
                                </div>
                            ) : notes.length === 0 ? (
                                <Card className="border-border p-6 text-center">
                                    <p className="text-sm text-muted-foreground">
                                        No posts yet.
                                    </p>
                                </Card>
                            ) : (
                                notes.slice(0, 5).map((note) => (
                                    <NoteCard key={note.id} note={note} />
                                ))
                            )}
                        </div>
                    )}

                    {/* ─── Posts Tab ─── */}
                    {activeTab === "posts" && (
                        <div className="space-y-2 px-2">
                            {notesLoading ? (
                                <div className="space-y-2">
                                    {Array.from({ length: 5 }).map((_, i) => (
                                        <Skeleton key={i} className="h-24 w-full" />
                                    ))}
                                </div>
                            ) : notes.length === 0 ? (
                                <Card className="border-border p-6 text-center">
                                    <p className="text-sm text-muted-foreground">
                                        No posts yet.
                                    </p>
                                </Card>
                            ) : (
                                notes.map((note) => (
                                    <NoteCard key={note.id} note={note} />
                                ))
                            )}
                        </div>
                    )}

                    {/* ─── Comments Tab ─── */}
                    {activeTab === "comments" && (
                        <div className="space-y-2 px-2">
                            {commentsLoading ? (
                                <div className="space-y-2">
                                    {Array.from({ length: 5 }).map((_, i) => (
                                        <Skeleton key={i} className="h-16 w-full" />
                                    ))}
                                </div>
                            ) : (commentsData?.comments?.length ?? 0) === 0 ? (
                                <Card className="border-border p-6 text-center">
                                    <p className="text-sm text-muted-foreground">
                                        No comments yet.
                                    </p>
                                </Card>
                            ) : (
                                <>
                                    {commentsData!.comments.map((comment) => (
                                        <Card key={comment.id} className="border-border">
                                            <CardContent className="p-3">
                                                <Link
                                                    href={`/notes/${comment.note_id}`}
                                                    className="text-xs text-muted-foreground hover:text-primary"
                                                >
                                                    on {comment.note_title || `Note #${comment.note_id}`}
                                                </Link>
                                                <p className="text-sm mt-1">{comment.body}</p>
                                                <div className="flex items-center gap-3 mt-1.5 text-xs text-muted-foreground">
                                                    <span>▲ {comment.upvotes}</span>
                                                    <span>▼ {comment.downvotes}</span>
                                                    <span>{formatDate(comment.created_at)}</span>
                                                </div>
                                            </CardContent>
                                        </Card>
                                    ))}

                                    {commentsData && commentsData.total > commentsData.limit && (
                                        <div className="flex items-center justify-center gap-2 mt-4">
                                            <button
                                                className="text-sm text-muted-foreground hover:text-foreground disabled:opacity-50"
                                                disabled={commentsPage <= 1}
                                                onClick={() => setCommentsPage((p) => p - 1)}
                                            >
                                                ← Previous
                                            </button>
                                            <span className="text-xs text-muted-foreground">
                                                Page {commentsPage} of {Math.ceil(commentsData.total / commentsData.limit)}
                                            </span>
                                            <button
                                                className="text-sm text-muted-foreground hover:text-foreground disabled:opacity-50"
                                                disabled={commentsPage >= Math.ceil(commentsData.total / commentsData.limit)}
                                                onClick={() => setCommentsPage((p) => p + 1)}
                                            >
                                                Next →
                                            </button>
                                        </div>
                                    )}
                                </>
                            )}
                        </div>
                    )}
                </div>
            </main>

            {/* Right sidebar — profile info card */}
            <aside className="hidden lg:block w-72 shrink-0 border-l border-border">
                <div className="sticky top-12 h-[calc(100vh-48px)] overflow-y-auto p-4 space-y-4">
                    <Card className="border-border">
                        <CardHeader className="py-3 px-4">
                            <CardTitle className="text-sm font-semibold">
                                About {profile.display_name || profile.username}
                            </CardTitle>
                        </CardHeader>
                        <CardContent className="px-4 pb-4 pt-0 space-y-3">
                            {profile.bio && (
                                <>
                                    <p className="text-xs text-muted-foreground leading-relaxed">
                                        {profile.bio}
                                    </p>
                                    <Separator />
                                </>
                            )}
                            <div className="grid grid-cols-2 gap-2 text-xs">
                                <div>
                                    <p className="font-semibold text-foreground">Posts</p>
                                    <p className="text-muted-foreground">
                                        {notesData?.total ?? 0}
                                    </p>
                                </div>
                                <div className="flex items-start gap-1">
                                    <div>
                                        <p className="font-semibold text-foreground">Joined</p>
                                        <p className="text-muted-foreground">
                                            {formatDate(profile.created_at)}
                                        </p>
                                    </div>
                                </div>
                            </div>
                            <Separator />
                            <div className="grid grid-cols-2 gap-2 text-xs">
                                <div>
                                    <p className="font-semibold text-foreground">Post Notoriety</p>
                                    <p className="text-muted-foreground">{Math.round(profile.post_karma ?? 0)}</p>
                                </div>
                                <div>
                                    <p className="font-semibold text-foreground">Comment Notoriety</p>
                                    <p className="text-muted-foreground">{Math.round(profile.comment_karma ?? 0)}</p>
                                </div>
                            </div>
                        </CardContent>
                    </Card>
                </div>
            </aside>
        </div>
    );
}
