// payouts.ts — Stripe Connect payout API service.
import { apiGet, apiPost } from "@/lib/api-client";
import type { StripeConnectResponse, StripeStatusResponse } from "@/types";

/** POST /me/stripe/connect — Start Stripe Connect onboarding. */
export function setupStripeConnect(): Promise<StripeConnectResponse> {
    return apiPost("/me/stripe/connect");
}

/** GET /me/stripe/status — Get Stripe Connect account status. */
export function getStripeStatus(): Promise<StripeStatusResponse> {
    return apiGet("/me/stripe/status");
}

/** POST /me/stripe/refresh-link — Get a new Stripe onboarding link. */
export function refreshStripeLink(): Promise<StripeConnectResponse> {
    return apiPost("/me/stripe/refresh-link");
}
