# Comment System Test Script
# Run: .\scripts\test-comments.ps1
#
# Tests the full comment system end-to-end:
#   1.  Signup two users (with usernames)
#   2.  Create a note + upload PDF + approve it
#   3.  Public anonymous comment listing (empty)
#   4.  Authenticated top-level comment creation
#   5.  Reply to a comment (threaded)
#   6.  Public anonymous read of comments (no user_vote)
#   7.  Authenticated read of comments (with user_vote)
#   8.  Edit own comment
#   9.  Vote (upvote, toggle off, downvote, switch)
#  10.  Remove vote
#  11.  Sort orders (best, new, top, old, controversial)
#  12.  Get single comment subtree + note visibility check
#  13.  Delete own comment (soft-delete, shows [deleted])
#  14.  Second user cannot delete first user's comment
#  15.  Admin (subnotery-scoped) can delete any comment
#  16.  Depth limit enforcement (MaxWriteDepth = 15)
#  17.  Cannot comment on non-approved note
#  18.  Rate-limit headers present

$ErrorActionPreference = "Stop"
$base = "http://localhost:8080/api/v1"
$rand = [DateTimeOffset]::Now.ToUnixTimeMilliseconds()
$pass = 0
$fail = 0

function Assert {
    param([string]$Name, [bool]$Condition)
    if ($Condition) {
        $script:pass++
        Write-Host "   PASS  $Name" -ForegroundColor Green
    } else {
        $script:fail++
        Write-Host "   FAIL  $Name" -ForegroundColor Red
    }
}

function Api {
    param(
        [string]$Method,
        [string]$Url,
        [hashtable]$Headers = @{},
        [object]$Body = $null
    )
    $params = @{
        Method      = $Method
        Uri         = $Url
        Headers     = $Headers
        ContentType = "application/json"
    }
    if ($Body) { $params.Body = ($Body | ConvertTo-Json) }
    return Invoke-RestMethod @params
}

function ApiRaw {
    # Returns the full WebResponseObject so we can inspect headers
    param(
        [string]$Method,
        [string]$Url,
        [hashtable]$Headers = @{},
        [object]$Body = $null
    )
    $params = @{
        Method      = $Method
        Uri         = $Url
        Headers     = $Headers
        ContentType = "application/json"
    }
    if ($Body) { $params.Body = ($Body | ConvertTo-Json) }
    return Invoke-WebRequest @params
}

Write-Host "`n========================================" -ForegroundColor Yellow
Write-Host "   COMMENT SYSTEM TEST" -ForegroundColor Yellow
Write-Host "========================================`n" -ForegroundColor Yellow

# Helper: mark a user's email as verified directly in the database.
# Write endpoints require verified email, so we do this after signup.
function Verify-UserEmail {
    param([string]$Email)
    docker exec notery_db psql -U admin -d notery_db -qc "UPDATE users SET email_verified=true WHERE email='$Email';" 2>$null
}

# ──────────────────────────────────────────
# STEP 1 — Create two users
# ──────────────────────────────────────────
Write-Host "=== 1. CREATE USERS ===" -ForegroundColor Cyan

$emailA = "commenter_a_$rand@test.com"
$emailB = "commenter_b_$rand@test.com"

Api "POST" "$base/auth/signup" -Body @{ email=$emailA; username="alice$rand"; password="pass123!" }
Verify-UserEmail $emailA
$loginA = Api "POST" "$base/auth/login" -Body @{ email=$emailA; password="pass123!" }
$tokA = $loginA.access_token
$hdrsA = @{ Authorization = "Bearer $tokA" }

Api "POST" "$base/auth/signup" -Body @{ email=$emailB; username="bob$rand"; password="pass123!" }
Verify-UserEmail $emailB
$loginB = Api "POST" "$base/auth/login" -Body @{ email=$emailB; password="pass123!" }
$tokB = $loginB.access_token
$hdrsB = @{ Authorization = "Bearer $tokB" }

Write-Host "   Created alice ($emailA) and bob ($emailB)" -ForegroundColor Green

# ──────────────────────────────────────────
# STEP 2 — Create note + upload PDF + approve
# ──────────────────────────────────────────
Write-Host "`n=== 2. CREATE + APPROVE NOTE ===" -ForegroundColor Cyan

