// vote-buttons.tsx — Reddit-style vertical vote buttons for notes.
"use client";

import { useState } from "react";
import { ArrowBigUp, ArrowBigDown } from "lucide-react";
import { cn } from "@/lib/utils";
import { formatVotes, netScore } from "@/lib/format";
import { useAuthStore } from "@/stores/auth-store";
import { upvoteNote, downvoteNote } from "@/services/notes";
import { useToast } from "@/hooks/use-toast";

interface VoteButtonsProps {
  noteId: number;
  upvotes: number;
  downvotes: number;
  /** Direction of user orientation ("vertical" for feed cards, "horizontal" for compact). */
  orientation?: "vertical" | "horizontal";
  className?: string;
}

export function VoteButtons({
  noteId,
  upvotes: initialUpvotes,
  downvotes: initialDownvotes,
  orientation = "vertical",
  className,
}: VoteButtonsProps) {
  const { isAuthenticated } = useAuthStore();
  const { toast } = useToast();
  const [upvotes, setUpvotes] = useState(initialUpvotes);
  const [downvotes, setDownvotes] = useState(initialDownvotes);
  const [userVote, setUserVote] = useState<"up" | "down" | null>(null);
  const [loading, setLoading] = useState(false);

  const score = netScore(upvotes, downvotes);

  const handleVote = async (direction: "up" | "down") => {
    if (!isAuthenticated) {
      toast({
        title: "Login required",
        description: "You need to log in to vote.",
        variant: "destructive",
      });
      return;
    }

    if (loading) return;
    setLoading(true);

    try {
      const fn = direction === "up" ? upvoteNote : downvoteNote;
      const res = await fn(noteId);
      setUpvotes(res.upvotes);
      setDownvotes(res.downvotes);
      setUserVote((prev) => (prev === direction ? null : direction));
    } catch {
      toast({
        title: "Vote failed",
        description: "Could not register your vote. Try again.",
        variant: "destructive",
      });
    } finally {
      setLoading(false);
    }
  };

  const isVertical = orientation === "vertical";

  return (
    <div
      className={cn(
        "flex items-center gap-0.5",
        isVertical ? "flex-col" : "flex-row",
        className
      )}
    >
      <button
        onClick={() => handleVote("up")}
        disabled={loading}
        className={cn(
          "p-1 rounded hover:bg-accent transition-colors",
          userVote === "up" ? "text-orange-500" : "text-muted-foreground hover:text-foreground"
        )}
        aria-label="Upvote"
      >
        <ArrowBigUp
          className={cn("h-5 w-5", userVote === "up" && "fill-current")}
        />
      </button>

      <span
        className={cn(
          "text-xs font-bold min-w-[2ch] text-center",
          userVote === "up" && "text-orange-500",
          userVote === "down" && "text-blue-500",
          !userVote && "text-foreground"
        )}
      >
        {formatVotes(score)}
      </span>

      <button
        onClick={() => handleVote("down")}
        disabled={loading}
        className={cn(
          "p-1 rounded hover:bg-accent transition-colors",
          userVote === "down" ? "text-blue-500" : "text-muted-foreground hover:text-foreground"
        )}
        aria-label="Downvote"
      >
        <ArrowBigDown
          className={cn("h-5 w-5", userVote === "down" && "fill-current")}
        />
      </button>
    </div>
  );
}
