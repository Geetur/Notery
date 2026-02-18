// page.tsx — Hot feed alias page.
"use client";

import { NoteFeed } from "@/components/feed/note-feed";
import { LeftSidebar } from "@/components/layout/left-sidebar";
import { RightSidebar } from "@/components/layout/right-sidebar";
import { cn } from "@/lib/utils";
import { useSidebarStore } from "@/stores/sidebar-store";

export default function HotPage() {
    const { collapsed } = useSidebarStore();
    return (
        <div className="flex px-0 py-0">
            <aside
                className={cn(
                    "hidden md:block shrink-0 border-r border-border transition-all duration-200",
                    collapsed ? "w-14" : "w-56"
                )}
            >
                <div className="sticky top-12 h-[calc(100vh-48px)] overflow-y-auto">
                    <LeftSidebar />
                </div>
            </aside>
            <main className="flex-1 min-w-0 px-4 py-4">
                <div className="max-w-3xl mx-auto">
                    <NoteFeed initialSort="hot" />
                </div>
            </main>
            <RightSidebar />
        </div>
    );
}
