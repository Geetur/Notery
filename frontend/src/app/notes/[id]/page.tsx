// page.tsx — Note detail page with Reddit-style layout.
// Shows note content via in-app PDF viewer, purchase widget in right sidebar, and comment section.
// No download functionality — all viewing is in-app only via the PDFViewer component.
"use client";

import { CommentSection } from "@/components/comments";
import { VoteButtons } from "@/components/feed/vote-buttons";
import { PDFViewer } from "@/components/pdf-viewer";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { useToast } from "@/hooks/use-toast";
import { formatDate, formatFileSize, formatPrice, timeAgo } from "@/lib/format";
import { getNoteById } from "@/services/notes";
import { addToCart, checkPurchaseStatus, purchaseNote } from "@/services/purchases";
import { useAuthStore } from "@/stores/auth-store";
import { useQuery } from "@tanstack/react-query";
import {
    ArrowLeft,
    CheckCircle,
    Clock,
    Eye,
    FileText,
    Loader2,
    Lock,
    ShoppingCart,
    User,
} from "lucide-react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useState } from "react";

export default function NoteDetailPage() {
    const params = useParams();
    const router = useRouter();
    const { toast } = useToast();
    const { isAuthenticated } = useAuthStore();
    const noteId = Number(params.id);
    const [purchasing, setPurchasing] = useState(false);
    const [addingToCart, setAddingToCart] = useState(false);

    const {
        data: note,
        isLoading,
        isError,
    } = useQuery({
        queryKey: ["note", noteId],
        queryFn: () => getNoteById(noteId),
        enabled: !!noteId && isAuthenticated,
    });

    const { data: purchaseStatus, refetch: refetchPurchase } = useQuery({
        queryKey: ["purchaseStatus", noteId],
        queryFn: () => checkPurchaseStatus(noteId),
        enabled: !!noteId && isAuthenticated,
    });

    const isOwned = purchaseStatus?.purchased === true;
    const isFree = note?.price === 0;
    const hasFullAccess = isOwned || isFree;

    const handlePurchase = async () => {
        if (!isAuthenticated) {
            router.push("/login");
            return;
        }
        setPurchasing(true);
        try {
            await purchaseNote(noteId);
            refetchPurchase();
            toast({ title: "Purchase successful!", description: "You now have full access to this note." });
        } catch {
            toast({ title: "Purchase failed", variant: "destructive" });
        } finally {
            setPurchasing(false);
        }
    };

    const handleAddToCart = async () => {
        if (!isAuthenticated) {
            router.push("/login");
            return;
        }
        setAddingToCart(true);
        try {
            await addToCart(noteId);
            toast({ title: "Added to cart" });
        } catch {
            toast({ title: "Failed to add to cart", variant: "destructive" });
        } finally {
            setAddingToCart(false);
        }
    };

    if (!isAuthenticated) {
        return (
            <div className="max-w-3xl mx-auto px-4 py-8 text-center">
                <Lock className="h-12 w-12 text-muted-foreground mx-auto mb-4" />
                <h2 className="text-xl font-bold mb-2">Login Required</h2>
                <p className="text-muted-foreground mb-4">
                    You need to be logged in to view note details.
                </p>
                <Button onClick={() => router.push("/login")}>Log In</Button>
            </div>
        );
    }

    if (isLoading) {
        return (
            <div className="max-w-[1000px] mx-auto flex gap-4 px-4 py-4">
                <div className="flex-1 space-y-3">
                    <Skeleton className="h-8 w-3/4" />
                    <Skeleton className="h-4 w-1/2" />
                    <Skeleton className="h-40 w-full" />
                </div>
                <div className="hidden lg:block w-80">
                    <Skeleton className="h-48" />
                </div>
            </div>
        );
    }

    if (isError || !note) {
        return (
            <div className="max-w-3xl mx-auto px-4 py-8 text-center">
                <p className="text-destructive mb-4">Failed to load note.</p>
                <Button variant="outline" onClick={() => router.back()}>
                    Go Back
                </Button>
            </div>
        );
    }

    return (
        <div className="max-w-[1000px] mx-auto flex gap-4 px-2 md:px-4 py-4">
            {/* Main content */}
            <main className="flex-1 min-w-0">
                {/* Back nav */}
                <Button
                    variant="ghost"
                    size="sm"
                    className="mb-2 -ml-2 text-muted-foreground"
                    onClick={() => router.back()}
                >
                    <ArrowLeft className="h-4 w-4 mr-1" />
                    Back
                </Button>

                {/* Note card */}
                <Card className="border-border">
                    <div className="flex">
                        {/* Vote column */}
                        <div className="p-3 pr-0">
                            <VoteButtons
                                noteId={note.id}
                                upvotes={note.upvotes}
                                downvotes={note.downvotes}
                            />
                        </div>

                        {/* Content */}
                        <div className="flex-1 p-3 pl-2">
                            {/* Meta */}
                            <div className="flex items-center gap-1.5 text-xs text-muted-foreground mb-2">
                                <Link
                                    href={`/n/${note.subnotery_id}`}
                                    className="font-semibold text-foreground hover:underline"
                                >
                                    n/{note.subnotery_id}
                                </Link>
                                <span>•</span>
                                <span>Posted by</span>
                                <Link
                                    href={`/user/${note.creator_id}`}
                                    className="hover:underline"
                                >
                                    u/{note.author}
                                </Link>
                                <span>•</span>
                                <span>{timeAgo(note.created_at)}</span>
                            </div>

                            {/* Title */}
                            <h1 className="text-xl font-bold text-foreground mb-2">
                                {note.title}
                            </h1>

                            {/* Badges */}
                            <div className="flex items-center gap-2 mb-4">
                                <Badge
                                    variant={note.price === 0 ? "secondary" : "default"}
                                >
                                    {formatPrice(note.price)}
                                </Badge>
                                {note.has_pdf && (
                                    <Badge variant="outline">
                                        <FileText className="h-3 w-3 mr-1" />
                                        PDF — {formatFileSize(note.pdf_size)}
                                    </Badge>
                                )}
                                <Badge
                                    variant="outline"
                                    className={
                                        note.status === "Approved"
                                            ? "border-green-500/50 text-green-500"
                                            : note.status === "Pending"
                                                ? "border-yellow-500/50 text-yellow-500"
                                                : "border-red-500/50 text-red-500"
                                    }
                                >
                                    {note.status}
                                </Badge>
                            </div>

                            <Separator className="mb-4" />

                            {/* Content area — in-app PDF viewer */}
                            {note.has_pdf ? (
                                hasFullAccess ? (
                                    <div>
                                        <div className="bg-green-500/10 border border-green-500/20 rounded-md p-3 mb-4">
                                            <div className="flex items-center gap-2 text-green-500 text-sm font-medium">
                                                <CheckCircle className="h-4 w-4" />
                                                {isOwned
                                                    ? "You own this note — full access granted"
                                                    : "This note is free — full access"}
                                            </div>
                                        </div>
                                        <PDFViewer
                                            noteId={note.id}
                                            mode="full"
                                            maxHeight={700}
                                        />
                                    </div>
                                ) : (
                                    <div>
                                        <div className="bg-yellow-500/10 border border-yellow-500/20 rounded-md p-3 mb-4">
                                            <div className="flex items-center gap-2 text-yellow-600 dark:text-yellow-400 text-sm font-medium">
                                                <Eye className="h-4 w-4" />
                                                Preview — purchase to view the full document
                                            </div>
                                        </div>
                                        <PDFViewer
                                            noteId={note.id}
                                            mode="preview"
                                            maxHeight={500}
                                        />
                                    </div>
                                )
                            ) : (
                                <div className="bg-muted/50 border border-border rounded-md p-6 text-center">
                                    <FileText className="h-8 w-8 text-muted-foreground mx-auto mb-2" />
                                    <p className="text-sm text-muted-foreground">
                                        No PDF content available for this note.
                                    </p>
                                </div>
                            )}
                        </div>
                    </div>
                </Card>

                {/* Comments */}
                <div className="mt-4">
                    <Card className="border-border p-4">
                        <CommentSection noteId={noteId} />
                    </Card>
                </div>
            </main>

            {/* Right sidebar — purchase widget */}
            <aside className="hidden lg:block w-80 shrink-0">
                <div className="sticky top-14 space-y-4">
                    {/* Purchase widget */}
                    <Card className="border-border">
                        <CardHeader className="py-3 px-4">
                            <CardTitle className="text-sm font-semibold">
                                {isOwned ? "Owned" : "Purchase"}
                            </CardTitle>
                        </CardHeader>
                        <CardContent className="px-4 pb-4 pt-0 space-y-3">
                            {/* Price */}
                            <div className="text-2xl font-bold text-foreground">
                                {formatPrice(note.price)}
                            </div>

                            {isOwned ? (
                                <div className="flex items-center gap-2 text-sm text-green-500">
                                    <CheckCircle className="h-4 w-4" />
                                    Purchased {purchaseStatus?.purchased_at
                                        ? formatDate(purchaseStatus.purchased_at)
                                        : ""}
                                </div>
                            ) : (
                                <>
                                    <Button
                                        className="w-full gap-2"
                                        onClick={handlePurchase}
                                        disabled={purchasing}
                                    >
                                        {purchasing ? (
                                            <Loader2 className="h-4 w-4 animate-spin" />
                                        ) : (
                                            <ShoppingCart className="h-4 w-4" />
                                        )}
                                        {isFree ? "Get for Free" : "Buy Now"}
                                    </Button>
                                    {!isFree && (
                                        <Button
                                            variant="outline"
                                            className="w-full gap-2"
                                            onClick={handleAddToCart}
                                            disabled={addingToCart}
                                        >
                                            {addingToCart ? (
                                                <Loader2 className="h-4 w-4 animate-spin" />
                                            ) : (
                                                <ShoppingCart className="h-4 w-4" />
                                            )}
                                            Add to Cart
                                        </Button>
                                    )}
                                </>
                            )}

                            <Separator />

                            {/* Note info */}
                            <div className="space-y-2 text-xs text-muted-foreground">
                                <div className="flex items-center gap-2">
                                    <User className="h-3.5 w-3.5" />
                                    <span>by {note.author}</span>
                                </div>
                                <div className="flex items-center gap-2">
                                    <Clock className="h-3.5 w-3.5" />
                                    <span>{formatDate(note.created_at)}</span>
                                </div>
                                {note.has_pdf && (
                                    <div className="flex items-center gap-2">
                                        <FileText className="h-3.5 w-3.5" />
                                        <span>PDF — {formatFileSize(note.pdf_size)}</span>
                                    </div>
                                )}
                            </div>
                        </CardContent>
                    </Card>

                    {/* Viewing info */}
                    <Card className="border-border">
                        <CardHeader className="py-3 px-4">
                            <CardTitle className="text-sm font-semibold">
                                {hasFullAccess ? "Full Access" : "Preview"}
                            </CardTitle>
                        </CardHeader>
                        <CardContent className="px-4 pb-4 pt-0">
                            <p className="text-xs text-muted-foreground">
                                {hasFullAccess
                                    ? "You have full access to this document. View all pages in the reader above."
                                    : note.has_pdf
                                        ? "You can preview the first few pages. Purchase to view the full document."
                                        : "No preview available for this note."}
                            </p>
                        </CardContent>
                    </Card>
                </div>
            </aside>
        </div>
    );
}
