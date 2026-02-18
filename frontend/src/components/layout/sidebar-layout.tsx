// sidebar-layout.tsx — Global layout wrapper that renders the left sidebar on all pages.
// Auth pages (login, signup, forgot-password, callback) exclude the sidebar.
// The sidebar is always collapsible and consistent across every page.
"use client";

import { LeftSidebar } from "@/components/layout/left-sidebar";
import { cn } from "@/lib/utils";
import { useSidebarStore } from "@/stores/sidebar-store";
import { usePathname } from "next/navigation";

/** Routes where the sidebar should be hidden (auth flows). */
const NO_SIDEBAR_ROUTES = ["/login", "/signup", "/forgot-password", "/auth"];

export function SidebarLayout({ children }: { children: React.ReactNode }) {
    const pathname = usePathname();
    const { collapsed } = useSidebarStore();

    const hideSidebar = NO_SIDEBAR_ROUTES.some(
        (r) => pathname === r || pathname.startsWith(r + "/")
    );

    if (hideSidebar) {
        return <>{children}</>;
    }

    return (
        <div className="flex">
            {/* Left sidebar — always present on desktop, flush to left edge */}
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

            {/* Page content — takes remaining space */}
            <div className="flex-1 min-w-0">{children}</div>
        </div>
    );
}
