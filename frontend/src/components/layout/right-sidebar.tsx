// right-sidebar.tsx — Reddit-style right sidebar.
// Shows default Notery info on homepage, or subnotery-specific info on community pages.
"use client";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
import { useAuthStore } from "@/stores/auth-store";
import Link from "next/link";
import type { ReactNode } from "react";

export interface SubnoterySidebarData {
    name: string;
    description?: string;
    content_type?: string;
    rules?: string;
    member_count: number;
    admins?: { id: number; username: string; admin_since?: string }[];
    created_at: string;
}

interface RightSidebarProps {
    children?: ReactNode;
    /** If provided, shows subnotery-specific info instead of the default. */
    subnotery?: SubnoterySidebarData;
}

export function RightSidebar({ children, subnotery }: RightSidebarProps) {
    const { isAuthenticated } = useAuthStore();

    return (
        <aside className="hidden lg:block w-72 shrink-0 border-l border-border">
            <div className="sticky top-12 h-[calc(100vh-48px)] overflow-y-auto p-4 flex flex-col gap-4">
                {children}

                {subnotery ? (
                    <>
                        {/* Subnotery about card */}
                        <Card className="border-border">
                            <CardHeader className="py-3 px-4">
                                <CardTitle className="text-sm font-semibold">
                                    About n/{subnotery.name}
                                </CardTitle>
                            </CardHeader>
                            <CardContent className="px-4 pb-4 pt-0">
                                <p className="text-xs text-muted-foreground leading-relaxed mb-3">
                                    {subnotery.description || "No description yet."}
                                </p>
                                <Separator className="mb-3" />
                                <div className="grid grid-cols-2 gap-2 text-xs mb-3">
                                    <div>
                                        <p className="font-semibold text-foreground">Members</p>
                                        <p className="text-muted-foreground">
                                            {subnotery.member_count}
                                        </p>
                                    </div>
                                    <div>
                                        <p className="font-semibold text-foreground">Content</p>
                                        <p className="text-muted-foreground">
                                            {subnotery.content_type || "PDF Notes"}
                                        </p>
                                    </div>
                                </div>
                                {!isAuthenticated && (
                                    <Button asChild className="w-full" size="sm">
                                        <Link href="/signup">Join Notery</Link>
                                    </Button>
                                )}
                            </CardContent>
                        </Card>

                        {/* Subnotery rules card */}
                        <Card className="border-border">
                            <CardHeader className="py-3 px-4">
                                <CardTitle className="text-sm font-semibold">
                                    Community Rules
                                </CardTitle>
                            </CardHeader>
                            <CardContent className="px-4 pb-4 pt-0">
                                {subnotery.rules ? (
                                    <div className="text-xs text-muted-foreground whitespace-pre-wrap leading-relaxed">
                                        {subnotery.rules}
                                    </div>
                                ) : (
                                    <p className="text-xs text-muted-foreground italic">
                                        No community rules set yet.
                                    </p>
                                )}
                            </CardContent>
                        </Card>

                        {/* Admins card */}
                        {subnotery.admins && subnotery.admins.length > 0 && (
                            <Card className="border-border">
                                <CardHeader className="py-3 px-4">
                                    <CardTitle className="text-sm font-semibold">
                                        Admins
                                    </CardTitle>
                                </CardHeader>
                                <CardContent className="px-4 pb-4 pt-0">
                                    <ul className="space-y-1.5">
                                        {subnotery.admins.map((admin) => (
                                            <li key={admin.id}>
                                                <Link
                                                    href={`/user/${admin.id}`}
                                                    className="text-xs text-foreground hover:text-primary transition-colors"
                                                >
                                                    u/{admin.username}
                                                </Link>
                                            </li>
                                        ))}
                                    </ul>
                                </CardContent>
                            </Card>
                        )}
                    </>
                ) : (
                    <>
                        {/* Default About card */}
                        <Card className="border-border">
                            <CardHeader className="py-3 px-4">
                                <CardTitle className="text-sm font-semibold">
                                    About Notery
                                </CardTitle>
                            </CardHeader>
                            <CardContent className="px-4 pb-4 pt-0">
                                <p className="text-xs text-muted-foreground leading-relaxed mb-3">
                                    A marketplace for study notes. Browse, buy, and sell
                                    high-quality notes, lecture summaries, and educational resources.
                                </p>
                                <Separator className="mb-3" />
                                <div className="grid grid-cols-2 gap-2 text-xs mb-3">
                                    <div>
                                        <p className="font-semibold text-foreground">Founded</p>
                                        <p className="text-muted-foreground">2026</p>
                                    </div>
                                    <div>
                                        <p className="font-semibold text-foreground">Content</p>
                                        <p className="text-muted-foreground">PDF Notes</p>
                                    </div>
                                </div>
                                {!isAuthenticated && (
                                    <Button asChild className="w-full" size="sm">
                                        <Link href="/signup">Join Notery</Link>
                                    </Button>
                                )}
                            </CardContent>
                        </Card>

                        {/* Default Rules card */}
                        <Card className="border-border">
                            <CardHeader className="py-3 px-4">
                                <CardTitle className="text-sm font-semibold">
                                    Marketplace Rules
                                </CardTitle>
                            </CardHeader>
                            <CardContent className="px-4 pb-4 pt-0">
                                <ol className="text-xs text-muted-foreground space-y-1.5 list-decimal list-inside">
                                    <li>Only original content allowed</li>
                                    <li>Notes must be in PDF format</li>
                                    <li>Prices are set in USD (cents)</li>
                                    <li>All notes reviewed before listing</li>
                                    <li>No plagiarised or copyrighted material</li>
                                </ol>
                            </CardContent>
                        </Card>
                    </>
                )}

                {/* Footer links */}
                <div className="px-4 flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">
                    <Link href="/about" className="hover:underline">About</Link>
                    <Link href="/help" className="hover:underline">Help</Link>
                    <span>© 2026 Notery</span>
                </div>
            </div>
        </aside>
    );
}
