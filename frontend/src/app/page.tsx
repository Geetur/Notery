// page.tsx — Home page: Reddit-style feed with left nav + main feed + right sidebar.
"use client";

import { NoteFeed } from "@/components/feed/note-feed";
import { LeftSidebar } from "@/components/layout/left-sidebar";
import { RightSidebar } from "@/components/layout/right-sidebar";

export default function HomePage() {
  return (
    <div className="max-w-[1200px] mx-auto flex gap-4 px-2 md:px-4 py-4">
      {/* Left sidebar — hidden on mobile */}
      <aside className="hidden md:block w-56 shrink-0">
        <div className="sticky top-14">
          <LeftSidebar />
        </div>
      </aside>

      {/* Main feed */}
      <main className="flex-1 min-w-0">
        <NoteFeed />
      </main>

      {/* Right sidebar — hidden on mobile & tablet */}
      <RightSidebar />
    </div>
  );
}
