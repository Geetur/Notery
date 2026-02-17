// page.tsx — User profile page (public view).
"use client";

import { useQuery } from "@tanstack/react-query";
import { useParams } from "next/navigation";
import { Calendar } from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Skeleton } from "@/components/ui/skeleton";
import { Separator } from "@/components/ui/separator";
import { getUserProfile } from "@/services/profile";
import { formatDate } from "@/lib/format";

export default function UserProfilePage() {
  const params = useParams();
  const userId = Number(params.id);

  const { data: profile, isLoading, isError } = useQuery({
    queryKey: ["userProfile", userId],
    queryFn: () => getUserProfile(userId),
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

  return (
    <div className="max-w-2xl mx-auto px-4 py-4">
      <Card className="border-border">
        {/* Banner */}
        <div className="h-24 bg-gradient-to-r from-primary/20 to-primary/5 rounded-t-lg" />

        <CardContent className="px-4 pb-4">
          {/* Avatar + name */}
          <div className="flex items-end gap-4 -mt-10 mb-4">
            <Avatar className="h-20 w-20 border-4 border-card">
              <AvatarImage src={profile.avatar_url} />
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
            <p className="text-sm text-foreground mb-4">{profile.bio}</p>
          )}

          <Separator className="mb-4" />

          {/* Stats */}
          <div className="grid grid-cols-2 gap-4 text-sm">
            <div className="flex items-center gap-2 text-muted-foreground">
              <Calendar className="h-4 w-4" />
              <span>Joined {formatDate(profile.created_at)}</span>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
