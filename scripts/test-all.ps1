#!/usr/bin/env pwsh
# test-all.ps1 — Unified test runner for all Notery test suites.
#
# Runs backend Go tests, go vet, frontend build & tests, and optionally
# E2E scripts (requires a running server + Docker).
#
# Usage:
#   .\scripts\test-all.ps1              # Backend + Frontend only
#   .\scripts\test-all.ps1 -E2E        # Also run E2E scripts (requires running server)
#   .\scripts\test-all.ps1 -K6         # Also run k6 load tests (requires running server)
#   .\scripts\test-all.ps1 -E2E -K6    # Run everything
#   .\scripts\test-all.ps1 -Verbose     # Show full output

param(
    [switch]$E2E,
    [switch]$K6,
    [switch]$Verbose
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Definition)
Set-Location $root

$failed = @()
$passed = @()
$startTime = Get-Date

function Write-Section($title) {
    Write-Host "`n═══════════════════════════════════════════════════════" -ForegroundColor Cyan
    Write-Host "  $title" -ForegroundColor Cyan
    Write-Host "═══════════════════════════════════════════════════════" -ForegroundColor Cyan
}

function Run-Step($name, $scriptBlock) {
    Write-Host "`n▸ $name..." -ForegroundColor Yellow
    $stepStart = Get-Date
    try {
        & $scriptBlock
        if ($LASTEXITCODE -and $LASTEXITCODE -ne 0) {
            throw "Exit code $LASTEXITCODE"
        }
        $elapsed = ((Get-Date) - $stepStart).TotalSeconds
        Write-Host "  ✓ $name passed (${elapsed}s)" -ForegroundColor Green
        $script:passed += $name
    } catch {
        $elapsed = ((Get-Date) - $stepStart).TotalSeconds
        Write-Host "  ✗ $name FAILED (${elapsed}s): $_" -ForegroundColor Red
        $script:failed += $name
    }
}

# ─── Backend ────────────────────────────────────────────────────────────────

Write-Section "BACKEND (Go)"

Run-Step "go vet" {
    go vet ./... 2>&1 | ForEach-Object { if ($Verbose) { Write-Host "    $_" } }
}

Run-Step "go test (all packages)" {
    $output = go test ./... -count=1 -timeout 60s 2>&1
    $output | ForEach-Object {
        if ($Verbose -or $_ -match "FAIL|PASS|ok") { Write-Host "    $_" }
    }
    if ($output -match "FAIL") { throw "One or more Go test packages failed" }
}

Run-Step "go test -race (race detector)" {
    $output = go test -race ./... -count=1 -timeout 120s 2>&1
    $output | ForEach-Object {
        if ($Verbose -or $_ -match "FAIL|PASS|ok") { Write-Host "    $_" }
    }
    if ($output -match "FAIL") { throw "Race condition detected" }
}

# ─── Frontend ───────────────────────────────────────────────────────────────

Write-Section "FRONTEND (Next.js)"

Push-Location frontend

Run-Step "npm install" {
    npm install --silent 2>&1 | ForEach-Object { if ($Verbose) { Write-Host "    $_" } }
}

Run-Step "npm run build (TypeScript + Next.js)" {
    $output = npm run build 2>&1
    $output | ForEach-Object { if ($Verbose) { Write-Host "    $_" } }
    if ($LASTEXITCODE -ne 0) { throw "Frontend build failed" }
}

Run-Step "npm test (Jest)" {
    $env:CI = "true"
    $output = npm test 2>&1
    $output | ForEach-Object {
        if ($Verbose -or $_ -match "Tests:|Test Suites:|FAIL|PASS") { Write-Host "    $_" }
    }
    if ($LASTEXITCODE -ne 0) { throw "Frontend tests failed" }
    Remove-Item Env:\CI -ErrorAction SilentlyContinue
}

Pop-Location

# ─── E2E Scripts (optional) ────────────────────────────────────────────────

if ($E2E) {
    Write-Section "E2E SCRIPTS (requires running server)"

    $scripts = @(
        "scripts/test-comments.ps1",
        "scripts/test-hot-feed.ps1",
        "scripts/test-pdf-workflow.ps1"
    )

    foreach ($script in $scripts) {
        $scriptName = Split-Path -Leaf $script
        Run-Step "E2E: $scriptName" {
            & ".\$script" 2>&1 | ForEach-Object { if ($Verbose) { Write-Host "    $_" } }
        }
    }
}

# ─── k6 Load Tests (optional) ──────────────────────────────────────────────

if ($K6) {
    Write-Section "K6 LOAD TESTS (requires running server)"

    Run-Step "k6 smoke test" {
        $output = k6 run scripts/k6/smoke-test.js 2>&1
        $output | ForEach-Object {
            if ($Verbose -or $_ -match "checks|http_req|running|default") { Write-Host "    $_" }
        }
        if ($LASTEXITCODE -ne 0) { throw "k6 smoke test failed" }
    }

    Run-Step "k6 auth flow" {
        $output = k6 run scripts/k6/auth-flow.js 2>&1
        $output | ForEach-Object {
            if ($Verbose -or $_ -match "checks|http_req|running|default|errors") { Write-Host "    $_" }
        }
        if ($LASTEXITCODE -ne 0) { throw "k6 auth flow test failed" }
    }

    Run-Step "k6 load test" {
        $output = k6 run scripts/k6/load-test.js 2>&1
        $output | ForEach-Object {
            if ($Verbose -or $_ -match "checks|http_req|running|default|errors") { Write-Host "    $_" }
        }
        if ($LASTEXITCODE -ne 0) { throw "k6 load test failed" }
    }
}

# ─── Summary ────────────────────────────────────────────────────────────────

$totalElapsed = ((Get-Date) - $startTime).TotalSeconds

Write-Host "`n" -NoNewline
Write-Section "TEST SUMMARY"
Write-Host ""
Write-Host "  Passed: $($passed.Count)" -ForegroundColor Green
foreach ($p in $passed) { Write-Host "    ✓ $p" -ForegroundColor Green }

if ($failed.Count -gt 0) {
    Write-Host ""
    Write-Host "  Failed: $($failed.Count)" -ForegroundColor Red
    foreach ($f in $failed) { Write-Host "    ✗ $f" -ForegroundColor Red }
}

Write-Host ""
Write-Host "  Total time: $([math]::Round($totalElapsed, 1))s" -ForegroundColor Cyan
Write-Host ""

if ($failed.Count -gt 0) {
    Write-Host "  RESULT: FAILED" -ForegroundColor Red
    exit 1
} else {
    Write-Host "  RESULT: ALL PASSED" -ForegroundColor Green
    exit 0
}
