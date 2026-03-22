# Notery — API Reference

Full endpoint reference for the Notery REST API. All routes are prefixed with `/api/v1` unless noted.

---

## Public (14 routes)

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/health` | Health check |
| `POST` | `/api/v1/auth/signup` | Register + issue tokens |
| `POST` | `/api/v1/auth/login` | Authenticate + issue tokens |
| `POST` | `/api/v1/auth/refresh` | Rotate refresh token |
| `POST` | `/api/v1/auth/logout` | Revoke refresh token |
| `POST` | `/api/v1/auth/forgot-password` | Request password reset email |
| `POST` | `/api/v1/auth/reset-password` | Reset password with token |
| `GET` | `/api/v1/auth/verify-email` | Verify email via token |
| `GET` | `/api/v1/auth/oauth/providers` | List configured OAuth providers |
| `GET` | `/api/v1/auth/oauth/google` | Start Google OAuth flow |
| `GET` | `/api/v1/auth/oauth/google/callback` | Google OAuth callback |
| `GET` | `/api/v1/auth/oauth/github` | Start GitHub OAuth flow |
| `GET` | `/api/v1/auth/oauth/github/callback` | GitHub OAuth callback |
| `POST` | `/api/v1/webhooks/stripe` | Stripe webhook (signature-verified) |

## Public with Optional Auth (11 routes)

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/feed/hot` | Hot feed (personalised votes if authenticated) |
| `GET` | `/api/v1/notes/:id/comments` | Threaded comments tree |
| `GET` | `/api/v1/comments/:comment_id` | Single comment subtree |
| `GET` | `/api/v1/users/:id/profile` | Public user profile (includes notoriety) |
| `GET` | `/api/v1/users/:id/avatar` | Public avatar proxy (24h cache) |
| `GET` | `/api/v1/search` | Multi-type search (`?q=&type=notes\|subnoteries\|users\|comments`) |
| `GET` | `/api/v1/subnoteries` | List all subnoteries (paginated) |
| `GET` | `/api/v1/subnoteries/:id` | Subnotery detail (admins, member count, min notoriety) |
| `GET` | `/api/v1/subnoteries/:id/notes` | Approved notes in subnotery (paginated, sortable) |
| `GET` | `/api/v1/notes/:id/thumbnail` | Note thumbnail image (24h cache) |
| `GET` | `/api/v1/subnoteries/:id/banner` | Subnotery banner image (24h cache) |

## Auth-Only (14 routes)

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/v1/auth/logout-all` | Revoke all refresh tokens |
| `POST` | `/api/v1/auth/resend-verification` | Resend verification email |
| `GET` | `/api/v1/notes/:id` | Get note by ID |
| `GET` | `/api/v1/notes/approved` | List approved notes (paginated) |
| `GET` | `/api/v1/notes/:id/content` | View/stream full PDF |
| `GET` | `/api/v1/notes/:id/preview` | Preview PDF (truncated: 1 page per 5 total) |
| `GET` | `/api/v1/cart` | View cart |
| `GET` | `/api/v1/notes/:id/purchased` | Check purchase status |
| `GET` | `/api/v1/me/purchases` | My purchased notes |
| `GET` | `/api/v1/me/purchases/history` | Purchase history (paginated) |
| `GET` | `/api/v1/me/profile` | Own profile (includes notoriety) |
| `GET` | `/api/v1/me/notes` | Own notes (filterable by status) |
| `GET` | `/api/v1/me/comments` | Own comments (flat, paginated) |
| `GET` | `/api/v1/orders/:order_id` | Order status |

## Verified — Requires JWT + Verified Email (25 routes)

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/v1/notes` | Create note (respects min post notoriety) |
| `POST` | `/api/v1/notes/:id/content` | Upload PDF |
| `POST` | `/api/v1/notes/:id/thumbnail` | Upload thumbnail image |
| `DELETE` | `/api/v1/notes/:id/thumbnail` | Delete thumbnail |
| `POST` | `/api/v1/notes/:id/upvote` | Upvote note |
| `POST` | `/api/v1/notes/:id/downvote` | Downvote note |
| `POST` | `/api/v1/cart` | Add to cart |
| `DELETE` | `/api/v1/cart/:item_id` | Remove from cart |
| `POST` | `/api/v1/checkout` | Checkout cart |
| `POST` | `/api/v1/notes/:id/purchase` | Direct purchase |
| `PATCH` | `/api/v1/me/profile` | Update own profile |
| `POST` | `/api/v1/me/avatar` | Upload avatar (≤ 5 MB) |
| `DELETE` | `/api/v1/me/avatar` | Delete avatar |
| `POST` | `/api/v1/notes/:id/comments` | Create comment (respects min comment notoriety) |
| `PUT` | `/api/v1/comments/:comment_id` | Edit comment |
| `DELETE` | `/api/v1/comments/:comment_id` | Soft-delete comment |
| `POST` | `/api/v1/comments/:comment_id/vote` | Vote on comment |
| `DELETE` | `/api/v1/comments/:comment_id/vote` | Remove comment vote |
| `POST` | `/api/v1/orders/:order_id/confirm` | Manual order reconciliation |
| `POST` | `/api/v1/subnoteries/:id/join` | Join subnotery |
| `POST` | `/api/v1/subnoteries/:id/leave` | Leave subnotery (admin succession) |
| `PATCH` | `/api/v1/subnoteries/:id/settings` | Update subnotery settings (admin only) |
| `POST` | `/api/v1/subnoteries/:id/banner` | Upload subnotery banner (admin only, ≤ 5 MB) |
| `DELETE` | `/api/v1/subnoteries/:id/banner` | Delete subnotery banner (admin only) |
| `DELETE` | `/api/v1/subnoteries/:id/admins/:uid` | Remove admin (hierarchy-based) |

## Admin (9 routes)

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/notes/pending` | List pending notes (paginated) |
| `PATCH` | `/api/v1/notes/:id/approve` | Approve note |
| `PATCH` | `/api/v1/notes/:id/reject` | Reject note |
| `PATCH` | `/api/v1/notes/:id/lock` | Lock note (disables comments) |
| `PATCH` | `/api/v1/notes/:id/unlock` | Unlock note |
| `DELETE` | `/api/v1/notes/:id` | Delete note |
| `GET` | `/api/v1/admin/notes/:id/preview` | Preview PDF during approval |
| `DELETE` | `/api/v1/admin/notes/:id/content` | Delete PDF content |
| `POST` | `/api/v1/subnoteries/:id/admins` | Add subnotery admin |
