# PDF Content System Architecture

## Overview

This document explains how the PDF book content system works in Notery. The system is designed to:
1. **Securely store PDFs** in Cloudflare R2 (S3-compatible storage)
2. **Prevent unauthorized downloads** by proxying content through authenticated endpoints
3. **Allow in-app viewing only** - users view PDFs through a frontend viewer, not direct downloads
4. **Support concurrent updates** - if a creator updates their PDF, all buyers see the new version

---

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              NOTERY API                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐        │
│  │   Auth Layer    │    │  Content Handler │    │ Purchase Handler│        │
│  │  (middleware)   │───▶│   (content.go)   │◀──│  (purchase.go)  │        │
│  └─────────────────┘    └────────┬─────────┘    └─────────────────┘        │
│                                  │                                          │
│                                  │ Access Control                           │
│                                  ▼                                          │
│  ┌───────────────────────────────────────────────────────────────────┐     │
│  │                    R2Client (database/r2.go)                       │     │
│  │  - UploadPDF(noteID, content)                                      │     │
│  │  - GetPDFContent(noteID) → streams to user                         │     │
│  │  - DeletePDF(noteID)                                               │     │
│  └────────────────────────────────┬──────────────────────────────────┘     │
│                                   │                                         │
└───────────────────────────────────┼─────────────────────────────────────────┘
                                    │
                                    │ S3 API (private)
                                    ▼
                    ┌───────────────────────────────┐
                    │      Cloudflare R2 Bucket     │
                    │   (notery-pdfs - PRIVATE)     │
                    │                               │
                    │  notes/1/content.pdf          │
                    │  notes/2/content.pdf          │
                    │  notes/3/content.pdf          │
                    │  ...                          │
                    └───────────────────────────────┘
```

---

## Key Components

### 1. R2 Storage Layer (`internal/database/r2.go`)

The R2 client wraps Cloudflare R2 (S3-compatible) operations:

```go
type R2Client struct {
    S3Client      *s3.Client      // For upload/delete operations
    PresignClient *s3.PresignClient // For generating signed URLs (internal use)
    BucketName    string
}
```

**Key Methods:**
- `UploadPDF(ctx, noteID, content, size)` - Stores PDF in R2
- `GetPDFContent(ctx, noteID)` - Retrieves PDF stream for proxying
- `DeletePDF(ctx, noteID)` - Removes PDF when note is rejected/deleted
- `PDFExists(ctx, noteID)` - Checks if PDF exists

**Object Key Format:** `notes/{noteID}/content.pdf`

### 2. Note Model Updates (`internal/models/note.go`)

Added PDF-related fields:
```go
type Note struct {
    // ... existing fields ...
    HasPDF        bool       `json:"has_pdf"`        // Whether PDF is uploaded
    PDFSize       int64      `json:"pdf_size"`       // Size in bytes
    PDFUploadedAt *time.Time `json:"pdf_uploaded_at"` // Upload timestamp
}
```

### 3. Purchase Model (`internal/models/purchase.go`)

Tracks who bought what:
```go
type Purchase struct {
    ID          uint      `json:"id"`
    UserID      uint      `json:"user_id"`      // Who bought
    NoteID      uint      `json:"note_id"`      // What they bought
    PricePaid   float64   `json:"price_paid"`   // Price at purchase time
    PurchasedAt time.Time `json:"purchased_at"` // When purchased
}
```

### 4. Content Handler (`internal/handlers/content.go`)

Manages PDF operations with access control:

**Access Control Matrix:**
| User Type        | Pending Notes | Approved Notes      |
|------------------|---------------|---------------------|
| Anonymous        | ❌ No         | ❌ No               |
| Authenticated    | ❌ No         | ✅ Only if purchased |
| Subnotery Admin  | ✅ Yes (their sub) | ✅ Yes          |
| Global Admin     | ✅ Yes (all)  | ✅ Yes              |

### 5. Purchase Handler (`internal/handlers/purchase.go`)

Handles the purchase flow:
- `CheckoutCart` - Process cart items, create purchase records
- `PurchaseSingleNote` - Direct "Buy Now" purchase
- `CheckPurchaseStatus` - Check if user owns a note
- `GetPurchaseHistory` - Paginated purchase history

---

## Data Flow

### Flow 1: Note Creator Uploads PDF

```
1. User creates note metadata (POST /api/v1/notes)
   → Note created with Status: "Pending", HasPDF: false

