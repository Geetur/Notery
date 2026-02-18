// page.tsx — Home page: Reddit-style feed with main feed + right sidebar.
"use client";

import { NoteFeed } from "@/components/feed/note-feed";
import { RightSidebar } from "@/components/layout/right-sidebar";

export default function HomePage() {
  return (
    <div className="flex">
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
