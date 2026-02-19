// purchases.ts — Purchase & cart API service.
import { apiDelete, apiGet, apiPost } from "@/lib/api-client";
import type {
    CartResponse,
    CheckoutResponse,
    MyPurchasesResponse,
    OrderStatusResponse,
    PaginationParams,
    PurchaseHistoryResponse,
    PurchaseStatusResponse,
    SinglePurchaseResponse,
} from "@/types";

// ─── Cart ─────────────────────────────────────────────────────────────────────

/** GET /cart — Get current cart items. */
export function getCart(): Promise<CartResponse> {
    return apiGet("/cart");
}

/** POST /cart — Add a note to cart. */
export function addToCart(noteId: number): Promise<{ message: string }> {
    return apiPost("/cart", { item_id: String(noteId) });
}

/** DELETE /cart/:itemId — Remove a note from cart. */
export function removeFromCart(itemId: string): Promise<{ message: string }> {
    return apiDelete(`/cart/${itemId}`);
}

// ─── Checkout ─────────────────────────────────────────────────────────────────

/** POST /checkout — Checkout the entire cart. */
export function checkoutCart(
    idempotencyKey?: string
): Promise<CheckoutResponse> {
    const body: { idempotency_key?: string } = {};
    if (idempotencyKey) body.idempotency_key = idempotencyKey;
    return apiPost("/checkout", body);
}

/** POST /notes/:id/purchase — Direct purchase of a single note. */
export function purchaseNote(
    noteId: number
): Promise<SinglePurchaseResponse> {
    return apiPost(`/notes/${noteId}/purchase`);
}

// ─── Purchase Status ──────────────────────────────────────────────────────────

/** GET /notes/:id/purchased — Check if user purchased a specific note. */
export function checkPurchaseStatus(
    noteId: number
): Promise<PurchaseStatusResponse> {
    return apiGet(`/notes/${noteId}/purchased`);
}

/** GET /me/purchases — Get all purchased notes. */
export function getMyPurchases(): Promise<MyPurchasesResponse> {
    return apiGet("/me/purchases");
}

/** GET /me/purchases/history — Paginated purchase history. */
export function getPurchaseHistory(
    params?: PaginationParams
): Promise<PurchaseHistoryResponse> {
    const query = new URLSearchParams();
    if (params?.page) query.set("page", String(params.page));
    if (params?.limit) query.set("limit", String(params.limit));
    const qs = query.toString();
    return apiGet(`/me/purchases/history${qs ? `?${qs}` : ""}`);
}

// ─── Orders ───────────────────────────────────────────────────────────────────

/** GET /orders/:orderId — Check order status. */
export function getOrderStatus(
    orderId: number
): Promise<OrderStatusResponse> {
    return apiGet(`/orders/${orderId}`);
}

/** POST /orders/:orderId/confirm — Manual reconciliation. */
export function confirmOrder(orderId: number): Promise<unknown> {
    return apiPost(`/orders/${orderId}/confirm`);
}
