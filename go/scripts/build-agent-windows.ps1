# Build stripped Windows agent + gzip sidecar (Go 1.20 — runs on Win7 through Win11).
# Version format (dev phase): yymmddhhMM local time.
$ErrorActionPreference = 'Stop'
$Root = Split-Path $PSScriptRoot -Parent
$env:Path = "C:\Program Files\Go\bin;" + $env:Path
$env:GOTOOLCHAIN = if ($env:GO_TOOLCHAIN) { $env:GO_TOOLCHAIN } else { 'go1.20.14' }
$env:CGO_ENABLED = '0'
Set-Location $Root

$Version = Get-Date -Format 'yyMMddHHmm'
Write-Host "Building woops-agent version=$Version (toolchain=$env:GOTOOLCHAIN)"

go build -ldflags="-s -w -X main.Version=$Version" -trimpath -o bin\agent.exe ./cmd/agent
Copy-Item -Force bin\agent.exe bin\woops-agent-windows-amd64.exe

$src = Join-Path $Root 'bin\woops-agent-windows-amd64.exe'
$dst = "$src.gz"
$in = [System.IO.File]::OpenRead($src)
$out = [System.IO.File]::Create($dst)
$gz = New-Object System.IO.Compression.GzipStream($out, [System.IO.Compression.CompressionLevel]::Optimal)
$in.CopyTo($gz)
$gz.Dispose(); $out.Dispose(); $in.Dispose()

Get-Item $src, $dst | Format-Table Name, Length -AutoSize
Write-Host "version: $Version"
