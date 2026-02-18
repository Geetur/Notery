// page.tsx — User profile page (public view) with posted notes.
"use client";

import { NoteCard } from "@/components/feed/note-card";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Card, CardContent } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { avatarUrl, formatDate } from "@/lib/format";
import { getUserNotes } from "@/services/notes";
import { getUserProfile } from "@/services/profile";
import { useQuery } from "@tanstack/react-query";
import { Calendar, FileText } from "lucide-react";
import Link from "next/link";
import { useParams } from "next/navigation";

export default function UserProfilePage() {
    const params = useParams();
    const userId = Number(params.id);

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
            <div className="max-w-2xl mx-auto px-4 py-4">
                <div className="flex items-center gap-4 mb-4">
                    <Skeleton className="h-20 w-20 rounded-full" />
                    <div className="space-y-2">
                        <Skeleton className="h-6 w-40" />
                        <Skeleton className="h-4 w-24" />
                    </div>
                </div>
                <Skeleton className="h-32 w-full" />
            </div>
        );
    }

    if (isError || !profile) {
        return (
            <div className="max-w-2xl mx-auto px-4 py-8 text-center">
                <p className="text-destructive">User not found.</p>
            </div>
        );
    }

    const notes = notesData?.notes ?? [];

    return (
        <div className="max-w-2xl mx-auto px-4 py-4 space-y-4">
            {/* Profile card */}
            <Card className="border-border">
                {/* Banner */}
                <div className="h-24 bg-gradient-to-r from-primary/20 to-primary/5 rounded-t-lg" />

                <CardContent className="px-4 pb-4">
                    {/* Avatar + name */}
                    <div className="flex items-end gap-4 -mt-10 mb-4">
                        <Avatar className="h-20 w-20 border-4 border-card">
                            <AvatarImage
                                src={avatarUrl(profile.id, profile.avatar_url)}
                            />
                            <AvatarFallback className="text-xl">
                                {profile.username[0]?.toUpperCase()}
                            </AvatarFallback>
                        </Avatar>
                        <div>
                            <h1 className="text-xl font-bold text-foreground">
                                {profile.display_name || profile.username}
                            </h1>
                            <p className="text-sm text-muted-foreground">
                                u/{profile.username}
                            </p>
                        </div>
                    </div>

                    {/* Bio */}
                    {profile.bio && (
                        <p className="text-sm text-foreground mb-4">
                            {profile.bio}
                        </p>
                    )}

                    <Separator className="mb-4" />

                    {/* Stats */}
                    <div className="grid grid-cols-2 gap-4 text-sm">
                        <div className="flex items-center gap-2 text-muted-foreground">
                            <Calendar className="h-4 w-4" />
                            <span>Joined {formatDate(profile.created_at)}</span>
                        </div>
                        <div className="flex items-center gap-2 text-muted-foreground">
                            <FileText className="h-4 w-4" />
                            <span>
                                {notesData?.total ?? 0} note
                                {(notesData?.total ?? 0) !== 1 ? "s" : ""}
                            </span>
                        </div>
                    </div>
                </CardContent>
            </Card>

            {/* Posted notes */}
            <div>
                <h2 className="text-lg font-semibold text-foreground mb-3">
                    Posts
                </h2>
                {notesLoading ? (
                    <div className="space-y-3">
                        <Skeleton className="h-24 w-full" />
                        <Skeleton className="h-24 w-full" />
                    </div>
                ) : notes.length === 0 ? (
                    <Card className="border-border p-6 text-center">
                        <p className="text-sm text-muted-foreground">
                            No posts yet.
                        </p>
                    </Card>
                ) : (
                    <div className="space-y-2">
                        {notes.map((note) => (
                            <div key={note.id}>
                                {/* Subnotery label at top like Reddit */}
                                <div className="text-xs text-muted-foreground mb-0.5 pl-2">
                                    <Link
                                        href={`/communities/${note.subnotery_id}`}
                                        className="font-semibold text-foreground hover:underline"
                                    >
                                        n/
                                        {note.subnotery_name ||
                                            note.subnotery_id}
                                    </Link>
                                    <span className="mx-1">&bull;</span>
                                    Posted by u/{profile.username}
                                </div>
                                <NoteCard note={note} viewMode="card" />
                            </div>
                        ))}
                    </div>
                )}
            </div>
        </div>
    );
}
