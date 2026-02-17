# PDF Content Workflow Test Script
# Run: .\scripts\test-pdf-workflow.ps1
#
# Tests the complete PDF upload, approval, purchase, and viewing workflow.
# Updated to include username on signup.

# need to add {"error":"Title, SubnoteryName, Author, and Price are required"}

$BaseUrl = "http://localhost:8080/api/v1"

Write-Host "====================================" -ForegroundColor Cyan
Write-Host "PDF Content Workflow Test" -ForegroundColor Cyan
Write-Host "====================================" -ForegroundColor Cyan

# Helper function
function Test-Endpoint {
    param(
        [string]$Method,
        [string]$Url,
        [hashtable]$Headers = @{},
        [object]$Body = $null,
        [string]$Description
    )

    Write-Host "`n>> $Description" -ForegroundColor Yellow
    Write-Host "   $Method $Url" -ForegroundColor Gray

    try {
        $params = @{
            Method      = $Method
            Uri         = $Url
            Headers     = $Headers
            ContentType = "application/json"
        }
        if ($Body) {
            $params.Body = ($Body | ConvertTo-Json)
        }

        $response = Invoke-RestMethod @params
        Write-Host "   SUCCESS" -ForegroundColor Green
        return $response
    }
    catch {
        Write-Host "   FAILED: $($_.Exception.Message)" -ForegroundColor Red
        if ($_.ErrorDetails.Message) {
            Write-Host "   Response: $($_.ErrorDetails.Message)" -ForegroundColor Red
        }
        return $null
    }
}

# Generate unique identifiers
$timestamp = [DateTimeOffset]::Now.ToUnixTimeMilliseconds()
$creatorEmail = "creator_$timestamp@test.com"
$buyerEmail   = "buyer_$timestamp@test.com"

# ============================================
# STEP 1: Create test users
# ============================================
Write-Host "`n`n=== STEP 1: Create Test Users ===" -ForegroundColor Cyan

$creatorSignup = Test-Endpoint -Method "POST" -Url "$BaseUrl/signup" -Body @{
    email    = $creatorEmail
    username = "creator$timestamp"
    password = "Test123!"
} -Description "Creating note creator user"

if (-not $creatorSignup) {
    Write-Host "Failed to create creator user. Exiting." -ForegroundColor Red
    exit 1
}

$creatorLogin = Test-Endpoint -Method "POST" -Url "$BaseUrl/login" -Body @{
    email    = $creatorEmail
    password = "Test123!"
} -Description "Logging in as creator"

if (-not $creatorLogin) {
    Write-Host "Failed to login creator. Exiting." -ForegroundColor Red
    exit 1
}
$creatorToken = $creatorLogin.access_token
Write-Host "   Creator token: $($creatorToken.Substring(0,20))..." -ForegroundColor Gray

$buyerSignup = Test-Endpoint -Method "POST" -Url "$BaseUrl/signup" -Body @{
    email    = $buyerEmail
    username = "buyer$timestamp"
    password = "Test123!"
} -Description "Creating buyer user"

if (-not $buyerSignup) {
    Write-Host "Failed to create buyer user. Exiting." -ForegroundColor Red
    exit 1
}

$buyerLogin = Test-Endpoint -Method "POST" -Url "$BaseUrl/login" -Body @{
    email    = $buyerEmail
    password = "Test123!"
} -Description "Logging in as buyer"

if (-not $buyerLogin) {
    Write-Host "Failed to login buyer. Exiting." -ForegroundColor Red
    exit 1
}
$buyerToken = $buyerLogin.access_token
Write-Host "   Buyer token: $($buyerToken.Substring(0,20))..." -ForegroundColor Gray

# ============================================
# STEP 2: Create a note (without PDF initially)
# ============================================
Write-Host "`n`n=== STEP 2: Create Note ===" -ForegroundColor Cyan

$noteResponse = Test-Endpoint -Method "POST" -Url "$BaseUrl/notes" -Headers @{
    Authorization = "Bearer $creatorToken"
} -Body @{
    title          = "Test Note with PDF"
    author         = "Test Author"
    price          = 999
    subnotery_name = "TestSubnotery_$timestamp"
} -Description "Creating a new note"

if (-not $noteResponse) {
    Write-Host "Failed to create note. Exiting." -ForegroundColor Red
    exit 1
}
$noteId = $noteResponse.ID
Write-Host "   Note ID: $noteId" -ForegroundColor Gray
Write-Host "   Status: $($noteResponse.Status)" -ForegroundColor Gray

