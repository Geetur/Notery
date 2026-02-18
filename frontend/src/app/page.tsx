// page.tsx — Home page: Reddit-style feed with left nav + main feed + right sidebar.
"use client";

import { NoteFeed } from "@/components/feed/note-feed";
import { LeftSidebar } from "@/components/layout/left-sidebar";
import { RightSidebar } from "@/components/layout/right-sidebar";
import { cn } from "@/lib/utils";
import { useSidebarStore } from "@/stores/sidebar-store";

export default function HomePage() {
  const { collapsed } = useSidebarStore();
  return (
    <div className="flex px-0 py-0">
      {/* Left sidebar — flush to edge, no card border */}
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

      {/* Main feed — takes all remaining space */}
      <main className="flex-1 min-w-0 px-4 py-4">
        <div className="max-w-3xl mx-auto">
          <NoteFeed />
        </div>
      </main>

      {/* Right sidebar — only bordered section */}
      <RightSidebar />
    </div>
  );
}
