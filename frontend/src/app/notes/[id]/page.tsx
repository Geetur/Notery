// page.tsx — Note detail page with Reddit-style layout.
// Shows note content via in-app PDF viewer with purchase widget integrated into the note card.
// No download functionality — all viewing is in-app only via the PDFViewer component.
// Admins see the full PDF for pending notes (not preview). Nobody can purchase pending notes.
"use client";

import { CommentSection } from "@/components/comments";
import { VoteButtons } from "@/components/feed/vote-buttons";
import { PDFViewer } from "@/components/pdf-viewer";
import { StripeCheckout } from "@/components/stripe-checkout";
import { SubnoteryAvatar } from "@/components/subnotery-avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { useToast } from "@/hooks/use-toast";
import { formatDate, formatFileSize, formatPrice, thumbnailUrl, timeAgo } from "@/lib/format";
import { approveNote, getNoteById, rejectNote } from "@/services/notes";
import { addToCart, checkPurchaseStatus, confirmOrder, getOrderStatus, purchaseNote } from "@/services/purchases";
import { getSubnotery } from "@/services/subnoteries";
import { useAuthStore } from "@/stores/auth-store";
import { useQuery, useQueryClient } from "@tanstack/react-query";
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
    XCircle,
} from "lucide-react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useState } from "react";

