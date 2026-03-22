// page.tsx — Post-payment redirect handler for Stripe 3D Secure / redirect-based payments.
// Stripe redirects here after confirmPayment() with redirect: "if_required".
// Reads payment_intent + payment_intent_client_secret from URL params,
// polls order status, and triggers reconciliation via the confirm endpoint.
"use client";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { STRIPE_PUBLISHABLE_KEY } from "@/lib/config";
import { confirmOrder, getOrderStatus } from "@/services/purchases";
import { loadStripe } from "@stripe/stripe-js";
import { CheckCircle, Loader2, XCircle } from "lucide-react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { Suspense, useCallback, useEffect, useState } from "react";

const stripePromise = STRIPE_PUBLISHABLE_KEY
    ? loadStripe(STRIPE_PUBLISHABLE_KEY)
    : null;

type Status = "loading" | "success" | "processing" | "failed";

function PaymentConfirmInner() {
    const searchParams = useSearchParams();
    const [status, setStatus] = useState<Status>("loading");
    const [message, setMessage] = useState("");

    const orderId = Number(searchParams.get("order_id") || 0);
    const clientSecret = searchParams.get("payment_intent_client_secret");

    const checkPaymentStatus = useCallback(async () => {
        if (!clientSecret || !stripePromise) {
            setStatus("failed");
            setMessage("Missing payment information.");
            return;
        }

        const stripe = await stripePromise;
        if (!stripe) {
            setStatus("failed");
            setMessage("Payment service unavailable.");
            return;
        }

        const { paymentIntent } = await stripe.retrievePaymentIntent(clientSecret);
        if (!paymentIntent) {
            setStatus("failed");
            setMessage("Could not retrieve payment status.");
            return;
        }

        switch (paymentIntent.status) {
            case "succeeded":
                // Trigger backend reconciliation if we have an order ID
                if (orderId > 0) {
                    try {
                        await confirmOrder(orderId);
                    } catch {
                        // Webhook may have already fulfilled — poll status instead
                    }
                    // Poll until fulfilled (max 10 attempts)
                    for (let i = 0; i < 10; i++) {
                        try {
                            const orderStatus = await getOrderStatus(orderId);
                            if (orderStatus.status === "fulfilled") {
                                setStatus("success");
                                setMessage("Payment successful! Your notes are now available.");
                                return;
                            }
                        } catch {
                            // ignore poll errors
                        }
                        await new Promise((r) => setTimeout(r, 1500));
                    }
                }
                setStatus("success");
                setMessage("Payment successful! Your notes should be available shortly.");
                break;
            case "processing":
                setStatus("processing");
                setMessage("Your payment is being processed. This may take a moment.");
                break;
            default:
                setStatus("failed");
                setMessage("Payment was not completed. Please try again.");
                break;
        }
    }, [clientSecret, orderId]);

    useEffect(() => {
        checkPaymentStatus();
    }, [checkPaymentStatus]);

    return (
        <div className="max-w-md mx-auto px-4 py-12">
            <Card className="border-border">
                <CardHeader className="text-center pb-2">
                    <CardTitle>
                        {status === "loading" && "Confirming Payment..."}
                        {status === "success" && "Payment Confirmed"}
                        {status === "processing" && "Processing Payment"}
                        {status === "failed" && "Payment Issue"}
                    </CardTitle>
                </CardHeader>
                <CardContent className="flex flex-col items-center gap-4 pt-4">
                    {status === "loading" && (
                        <Loader2 className="h-10 w-10 animate-spin text-primary" />
                    )}
                    {status === "success" && (
                        <div className="h-12 w-12 rounded-full bg-green-500/20 flex items-center justify-center">
                            <CheckCircle className="h-6 w-6 text-green-500" />
                        </div>
                    )}
                    {status === "processing" && (
                        <Loader2 className="h-10 w-10 animate-spin text-yellow-500" />
                    )}
                    {status === "failed" && (
                        <div className="h-12 w-12 rounded-full bg-destructive/20 flex items-center justify-center">
                            <XCircle className="h-6 w-6 text-destructive" />
                        </div>
                    )}

                    <p className="text-sm text-muted-foreground text-center">
                        {message}
                    </p>

                    <div className="flex gap-2 pt-2">
                        {status === "success" && (
                            <Button asChild>
                                <Link href="/purchases">View My Notes</Link>
                            </Button>
                        )}
                        {status === "failed" && (
                            <>
                                <Button variant="outline" asChild>
                                    <Link href="/cart">Back to Cart</Link>
                                </Button>
                                <Button asChild>
                                    <Link href="/">Browse Notes</Link>
                                </Button>
                            </>
                        )}
                        {status === "processing" && (
                            <Button variant="outline" asChild>
                                <Link href="/purchases">Check My Notes</Link>
                            </Button>
                        )}
                    </div>
                </CardContent>
            </Card>
        </div>
    );
}

export default function PaymentConfirmPage() {
    return (
        <Suspense
            fallback={
                <div className="max-w-md mx-auto px-4 py-12 flex justify-center">
                    <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
                </div>
            }
        >
            <PaymentConfirmInner />
        </Suspense>
    );
}
