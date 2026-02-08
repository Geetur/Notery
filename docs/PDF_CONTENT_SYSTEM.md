# PDF Content System Architecture

## Overview

This document explains how the PDF book content system works in Notery. The system is designed to:
1. **Securely store PDFs** in Cloudflare R2 (S3-compatible storage)
2. **Prevent unauthorized downloads** by proxying content through authenticated endpoints
3. **Allow in-app viewing only** — users view PDFs through a frontend viewer, not direct downloads
4. **Support concurrent updates** — if a creator updates their PDF, all buyers see the new version
5. **Enforce creator ownership** — only the note creator (or an admin) can upload PDF content

---

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              NOTERY API                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐        │
│  │   Auth Layer     │    │ Content Handler  │    │Purchase Handler │        │
│  │  (middleware)    │───▶│  (content.go)    │◀──│ (purchase.go)   │        │
│  └─────────────────┘    └────────┬─────────┘    └────────┬────────┘        │
│                                  │                       │                  │
│                                  │ Access Control        │ Order ──▶       │
│                                  ▼                       │ Purchase         │
│  ┌───────────────────────────────────────────────────────┴───────────┐     │
│  │                    R2Client (database/r2.go)                       │     │
│  │  - UploadPDF(noteID, content)                                     │     │
│  │  - GetPDFContent(noteID) → streams to user                       │     │
│  │  - DeletePDF(noteID)                                              │     │
│  └────────────────────────────────┬──────────────────────────────────┘     │
│                                   │                                         │
└───────────────────────────────────┼─────────────────────────────────────────┘
                                    │
                                    │ S3 API (private)
                                    ▼
                    ┌───────────────────────────────┐
                    │      Cloudflare R2 Bucket      │
                    │   (notery-pdfs — PRIVATE)      │
                    │                                │
                    │  notes/1/content.pdf           │
                    │  notes/2/content.pdf           │
                    │  ...                           │
                    └───────────────────────────────┘
```

---

## Key Components

### 1. R2 Storage Layer (`internal/database/r2.go`)

The R2 client wraps Cloudflare R2 (S3-compatible) operations:

```go
type R2Client struct {
    S3Client      *s3.Client
    PresignClient *s3.PresignClient
    BucketName    string
}
```

**Key Methods:**
- `UploadPDF(ctx, noteID, content, size)` — Stores PDF in R2
- `GetPDFContent(ctx, noteID)` — Retrieves PDF stream for proxying
- `DeletePDF(ctx, noteID)` — Removes PDF when note is rejected/deleted
- `PDFExists(ctx, noteID)` — Checks if PDF exists

**Object Key Format:** `notes/{noteID}/content.pdf`

### 2. Note Model (`internal/models/note.go`)

PDF-related fields on the `Note` struct:

```go
type Note struct {
    // ...
    Price         int64      `json:"price"`          // In cents (499 = $4.99)
    Status        NoteStatus `json:"status"`         // StatusPending | StatusApproved | StatusRejected
    HasPDF        bool       `json:"has_pdf"`
    PDFSize       int64      `json:"pdf_size"`
    PDFUploadedAt *time.Time `json:"pdf_uploaded_at"`
    CreatorID     uint64     `json:"creator_id"`     // Used for upload authorisation
}
```

### 3. Purchase Model (`internal/models/purchase.go`)

Tracks who bought what:

```go
type Purchase struct {
    ID          uint      `json:"id"`
    UserID      uint      `json:"user_id"`
    NoteID      uint      `json:"note_id"`
    PricePaid   int64     `json:"price_paid"`    // Cents at purchase time
    PurchasedAt time.Time `json:"purchased_at"`
}
```

### 4. Order Model (`internal/models/order.go`)

Orders capture the payment session before purchases are created:

```go
type Order struct {
    ID              uint        `json:"id"`
    UserID          uint64      `json:"user_id"`
    Status          OrderStatus `json:"status"`          // pending → paid → fulfilled
    TotalCents      int64       `json:"total_cents"`
    IdempotencyKey  string      `json:"idempotency_key"` // Per-user composite unique
    PaymentIntentID string      `json:"payment_intent_id,omitempty"`
    Items           []OrderItem `json:"items,omitempty"`
}
```

**Order lifecycle:**

```
pending ──▶ paid ──▶ fulfilled
   │                     │
   └──▶ failed     refunded