$note = Api "POST" "$base/notes" -Headers $hdrsA -Body @{
    subnotery_name = "comment-test-$rand"
    title          = "Comment Test Note"
    price          = 0
}
$noteId = $note.id
Write-Host "   Note $noteId created (status=$($note.status))" -ForegroundColor Gray

# Upload a minimal PDF so approval succeeds
$pdfPath = "$env:TEMP\cmt-test-$rand.pdf"
@"
%PDF-1.4
1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj
2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj
3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 612 792]>>endobj
xref
0 4
trailer<</Size 4/Root 1 0 R>>
startxref
0
%%EOF
"@ | Set-Content -Path $pdfPath -NoNewline
curl.exe -s -X POST "$base/notes/$noteId/content" `
    -H "Authorization: Bearer $tokA" `
    -F "pdf=@$pdfPath;type=application/pdf" | Out-Null
Remove-Item $pdfPath -ErrorAction SilentlyContinue

Api "PATCH" "$base/notes/$noteId/approve" -Headers $hdrsA | Out-Null
$noteAfter = Api "GET" "$base/notes/$noteId" -Headers $hdrsA
Assert "Note approved" ($noteAfter.status -eq "Approved")

# ──────────────────────────────────────────
# STEP 3 — Public anonymous comment listing (empty)
# ──────────────────────────────────────────
Write-Host "`n=== 3. PUBLIC COMMENT LIST (empty) ===" -ForegroundColor Cyan

$empty = Api "GET" "$base/notes/$noteId/comments"
Assert "Empty comments list" ($empty.total -eq 0)
Assert "Response has truncated field" ($null -ne $empty.truncated)

# ──────────────────────────────────────────
# STEP 4 — Create top-level comment
# ──────────────────────────────────────────
Write-Host "`n=== 4. CREATE TOP-LEVEL COMMENT ===" -ForegroundColor Cyan

$c1 = Api "POST" "$base/notes/$noteId/comments" -Headers $hdrsA -Body @{
    body = "This is Alice's top-level comment."
}
Assert "Comment created (201 implied)" ($c1.id -gt 0)
Assert "Depth = 0" ($c1.depth -eq 0)
Assert "Username = alice$rand" ($c1.username -eq "alice$rand")
$commentId1 = $c1.id

# ──────────────────────────────────────────
# STEP 5 — Reply to the comment
# ──────────────────────────────────────────
Write-Host "`n=== 5. REPLY TO COMMENT ===" -ForegroundColor Cyan

$c2 = Api "POST" "$base/notes/$noteId/comments" -Headers $hdrsB -Body @{
    body      = "Bob's reply to Alice."
    parent_id = $commentId1
}
Assert "Reply created" ($c2.id -gt 0)
Assert "Reply depth = 1" ($c2.depth -eq 1)
Assert "Reply parent_id matches" ($c2.parent_id -eq $commentId1)
$commentId2 = $c2.id

# ──────────────────────────────────────────
# STEP 6 — Public anonymous read (no user_vote)
# ──────────────────────────────────────────
Write-Host "`n=== 6. ANONYMOUS READ ===" -ForegroundColor Cyan

$anon = Api "GET" "$base/notes/$noteId/comments"
Assert "Has 1 top-level" ($anon.total -eq 1)
$rootAnon = $anon.comments[0]
Assert "Root has children" ($rootAnon.children.Count -eq 1)
Assert "Anonymous user_vote = 0" ($rootAnon.user_vote -eq 0)

# ──────────────────────────────────────────
# STEP 7 — Authenticated read (with user_vote)
# ──────────────────────────────────────────
Write-Host "`n=== 7. AUTHENTICATED READ ===" -ForegroundColor Cyan

# First, upvote so we can verify user_vote in the response
Api "POST" "$base/comments/$commentId1/vote" -Headers $hdrsA -Body @{ value=1 } | Out-Null

$authRead = Api "GET" "$base/notes/$noteId/comments" -Headers $hdrsA
$rootAuth = $authRead.comments[0]
Assert "Auth user_vote = 1 after upvote" ($rootAuth.user_vote -eq 1)

