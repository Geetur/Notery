// page.tsx — Public user profile page: Reddit-style with Overview and Posts tabs.
// Shows profile banner, avatar, username, bio, joined date, and posted notes.
// No edit controls — those are only on the own-profile page (/profile).
"use client";

import { NoteCard } from "@/components/feed/note-card";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { avatarUrl, formatDate } from "@/lib/format";
import { getUserNotes } from "@/services/notes";
import { getUserProfile } from "@/services/profile";
import { useQuery } from "@tanstack/react-query";
import { FileText } from "lucide-react";
import { useParams } from "next/navigation";
import { useState } from "react";

type UserTab = "overview" | "posts";

export default function UserProfilePage() {
    const params = useParams();
    const userId = Number(params.id);
    const [activeTab, setActiveTab] = useState<UserTab>("overview");

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
    ];

    return (
        <div className="flex">
            <main className="flex-1 min-w-0 px-4 py-0">
                {/* Profile banner */}
                <div className="h-28 bg-gradient-to-r from-primary/20 via-primary/10 to-primary/5 -mx-4" />

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