```

### 5. Content Handler (`internal/handlers/content.go`)

Manages PDF operations with access control:

**Access Control Matrix:**

| User Type       | Upload (Pending) | View Pending   | View Approved        |
| --------------- | ---------------- | -------------- | -------------------- |
| Anonymous       | No               | No             | No                   |
| Authenticated   | No               | No             | Only if purchased    |
| Note Creator    | **Yes**          | Yes            | Yes                  |
| Subnotery Admin | Yes              | Yes (their sub)| Yes                  |
| Global Admin    | Yes              | Yes (all)      | Yes                  |

> **Upload authorisation** — `UploadNotePDF` now verifies `note.CreatorID == userID` or admin access via `CheckNoteAccess()`. Anonymous or unrelated users are rejected with 403.

### 6. Purchase Handler (`internal/handlers/purchase.go`)

Handles the purchase flow:
- `CheckoutCart` — Process cart items, create Order + OrderItems + Purchases in a DB transaction
- `PurchaseSingleNote` — Direct "Buy Now" purchase
- `CheckPurchaseStatus` — Check if user owns a note
- `GetPurchaseHistory` — Paginated purchase history

---

## Data Flow

### Flow 1: Note Creator Uploads PDF

```
1. User creates note metadata (POST /api/v1/notes)
   → Note created with Status: StatusPending, HasPDF: false

2. User uploads PDF (POST /api/v1/notes/:id/content)
   → Handler verifies: note is pending AND user is creator (or admin)
   → PDF stored in R2 as "notes/{id}/content.pdf"
   → Note updated: HasPDF: true, PDFSize: X, PDFUploadedAt: now

3. Note waits for admin approval
```

### Flow 2: Admin Reviews & Approves

```
1. Admin fetches pending notes (GET /api/v1/notes/pending)
   → Subnotery admins see only their community's notes
   → Global admins see all pending notes

2. Admin previews PDF (GET /api/v1/admin/notes/:id/preview)
   → Middleware resolves subnotery scope from note's SubnoteryID
   → PDF streamed from R2 through API (not direct R2 URL)

3. Admin approves (PATCH /api/v1/notes/:id/approve)
   → Validates HasPDF: true (required for approval)
   → Status changed to StatusApproved
   → Note indexed in Meilisearch, added to hot feed
```

### Flow 3: User Purchases & Views Note

```
1. User finds approved note (via search or hot feed)

2. User adds to cart (POST /api/v1/cart)
   → Validates note status is StatusApproved
   → Item added to Redis set: "cart:{userID}"

3. User checkouts (POST /api/v1/checkout)
   → Creates Order (pending) + OrderItems in a DB transaction
   → Validates all items are approved
   → Transitions Order → paid → fulfilled
   → Creates Purchase records
   → Clears cart

4. User views purchased note (GET /api/v1/notes/:id/content)
   → Access control checks Purchase record exists
   → PDF streamed from R2 through API
   → Response headers enforce inline viewing
```

### Flow 4: Voting

```
1. User votes (POST /api/v1/notes/:id/upvote)
   → DB transaction: check votes table → insert/update/delete vote + update note counters
   → Redis vote cache updated (best-effort) after commit
   → Hotness recalculated (Reddit-style algorithm)
   → Note's position in Redis sorted sets updated
