// page.tsx — Shopping cart page.
"use client";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { useToast } from "@/hooks/use-toast";
import { formatPrice } from "@/lib/format";
import { getNoteById } from "@/services/notes";
import { checkoutCart, getCart, removeFromCart } from "@/services/purchases";
import { useAuthStore } from "@/stores/auth-store";
import type { Note } from "@/types";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, CreditCard, Loader2, ShoppingCart, Trash2 } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";

export default function CartPage() {
    const router = useRouter();
    const { isAuthenticated } = useAuthStore();
    const { toast } = useToast();
    const queryClient = useQueryClient();
    const [checkingOut, setCheckingOut] = useState(false);

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

    const totalCents = cartNotes?.reduce((sum, n) => sum + n.price, 0) ?? 0;

    const handleRemove = async (itemId: string) => {
        try {
            await removeFromCart(itemId);
            queryClient.invalidateQueries({ queryKey: ["cart"] });
            toast({ title: "Removed from cart" });
        } catch {
            toast({ title: "Failed to remove item", variant: "destructive" });
        }
    };

    const handleCheckout = async () => {
        setCheckingOut(true);
        try {
            const key = crypto.randomUUID();
            const res = await checkoutCart(key);
            if (res.status === "fulfilled") {
                toast({ title: "Purchase complete!", description: `${res.purchased_count} note(s) purchased.` });
                // Await critical invalidations so My Notes page has fresh data when we redirect
                await Promise.all([
                    queryClient.invalidateQueries({ queryKey: ["cart"] }),
                    queryClient.invalidateQueries({ queryKey: ["purchaseHistory"] }),
                    queryClient.invalidateQueries({ queryKey: ["myPurchases"] }),
                    queryClient.invalidateQueries({ queryKey: ["purchaseStatus"] }),
                    queryClient.invalidateQueries({ queryKey: ["note"] }),
                ]);
                router.push("/purchases");
            } else if (res.client_secret) {
                // Stripe payment flow would go here
                toast({ title: "Payment initiated", description: "Complete payment to finalize." });
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
                    <div className="space-y-2">
                        {cartNotes?.map((note) => (
                            <Card key={note.id} className="border-border">
                                <CardContent className="flex items-center gap-3 p-3">
                                    <div className="flex-1 min-w-0">
                                        <Link
                                            href={`/notes/${note.id}`}
                                            className="font-medium text-sm hover:text-primary"
                                        >
                                            {note.title}
                                        </Link>
                                        <p className="text-xs text-muted-foreground">
                                            by {note.author}
                                        </p>
                                    </div>
                                    <span className="font-semibold text-sm">
                                        {formatPrice(note.price)}
                                    </span>
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

                    <Separator className="my-4" />

                    {/* Checkout summary */}
                    <Card className="border-border">
                        <CardHeader className="py-3 px-4">
                            <CardTitle className="text-sm">Order Summary</CardTitle>
                        </CardHeader>
                        <CardContent className="px-4 pb-4 pt-0">
                            <div className="flex justify-between text-sm mb-3">
                                <span className="text-muted-foreground">
                                    {cartItemIds.length} item{cartItemIds.length > 1 ? "s" : ""}
                                </span>
                                <span className="font-bold text-lg">
                                    {formatPrice(totalCents)}
                                </span>
                            </div>
                            <Button
                                className="w-full gap-2"
                                onClick={handleCheckout}
                                disabled={checkingOut}
                            >
                                {checkingOut ? (
                                    <Loader2 className="h-4 w-4 animate-spin" />
                                ) : (
                                    <CreditCard className="h-4 w-4" />
                                )}
                                Checkout
                            </Button>
                        </CardContent>
                    </Card>
                </>
            )}
        </div>
    );
}
