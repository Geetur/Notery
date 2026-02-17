// sort-tabs.tsx — Reddit-style sorting tabs: Hot / New / Top with time filter.
"use client";

import { Flame, Clock, TrendingUp, LayoutGrid, LayoutList } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";
import { useFeedStore } from "@/stores/feed-store";
import type { FeedSort, TimeFilter, ViewMode } from "@/types";

const SORT_OPTIONS: { value: FeedSort; label: string; icon: React.ReactNode }[] = [
  { value: "hot", label: "Hot", icon: <Flame className="h-4 w-4" /> },
  { value: "new", label: "New", icon: <Clock className="h-4 w-4" /> },
  { value: "top", label: "Top", icon: <TrendingUp className="h-4 w-4" /> },
];

const TIME_OPTIONS: { value: TimeFilter; label: string }[] = [
  { value: "day", label: "Today" },
  { value: "week", label: "This Week" },
  { value: "month", label: "This Month" },
  { value: "year", label: "This Year" },
  { value: "all", label: "All Time" },
];

export function SortTabs() {
  const { sort, timeFilter, viewMode, setSort, setTimeFilter, setViewMode } =
    useFeedStore();

  return (
    <div className="flex items-center gap-1 rounded-md border border-border bg-card p-1.5 mb-3">
      {/* Sort buttons */}
      <div className="flex items-center gap-0.5">
        {SORT_OPTIONS.map((opt) => (
          <Button
            key={opt.value}
            variant={sort === opt.value ? "secondary" : "ghost"}
            size="sm"
            className={cn(
              "h-8 gap-1.5 text-xs font-medium",
              sort === opt.value && "bg-secondary"
            )}
            onClick={() => setSort(opt.value)}
          >
            {opt.icon}
            {opt.label}
          </Button>
        ))}
      </div>

      {/* Time filter (only for Top sort) */}
      {sort === "top" && (
        <Select
          value={timeFilter}
          onValueChange={(v) => setTimeFilter(v as TimeFilter)}
        >
          <SelectTrigger className="h-8 w-[120px] text-xs">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {TIME_OPTIONS.map((opt) => (
              <SelectItem key={opt.value} value={opt.value} className="text-xs">
                {opt.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}

      {/* Spacer */}
      <div className="flex-1" />

      {/* View mode toggle */}
      <div className="flex items-center border border-border rounded-md">
        <Button
          variant={viewMode === "card" ? "secondary" : "ghost"}
          size="icon"
          className="h-7 w-7 rounded-r-none"
          onClick={() => setViewMode("card" as ViewMode)}
        >
          <LayoutGrid className="h-3.5 w-3.5" />
        </Button>
        <Button
          variant={viewMode === "compact" ? "secondary" : "ghost"}
          size="icon"
          className="h-7 w-7 rounded-l-none"
          onClick={() => setViewMode("compact" as ViewMode)}
        >
          <LayoutList className="h-3.5 w-3.5" />
        </Button>
      </div>
    </div>
  );
}
