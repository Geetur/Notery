// stripe-checkout.tsx — Stripe payment confirmation dialog.
// Wraps Stripe PaymentElement for confirming payments when the backend returns a client_secret.
"use client";

import { Button } from "@/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogHeader,
    DialogTitle,
} from "@/components/ui/dialog";
import { STRIPE_PUBLISHABLE_KEY } from "@/lib/config";
import { formatPrice } from "@/lib/format";
import {
    Elements,
    PaymentElement,
    useElements,
    useStripe,
} from "@stripe/react-stripe-js";
import { loadStripe } from "@stripe/stripe-js";
import { AlertCircle, Loader2, Lock } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";

// Stripe singleton — loaded once per app lifecycle.
const stripePromise = STRIPE_PUBLISHABLE_KEY
    ? loadStripe(STRIPE_PUBLISHABLE_KEY)
    : null;

// ─── Props ────────────────────────────────────────────────────────────────────

interface StripeCheckoutProps {
    open: boolean;
    onClose: () => void;
    clientSecret: string;
    totalCents: number;
    orderId: number;
    /** Called after payment confirmation succeeds so the parent can poll or refetch. */
    onPaymentSuccess: (orderId: number) => void;
}

// ─── Inner form (must be inside Elements provider) ────────────────────────────

function CheckoutForm({
    totalCents,
    orderId,
    onPaymentSuccess,
    onClose,
}: {
    totalCents: number;
    orderId: number;
    onPaymentSuccess: (orderId: number) => void;
    onClose: () => void;
}) {
    const stripe = useStripe();
    const elements = useElements();
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [elementReady, setElementReady] = useState(false);

    const handleSubmit = useCallback(
        async (e: React.FormEvent) => {
            e.preventDefault();
            if (!stripe || !elements) return;

            setLoading(true);
            setError(null);

            const result = await stripe.confirmPayment({
                elements,
                confirmParams: {
                    return_url: `${window.location.origin}/payment/confirm?order_id=${orderId}`,
                },
                redirect: "if_required",
            });

            if (result.error) {
                setError(
                    result.error.message ?? "Payment failed. Please try again."
                );
                setLoading(false);
            } else if (
                result.paymentIntent?.status === "succeeded" ||
                result.paymentIntent?.status === "processing"
            ) {
                onPaymentSuccess(orderId);
            } else {
                setError("Unexpected payment status. Please check your order.");
                setLoading(false);
            }
        },
        [stripe, elements, orderId, onPaymentSuccess]
    );

    return (
        <form onSubmit={handleSubmit} className="space-y-4">
            {!elementReady && (
                <div className="flex items-center justify-center py-8">
                    <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
                    <span className="ml-2 text-sm text-muted-foreground">Loading payment form…</span>
                </div>
            )}
            <div className={elementReady ? "" : "hidden"}>
                <PaymentElement
                    options={{
                        layout: "tabs",
                    }}
                    onReady={() => setElementReady(true)}
                />
            </div>

            {error && (
                <div className="flex items-center gap-2 text-sm text-destructive bg-destructive/10 p-3 rounded-md">
                    <AlertCircle className="h-4 w-4 shrink-0" />
                    <span>{error}</span>
                </div>
            )}

            <div className="flex items-center justify-between pt-2">
                <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={onClose}
                    disabled={loading}
                >
                    Cancel
                </Button>
                <Button type="submit" disabled={!stripe || !elementReady || loading} size="sm">
                    {loading ? (
                        <Loader2 className="h-4 w-4 animate-spin mr-1.5" />
                    ) : (
                        <Lock className="h-4 w-4 mr-1.5" />
                    )}
                    Pay {formatPrice(totalCents)}
                </Button>
            </div>

            <p className="text-[11px] text-muted-foreground text-center">
                Payments are securely processed by Stripe.
            </p>
        </form>
    );
}

// ─── Main component ───────────────────────────────────────────────────────────

export function StripeCheckout({
    open,
    onClose,
    clientSecret,
    totalCents,
    orderId,
    onPaymentSuccess,
}: StripeCheckoutProps) {
    const [paymentComplete, setPaymentComplete] = useState(false);

    const handleSuccess = useCallback(
        (id: number) => {
            setPaymentComplete(true);
            onPaymentSuccess(id);
        },
        [onPaymentSuccess]
    );

    // Reset paymentComplete when the dialog reopens with a new clientSecret
    useEffect(() => {
        if (open) setPaymentComplete(false);
    }, [open, clientSecret]);

    const handleClose = useCallback(() => {
        setPaymentComplete(false);
        onClose();
    }, [onClose]);

    const elementsOptions = useMemo(
        () => ({
            clientSecret,
            appearance: {
                theme: "stripe" as const,
                variables: {
                    colorPrimary: "#f97316",
                    borderRadius: "6px",
                },
            },
        }),
        [clientSecret]
    );

    if (!stripePromise) {
        return (
            <Dialog open={open} onOpenChange={handleClose}>
                <DialogContent className="sm:max-w-md">
                    <DialogHeader>
                        <DialogTitle>Payment Unavailable</DialogTitle>
                        <DialogDescription>
                            Payment processing is not configured. Please contact
                            support.
                        </DialogDescription>
                    </DialogHeader>
                </DialogContent>
            </Dialog>
        );
    }

    return (
        <Dialog open={open} onOpenChange={handleClose}>
            <DialogContent className="sm:max-w-md">
                <DialogHeader>
                    <DialogTitle>
                        {paymentComplete
                            ? "Payment Successful!"
                            : "Complete Payment"}
                    </DialogTitle>
                    <DialogDescription>
                        {paymentComplete
                            ? "Your purchase has been confirmed."
                            : `Total: ${formatPrice(totalCents)}`}
                    </DialogDescription>
                </DialogHeader>

                {paymentComplete ? (
                    <div className="flex flex-col items-center gap-3 py-4">
                        <div className="h-12 w-12 rounded-full bg-green-500/20 flex items-center justify-center">
                            <svg
                                className="h-6 w-6 text-green-500"
                                fill="none"
                                viewBox="0 0 24 24"
                                stroke="currentColor"
                                strokeWidth={2}
                            >
                                <path
                                    strokeLinecap="round"
                                    strokeLinejoin="round"
                                    d="M5 13l4 4L19 7"
                                />
                            </svg>
                        </div>
                        <p className="text-sm text-muted-foreground">
                            You now have full access to your purchased notes.
                        </p>
                        <Button size="sm" onClick={handleClose}>
                            Done
                        </Button>
                    </div>
                ) : (
                    <Elements stripe={stripePromise} options={elementsOptions}>
                        <CheckoutForm
                            totalCents={totalCents}
                            orderId={orderId}
                            onPaymentSuccess={handleSuccess}
                            onClose={handleClose}
                        />
                    </Elements>
                )}
            </DialogContent>
        </Dialog>
    );
}
