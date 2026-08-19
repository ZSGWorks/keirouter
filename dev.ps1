$ErrorActionPreference = "Stop"

$repoRoot = $PSScriptRoot
$frontendDir = Join-Path $repoRoot "frontend"
$backendDir = Join-Path $repoRoot "backend"

Write-Host "KeiRouter setup and run for Windows" -ForegroundColor Cyan

# Install frontend dependencies if node_modules does not exist.
if (-Not (Test-Path (Join-Path $frontendDir "node_modules"))) {
    Write-Host "Installing frontend deps..." -ForegroundColor DarkGray
    Push-Location $frontendDir
    try {
        npm ci
    }
    finally {
        Pop-Location
    }
}

# Download backend Go modules.
Write-Host "Downloading Go modules..." -ForegroundColor DarkGray
Push-Location $backendDir
try {
    go mod download
}
finally {
    Pop-Location
}

Write-Host "Dependencies ready. Starting dev servers in new windows..." -ForegroundColor Green
Write-Host "   Backend  -> http://localhost:20180"
Write-Host "   Dashboard-> http://localhost:5180"
Write-Host "   Password -> keirouter"

# Start the backend in a new PowerShell window.
Start-Process powershell -ArgumentList "-NoExit", "-Command", "go run ./cmd/keirouter" -WorkingDirectory $backendDir -WindowStyle Normal

# Wait a moment for the backend to initialize
Start-Sleep -Seconds 2

# Start the frontend in a new PowerShell window.
Start-Process powershell -ArgumentList "-NoExit", "-Command", "npm run dev" -WorkingDirectory $frontendDir -WindowStyle Normal

Write-Host "Servers are running in separate windows. Close those windows to stop them." -ForegroundColor Yellow
