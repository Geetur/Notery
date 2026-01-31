# Hot Feed Pipeline Test Script
# Run: .\scripts\test-hot-feed.ps1

$ErrorActionPreference = "Stop"
$base = "http://localhost:8080/api/v1"
$rand = Get-Random -Maximum 99999

Write-Host "`n========================================" -ForegroundColor Yellow
Write-Host "   HOT FEED PIPELINE TEST" -ForegroundColor Yellow
Write-Host "========================================`n" -ForegroundColor Yellow

# 1. SIGNUP
Write-Host "=== 1. SIGNUP ===" -ForegroundColor Cyan
$email = "hottest$rand@test.com"
$signup = Invoke-RestMethod -Uri "$base/signup" -Method POST -ContentType "application/json" -Body (@{
    email    = $email
    password = "testpass123"
} | ConvertTo-Json)
Write-Host "Created user ID: $($signup.user_id)" -ForegroundColor Green

# 2. LOGIN
Write-Host "`n=== 2. LOGIN ===" -ForegroundColor Cyan
$login = Invoke-RestMethod -Uri "$base/login" -Method POST -ContentType "application/json" -Body (@{
    email    = $email
    password = "testpass123"
} | ConvertTo-Json)
$token = $login.token
Write-Host "Got token: $($token.Substring(0,40))..." -ForegroundColor Green
$headers = @{ Authorization = "Bearer $token" }

# 3. CREATE NOTE
Write-Host "`n=== 3. CREATE NOTE ===" -ForegroundColor Cyan
$note = Invoke-RestMethod -Uri "$base/notes" -Method POST -Headers $headers -ContentType "application/json" -Body (@{
    subnotery_name = "test-subnotery-$rand"
    title          = "Hot Test Note $rand"
    author         = "Test Author"
    price          = 9.99
} | ConvertTo-Json)
$noteId = $note.id
Write-Host "Created note ID: $noteId" -ForegroundColor Green
Write-Host "Note status: $($note.status)" -ForegroundColor Yellow

# 4. CHECK HOT FEED (should be empty - note not approved yet)
Write-Host "`n=== 4. HOT FEED (before approval) ===" -ForegroundColor Cyan
$feed = Invoke-RestMethod -Uri "$base/feed/hot" -Method GET
Write-Host "Notes in feed: $($feed.notes.Count)" -ForegroundColor Yellow

# 5. APPROVE NOTE (requires admin - this user is admin of their subnotery)
Write-Host "`n=== 5. APPROVE NOTE ===" -ForegroundColor Cyan
try {
    $approved = Invoke-RestMethod -Uri "$base/notes/$noteId/approve" -Method PATCH -Headers $headers
    Write-Host "Approved: $($approved.message)" -ForegroundColor Green
}
catch {
    Write-Host "Approval response: $_" -ForegroundColor Yellow
}

# 6. CHECK HOT FEED (should now have our note)
Write-Host "`n=== 6. HOT FEED (after approval) ===" -ForegroundColor Cyan
$feed = Invoke-RestMethod -Uri "$base/feed/hot" -Method GET
Write-Host "Notes in feed: $($feed.notes.Count)" -ForegroundColor Green
if ($feed.notes.Count -gt 0) {
    Write-Host "First note: $($feed.notes[0].title)" -ForegroundColor Green
    Write-Host "Hotness: $($feed.notes[0].hotness)" -ForegroundColor Green
}

# 7. UPVOTE THE NOTE
Write-Host "`n=== 7. UPVOTE ===" -ForegroundColor Cyan
$upvote = Invoke-RestMethod -Uri "$base/notes/$noteId/upvote" -Method POST -Headers $headers
Write-Host "Upvotes: $($upvote.upvotes), Downvotes: $($upvote.downvotes)" -ForegroundColor Green
Write-Host "New hotness: $($upvote.hotness)" -ForegroundColor Green

# 8. UPVOTE AGAIN (should toggle off)
Write-Host "`n=== 8. UPVOTE AGAIN (toggle off) ===" -ForegroundColor Cyan
$upvote2 = Invoke-RestMethod -Uri "$base/notes/$noteId/upvote" -Method POST -Headers $headers
Write-Host "Upvotes: $($upvote2.upvotes), Downvotes: $($upvote2.downvotes)" -ForegroundColor Green

# 9. DOWNVOTE
Write-Host "`n=== 9. DOWNVOTE ===" -ForegroundColor Cyan
$downvote = Invoke-RestMethod -Uri "$base/notes/$noteId/downvote" -Method POST -Headers $headers
Write-Host "Upvotes: $($downvote.upvotes), Downvotes: $($downvote.downvotes)" -ForegroundColor Green
Write-Host "New hotness: $($downvote.hotness)" -ForegroundColor Green

# 10. CHECK PERSONALIZED FEED (logged in)
Write-Host "`n=== 10. PERSONALIZED FEED (logged in) ===" -ForegroundColor Cyan
$personalFeed = Invoke-RestMethod -Uri "$base/feed/hot" -Method GET -Headers $headers
Write-Host "Personalized notes: $($personalFeed.notes.Count)" -ForegroundColor Green

# 11. CHECK GLOBAL FEED (anonymous)
Write-Host "`n=== 11. GLOBAL FEED (anonymous) ===" -ForegroundColor Cyan
$globalFeed = Invoke-RestMethod -Uri "$base/feed/hot" -Method GET
Write-Host "Global notes: $($globalFeed.notes.Count)" -ForegroundColor Green

Write-Host "`n========================================" -ForegroundColor Yellow
Write-Host "   TEST COMPLETE!" -ForegroundColor Green
Write-Host "========================================`n" -ForegroundColor Yellow
