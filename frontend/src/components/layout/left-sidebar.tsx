// left-sidebar.tsx — Reddit-style left navigation sidebar with collapse support.
// Shows navigation links, subnoteries, and quick actions.
"use client";

import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Separator } from "@/components/ui/separator";
import {
    Tooltip,
    TooltipContent,
    TooltipProvider,
    TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { useAuthStore } from "@/stores/auth-store";
import { useSidebarStore } from "@/stores/sidebar-store";
import {
    Bookmark,
    BookOpen,
    ChevronLeft,
    ChevronRight,
    Compass,
    FileText,
    Home,
    ShoppingCart,
    TrendingUp,
    User,
    Users,
} from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";

interface SidebarLinkProps {
    href: string;
    icon: React.ReactNode;
    label: string;
    active?: boolean;
    collapsed?: boolean;
}

function SidebarLink({ href, icon, label, active, collapsed }: SidebarLinkProps) {
    const link = (
        <Link
            href={href}
            className={cn(
                "flex items-center gap-3 px-4 py-2 text-sm rounded-md transition-colors",
                collapsed && "justify-center px-2",
                active
                    ? "bg-primary/10 text-primary font-medium"
                    : "text-muted-foreground hover:bg-accent hover:text-foreground"
            )}
        >
            {icon}
            {!collapsed && <span>{label}</span>}
        </Link>
    );

    if (collapsed) {
        return (
            <Tooltip delayDuration={0}>
                <TooltipTrigger asChild>{link}</TooltipTrigger>
                <TooltipContent side="right" sideOffset={8}>
                    {label}
                </TooltipContent>
            </Tooltip>
        );
    }

    return link;
}

interface LeftSidebarProps {
    mobile?: boolean;
}

export function LeftSidebar({ mobile }: LeftSidebarProps) {
    const pathname = usePathname();
    const { isAuthenticated } = useAuthStore();
    const { collapsed, toggle } = useSidebarStore();

    // Mobile sidebar is never collapsed
    const isCollapsed = mobile ? false : collapsed;
    const iconSize = "h-4 w-4";

    return (
        <TooltipProvider>
            <ScrollArea className={cn("h-full", mobile ? "pt-12" : "")}>
                <div className="flex flex-col gap-0.5 py-2">
                    {/* Collapse toggle (desktop only) */}
                    {!mobile && (
                        <div className={cn("flex px-2 mb-1", isCollapsed ? "justify-center" : "justify-end")}>
                            <Button
                                variant="ghost"
                                size="icon"
                                className="h-7 w-7"
                                onClick={toggle}
                            >
                                {isCollapsed ? (
                                    <ChevronRight className="h-4 w-4" />
                                ) : (
                                    <ChevronLeft className="h-4 w-4" />
                                )}
                            </Button>
                        </div>
                    )}

                    {/* Main navigation */}
                    <div className="px-2">
                        <SidebarLink
                            href="/"
                            icon={<Home className={iconSize} />}
                            label="Home"
                            active={pathname === "/"}
                            collapsed={isCollapsed}
                        />
                        <SidebarLink
                            href="/hot"
                            icon={<TrendingUp className={iconSize} />}
                            label="Hot"
                            active={pathname === "/hot"}
                            collapsed={isCollapsed}
                        />
                        <SidebarLink
                            href="/new"
                            icon={<Compass className={iconSize} />}
                            label="New"
                            active={pathname === "/new"}
                            collapsed={isCollapsed}
                        />
                        <SidebarLink
                            href="/communities"
                            icon={<Users className={iconSize} />}
                            label="Communities"
                            active={pathname.startsWith("/communities")}
                            collapsed={isCollapsed}
                        />
                    </div>

                    <Separator className="my-2" />

                    {/* Logged-in links */}
                    {isAuthenticated && (
                        <>
                            {!isCollapsed && (
                                <div className="px-4 py-1">
                                    <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
                                        My Stuff
                                    </p>
                                </div>
                            )}
                            <div className="px-2">
                                <SidebarLink
                                    href="/purchases"
                                    icon={<BookOpen className={iconSize} />}
                                    label="My Notes"
                                    active={pathname === "/purchases"}
                                    collapsed={isCollapsed}
                                />
                                <SidebarLink
                                    href="/bookmarks"
                                    icon={<Bookmark className={iconSize} />}
                                    label="Bookmarks"
                                    active={pathname === "/bookmarks"}
                                    collapsed={isCollapsed}
                                />
                                <SidebarLink
                                    href="/cart"
                                    icon={<ShoppingCart className={iconSize} />}
                                    label="Cart"
                                    active={pathname === "/cart"}
                                    collapsed={isCollapsed}
                                />
                                <SidebarLink
                                    href="/submit"
                                    icon={<FileText className={iconSize} />}
                                    label="Create Note"
                                    active={pathname === "/submit"}
                                    collapsed={isCollapsed}
                                />
                                <SidebarLink
                                    href="/profile"
                                    icon={<User className={iconSize} />}
                                    label="Profile"
                                    active={pathname === "/profile"}
                                    collapsed={isCollapsed}
                                />
                            </div>
                            <Separator className="my-2" />
                        </>
                    )}

                    {/* About section (hidden when collapsed) */}
                    {!isCollapsed && (
                        <div className="px-4 py-2">
                            <p className="text-xs text-muted-foreground leading-relaxed">
                                Notery is a marketplace for notes. Buy and sell study notes, lecture
                                summaries, and educational content.
                            </p>
                        </div>
                    )}
                </div>
            </ScrollArea>
        </TooltipProvider>
    );
}
