# Hot Feed Pipeline Test Script
# Run: .\scripts\test-hot-feed.ps1
#
# Tests: signup (with username), login, note creation, PDF upload,
#        admin approval, hot feed population, voting, and personalized feed.

$ErrorActionPreference = "Stop"
$base = "http://localhost:8080/api/v1"
$rand = [DateTimeOffset]::Now.ToUnixTimeMilliseconds()

Write-Host "`n========================================" -ForegroundColor Yellow
Write-Host "   HOT FEED PIPELINE TEST" -ForegroundColor Yellow
Write-Host "========================================`n" -ForegroundColor Yellow

# 1. SIGNUP
Write-Host "=== 1. SIGNUP ===" -ForegroundColor Cyan
$email = "hottest$rand@test.com"
$signup = Invoke-RestMethod -Uri "$base/auth/signup" -Method POST -ContentType "application/json" -Body (@{
    email    = $email
    username = "hotuser$rand"
    password = "testpass123"
} | ConvertTo-Json)
Write-Host "Created user ID: $($signup.user_id)" -ForegroundColor Green

# Verify email directly in DB (write endpoints require verified email)
docker exec notery_db psql -U admin -d notery_db -qc "UPDATE users SET email_verified=true WHERE email='$email';" 2>$null

# 2. LOGIN
Write-Host "`n=== 2. LOGIN ===" -ForegroundColor Cyan
$login = Invoke-RestMethod -Uri "$base/auth/login" -Method POST -ContentType "application/json" -Body (@{
    email    = $email
    password = "testpass123"
} | ConvertTo-Json)
$token = $login.access_token
Write-Host "Got token: $($token.Substring(0,40))..." -ForegroundColor Green
$headers = @{ Authorization = "Bearer $token" }

# 3. CREATE NOTE
Write-Host "`n=== 3. CREATE NOTE ===" -ForegroundColor Cyan
$note = Invoke-RestMethod -Uri "$base/notes" -Method POST -Headers $headers -ContentType "application/json" -Body (@{
    subnotery_name = "test-subnotery-$rand"
    title          = "Hot Test Note $rand"
    price          = 999
} | ConvertTo-Json)
$noteId = $note.id
Write-Host "Created note ID: $noteId" -ForegroundColor Green
Write-Host "Note status: $($note.status)" -ForegroundColor Yellow

# 4. UPLOAD PDF (required before approval)
Write-Host "`n=== 4. UPLOAD PDF ===" -ForegroundColor Cyan
$testPdfPath = "$env:TEMP\hot-test-$rand.pdf"
$pdfContent = @"
%PDF-1.4
1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj
2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj
3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 612 792]/Contents 4 0 R>>endobj
4 0 obj<</Length 44>>stream
BT /F1 24 Tf 100 700 Td (Test) Tj ET
endstream
endobj
xref
0 5
trailer<</Size 5/Root 1 0 R>>
startxref
0
%%EOF
"@
[System.IO.File]::WriteAllText($testPdfPath, $pdfContent)
try {
    $curlOutput = curl.exe -s -X POST "$base/notes/$noteId/content" `
        -H "Authorization: Bearer $token" `
        -F "pdf=@$testPdfPath;type=application/pdf"
    $uploadResp = $curlOutput | ConvertFrom-Json
    if ($uploadResp.error) {
        Write-Host "PDF upload failed: $($uploadResp.error)" -ForegroundColor Red
    } else {
        Write-Host "PDF uploaded ($($uploadResp.pdf_size) bytes)" -ForegroundColor Green
    }
} finally {
    Remove-Item $testPdfPath -ErrorAction SilentlyContinue
}

# 5. CHECK HOT FEED (should be empty - note not approved yet)
Write-Host "`n=== 5. HOT FEED (before approval) ===" -ForegroundColor Cyan
$feed = Invoke-RestMethod -Uri "$base/feed/hot" -Method GET
Write-Host "Notes in feed: $($feed.notes.Count)" -ForegroundColor Yellow

# 6. APPROVE NOTE (creator is auto-admin of the new subnotery)
Write-Host "`n=== 6. APPROVE NOTE ===" -ForegroundColor Cyan
try {
    $approved = Invoke-RestMethod -Uri "$base/notes/$noteId/approve" -Method PATCH -Headers $headers
    Write-Host "Approved: $($approved.message)" -ForegroundColor Green
}
catch {
    Write-Host "Approval response: $_" -ForegroundColor Yellow
}

# 7. CHECK HOT FEED (should now have our note)
Write-Host "`n=== 7. HOT FEED (after approval) ===" -ForegroundColor Cyan
$feed = Invoke-RestMethod -Uri "$base/feed/hot" -Method GET
Write-Host "Notes in feed: $($feed.notes.Count)" -ForegroundColor Green
if ($feed.notes.Count -gt 0) {
    Write-Host "First note: $($feed.notes[0].title)" -ForegroundColor Green
    Write-Host "Hotness: $($feed.notes[0].hotness)" -ForegroundColor Green
}

# 8. UPVOTE THE NOTE
Write-Host "`n=== 8. UPVOTE ===" -ForegroundColor Cyan
$upvote = Invoke-RestMethod -Uri "$base/notes/$noteId/upvote" -Method POST -Headers $headers
Write-Host "Upvotes: $($upvote.upvotes), Downvotes: $($upvote.downvotes)" -ForegroundColor Green
Write-Host "New hotness: $($upvote.hotness)" -ForegroundColor Green

# 9. UPVOTE AGAIN (should toggle off)
Write-Host "`n=== 9. UPVOTE AGAIN (toggle off) ===" -ForegroundColor Cyan
$upvote2 = Invoke-RestMethod -Uri "$base/notes/$noteId/upvote" -Method POST -Headers $headers
Write-Host "Upvotes: $($upvote2.upvotes), Downvotes: $($upvote2.downvotes)" -ForegroundColor Green

# 10. DOWNVOTE
Write-Host "`n=== 10. DOWNVOTE ===" -ForegroundColor Cyan
$downvote = Invoke-RestMethod -Uri "$base/notes/$noteId/downvote" -Method POST -Headers $headers
Write-Host "Upvotes: $($downvote.upvotes), Downvotes: $($downvote.downvotes)" -ForegroundColor Green
Write-Host "New hotness: $($downvote.hotness)" -ForegroundColor Green

# 11. CHECK PERSONALIZED FEED (logged in)
Write-Host "`n=== 11. PERSONALIZED FEED (logged in) ===" -ForegroundColor Cyan
$personalFeed = Invoke-RestMethod -Uri "$base/feed/hot" -Method GET -Headers $headers
Write-Host "Personalized notes: $($personalFeed.notes.Count)" -ForegroundColor Green

# 12. CHECK GLOBAL FEED (anonymous)
Write-Host "`n=== 12. GLOBAL FEED (anonymous) ===" -ForegroundColor Cyan
$globalFeed = Invoke-RestMethod -Uri "$base/feed/hot" -Method GET
Write-Host "Global notes: $($globalFeed.notes.Count)" -ForegroundColor Green

Write-Host "`n========================================" -ForegroundColor Yellow
Write-Host "   TEST COMPLETE!" -ForegroundColor Green
Write-Host "========================================`n" -ForegroundColor Yellow
