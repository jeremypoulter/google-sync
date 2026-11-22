# Builds gmigrate.exe with the family OAuth client built in.
#   .\scripts\build.ps1 -ClientFile C:\path\client_secret.json [-Version 0.1.0]
# Keep the resulting exe within the family: it contains the client secret.
param(
    [Parameter(Mandatory = $true)][string]$ClientFile,
    [string]$Version = "dev"
)
$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")
$flags = go run ./scripts/ldflags $ClientFile $Version
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
$env:GOOS = "windows"; $env:GOARCH = "amd64"; $env:CGO_ENABLED = "0"
go build -trimpath -ldflags "$flags" -o dist/gmigrate.exe ./cmd/gmigrate
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Write-Host "Built dist/gmigrate.exe"