# ============================================
# STEP 3: Upload PDF for the note
# ============================================
Write-Host "`n`n=== STEP 3: Upload PDF ===" -ForegroundColor Cyan

$testPdfPath = "$env:TEMP\test-note-$timestamp.pdf"
$pdfContent = @"
%PDF-1.4
1 0 obj
<< /Type /Catalog /Pages 2 0 R >>
endobj
2 0 obj
<< /Type /Pages /Kids [3 0 R] /Count 1 >>
endobj
3 0 obj
<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R >>
endobj
4 0 obj
<< /Length 44 >>
stream
BT
/F1 24 Tf
100 700 Td
(Test PDF Content) Tj
ET
endstream
endobj
xref
0 5
0000000000 65535 f 
0000000009 00000 n 
0000000058 00000 n 
0000000115 00000 n 
0000000202 00000 n 
trailer
<< /Size 5 /Root 1 0 R >>
startxref
295
%%EOF
"@
[System.IO.File]::WriteAllText($testPdfPath, $pdfContent)
Write-Host "   Created test PDF at: $testPdfPath" -ForegroundColor Gray

Write-Host "`n>> Uploading PDF to note" -ForegroundColor Yellow
Write-Host "   POST $BaseUrl/notes/$noteId/content" -ForegroundColor Gray

try {
    $curlOutput = curl.exe -s -X POST "$BaseUrl/notes/$noteId/content" `
        -H "Authorization: Bearer $creatorToken" `
        -F "pdf=@$testPdfPath;type=application/pdf"

    $uploadResponse = $curlOutput | ConvertFrom-Json

    if ($uploadResponse.error) {
        Write-Host "   FAILED: $($uploadResponse.error)" -ForegroundColor Red
    } else {
        Write-Host "   SUCCESS" -ForegroundColor Green
        Write-Host "   PDF Size: $($uploadResponse.pdf_size) bytes" -ForegroundColor Gray
    }
}
catch {
    Write-Host "   FAILED: $($_.Exception.Message)" -ForegroundColor Red
    Write-Host "   curl output: $curlOutput" -ForegroundColor Red
}

Remove-Item $testPdfPath -ErrorAction SilentlyContinue

# ============================================
# STEP 4: Verify note has PDF
# ============================================
Write-Host "`n`n=== STEP 4: Verify Note Has PDF ===" -ForegroundColor Cyan

$noteCheck = Test-Endpoint -Method "GET" -Url "$BaseUrl/notes/$noteId" -Headers @{
    Authorization = "Bearer $creatorToken"
} -Description "Getting note details"

if ($noteCheck) {
    Write-Host "   Has PDF: $($noteCheck.has_pdf)" -ForegroundColor Gray
    Write-Host "   Status: $($noteCheck.status)" -ForegroundColor Gray
}

# ============================================
# STEP 5: Buyer tries to view PDF (should fail — not purchased)
# ============================================
Write-Host "`n`n=== STEP 5: Buyer Tries to View (Should Fail) ===" -ForegroundColor Cyan

Write-Host "`n>> Buyer attempting to view PDF without purchase" -ForegroundColor Yellow
Write-Host "   GET $BaseUrl/notes/$noteId/content" -ForegroundColor Gray

try {
    Invoke-RestMethod -Uri "$BaseUrl/notes/$noteId/content" `
        -Method GET `
        -Headers @{ Authorization = "Bearer $buyerToken" }
    Write-Host "   UNEXPECTED SUCCESS - Should have failed!" -ForegroundColor Red
}
catch {
    if ($_.Exception.Response.StatusCode.value__ -eq 403) {
        Write-Host "   EXPECTED FAILURE: Access denied (not purchased)" -ForegroundColor Green
    } else {
        Write-Host "   FAILED: $($_.Exception.Message)" -ForegroundColor Yellow
    }
}

# ============================================
# STEP 6: Admin approves the note
# ============================================
Write-Host "`n`n=== STEP 6: Admin Approves Note ===" -ForegroundColor Cyan

$approveResponse = Test-Endpoint -Method "PATCH" -Url "$BaseUrl/notes/$noteId/approve" -Headers @{
    Authorization = "Bearer $creatorToken"
} -Description "Creator (as subnotery admin) approving the note"

if ($approveResponse) {
    Write-Host "   Note approved successfully!" -ForegroundColor Gray
}

$noteAfterApproval = Test-Endpoint -Method "GET" -Url "$BaseUrl/notes/$noteId" -Headers @{
    Authorization = "Bearer $creatorToken"
} -Description "Checking note status after approval"

