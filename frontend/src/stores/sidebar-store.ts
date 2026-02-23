// sidebar-store.ts — Zustand store for sidebar collapsed state.
import { create } from "zustand";

interface SidebarState {
    collapsed: boolean;
    toggle: () => void;
}

export const useSidebarStore = create<SidebarState>((set) => ({
    collapsed: false,
    toggle: () => set((s) => ({ collapsed: !s.collapsed })),
}));