```

---

## Security Design

### Why Proxy Instead of Presigned URLs?

We could give users time-limited presigned URLs directly to R2, but:

1. **Users could share URLs** — Even short-lived URLs can be shared while valid
2. **No download prevention** — Direct URLs allow saving the file
3. **No viewing control** — Can't enforce our PDF viewer UI

By proxying:
- All access goes through our authentication
- We set `Content-Disposition: inline` to discourage downloads
- We can track who views what
- Frontend can use a viewer that disables download (e.g., PDF.js)

### Response Headers

When serving PDFs:
```
Content-Type: application/pdf
Content-Disposition: inline              # View in browser, not download
Cache-Control: no-store, no-cache, ...   # Prevent caching
X-Frame-Options: SAMEORIGIN              # Only embed on our domain
```

### Upload Authorization

The `UploadNotePDF` handler enforces:
1. Note must be in `StatusPending` (prevents bait-and-switch after approval)
2. Uploader must be the note's `CreatorID`, or a subnotery admin, or a global admin

This prevents content hijacking where a malicious authenticated user could replace another creator's pending PDF.

### Admin Middleware Scoping

`RequireAdmin` resolves the subnotery scope from either:
- `:subnotery_id` URL parameter (for subnotery-management routes)
- The note's `SubnoteryID` looked up from `:id` (for note-management routes)

The parsed subnotery ID is assigned and enforced — a subnotery admin for community A cannot act on community B's resources. Global admins bypass this check entirely.

---

## API Endpoints

### Protected Endpoints (Require Auth)

| Method | Path                            | Description                        |
| ------ | ------------------------------- | ---------------------------------- |
| POST   | `/api/v1/notes/:id/content`     | Upload PDF (creator/admin only)    |
| GET    | `/api/v1/notes/:id/content`     | View/stream PDF (purchase/admin)   |
| POST   | `/api/v1/checkout`              | Checkout cart → create order       |
| POST   | `/api/v1/notes/:id/purchase`    | Direct purchase (bypass cart)      |
| GET    | `/api/v1/notes/:id/purchased`   | Check if user purchased note       |
| GET    | `/api/v1/me/purchases`          | Get all purchased notes            |
| GET    | `/api/v1/me/purchases/history`  | Paginated purchase history         |
| POST   | `/api/v1/notes/:id/upvote`      | Upvote a note                      |
| POST   | `/api/v1/notes/:id/downvote`    | Downvote a note                    |

### Admin Endpoints (Require Admin Auth)

| Method | Path                                  | Description             |
| ------ | ------------------------------------- | ----------------------- |
| GET    | `/api/v1/admin/notes/:id/preview`     | Preview pending note PDF |
| DELETE | `/api/v1/admin/notes/:id/content`     | Delete PDF only          |

---

## Environment Variables

Add these to your `.env`:

```env
# Cloudflare R2 Configuration
R2_ACCOUNT_ID=your_cloudflare_account_id
R2_ACCESS_KEY_ID=your_r2_access_key
R2_SECRET_ACCESS_KEY=your_r2_secret_key
R2_BUCKET_NAME=notery-pdfs
```

---

## Frontend Integration Guide

### Recommended PDF Viewer: PDF.js

Use Mozilla's PDF.js to render PDFs without native download UI:

```javascript
// Example with react-pdf (PDF.js wrapper)
import { Document, Page } from 'react-pdf';

function NoteViewer({ noteId, authToken }) {
  return (
    <Document
      file={{
        url: `/api/v1/notes/${noteId}/content`,
        httpHeaders: { Authorization: `Bearer ${authToken}` }
      }}
      options={{ disableTextLayer: true }}
    >
      <Page pageNumber={1} />
    </Document>
  );
}
```

### Key Frontend Considerations

1. **No download button** — Don't expose a download feature
2. **Disable right-click** — Prevent "Save As" context menu
3. **Disable print** — Or watermark printed pages
4. **Disable text selection** — Prevents copy-paste of content
5. **Session timeout** — Re-authenticate after inactivity

---

## Testing the Flow

### 1. Create a Note with PDF

```bash
# Create note metadata (price in cents: 999 = $9.99)
curl -X POST http://localhost:8080/api/v1/notes \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"subnotery_name": "CS101", "title": "Algorithms Notes", "author": "John", "price": 999}'

# Upload PDF (only the note creator can do this)
curl -X POST http://localhost:8080/api/v1/notes/1/content \
  -H "Authorization: Bearer $TOKEN" \
  -F "pdf=@/path/to/your/notes.pdf"
```

### 2. Admin Approves

```bash
# Preview PDF (as admin — middleware resolves subnotery scope from note)
curl http://localhost:8080/api/v1/admin/notes/1/preview \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  --output preview.pdf

# Approve
curl -X PATCH http://localhost:8080/api/v1/notes/1/approve \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

### 3. User Purchases and Views

```bash
# Purchase (creates Order → Purchase in one transaction)
curl -X POST http://localhost:8080/api/v1/notes/1/purchase \
  -H "Authorization: Bearer $USER_TOKEN"

# View (streams PDF through proxy)
curl http://localhost:8080/api/v1/notes/1/content \
  -H "Authorization: Bearer $USER_TOKEN" \
  --output viewed.pdf
```

---

## Future Enhancements

1. **Payment Integration** — Integrate Stripe/PayPal; Order stays `pending` until webhook confirms payment
2. **Watermarking** — Add user-specific watermarks to PDFs at view time
3. **View Analytics** — Track viewing patterns (time spent, pages viewed)
4. **Subscription Model** — Time-limited access with `ExpiresAt` field
5. **Refunds** — Transition Order to `refunded` and invalidate Purchase records
6. **Version History** — Track PDF updates, allow viewing previous versions
