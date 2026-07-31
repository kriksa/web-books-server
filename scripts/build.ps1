# Windows build: web (Vite) + Go binary
$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $Root

$Web = Join-Path $Root "web"
$Embed = Join-Path $Root "internal\spaembed\spa"
$Assets = Join-Path $Root "assets"

# Sync static assets into web/public
$Public = Join-Path $Web "public"
if (Test-Path $Public) { Remove-Item -Recurse -Force $Public }
New-Item -ItemType Directory -Force -Path (Join-Path $Public "backgrounds") | Out-Null
foreach ($f in @("favicon.svg", "favicon.ico", "apple-touch-icon.png")) {
    $src = Join-Path $Assets $f
    if (Test-Path $src) { Copy-Item $src $Public }
}
$bgSrc = Join-Path $Assets "backgrounds"
if (Test-Path $bgSrc) {
    Copy-Item -Path (Join-Path $bgSrc "*") -Destination (Join-Path $Public "backgrounds") -Recurse -Force
}

Push-Location $Web
npm ci
npm run build
Pop-Location

if (-not (Test-Path (Join-Path $Embed "index.html"))) {
    throw "SPA build missing: $Embed\index.html"
}

go mod tidy
go build -ldflags="-s -w" -o (Join-Path $Root "bin\web_books.exe") ./cmd/web_books
Write-Host "Built: $(Join-Path $Root 'bin\web_books.exe')"
