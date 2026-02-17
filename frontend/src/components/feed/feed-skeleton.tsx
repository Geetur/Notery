// feed-skeleton.tsx — Skeleton loading state for the note feed.
"use client";

import { Skeleton } from "@/components/ui/skeleton";
import { Card } from "@/components/ui/card";

function NoteCardSkeleton() {
  return (
    <Card className="border-border">
      <div className="flex p-3 gap-3">
        {/* Vote column */}
        <div className="flex flex-col items-center gap-1">
          <Skeleton className="h-5 w-5 rounded" />
          <Skeleton className="h-4 w-6" />
          <Skeleton className="h-5 w-5 rounded" />
        </div>

        {/* Content */}
        <div className="flex-1 space-y-2">
          <Skeleton className="h-3 w-48" />
          <Skeleton className="h-5 w-full max-w-md" />
          <div className="flex gap-2 mt-2">
            <Skeleton className="h-5 w-14 rounded-full" />
            <Skeleton className="h-5 w-10 rounded-full" />
          </div>
          <div className="flex gap-4 mt-2">
            <Skeleton className="h-4 w-24" />
          </div>
        </div>
      </div>
    </Card>
  );
}

interface FeedSkeletonProps {
  count?: number;
}

export function FeedSkeleton({ count = 5 }: FeedSkeletonProps) {
  return (
    <div className="space-y-2">
      {Array.from({ length: count }).map((_, i) => (
        <NoteCardSkeleton key={i} />
      ))}
    </div>
  );
}
