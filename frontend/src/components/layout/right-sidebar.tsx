// right-sidebar.tsx — Reddit-style right sidebar.
// Shows trending notes, community info, and marketplace stats.
"use client";

import type { ReactNode } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { useAuthStore } from "@/stores/auth-store";
import Link from "next/link";

interface RightSidebarProps {
  children?: ReactNode;
}

export function RightSidebar({ children }: RightSidebarProps) {
  const { isAuthenticated } = useAuthStore();

  return (
    <aside className="hidden lg:block w-80 shrink-0">
      <div className="sticky top-14 flex flex-col gap-4">
        {children}

        {/* About card */}
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

        {/* Rules card */}
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
