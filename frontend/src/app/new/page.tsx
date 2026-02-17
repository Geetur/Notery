// page.tsx — New (latest) feed alias page.
"use client";

import { LeftSidebar } from "@/components/layout/left-sidebar";
import { RightSidebar } from "@/components/layout/right-sidebar";
import { NoteFeed } from "@/components/feed/note-feed";

export default function NewPage() {
  return (
    <div className="flex max-w-[1200px] mx-auto px-4 py-4 gap-4">
      <aside className="hidden lg:block w-56 shrink-0">
        <LeftSidebar />
      </aside>
      <main className="flex-1 min-w-0">
        <NoteFeed initialSort="new" />
      </main>
      <aside className="hidden xl:block w-80 shrink-0">
        <RightSidebar />
      </aside>
    </div>
  );
}