export default function NoteDetailPage() {
    const params = useParams();
    const router = useRouter();
    const { toast } = useToast();
    const { isAuthenticated, user: currentUser } = useAuthStore();
    const noteId = Number(params.id);
    const [purchasing, setPurchasing] = useState(false);
    const [addingToCart, setAddingToCart] = useState(false);
    const [approving, setApproving] = useState(false);
    const [rejecting, setRejecting] = useState(false);
    const queryClient = useQueryClient();

    // Stripe payment dialog state
    const [stripeOpen, setStripeOpen] = useState(false);
    const [stripeClientSecret, setStripeClientSecret] = useState("");
    const [stripeOrderId, setStripeOrderId] = useState(0);
    const [stripeTotalCents, setStripeTotalCents] = useState(0);

    const {
        data: note,
        isLoading,
        isError,
    } = useQuery({
        queryKey: ["note", noteId],
        queryFn: () => getNoteById(noteId),
        enabled: !!noteId && isAuthenticated,
    });

    const { data: purchaseStatus } = useQuery({
        queryKey: ["purchaseStatus", noteId],
        queryFn: () => checkPurchaseStatus(noteId),
        enabled: !!noteId && isAuthenticated,
    });

    const { data: subnoteryDetail } = useQuery({
        queryKey: ["subnotery", note?.subnotery_id],
        queryFn: () => getSubnotery(note!.subnotery_id),
        enabled: !!note?.subnotery_id,
    });

    const isNoteAdmin =
        currentUser &&
        subnoteryDetail?.admins?.some((a) => a.id === currentUser.id);

    const isOwned = purchaseStatus?.purchased === true;
    const isFree = note?.price === 0;
    const isPending = note?.status === "Pending";
    // Use the backend-computed has_full_access (covers creator, admin, purchased, free).
    // Fallback to client-side checks for immediate UI display.
    const hasFullAccess = note?.has_full_access || isPending || isOwned || isFree;
    // Purchase UI is only shown for approved, non-free, non-owned notes that user doesn't have full access to.
    const isApproved = note?.status === "Approved";

    const handlePurchase = async () => {
        if (!isAuthenticated) {
            router.push("/login");
            return;
        }
        setPurchasing(true);
        try {
            const res = await purchaseNote(noteId);

            // Only mark as purchased if the order was actually fulfilled (dev mode / free note).
            // If Stripe is active, status will be "pending" — the user must complete payment first.
            if (res.status === "fulfilled") {
                // Set purchase status so the UI updates immediately
                queryClient.setQueryData(["purchaseStatus", noteId], {
                    purchased: true,
                    purchased_at: res.purchased_at ?? new Date().toISOString(),
                });

                // Force refetch note data so has_full_access is recomputed server-side
                await queryClient.refetchQueries({ queryKey: ["note", noteId], exact: true });

                // Force refetch purchase status to confirm (awaited to prevent race)
                await queryClient.refetchQueries({ queryKey: ["purchaseStatus", noteId], exact: true });

                // Refresh purchase history (for My Notes / profile)
                queryClient.invalidateQueries({ queryKey: ["purchaseHistory"] });
                queryClient.invalidateQueries({ queryKey: ["myPurchases"] });

                toast({ title: "Purchase successful!", description: "You now have full access to this note." });
            } else if (res.client_secret) {
                // Stripe payment flow — open checkout dialog
                setStripeClientSecret(res.client_secret);
                setStripeOrderId(res.order_id);
                setStripeTotalCents(res.total_cents ?? note?.price ?? 0);
                setStripeOpen(true);
            } else {
                toast({ title: "Purchase initiated", description: "Your order is being processed." });
            }
        } catch (err: unknown) {
            const msg = err instanceof Error ? err.message : "Something went wrong";
            toast({ title: "Purchase failed", description: msg, variant: "destructive" });
        } finally {
            setPurchasing(false);
        }
    };

    const handleAddToCart = async () => {
        if (!isAuthenticated) {
            router.push("/login");
            return;
        }
        if (isOwned) {
            toast({ title: "Already purchased", description: "You already own this note.", variant: "destructive" });
            return;
        }
        setAddingToCart(true);
        try {
            await addToCart(noteId);
            toast({ title: "Added to cart" });
        } catch (err: unknown) {
            const msg = err instanceof Error ? err.message : "Failed to add to cart";
            toast({ title: "Failed to add to cart", description: msg, variant: "destructive" });
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
            <div className="max-w-4xl mx-auto px-4 py-4 space-y-3">
                <Skeleton className="h-8 w-3/4" />
                <Skeleton className="h-4 w-1/2" />
                <Skeleton className="h-40 w-full" />
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
        <div className="max-w-4xl mx-auto px-2 md:px-4 py-4">
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

            {/* Note card — single unified card with all content */}
            <Card className="border-border">
                <div className="flex">
                    {/* Vote column */}
                    <div className="p-3 pr-0">
                        <VoteButtons
                            noteId={note.id}
                            upvotes={note.upvotes}
                            downvotes={note.downvotes}
                            initialUserVote={note.user_vote}
                            noteStatus={note.status}
                        />
                    </div>

                    {/* Content */}
                    <div className="flex-1 p-3 pl-2">
                        {/* Meta */}
                        <div className="mb-2 space-y-0.5">
                            <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
                                <SubnoteryAvatar
                                    subnoteryId={note.subnotery_id}
                                    profilePictureUrl={note.subnotery_profile_picture_url}
                                    name={note.subnotery_name}
                                    size="sm"
                                />
                                <Link
                                    href={`/communities/${note.subnotery_id}`}
                                    className="font-semibold text-foreground hover:underline"
                                >
                                    n/{note.subnotery_name || note.subnotery_id}
                                </Link>
                                <span>•</span>
                                <span>{timeAgo(note.created_at)}</span>
                            </div>
                            <div className="flex items-center gap-1.5 text-xs text-muted-foreground pl-[26px]">
                                <span>Posted by</span>
                                <Link
                                    href={`/user/${note.creator_id}`}
                                    className="hover:underline"
                                >
                                    u/{note.author}
                                </Link>
                            </div>
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
                            {note.status !== "Approved" && (
                                <Badge
                                    variant="outline"
                                    className={
                                        note.status === "Pending"
                                            ? "border-yellow-500/50 text-yellow-500"
                                            : "border-red-500/50 text-red-500"
                                    }
                                >
                                    {note.status}
                                </Badge>
                            )}
                        </div>

                        {/* Description */}
                        {note.description && (
                            <p className="text-sm text-muted-foreground leading-relaxed mb-4 whitespace-pre-wrap">
                                {note.description}
                            </p>
                        )}

                        {/* Thumbnail */}
                        {note.has_thumbnail && note.thumbnail_url && (
                            <div className="mb-4 rounded-md overflow-hidden border border-border">
                                {/* eslint-disable-next-line @next/next/no-img-element */}
                                <img
                                    src={thumbnailUrl(note.id, note.thumbnail_url)}
                                    alt={`Thumbnail for ${note.title}`}
                                    className="w-full max-h-[400px] object-contain bg-muted/30"
                                />
                            </div>
                        )}

                        {/* Purchase / ownership widget — integrated into the note card */}
                        {isApproved && (
                            <div className="mb-4">
                                {isOwned ? (
                                    <div className="bg-green-500/10 border border-green-500/20 rounded-md p-3">
                                        <div className="flex items-center gap-2 text-green-500 text-sm font-medium">
                                            <CheckCircle className="h-4 w-4" />
                                            You own this note
                                            {purchaseStatus?.purchased_at
                                                ? ` — purchased ${formatDate(purchaseStatus.purchased_at)}`
                                                : " — full access granted"}
                                        </div>
                                    </div>
                                ) : isFree ? (
                                    <div className="bg-green-500/10 border border-green-500/20 rounded-md p-3">
                                        <div className="flex items-center gap-2 text-green-500 text-sm font-medium">
                                            <CheckCircle className="h-4 w-4" />
                                            This note is free — full access
                                        </div>
                                    </div>
                                ) : hasFullAccess ? (
                                    <div className="bg-green-500/10 border border-green-500/20 rounded-md p-3">
                                        <div className="flex items-center gap-2 text-green-500 text-sm font-medium">
                                            <CheckCircle className="h-4 w-4" />
                                            Full access granted
                                        </div>
                                    </div>
                                ) : (
                                    <div className="border border-border rounded-md p-4">
                                        <div className="flex items-center justify-between gap-4">
                                            <div className="flex items-center gap-3">
                                                <div className="text-xl font-bold text-foreground">
                                                    {formatPrice(note.price)}
                                                </div>
                                                <div className="text-xs text-muted-foreground space-y-0.5">
                                                    <div className="flex items-center gap-1">
                                                        <User className="h-3 w-3" />
                                                        by {note.author}
                                                    </div>
                                                    <div className="flex items-center gap-1">
                                                        <Clock className="h-3 w-3" />
                                                        {formatDate(note.created_at)}
                                                    </div>
                                                </div>
                                            </div>
                                            <div className="flex gap-2">
                                                <Button
                                                    className="gap-2"
                                                    onClick={handlePurchase}
                                                    disabled={purchasing}
                                                >
                                                    {purchasing ? (
                                                        <Loader2 className="h-4 w-4 animate-spin" />
                                                    ) : (
                                                        <ShoppingCart className="h-4 w-4" />
                                                    )}
                                                    Buy Now
                                                </Button>
                                                <Button
                                                    variant="outline"
                                                    className="gap-2"
                                                    onClick={handleAddToCart}
                                                    disabled={addingToCart || isOwned}
                                                >
                                                    {addingToCart ? (
                                                        <Loader2 className="h-4 w-4 animate-spin" />
                                                    ) : (
                                                        <ShoppingCart className="h-4 w-4" />
                                                    )}
                                                    Add to Cart
                                                </Button>
                                            </div>
                                        </div>
                                    </div>
                                )}
                            </div>
                        )}

                        {/* Pending note admin banner with approve/reject */}
                        {isPending && (
                            <div className="bg-yellow-500/10 border border-yellow-500/20 rounded-md p-3 mb-4">
                                <div className="flex items-center justify-between">
                                    <div className="flex items-center gap-2 text-yellow-600 dark:text-yellow-400 text-sm font-medium">
                                        <Eye className="h-4 w-4" />
                                        Admin review — viewing full document
                                    </div>
                                    {currentUser && (
                                        <div className="flex gap-2">
                                            <Button
                                                size="sm"
                                                variant="default"
                                                disabled={approving || !note?.has_pdf}
                                                title={!note?.has_pdf ? "PDF required before approval" : "Approve this note"}
                                                onClick={async () => {
                                                    setApproving(true);
                                                    try {
                                                        await approveNote(noteId);
                                                        toast({ title: "Approved", description: "Note has been approved." });
                                                        queryClient.invalidateQueries({ queryKey: ["note", noteId] });
                                                    } catch (err: unknown) {
                                                        const msg = err instanceof Error ? err.message : "Failed to approve";
                                                        toast({ title: "Error", description: msg, variant: "destructive" });
                                                    } finally {
                                                        setApproving(false);
                                                    }
                                                }}
                                            >
                                                {approving ? <Loader2 className="h-4 w-4 animate-spin mr-1" /> : <CheckCircle className="h-4 w-4 mr-1" />}
                                                Approve
                                            </Button>
                                            <Button
                                                size="sm"
                                                variant="destructive"
                                                disabled={rejecting}
                                                onClick={async () => {
                                                    setRejecting(true);
                                                    try {
                                                        await rejectNote(noteId);
                                                        toast({ title: "Rejected", description: "Note has been rejected." });
                                                        router.back();
                                                    } catch (err: unknown) {
                                                        const msg = err instanceof Error ? err.message : "Failed to reject";
                                                        toast({ title: "Error", description: msg, variant: "destructive" });
                                                    } finally {
                                                        setRejecting(false);
                                                    }
                                                }}
                                            >
                                                {rejecting ? <Loader2 className="h-4 w-4 animate-spin mr-1" /> : <XCircle className="h-4 w-4 mr-1" />}
                                                Reject
                                            </Button>
                                        </div>
                                    )}
                                </div>
                            </div>
                        )}

                        <Separator className="mb-4" />

                        {/* Content area — in-app PDF viewer */}
                        {note.has_pdf ? (
                            hasFullAccess ? (
                                <PDFViewer
                                    key={`full-${note.id}`}
                                    noteId={note.id}
                                    mode="full"
                                    maxHeight={700}
                                />
                            ) : (
                                <div>
                                    <div className="bg-yellow-500/10 border border-yellow-500/20 rounded-md p-3 mb-4">
                                        <div className="flex items-center gap-2 text-yellow-600 dark:text-yellow-400 text-sm font-medium">
                                            <Eye className="h-4 w-4" />
                                            Preview — purchase to view the full document
                                        </div>
                                    </div>
                                    <PDFViewer
                                        key={`preview-${note.id}`}
                                        noteId={note.id}
                                        mode="preview"
                                        maxHeight={500}
                                        totalPages={note.pdf_pages}
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
                    {note.is_locked ? (
                        <div className="flex items-center gap-2 text-sm text-orange-500 py-2">
                            <Lock className="h-4 w-4" />
                            This note is locked — comments are disabled.
                        </div>
                    ) : (
                        <CommentSection noteId={noteId} isAdmin={!!isNoteAdmin} />
                    )}
                </Card>
            </div>

            {/* Stripe payment dialog for single-note purchase */}
            <StripeCheckout
                open={stripeOpen}
                onClose={() => {
                    setStripeOpen(false);
                    setStripeClientSecret("");
                    setStripeOrderId(0);
                    setStripeTotalCents(0);
                }}
                clientSecret={stripeClientSecret}
                totalCents={stripeTotalCents}
                orderId={stripeOrderId}
                onPaymentSuccess={async (orderId) => {
                    // Trigger backend reconciliation
                    try {
                        await confirmOrder(orderId);
                    } catch {
                        // Webhook may have already fulfilled — poll below
                    }
                    // Poll until fulfilled (max 10 attempts, 1.5s apart)
                    for (let i = 0; i < 10; i++) {
                        try {
                            const status = await getOrderStatus(orderId);
                            if (status.status === "fulfilled") break;
                        } catch {
                            // ignore poll errors
                        }
                        await new Promise((r) => setTimeout(r, 1500));
                    }
                    // Refresh data and close dialog
                    setStripeOpen(false);
                    setStripeClientSecret("");
                    setStripeOrderId(0);
                    setStripeTotalCents(0);
                    queryClient.invalidateQueries({ queryKey: ["note", noteId] });
                    queryClient.invalidateQueries({ queryKey: ["purchaseStatus", noteId] });
                    toast({ title: "Payment successful", description: "You now have full access to this note." });
                }}
            />
        </div>
    );
}
