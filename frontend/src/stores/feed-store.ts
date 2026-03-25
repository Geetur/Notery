// feed-store.ts — Zustand store for feed view preferences (persisted in localStorage).
import type { FeedSort, TimeFilter, ViewMode } from "@/types";
import { create } from "zustand";

interface FeedState {
    sort: FeedSort;
    timeFilter: TimeFilter;
    viewMode: ViewMode;
    setSort: (sort: FeedSort) => void;
    setTimeFilter: (filter: TimeFilter) => void;
    setViewMode: (mode: ViewMode) => void;
}

export const useFeedStore = create<FeedState>((set) => ({
    sort: "hot",
    timeFilter: "all",
    viewMode: "card",
    setSort: (sort) => set({ sort }),
    setTimeFilter: (filter) => set({ timeFilter: filter }),
    setViewMode: (mode) => set({ viewMode: mode }),
}));
