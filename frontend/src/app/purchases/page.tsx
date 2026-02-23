// page.tsx — My Notes page (created + purchased notes).
// Shows all notes a user has access to: ones they created and ones they purchased.
// No download functionality — all viewing is in-app only.
"use client";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { formatDate, formatPrice } from "@/lib/format";
import { cn } from "@/lib/utils";
import { getMyNotes } from "@/services/notes";
import { getPurchaseHistory } from "@/services/purchases";
import { useAuthStore } from "@/stores/auth-store";
import { useQuery } from "@tanstack/react-query";
import { ArrowLeft, BookOpen, Calendar, Eye, FileText, PenTool } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";

type MyNotesTab = "purchased" | "created";

export default function PurchasesPage() {
    const router = useRouter();
    const { isAuthenticated, loading } = useAuthStore();
    const [tab, setTab] = useState<MyNotesTab>("purchased");
    const [purchasedPage, setPurchasedPage] = useState(1);
    const [createdPage, setCreatedPage] = useState(1);

    const {
        data: purchaseData,
        isLoading: purchaseLoading,
        isError: purchaseError,
    } = useQuery({
        queryKey: ["purchaseHistory", purchasedPage],
        queryFn: () => getPurchaseHistory({ page: purchasedPage, limit: 20 }),
        enabled: isAuthenticated && tab === "purchased",
    });

    const {
        data: createdData,
        isLoading: createdLoading,
        isError: createdError,
    } = useQuery({
        queryKey: ["myCreatedNotes", createdPage],
        queryFn: () => getMyNotes({ page: createdPage, limit: 20 }),
        enabled: isAuthenticated && tab === "created",
    });

    useEffect(() => {
        if (!isAuthenticated && !loading) {
            router.push("/login");
        }
    }, [isAuthenticated, loading, router]);

    if (!isAuthenticated) {
        return null;
    }

    const isLoading = tab === "purchased" ? purchaseLoading : createdLoading;
    const isError = tab === "purchased" ? purchaseError : createdError;

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
                My Notes
            </h1>

            {/* Sub-tabs */}
            <div className="flex gap-1 mb-4 border-b border-border">
                <button
                    onClick={() => setTab("purchased")}
                    className={cn(
                        "px-3 py-2 text-sm font-medium border-b-2 transition-colors",
                        tab === "purchased"
                            ? "border-primary text-primary"
                            : "border-transparent text-muted-foreground hover:text-foreground"
                    )}
                >
                    Purchased
                </button>
                <button
                    onClick={() => setTab("created")}
                    className={cn(
                        "px-3 py-2 text-sm font-medium border-b-2 transition-colors",
                        tab === "created"
                            ? "border-primary text-primary"
                            : "border-transparent text-muted-foreground hover:text-foreground"
                    )}
                >
                    Created
                </button>
            </div>

            {isLoading ? (
                <div className="space-y-3">
                    {Array.from({ length: 5 }).map((_, i) => (
                        <Skeleton key={i} className="h-16 w-full" />
                    ))}
                </div>
            ) : isError ? (
                <p className="text-sm text-destructive">
                    Failed to load {tab === "purchased" ? "purchases" : "notes"}.
                </p>
            ) : tab === "purchased" ? (
                /* ── Purchased notes tab ── */
                (purchaseData?.purchases?.length ?? 0) === 0 ? (
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
                            {purchaseData!.purchases.map((purchase) => (
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

                        {purchaseData && purchaseData.total > purchaseData.limit && (
                            <div className="flex items-center justify-center gap-2 mt-4">
                                <Button
                                    variant="outline"
                                    size="sm"
                                    disabled={purchasedPage <= 1}
                                    onClick={() => setPurchasedPage((p) => p - 1)}
                                >
                                    Previous
                                </Button>
                                <span className="text-xs text-muted-foreground">
                                    Page {purchasedPage} of {Math.ceil(purchaseData.total / purchaseData.limit)}
                                </span>
                                <Button
                                    variant="outline"
                                    size="sm"
                                    disabled={purchasedPage >= Math.ceil(purchaseData.total / purchaseData.limit)}
                                    onClick={() => setPurchasedPage((p) => p + 1)}
                                >
                                    Next
                                </Button>
                            </div>
                        )}
                    </>
                )
            ) : (
                /* ── Created notes tab ── */
                (createdData?.notes?.length ?? 0) === 0 ? (
                    <div className="text-center py-12">
                        <PenTool className="h-12 w-12 text-muted-foreground mx-auto mb-4" />
                        <p className="text-muted-foreground mb-4">
                            You haven&apos;t created any notes yet.
                        </p>
                        <Button asChild>
                            <Link href="/submit">Create a Note</Link>
                        </Button>
                    </div>
                ) : (
                    <>
                        <div className="space-y-2">
                            {createdData!.notes.map((note) => (
                                <Card key={note.id} className="border-border">
                                    <CardContent className="flex items-center gap-3 p-3">
                                        <div className="h-10 w-10 rounded bg-primary/10 flex items-center justify-center shrink-0">
                                            <PenTool className="h-5 w-5 text-primary" />
                                        </div>
                                        <div className="flex-1 min-w-0">
                                            <Link
                                                href={`/notes/${note.id}`}
                                                className="font-medium text-sm hover:text-primary block truncate"
                                            >
                                                {note.title}
                                            </Link>
                                            <div className="flex items-center gap-2 text-xs text-muted-foreground">
                                                <span>{note.subnotery_name}</span>
                                                <span>•</span>
                                                <Badge
                                                    variant={
                                                        note.status === "Approved"
                                                            ? "default"
                                                            : note.status === "Pending"
                                                                ? "secondary"
                                                                : "destructive"
                                                    }
                                                    className="text-[10px] px-1.5 py-0"
                                                >
                                                    {note.status}
                                                </Badge>
                                                <span>•</span>
                                                <span className="flex items-center gap-1">
                                                    <Calendar className="h-3 w-3" />
                                                    {formatDate(note.created_at)}
                                                </span>
                                            </div>
                                        </div>
                                        <Badge variant="secondary">{formatPrice(note.price)}</Badge>
                                        <Button
                                            variant="outline"
                                            size="sm"
                                            className="h-8 gap-1"
                                            asChild
                                        >
                                            <Link href={`/notes/${note.id}`}>
                                                <Eye className="h-3.5 w-3.5" />
                                                View
                                            </Link>
                                        </Button>
                                    </CardContent>
                                </Card>
                            ))}
                        </div>

                        {createdData && createdData.total > createdData.limit && (
                            <div className="flex items-center justify-center gap-2 mt-4">
                                <Button
                                    variant="outline"
                                    size="sm"
                                    disabled={createdPage <= 1}
                                    onClick={() => setCreatedPage((p) => p - 1)}
                                >
                                    Previous
                                </Button>
                                <span className="text-xs text-muted-foreground">
                                    Page {createdPage} of {Math.ceil(createdData.total / createdData.limit)}
                                </span>
                                <Button
                                    variant="outline"
                                    size="sm"
                                    disabled={createdPage >= Math.ceil(createdData.total / createdData.limit)}
                                    onClick={() => setCreatedPage((p) => p + 1)}
                                >
                                    Next
                                </Button>
                            </div>
                        )}
                    </>
                )
            )}
        </div>
    );
}
