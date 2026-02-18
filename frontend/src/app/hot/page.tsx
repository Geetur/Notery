// page.tsx — Hot feed alias page.
"use client";

import { NoteFeed } from "@/components/feed/note-feed";
import { RightSidebar } from "@/components/layout/right-sidebar";

export default function HotPage() {
    return (
        <div className="flex">
            <main className="flex-1 min-w-0 px-4 py-4">
                <div className="max-w-3xl mx-auto">
                    <NoteFeed initialSort="hot" />
                </div>
            </main>
            <RightSidebar />
        </div>
    );
}