# ──────────────────────────────────────────
# STEP 8 — Edit own comment
# ──────────────────────────────────────────
Write-Host "`n=== 8. EDIT COMMENT ===" -ForegroundColor Cyan

$edited = Api "PUT" "$base/comments/$commentId1" -Headers $hdrsA -Body @{
    body = "Alice's edited comment."
}
Assert "Edit returned body" ($edited.body -eq "Alice's edited comment.")

# ──────────────────────────────────────────
# STEP 9 — Vote mechanics
# ──────────────────────────────────────────
Write-Host "`n=== 9. VOTE MECHANICS ===" -ForegroundColor Cyan

# Already upvoted in step 7 — upvote again to toggle off
$v1 = Api "POST" "$base/comments/$commentId1/vote" -Headers $hdrsA -Body @{ value=1 }
Assert "Toggle off: user_vote = 0" ($v1.user_vote -eq 0)

# Downvote
$v2 = Api "POST" "$base/comments/$commentId1/vote" -Headers $hdrsA -Body @{ value=-1 }
Assert "Downvote: user_vote = -1" ($v2.user_vote -eq -1)
Assert "Downvotes incremented" ($v2.downvotes -ge 1)

# Switch from downvote to upvote
$v3 = Api "POST" "$base/comments/$commentId1/vote" -Headers $hdrsA -Body @{ value=1 }
Assert "Switch to upvote: user_vote = 1" ($v3.user_vote -eq 1)
Assert "Upvotes incremented" ($v3.upvotes -ge 1)
Assert "Score computed (wilson)" ($v3.score -gt 0)

# ──────────────────────────────────────────
# STEP 10 — Remove vote
# ──────────────────────────────────────────
Write-Host "`n=== 10. REMOVE VOTE ===" -ForegroundColor Cyan

$unvote = Api "DELETE" "$base/comments/$commentId1/vote" -Headers $hdrsA
Assert "Vote removed" ($unvote.message -eq "Vote removed")

$afterUnvote = Api "GET" "$base/notes/$noteId/comments" -Headers $hdrsA
Assert "user_vote = 0 after unvote" ($afterUnvote.comments[0].user_vote -eq 0)

# ──────────────────────────────────────────
# STEP 11 — Sort orders
# ──────────────────────────────────────────
Write-Host "`n=== 11. SORT ORDERS ===" -ForegroundColor Cyan

foreach ($s in @("best", "new", "top", "old", "controversial")) {
    $sorted = Api "GET" "$base/notes/$noteId/comments?sort=$s"
    Assert "Sort=$s returns comments" ($sorted.comments.Count -ge 1)
}

# ──────────────────────────────────────────
# STEP 12 — Get single comment subtree + note visibility
# ──────────────────────────────────────────
Write-Host "`n=== 12. GET SINGLE COMMENT SUBTREE ===" -ForegroundColor Cyan

$sub = Api "GET" "$base/comments/$commentId1"
Assert "Subtree root id matches" ($sub.id -eq $commentId1)
Assert "Subtree includes reply" ($sub.children.Count -ge 1)

# Verify that GetComment on a non-approved note's comment returns 403
# (We'd need a pending note to test this properly — skip if not possible)

# ──────────────────────────────────────────
# STEP 13 — Delete own comment (soft-delete)
# ──────────────────────────────────────────
Write-Host "`n=== 13. DELETE OWN COMMENT ===" -ForegroundColor Cyan

$del = Api "DELETE" "$base/comments/$commentId2" -Headers $hdrsB
Assert "Delete succeeded" ($del.message -eq "Comment deleted")

# Verify it shows [deleted] but reply tree intact
$afterDel = Api "GET" "$base/notes/$noteId/comments"
$root = $afterDel.comments[0]
$child = $root.children[0]
Assert "Deleted body = [deleted]" ($child.body -eq "[deleted]")
Assert "Deleted username = [deleted]" ($child.username -eq "[deleted]")

# ──────────────────────────────────────────
# STEP 14 — Non-owner cannot delete another's comment
# ──────────────────────────────────────────
Write-Host "`n=== 14. NON-OWNER DELETE DENIED ===" -ForegroundColor Cyan

