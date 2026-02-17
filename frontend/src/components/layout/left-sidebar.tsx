// left-sidebar.tsx — Reddit-style left navigation sidebar.
// Shows navigation links, subnoteries, and quick actions.
"use client";

import { ScrollArea } from "@/components/ui/scroll-area";
import { Separator } from "@/components/ui/separator";
import { cn } from "@/lib/utils";
import { useAuthStore } from "@/stores/auth-store";
import {
    BookOpen,
    Compass,
    FileText,
    Home,
    Search,
    ShoppingCart,
    TrendingUp,
    User,
} from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";

interface SidebarLinkProps {
    href: string;
    icon: React.ReactNode;
    label: string;
    active?: boolean;
}

function SidebarLink({ href, icon, label, active }: SidebarLinkProps) {
    return (
        <Link
            href={href}
            className={cn(
                "flex items-center gap-3 px-4 py-2 text-sm rounded-md transition-colors",
                active
                    ? "bg-primary/10 text-primary font-medium"
                    : "text-muted-foreground hover:bg-accent hover:text-foreground"
            )}
        >
            {icon}
            <span>{label}</span>
        </Link>
    );
}

interface LeftSidebarProps {
    mobile?: boolean;
}

export function LeftSidebar({ mobile }: LeftSidebarProps) {
    const pathname = usePathname();
    const { isAuthenticated } = useAuthStore();

    const iconSize = "h-4 w-4";

    return (
        <ScrollArea className={cn("h-full", mobile ? "pt-12" : "")}>
            <div className="flex flex-col gap-0.5 py-2">
                {/* Main navigation */}
                <div className="px-2">
                    <SidebarLink
                        href="/"
                        icon={<Home className={iconSize} />}
                        label="Home"
                        active={pathname === "/"}
                    />
                    <SidebarLink
                        href="/hot"
                        icon={<TrendingUp className={iconSize} />}
                        label="Hot"
                        active={pathname === "/hot"}
                    />
                    <SidebarLink
                        href="/new"
                        icon={<Compass className={iconSize} />}
                        label="New"
                        active={pathname === "/new"}
                    />
                    <SidebarLink
                        href="/search"
                        icon={<Search className={iconSize} />}
                        label="Explore"
                        active={pathname === "/search"}
                    />
                </div>

                <Separator className="my-2" />

                {/* Logged-in links */}
                {isAuthenticated && (
                    <>
                        <div className="px-4 py-1">
                            <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
                                My Stuff
                            </p>
                        </div>
                        <div className="px-2">
                            <SidebarLink
                                href="/purchases"
                                icon={<BookOpen className={iconSize} />}
                                label="My Purchases"
                                active={pathname === "/purchases"}
                            />
                            <SidebarLink
                                href="/cart"
                                icon={<ShoppingCart className={iconSize} />}
                                label="Cart"
                                active={pathname === "/cart"}
                            />
                            <SidebarLink
                                href="/submit"
                                icon={<FileText className={iconSize} />}
                                label="Create Note"
                                active={pathname === "/submit"}
                            />
                            <SidebarLink
                                href="/profile"
                                icon={<User className={iconSize} />}
                                label="Profile"
                                active={pathname === "/profile"}
                            />
                        </div>
                        <Separator className="my-2" />
                    </>
                )}

                {/* About section */}
                <div className="px-4 py-2">
                    <p className="text-xs text-muted-foreground leading-relaxed">
                        Notery is a marketplace for notes. Buy and sell study notes, lecture
                        summaries, and educational content.
                    </p>
                </div>
            </div>
        </ScrollArea>
    );
}