2. User uploads PDF (POST /api/v1/notes/:id/content)
   → PDF stored in R2 as "notes/{id}/content.pdf"
   → Note updated: HasPDF: true, PDFSize: X, PDFUploadedAt: now

3. Note waits for admin approval
```

### Flow 2: Admin Reviews & Approves

```
1. Admin fetches pending notes (GET /api/v1/notes/pending)
   → Returns notes where Status: "Pending"
   → Scoped to admin's subnoteries (or all for global admin)

2. Admin previews PDF (GET /api/v1/admin/notes/:id/preview)
   → Access control checks admin status
   → PDF streamed from R2 through API (not direct R2 URL)

3. Admin approves (PATCH /api/v1/notes/:id/approve)
   → Validates HasPDF: true (required for approval)
   → Status changed to "Approved"
   → Note indexed in Meilisearch, added to hot feed
```

### Flow 3: User Purchases & Views Note

```
1. User finds approved note (via search or feed)

2. User adds to cart (POST /api/v1/cart)
   → Item added to Redis set: "cart:{userID}"

3. User checkouts (POST /api/v1/checkout)
   → Validates all items are approved
   → Creates Purchase records
   → Clears cart

4. User views purchased note (GET /api/v1/notes/:id/content)
   → Access control checks Purchase record exists
   → PDF streamed from R2 through API
   → Response headers encourage inline viewing
```

---

## Security Design

### Why Proxy Instead of Presigned URLs?

We could give users time-limited presigned URLs directly to R2, but:

1. **Users could share URLs** - Even short-lived URLs can be shared while valid
2. **No download prevention** - Direct URLs allow saving the file
3. **No viewing control** - Can't enforce our PDF viewer UI

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

---

## API Endpoints

### Protected Endpoints (Require Auth)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/notes/:id/content` | Upload PDF for a note |
| GET | `/api/v1/notes/:id/content` | View/stream PDF (requires purchase or admin) |
| POST | `/api/v1/checkout` | Checkout cart, create purchases |
| POST | `/api/v1/notes/:id/purchase` | Direct purchase (bypass cart) |
| GET | `/api/v1/notes/:id/purchased` | Check if user purchased note |
| GET | `/api/v1/me/purchases` | Get all purchased notes |
| GET | `/api/v1/me/purchases/history` | Paginated purchase history |

### Admin Endpoints (Require Admin Auth)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/admin/notes/:id/preview` | Preview pending note PDF |
| DELETE | `/api/v1/admin/notes/:id/content` | Delete PDF only |

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
      // Disable text selection to prevent copy-paste
      options={{ disableTextLayer: true }}
    >
      <Page pageNumber={1} />
    </Document>
  );
}
```

### Key Frontend Considerations

1. **No download button** - Don't expose a download feature
2. **Disable right-click** - Prevent "Save As" context menu
3. **Disable print** - Or watermark printed pages
4. **Disable text selection** - Prevents copy-paste of content
5. **Session timeout** - Re-authenticate after inactivity

---

## Testing the Flow

### 1. Create a Note with PDF

```bash
# Create note metadata
curl -X POST http://localhost:8080/api/v1/notes \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"subnotery_name": "CS101", "title": "Algorithms Notes", "author": "John", "price": 9.99}'

# Upload PDF (note the note ID from previous response)
curl -X POST http://localhost:8080/api/v1/notes/1/content \
  -H "Authorization: Bearer $TOKEN" \
  -F "pdf=@/path/to/your/notes.pdf"
```

### 2. Admin Approves

```bash
# Preview PDF (as admin)
curl http://localhost:8080/api/v1/admin/notes/1/preview \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  --output preview.pdf

# Approve
curl -X PATCH http://localhost:8080/api/v1/notes/1/approve \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

### 3. User Purchases and Views

```bash
# Purchase
curl -X POST http://localhost:8080/api/v1/notes/1/purchase \
  -H "Authorization: Bearer $USER_TOKEN"

# View (streams PDF)
curl http://localhost:8080/api/v1/notes/1/content \
  -H "Authorization: Bearer $USER_TOKEN" \
  --output viewed.pdf
```

---

## Future Enhancements

1. **Payment Integration** - Integrate Stripe/PayPal before creating Purchase records
2. **Watermarking** - Add user-specific watermarks to PDFs at view time
3. **View Analytics** - Track viewing patterns (time spent, pages viewed)
4. **Subscription Model** - Time-limited access with ExpiresAt field
5. **Refunds** - Add refund flow that invalidates Purchase records
6. **Version History** - Track PDF updates, allow viewing previous versions
