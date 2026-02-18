// api.ts — TypeScript types mirroring the Notery Go API models and response shapes.
// Every type here corresponds to a model in internal/models/ or a response shape
// from internal/handlers/. Prices are always in cents (499 = $4.99).

// ─── Enums / Constants ────────────────────────────────────────────────────────

export type NoteStatus = "Pending" | "Approved" | "Rejected";
export type VoteDirection = "up" | "down";
export type OrderStatus = "pending" | "paid" | "fulfilled" | "failed" | "refunded";
export type ProfileVisibility = "public" | "private";
export type CommentSortOrder = "best" | "new" | "top" | "controversial" | "old";
export type SearchType = "notes" | "subnoteries" | "users" | "comments";

// ─── Domain Models ────────────────────────────────────────────────────────────

export interface Note {
    id: number;
    creator_id: number;
    title: string;
    author: string;
    status: NoteStatus;
    subnotery_id: number;
    price: number; // cents
    has_pdf: boolean;
    pdf_size: number;
    pdf_uploaded_at: string | null;
    upvotes: number;
    downvotes: number;
    hotness: number;
    created_at: string;
    updated_at: string;
}

export interface Subnotery {
    id: number;
    name: string;
    admins?: User[];
    members?: User[];
    created_at: string;
    updated_at: string;
}

export interface User {
    id: number;
    username: string;
    display_name: string;
    is_global_admin: boolean;
    created_at: string;
    updated_at: string;
}

export interface Vote {
    id: number;
    user_id: number;
    note_id: number;
    direction: VoteDirection;
}

export interface Order {
    id: number;
    user_id: number;
    status: OrderStatus;
    total_cents: number;
    currency: string;
    idempotency_key: string;
    payment_intent_id?: string;
    paid_at?: string;
    failed_at?: string;
    failure_reason?: string;
    created_at: string;
    updated_at: string;
    items?: OrderItem[];
}

export interface OrderItem {
    id: number;
    order_id: number;
    note_id: number;
    price_cents: number;
}

export interface Purchase {
    id: number;
    user_id: number;
    note_id: number;
    order_id: number;
    price_paid: number; // cents
    purchased_at: string;
}

// ─── Auth ─────────────────────────────────────────────────────────────────────

export interface AuthRequest {
    email: string;
    password: string;
    username?: string;
}

export interface AuthResponse {
    access_token: string;
    refresh_token: string;
    message?: string;
    user_id?: number;
}

export interface RefreshRequest {
    refresh_token: string;
}

export interface ForgotPasswordRequest {
    email: string;
}

export interface ResetPasswordRequest {
    token: string;
    new_password: string;
}

export interface OAuthProviders {
    google: boolean;
    github: boolean;
}

// ─── Profile ──────────────────────────────────────────────────────────────────

export interface SelfProfile {
    id: number;
    email: string;
    username: string;
    display_name: string;
    bio: string;
    avatar_url: string;
    profile_visibility: ProfileVisibility;
    profile_updated_at: string | null;
    email_verified: boolean;
    created_at: string;
    updated_at: string;
}

export interface PublicProfile {
    id: number;
    username: string;
    display_name: string;
    created_at: string;
    bio?: string;
    avatar_url?: string;
}

export interface UpdateProfileRequest {
    bio?: string;
    avatar_url?: string;
    profile_visibility?: ProfileVisibility;
}

// ─── Comments ─────────────────────────────────────────────────────────────────

export interface CommentResponse {
    id: number;
    note_id: number;
    user_id: number;
    username: string;
    parent_id: number | null;
    body: string;
    upvotes: number;
    downvotes: number;
    score: number;
    depth: number;
    is_deleted: boolean;
    is_edited: boolean;
    created_at: string;
    user_vote: -1 | 0 | 1;
    children: CommentResponse[];
    has_more_replies?: boolean;
}

export interface CreateCommentRequest {
    body: string;
    parent_id?: number;
}

export interface EditCommentRequest {
    body: string;
}

export interface CommentVoteResponse {
    comment_id: number;
    upvotes: number;
    downvotes: number;
    score: number;
    user_vote: -1 | 0 | 1;
}

// ─── Feed ─────────────────────────────────────────────────────────────────────

export interface FeedResponse {
    notes: Note[];
    page: number;
    limit: number;
}

// ─── Notes ────────────────────────────────────────────────────────────────────

export interface NotesListResponse {
    notes: Note[];
    total: number;
    page: number;
    limit: number;
}

export interface VoteResponse {
    upvotes: number;
    downvotes: number;
    hotness: number;
}

export interface CreateNoteRequest {
    title: string;
    subnotery_name: string;
    price: number; // cents
}

// ─── Cart ─────────────────────────────────────────────────────────────────────

export interface CartResponse {
    cart: string[]; // note IDs as strings
}

export interface AddToCartRequest {
    note_id: number;
}

// ─── Purchase / Checkout ──────────────────────────────────────────────────────

export interface CheckoutResponse {
    order_id: number;
    status: OrderStatus;
    total_cents: number;
    purchased_count?: number;
    client_secret?: string;
    payment_intent_id?: string;
    warnings?: string[];
    idempotent?: boolean;
}

export interface SinglePurchaseResponse {
    message?: string;
    order_id: number;
    status: OrderStatus;
    note_id: number;
    note_title?: string;
    total_cents?: number;
    price_paid?: number;
    purchased_at?: string;
    client_secret?: string;
    payment_intent_id?: string;
}

export interface PurchaseStatusResponse {
    purchased: boolean;
    purchased_at?: string;
    price_paid?: number;
}

export interface PurchaseWithNote {
    purchase_id: number;
    note_id: number;
    note_title: string;
    note_author: string;
    price_paid: number;
    purchased_at: string;
    has_pdf: boolean;
}

export interface PurchaseHistoryResponse {
    purchases: PurchaseWithNote[];
    page: number;
    limit: number;
    total: number;
}

export interface MyPurchasesResponse {
    purchases: (Note & { price_paid: number; purchased_at: string })[];
}

// ─── Search ───────────────────────────────────────────────────────────────────

export interface SearchResponse<T = unknown> {
    type: SearchType;
    results: T[];
    total: number;
    page: number;
    limit: number;
}

export interface CommentSearchResult {
    id: number;
    note_id: number;
    user_id: number;
    username: string;
    body: string;
    depth: number;
    upvotes: number;
    downvotes: number;
    created_at: string;
}

// ─── Comments List Response ───────────────────────────────────────────────────

export interface CommentsListResponse {
    comments: CommentResponse[];
    total: number;
    page: number;
    limit: number;
    sort: CommentSortOrder;
    truncated: boolean;
}

// ─── Order Status ─────────────────────────────────────────────────────────────

export interface OrderStatusResponse {
    order_id: number;
    status: OrderStatus;
    total_cents: number;
    items: OrderItem[];
    created_at: string;
    paid_at?: string;
    failed_at?: string;
    failure_reason?: string;
}

// ─── Generic API Error ────────────────────────────────────────────────────────

export interface ApiError {
    error: string;
    [key: string]: unknown;
}

// ─── Pagination ───────────────────────────────────────────────────────────────

export interface PaginationParams {
    page?: number;
    limit?: number;
}

// ─── Feed Sort (UI-only, not in API) ──────────────────────────────────────────

export type FeedSort = "hot" | "new" | "top";
export type TimeFilter = "day" | "week" | "month" | "year" | "all";
export type ViewMode = "card" | "compact";
