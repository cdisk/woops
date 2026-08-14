# Dot-source into the current PowerShell session:
#   . .\scripts\load-env.ps1
# Optional path (only that file; skips .env.local overlay):
#   . .\scripts\load-env.ps1 -Path D:\Code\ops\.env
param(
    [string]$Path = ""
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent

function Import-EnvFile([string]$FilePath) {
    if (-not (Test-Path -LiteralPath $FilePath)) {
        return $false
    }
    Get-Content -LiteralPath $FilePath | ForEach-Object {
        $line = $_.Trim()
        if ($line -eq '' -or $line.StartsWith('#')) { return }
        $eq = $line.IndexOf('=')
        if ($eq -lt 1) { return }
        $key = $line.Substring(0, $eq).Trim()
        $val = $line.Substring($eq + 1).Trim()
        if (($val.StartsWith('"') -and $val.EndsWith('"')) -or ($val.StartsWith("'") -and $val.EndsWith("'"))) {
            $val = $val.Substring(1, $val.Length - 2)
        }
        Set-Item -Path "Env:$key" -Value $val
    }
    Write-Host "Loaded env from $FilePath"
    return $true
}

if ($Path) {
    if (-not (Import-EnvFile $Path)) {
        throw ".env not found at $Path"
    }
    return
}

$base = Join-Path $repoRoot '.env'
if (-not (Import-EnvFile $base)) {
    $example = Join-Path $repoRoot '.env.example'
    throw ".env not found at $base. Copy .env.example first: copy `"$example`" `"$base`""
}
$local = Join-Path $repoRoot '.env.local'
if (Import-EnvFile $local) {
    Write-Host "Applied local overrides from .env.local"
}
