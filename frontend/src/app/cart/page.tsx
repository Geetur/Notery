// page.tsx — Shopping cart page with checkbox selection, pagination, and buy selected/buy all.
"use client";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { useToast } from "@/hooks/use-toast";
import { formatPrice } from "@/lib/format";
import { getNoteById } from "@/services/notes";
import {
    checkoutCart,
    checkoutSelected,
    getCart,
    removeFromCart,
} from "@/services/purchases";
import { useAuthStore } from "@/stores/auth-store";
import type { Note } from "@/types";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
    ArrowLeft,
    ChevronLeft,
    ChevronRight,
    CreditCard,
    Eye,
    Loader2,
    ShoppingCart,
    Trash2,
} from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

const ITEMS_PER_PAGE = 10;

export default function CartPage() {
    const router = useRouter();
    const { isAuthenticated } = useAuthStore();
    const { toast } = useToast();
    const queryClient = useQueryClient();
    const [checkingOut, setCheckingOut] = useState(false);
    const [selected, setSelected] = useState<Set<number>>(new Set());
    const [page, setPage] = useState(1);

    const { data: cartData, isLoading } = useQuery({
        queryKey: ["cart"],
        queryFn: getCart,
        enabled: isAuthenticated,
    });

    const cartItemIds = cartData?.cart ?? [];

    // Fetch note details for each cart item
    const { data: cartNotes, isLoading: loadingNotes } = useQuery({
        queryKey: ["cartNotes", cartItemIds],
        queryFn: async () => {
            const notes = await Promise.all(
                cartItemIds.map((id) => getNoteById(Number(id)).catch(() => null))
            );
            return notes.filter(Boolean) as Note[];
        },
        enabled: cartItemIds.length > 0,
    });

    const allNotes = cartNotes ?? [];
    const totalPages = Math.max(1, Math.ceil(allNotes.length / ITEMS_PER_PAGE));
    const pagedNotes = allNotes.slice(
        (page - 1) * ITEMS_PER_PAGE,
        page * ITEMS_PER_PAGE
    );

    const totalCents = allNotes.reduce((sum, n) => sum + n.price, 0);
    const selectedNotes = allNotes.filter((n) => selected.has(n.id));
    const selectedCents = selectedNotes.reduce((sum, n) => sum + n.price, 0);

    const allSelected = useMemo(
        () => allNotes.length > 0 && allNotes.every((n) => selected.has(n.id)),
        [allNotes, selected]
    );

    const toggleSelect = (id: number) => {
        setSelected((prev) => {
            const next = new Set(prev);
            if (next.has(id)) next.delete(id);
            else next.add(id);
            return next;
        });
    };

    const toggleSelectAll = () => {
        if (allSelected) {
            setSelected(new Set());
        } else {
            setSelected(new Set(allNotes.map((n) => n.id)));
        }
    };

    const handleRemove = async (itemId: string) => {
        const numId = Number(itemId);
        setSelected((prev) => {
            const next = new Set(prev);
            next.delete(numId);
            return next;
        });
        try {
            await removeFromCart(itemId);
            queryClient.invalidateQueries({ queryKey: ["cart"] });
            toast({ title: "Removed from cart" });
        } catch {
            toast({ title: "Failed to remove item", variant: "destructive" });
        }
    };

    const invalidateAfterPurchase = () =>
        Promise.all([
            queryClient.invalidateQueries({ queryKey: ["cart"] }),
            queryClient.invalidateQueries({ queryKey: ["purchaseHistory"] }),
            queryClient.invalidateQueries({ queryKey: ["myPurchases"] }),
            queryClient.invalidateQueries({ queryKey: ["purchaseStatus"] }),
            queryClient.invalidateQueries({ queryKey: ["note"] }),
        ]);

    const handleCheckoutAll = async () => {
        setCheckingOut(true);
        try {
            const key = crypto.randomUUID();
            const res = await checkoutCart(key);
            if (res.status === "fulfilled") {
                toast({
                    title: "Purchase complete!",
                    description: `${res.purchased_count} note(s) purchased.`,
                });
                await invalidateAfterPurchase();
                router.push("/purchases");
            } else if (res.client_secret) {
                toast({
                    title: "Payment initiated",
                    description: "Complete payment to finalize.",
                });
            }
        } catch {
            toast({ title: "Checkout failed", variant: "destructive" });
        } finally {
            setCheckingOut(false);
        }
    };

    const handleCheckoutSelected = async () => {
        if (selected.size === 0) {
            toast({ title: "No items selected", variant: "destructive" });
            return;
        }
        setCheckingOut(true);
        try {
            const key = crypto.randomUUID();
            const ids = Array.from(selected).map(String);
            const res = await checkoutSelected(ids, key);
            if (res.status === "fulfilled") {
                toast({
                    title: "Purchase complete!",
                    description: `${res.purchased_count} note(s) purchased.`,
                });
                setSelected(new Set());
                await invalidateAfterPurchase();
                router.push("/purchases");
            } else if (res.client_secret) {
                toast({
                    title: "Payment initiated",
                    description: "Complete payment to finalize.",
                });
            }
        } catch {
            toast({ title: "Checkout failed", variant: "destructive" });
        } finally {
            setCheckingOut(false);
        }
    };

    if (!isAuthenticated) {
        return (
            <div className="max-w-2xl mx-auto px-4 py-8 text-center">
                <ShoppingCart className="h-12 w-12 text-muted-foreground mx-auto mb-4" />
                <h2 className="text-xl font-bold mb-2">Login Required</h2>
                <p className="text-muted-foreground mb-4">Log in to view your cart.</p>
                <Button onClick={() => router.push("/login")}>Log In</Button>
            </div>
        );
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
                <ShoppingCart className="h-5 w-5" />
                Shopping Cart
                {allNotes.length > 0 && (
                    <span className="text-sm font-normal text-muted-foreground">
                        ({allNotes.length} item{allNotes.length !== 1 ? "s" : ""})
                    </span>
                )}
            </h1>

            {isLoading || loadingNotes ? (
                <div className="space-y-3">
                    {Array.from({ length: 3 }).map((_, i) => (
                        <Skeleton key={i} className="h-16 w-full" />
                    ))}
                </div>
            ) : cartItemIds.length === 0 ? (
                <div className="text-center py-12">
                    <ShoppingCart className="h-12 w-12 text-muted-foreground mx-auto mb-4" />
                    <p className="text-muted-foreground mb-4">Your cart is empty.</p>
                    <Button asChild>
                        <Link href="/">Browse Notes</Link>
                    </Button>
                </div>
            ) : (
                <>
                    {/* Select all header */}
                    <div className="flex items-center gap-2 mb-2 px-1">
                        <Checkbox
                            checked={allSelected}
                            onCheckedChange={toggleSelectAll}
                            id="select-all"
                        />
                        <label
                            htmlFor="select-all"
                            className="text-sm text-muted-foreground cursor-pointer"
                        >
                            Select all
                        </label>
                        {selected.size > 0 && (
                            <span className="text-xs text-muted-foreground ml-auto">
                                {selected.size} selected &middot; {formatPrice(selectedCents)}
                            </span>
                        )}
                    </div>

                    {/* Cart items */}
                    <div className="space-y-2">
                        {pagedNotes.map((note) => (
                            <Card
                                key={note.id}
                                className={`border-border transition-colors ${
                                    selected.has(note.id)
                                        ? "ring-1 ring-primary/50 bg-primary/5"
                                        : ""
                                }`}
                            >
                                <CardContent className="flex items-center gap-3 p-3">
                                    <Checkbox
                                        checked={selected.has(note.id)}
                                        onCheckedChange={() => toggleSelect(note.id)}
                                    />
                                    <div className="flex-1 min-w-0">
                                        <Link
                                            href={`/notes/${note.id}`}
                                            className="font-medium text-sm hover:text-primary"
                                        >
                                            {note.title}
                                        </Link>
                                        <p className="text-xs text-muted-foreground">
                                            by {note.author} &middot;{" "}
                                            {note.subnotery_name && `n/${note.subnotery_name}`}
                                        </p>
                                    </div>
                                    <span className="font-semibold text-sm whitespace-nowrap">
                                        {formatPrice(note.price)}
                                    </span>
                                    <Button
                                        variant="ghost"
                                        size="icon"
                                        className="h-8 w-8 text-muted-foreground hover:text-primary"
                                        asChild
                                    >
                                        <Link href={`/notes/${note.id}`}>
                                            <Eye className="h-4 w-4" />
                                        </Link>
                                    </Button>
                                    <Button
                                        variant="ghost"
                                        size="icon"
                                        className="h-8 w-8 text-muted-foreground hover:text-destructive"
                                        onClick={() => handleRemove(String(note.id))}
                                    >
                                        <Trash2 className="h-4 w-4" />
                                    </Button>
                                </CardContent>
                            </Card>
                        ))}
                    </div>

                    {/* Pagination */}
                    {totalPages > 1 && (
                        <div className="flex items-center justify-center gap-2 mt-4">
                            <Button
                                variant="outline"
                                size="sm"
                                disabled={page <= 1}
                                onClick={() => setPage((p) => p - 1)}
                            >
                                <ChevronLeft className="h-4 w-4" />
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
                                <ChevronRight className="h-4 w-4" />
                            </Button>
                        </div>
                    )}

                    <Separator className="my-4" />

                    {/* Checkout summary */}
                    <Card className="border-border">
                        <CardHeader className="py-3 px-4">
                            <CardTitle className="text-sm">Order Summary</CardTitle>
                        </CardHeader>
                        <CardContent className="px-4 pb-4 pt-0 space-y-3">
                            <div className="flex justify-between text-sm">
                                <span className="text-muted-foreground">
                                    Cart total ({allNotes.length} item
                                    {allNotes.length !== 1 ? "s" : ""})
                                </span>
                                <span className="font-bold">{formatPrice(totalCents)}</span>
                            </div>
                            {selected.size > 0 && selected.size < allNotes.length && (
                                <div className="flex justify-between text-sm">
                                    <span className="text-muted-foreground">
                                        Selected ({selected.size} item
                                        {selected.size !== 1 ? "s" : ""})
                                    </span>
                                    <span className="font-bold">
                                        {formatPrice(selectedCents)}
                                    </span>
                                </div>
                            )}
                            <div className="flex gap-2">
                                {selected.size > 0 && selected.size < allNotes.length && (
                                    <Button
                                        className="flex-1 gap-2"
                                        variant="outline"
                                        onClick={handleCheckoutSelected}
                                        disabled={checkingOut}
                                    >
                                        {checkingOut ? (
                                            <Loader2 className="h-4 w-4 animate-spin" />
                                        ) : (
                                            <CreditCard className="h-4 w-4" />
                                        )}
                                        Buy Selected ({selected.size})
                                    </Button>
                                )}
                                <Button
                                    className="flex-1 gap-2"
                                    onClick={handleCheckoutAll}
                                    disabled={checkingOut}
                                >
                                    {checkingOut ? (
                                        <Loader2 className="h-4 w-4 animate-spin" />
                                    ) : (
                                        <CreditCard className="h-4 w-4" />
                                    )}
                                    Buy All ({formatPrice(totalCents)})
                                </Button>
                            </div>
                        </CardContent>
                    </Card>
                </>
            )}
        </div>
    );
}