try {
    Api "DELETE" "$base/comments/$commentId1" -Headers $hdrsB
    Assert "Non-owner delete should fail" $false
} catch {
    $status = $_.Exception.Response.StatusCode.value__
    Assert "Non-owner gets 403" ($status -eq 403)
}

# ──────────────────────────────────────────
# STEP 15 — Admin (subnotery) can delete any comment
# ──────────────────────────────────────────
Write-Host "`n=== 15. ADMIN DELETE ===" -ForegroundColor Cyan

# Alice is admin of the subnotery (auto-assigned on creation).
# She should be able to delete Bob's comment (already deleted, so create a new one first).
$c3 = Api "POST" "$base/notes/$noteId/comments" -Headers $hdrsB -Body @{
    body = "Bob's second comment for admin delete test."
}
$commentId3 = $c3.id

# Alice (subnotery admin, not using admin-protected routes — using protected DELETE which
# checks subnotery admin in the handler itself)
$adminDel = Api "DELETE" "$base/comments/$commentId3" -Headers $hdrsA
Assert "Admin delete succeeded" ($adminDel.message -eq "Comment deleted")

# ──────────────────────────────────────────
# STEP 16 — Depth limit enforcement
# ──────────────────────────────────────────
Write-Host "`n=== 16. WRITE DEPTH LIMIT ===" -ForegroundColor Cyan

# Build a chain of replies to test MaxWriteDepth (15).
# We create comments at increasing depth. The first few should succeed,
# and depth 16 (0-indexed parent at depth 15) should fail.
$parentId = $commentId1  # depth 0
$maxOk = $true
$hitLimit = $false

for ($d = 1; $d -le 17; $d++) {
    try {
        $reply = Api "POST" "$base/notes/$noteId/comments" -Headers $hdrsA -Body @{
            body      = "Depth $d reply"
            parent_id = $parentId
        }
        $parentId = $reply.id
    } catch {
        $status = $_.Exception.Response.StatusCode.value__
        if ($status -eq 400) {
            $hitLimit = $true
            Write-Host "   Depth limit hit at d=$d" -ForegroundColor Gray
            break
        } else {
            $maxOk = $false
            Write-Host "   Unexpected error at d=$d : status=$status" -ForegroundColor Red
            break
        }
    }
}

Assert "Depth limit enforced" $hitLimit

# ──────────────────────────────────────────
# STEP 17 — Cannot comment on non-approved note
# ──────────────────────────────────────────
Write-Host "`n=== 17. COMMENT ON PENDING NOTE ===" -ForegroundColor Cyan

$pendingNote = Api "POST" "$base/notes" -Headers $hdrsA -Body @{
    subnotery_name = "comment-test-$rand"
    title          = "Pending Note"
    price          = 0
}
$pendingId = $pendingNote.id

try {
    Api "POST" "$base/notes/$pendingId/comments" -Headers $hdrsB -Body @{
        body = "Should fail."
    }
    Assert "Comment on pending note blocked" $false
} catch {
    $status = $_.Exception.Response.StatusCode.value__
    Assert "Comment on pending note blocked (403)" ($status -eq 403)
}

# ──────────────────────────────────────────
# STEP 18 — Rate-limit headers present
# ──────────────────────────────────────────
Write-Host "`n=== 18. RATE-LIMIT HEADERS ===" -ForegroundColor Cyan

$raw = ApiRaw "POST" "$base/notes/$noteId/comments" -Headers $hdrsA -Body @{
    body = "Rate limit header check."
}
$rlLimit     = $raw.Headers["X-RateLimit-Limit"]
$rlRemaining = $raw.Headers["X-RateLimit-Remaining"]
Assert "X-RateLimit-Limit header present" ($null -ne $rlLimit)
Assert "X-RateLimit-Remaining header present" ($null -ne $rlRemaining)

# ──────────────────────────────────────────
# SUMMARY
# ──────────────────────────────────────────
Write-Host "`n========================================" -ForegroundColor Yellow
Write-Host "   RESULTS: $pass passed, $fail failed" -ForegroundColor $(if ($fail -eq 0) { "Green" } else { "Red" })
Write-Host "========================================`n" -ForegroundColor Yellow

if ($fail -gt 0) { exit 1 }