if ($noteAfterApproval) {
    Write-Host "   Status after approval: $($noteAfterApproval.status)" -ForegroundColor Gray
}

# ============================================
# STEP 7: Purchase the note
# ============================================
Write-Host "`n`n=== STEP 7: Purchase Note ===" -ForegroundColor Cyan

$purchaseResponse = Test-Endpoint -Method "POST" -Url "$BaseUrl/notes/$noteId/purchase" -Headers @{
    Authorization = "Bearer $buyerToken"
} -Description "Buyer purchasing the note"

if ($purchaseResponse) {
    Write-Host "   Purchase ID: $($purchaseResponse.purchase.id)" -ForegroundColor Gray
    Write-Host "   Price Paid: $($purchaseResponse.purchase.price_paid)" -ForegroundColor Gray
}

# ============================================
# STEP 8: Check purchase status
# ============================================
Write-Host "`n`n=== STEP 8: Check Purchase Status ===" -ForegroundColor Cyan

$statusResponse = Test-Endpoint -Method "GET" -Url "$BaseUrl/notes/$noteId/purchased" -Headers @{
    Authorization = "Bearer $buyerToken"
} -Description "Checking if note is purchased"

if ($statusResponse) {
    Write-Host "   Purchased: $($statusResponse.purchased)" -ForegroundColor Gray
}

# ============================================
# STEP 9: View PDF after purchase
# ============================================
Write-Host "`n`n=== STEP 9: View PDF After Purchase ===" -ForegroundColor Cyan

Write-Host "`n>> Buyer viewing PDF after purchase" -ForegroundColor Yellow
Write-Host "   GET $BaseUrl/notes/$noteId/content" -ForegroundColor Gray

try {
    $webRequest = [System.Net.WebRequest]::Create("$BaseUrl/notes/$noteId/content")
    $webRequest.Method = "GET"
    $webRequest.Headers.Add("Authorization", "Bearer $buyerToken")

    $response = $webRequest.GetResponse()
    $contentType = $response.ContentType
    $contentLength = $response.ContentLength

    Write-Host "   SUCCESS" -ForegroundColor Green
    Write-Host "   Content-Type: $contentType" -ForegroundColor Gray
    Write-Host "   Content-Length: $contentLength bytes" -ForegroundColor Gray

    $response.Close()
}
catch {
    Write-Host "   FAILED: $($_.Exception.Message)" -ForegroundColor Red
}

# ============================================
# STEP 10: Check purchase history
# ============================================
Write-Host "`n`n=== STEP 10: Purchase History ===" -ForegroundColor Cyan

$historyResponse = Test-Endpoint -Method "GET" -Url "$BaseUrl/me/purchases/history" -Headers @{
    Authorization = "Bearer $buyerToken"
} -Description "Getting purchase history"

if ($historyResponse -and $historyResponse.purchases) {
    Write-Host "   Total purchases: $($historyResponse.purchases.Count)" -ForegroundColor Gray
}

# ============================================
# STEP 11: Creator views own note PDF
# ============================================
Write-Host "`n`n=== STEP 11: Creator Views Own PDF ===" -ForegroundColor Cyan

Write-Host "`n>> Creator viewing their own PDF" -ForegroundColor Yellow
Write-Host "   GET $BaseUrl/notes/$noteId/content" -ForegroundColor Gray

try {
    $webRequest = [System.Net.WebRequest]::Create("$BaseUrl/notes/$noteId/content")
    $webRequest.Method = "GET"
    $webRequest.Headers.Add("Authorization", "Bearer $creatorToken")

    $response = $webRequest.GetResponse()
    Write-Host "   SUCCESS - Creator can view their own PDF" -ForegroundColor Green
    $response.Close()
}
catch {
    Write-Host "   FAILED: $($_.Exception.Message)" -ForegroundColor Red
}

# ============================================
# Summary
# ============================================
Write-Host "`n`n====================================" -ForegroundColor Cyan
Write-Host "Test Complete!" -ForegroundColor Cyan
Write-Host "====================================" -ForegroundColor Cyan
Write-Host @"

Summary:
- Created note creator and buyer accounts (with usernames)
- Created a note with PDF upload
- Verified access control (buyer blocked without purchase)
- Admin approved note (requires PDF)
- Purchased note
- Verified buyer can view PDF after purchase
- Creator can always view their own PDF
"@ -ForegroundColor Gray
