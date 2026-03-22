// page.tsx — Communities (subnoteries) listing page.
// Shows all communities with member/admin counts and links to detail pages.
"use client";

import { RightSidebar } from "@/components/layout/right-sidebar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { API_BASE_URL } from "@/lib/config";
import { timeAgo } from "@/lib/format";
import { listSubnoteries } from "@/services/subnoteries";
import type { SubnoteryListItem } from "@/types";
import { ChevronLeft, ChevronRight, Users } from "lucide-react";
import Link from "next/link";
import { useEffect, useState } from "react";

const PAGE_SIZE = 20;

export default function CommunitiesPage() {
    const [subnoteries, setSubnoteries] = useState<SubnoteryListItem[]>([]);
    const [total, setTotal] = useState(0);
    const [page, setPage] = useState(1);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        setLoading(true);
        setError(null);
        listSubnoteries({ page, limit: PAGE_SIZE })
            .then((res) => {
                setSubnoteries(res.subnoteries ?? []);
                setTotal(res.total);
            })
            .catch((err) => setError(err.message || "Failed to load communities"))
            .finally(() => setLoading(false));
    }, [page]);

    const totalPages = Math.ceil(total / PAGE_SIZE);

    return (
        <div className="flex">
            <main className="flex-1 min-w-0 px-6 py-4">
                <div className="max-w-3xl mx-auto">
                    <h1 className="text-2xl font-bold mb-4">Communities</h1>
                    <p className="text-muted-foreground mb-6">
                        Browse all communities. Communities are auto-created when a note is
                        submitted to a new community name.
                    </p>

                    {loading && (
                        <div className="space-y-3">
                            {Array.from({ length: 5 }).map((_, i) => (
                                <Skeleton key={i} className="h-20 w-full rounded-lg" />
                            ))}
                        </div>
                    )}

                    {error && (
                        <Card className="p-6 text-center text-destructive">{error}</Card>
                    )}

                    {!loading && !error && subnoteries.length === 0 && (
                        <Card className="p-6 text-center text-muted-foreground">
                            No communities yet. Create one by submitting a note!
                        </Card>
                    )}

                    {!loading && !error && subnoteries.length > 0 && (
                        <div className="space-y-2">
                            {subnoteries.map((sub) => (
                                <Link key={sub.id} href={`/communities/${sub.id}`}>
                                    <Card className="overflow-hidden hover:border-primary/30 transition-colors cursor-pointer">
                                        {sub.banner_url && (
                                            <div className="h-20 w-full bg-muted overflow-hidden">
                                                <img
                                                    src={`${API_BASE_URL}/api/v1/subnoteries/${sub.id}/banner`}
                                                    alt={`Banner for n/${sub.name}`}
                                                    className="w-full h-full object-cover"
                                                />
                                            </div>
                                        )}
                                        <div className="p-4 flex items-center justify-between">
                                            <div className="flex-1 min-w-0">
                                                <h3 className="font-semibold text-lg">
                                                    n/{sub.name}
                                                </h3>
                                                <div className="flex items-center gap-4 mt-1 text-sm text-muted-foreground">
                                                    <span className="flex items-center gap-1">
                                                        <Users className="h-3.5 w-3.5" />
                                                        {sub.member_count}{" "}
                                                        {sub.member_count === 1
                                                            ? "member"
                                                            : "members"}
                                                    </span>
                                                    <span>
                                                        {sub.admin_count}{" "}
                                                        {sub.admin_count === 1
                                                            ? "admin"
                                                            : "admins"}
                                                    </span>
                                                    <span>
                                                        Created {timeAgo(sub.created_at)}
                                                    </span>
                                                </div>
                                            </div>
                                            <Badge variant="secondary" className="shrink-0">
                                                View
                                            </Badge>
                                        </div>
                                    </Card>
                                </Link>
                            ))}
                        </div>
                    )}

                    {/* Pagination */}
                    {totalPages > 1 && (
                        <div className="flex items-center justify-center gap-4 mt-6">
                            <Button
                                variant="outline"
                                size="sm"
                                disabled={page <= 1}
                                onClick={() => setPage((p) => Math.max(1, p - 1))}
                            >
                                <ChevronLeft className="h-4 w-4 mr-1" />
                                Previous
                            </Button>
                            <span className="text-sm text-muted-foreground">
                                Page {page} of {totalPages}
                            </span>
                            <Button
                                variant="outline"
                                size="sm"
                                disabled={page >= totalPages}
                                onClick={() => setPage((p) => p + 1)}
                            >
                                Next
                                <ChevronRight className="h-4 w-4 ml-1" />
                            </Button>
                        </div>
                    )}
                </div>
            </main>
            <RightSidebar />
        </div>
    );
}
