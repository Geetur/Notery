// page.tsx — My Purchases page.
// Lists purchased notes with links to view them in the in-app PDF viewer.
// No download functionality — all viewing is in-app only.
"use client";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { formatDate, formatPrice } from "@/lib/format";
import { getPurchaseHistory } from "@/services/purchases";
import { useAuthStore } from "@/stores/auth-store";
import { useQuery } from "@tanstack/react-query";
import { ArrowLeft, BookOpen, Calendar, Eye, FileText } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";

export default function PurchasesPage() {
    const router = useRouter();
    const { isAuthenticated, loading } = useAuthStore();
    const [page, setPage] = useState(1);

    const { data, isLoading, isError } = useQuery({
        queryKey: ["purchaseHistory", page],
        queryFn: () => getPurchaseHistory({ page, limit: 20 }),
        enabled: isAuthenticated,
    });

    useEffect(() => {
        if (!isAuthenticated && !loading) {
            router.push("/login");
        }
    }, [isAuthenticated, loading, router]);

    if (!isAuthenticated) {
        return null;
    }

    return (
        <div className="max-w-2xl mx-auto px-4 py-4">
            <Button
                variant="ghost"
                size="sm"
                className="mb-3 -ml-2 text-muted-foreground"
                onClick={() => router.back()}
            >
                <ArrowLeft className="h-4 w-4 mr-1" /> Back
            </Button>

            <h1 className="text-xl font-bold mb-4 flex items-center gap-2">
                <BookOpen className="h-5 w-5" />
                My Purchases
            </h1>

            {isLoading ? (
                <div className="space-y-3">
                    {Array.from({ length: 5 }).map((_, i) => (
                        <Skeleton key={i} className="h-16 w-full" />
                    ))}
                </div>
            ) : isError ? (
                <p className="text-sm text-destructive">Failed to load purchases.</p>
            ) : (data?.purchases?.length ?? 0) === 0 ? (
                <div className="text-center py-12">
                    <BookOpen className="h-12 w-12 text-muted-foreground mx-auto mb-4" />
                    <p className="text-muted-foreground mb-4">
                        You haven&apos;t purchased any notes yet.
                    </p>
                    <Button asChild>
                        <Link href="/">Browse Notes</Link>
                    </Button>
                </div>
            ) : (
                <>
                    <div className="space-y-2">
                        {data!.purchases.map((purchase) => (
                            <Card key={purchase.purchase_id} className="border-border">
                                <CardContent className="flex items-center gap-3 p-3">
                                    <div className="h-10 w-10 rounded bg-primary/10 flex items-center justify-center shrink-0">
                                        <FileText className="h-5 w-5 text-primary" />
                                    </div>
                                    <div className="flex-1 min-w-0">
                                        <Link
                                            href={`/notes/${purchase.note_id}`}
                                            className="font-medium text-sm hover:text-primary block truncate"
                                        >
                                            {purchase.note_title}
                                        </Link>
                                        <div className="flex items-center gap-2 text-xs text-muted-foreground">
                                            <span>by {purchase.note_author}</span>
                                            <span>•</span>
                                            <span className="flex items-center gap-1">
                                                <Calendar className="h-3 w-3" />
                                                {formatDate(purchase.purchased_at)}
                                            </span>
                                        </div>
                                    </div>
                                    <Badge variant="secondary">{formatPrice(purchase.price_paid)}</Badge>
                                    {purchase.has_pdf && (
                                        <Button
                                            variant="outline"
                                            size="sm"
                                            className="h-8 gap-1"
                                            asChild
                                        >
                                            <Link href={`/notes/${purchase.note_id}`}>
                                                <Eye className="h-3.5 w-3.5" />
                                                View
                                            </Link>
                                        </Button>
                                    )}
                                </CardContent>
                            </Card>
                        ))}
                    </div>

                    {/* Pagination */}
                    {data && data.total > data.limit && (
                        <div className="flex items-center justify-center gap-2 mt-4">
                            <Button
                                variant="outline"
                                size="sm"
                                disabled={page <= 1}
                                onClick={() => setPage((p) => p - 1)}
                            >
                                Previous
                            </Button>
                            <span className="text-xs text-muted-foreground">
                                Page {page} of {Math.ceil(data.total / data.limit)}
                            </span>
                            <Button
                                variant="outline"
                                size="sm"
                                disabled={page >= Math.ceil(data.total / data.limit)}
                                onClick={() => setPage((p) => p + 1)}
                            >
                                Next
                            </Button>
                        </div>
                    )}
                </>
            )}
        </div>
    );
}
