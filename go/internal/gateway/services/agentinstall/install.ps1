# Served as GET /i/{code}/install.ps1 - placeholders filled by gateway.
$ErrorActionPreference = 'Stop'
$GatewayBase = '{{GATEWAY_BASE}}'
$InstallCode = '{{INSTALL_CODE}}'
$GatewayTlsSpkiSha256 = '{{GATEWAY_TLS_SPKI_SHA256}}'
$AgentSha256Amd64 = '{{AGENT_SHA256_AMD64}}'
$AgentSha256Arm64 = '{{AGENT_SHA256_ARM64}}'
$Os = 'windows'
$Arch = if ($env:PROCESSOR_ARCHITECTURE -match 'ARM64') { 'arm64' } else { 'amd64' }

$ConfDir = Join-Path $env:ProgramData 'woops-agent'
$BinDir = Join-Path $env:ProgramFiles 'woops-agent'
$Bin = Join-Path $BinDir 'woops-agent.exe'
# Keep a real .exe suffix so Windows will execute -version on the staged file.
$BinNew = Join-Path $BinDir 'woops-agent-new.exe'
$ServiceName = 'woops-agent'

# Program Files + Windows Service require an elevated shell.
# Use a plain ASCII throw (no @"@" here-string): Windows PowerShell 5.1 defaults to
# system ANSI when the file has no BOM, and UTF-8 Chinese inside here-strings breaks parsing.
$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
  throw 'Administrator privileges required to install Woops Agent (writes under Program Files and registers a Windows service). Right-click PowerShell / Windows Terminal -> Run as administrator, then re-run the install command.'
}

New-Item -ItemType Directory -Force -Path $ConfDir, $BinDir | Out-Null

function Invoke-GwRequest {
  param(
    [Parameter(Mandatory = $true)][string]$Uri,
    [string]$OutFile
  )
  $pin = ($GatewayTlsSpkiSha256 | ForEach-Object { $_.Trim() })
  if ($pin) {
    $curl = Get-Command curl.exe -ErrorAction SilentlyContinue
    if (-not $curl) { throw 'gatewayTlsSpkiSha256 set but curl.exe not found (install curl first; see Console tip)' }
    $hex = $pin -replace '^sha256[:/]+', ''
    $bytes = New-Object byte[] ($hex.Length / 2)
    for ($i = 0; $i -lt $hex.Length; $i += 2) {
      $bytes[$i / 2] = [Convert]::ToByte($hex.Substring($i, 2), 16)
    }
    $b64 = [Convert]::ToBase64String($bytes)
    # -k: self-signed fails CA check before pin; pin still enforced by curl
    $args = @('-fsSL', '-k', '--pinnedpubkey', "sha256//$b64")
    if ($OutFile) { $args += @('-o', $OutFile) }
    $args += $Uri
    & curl.exe @args
    $code = $LASTEXITCODE
    if ($code -ne 0) { throw "gateway download failed (curl exit=$code)" }
    return
  }
  if ($OutFile) {
    Invoke-WebRequest -Uri $Uri -OutFile $OutFile -UseBasicParsing
    return
  }
  return Invoke-RestMethod -Uri $Uri -UseBasicParsing
}

# Persist install-time proxy env so the Windows Service Agent can CONNECT to Gateway.
function Get-InstallGatewayProxy {
  foreach ($name in @('https_proxy', 'HTTPS_PROXY', 'ALL_PROXY', 'all_proxy', 'http_proxy', 'HTTP_PROXY')) {
    $v = [Environment]::GetEnvironmentVariable($name, 'Process')
    if (-not $v) { $v = [Environment]::GetEnvironmentVariable($name, 'User') }
    if (-not $v) { $v = [Environment]::GetEnvironmentVariable($name, 'Machine') }
    if ($v -and $v.Trim()) { return $v.Trim() }
  }
  return $null
}

function Test-AgentSha256([string]$Path) {
  $expect = if ($Arch -eq 'arm64') { $AgentSha256Arm64 } else { $AgentSha256Amd64 }
  if (-not $expect) {
    Write-Host "==> WARNING: no embedded agent sha256 for $Arch; skip integrity check"
    return
  }
  $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $Path).Hash.ToLowerInvariant()
  if ($hash -ne $expect.ToLowerInvariant()) {
    throw "agent sha256 mismatch (got $hash want $expect)"
  }
  Write-Host '==> Agent sha256 ok'
}

function Stop-NamedService([string]$Name) {
  $svc = Get-Service -Name $Name -ErrorAction SilentlyContinue
  if ($svc -and $svc.Status -ne 'Stopped') {
    Stop-Service -Name $Name -Force -ErrorAction SilentlyContinue
    try { $svc.WaitForStatus('Stopped', [TimeSpan]::FromSeconds(20)) } catch {}
  }
}

function Stop-WoopsAgent {
  Stop-NamedService $ServiceName
  Get-Process -Name 'woops-agent' -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
}

$svcRunning = $false
try {
  $svcRunning = (Get-Service -Name $ServiceName -ErrorAction Stop).Status -eq 'Running'
} catch {}
$Live = $svcRunning -or [bool](Get-Process -Name 'woops-agent' -ErrorAction SilentlyContinue)
if ($Live) {
  Write-Host '==> Live update: keep current agent until download and staging finish'
} else {
  Write-Host '==> Stopping previous woops-agent (if any)'
  Stop-WoopsAgent
  Start-Sleep -Seconds 1
}

Remove-Item -Force -ErrorAction SilentlyContinue (Join-Path $ConfDir 'agent-id')

Write-Host "==> Downloading woops-agent ($Os/$Arch)..."
$Url = "$GatewayBase/i/$InstallCode/agent/$Os/$Arch"
$Dest = if ($Live) { $BinNew } else { $Bin }
$Gz = "$Dest.gz"
try {
  Invoke-GwRequest -Uri ($Url + '?format=gz') -OutFile $Gz
  $in = [System.IO.File]::OpenRead($Gz)
  $out = [System.IO.File]::Create($Dest)
  $gs = New-Object System.IO.Compression.GzipStream($in, [System.IO.Compression.CompressionMode]::Decompress)
  $gs.CopyTo($out)
  $gs.Dispose(); $out.Dispose(); $in.Dispose()
  Remove-Item -Force $Gz
  Write-Host '    (gzip transfer)'
} catch {
  Remove-Item -Force -ErrorAction SilentlyContinue $Gz
  Write-Host '    (gzip unavailable; downloading raw exe)'
  Invoke-GwRequest -Uri $Url -OutFile $Dest
}
Test-AgentSha256 $Dest

# ConPTY needs Windows 10 1809 / Server 2019 (build 17763+). Older hosts need WinPTY.
# Live update: never overwrite locked winpty.* - stage *-new* and swap after stop.
$Build = 0
try { $Build = [int](Get-CimInstance Win32_OperatingSystem).BuildNumber } catch {}
$WinptyDll = Join-Path $BinDir 'winpty.dll'
$WinptyAgent = Join-Path $BinDir 'winpty-agent.exe'
$WinptyDllNew = Join-Path $BinDir 'winpty-new.dll'
$WinptyAgentNew = Join-Path $BinDir 'winpty-agent-new.exe'
$NeedWinpty = ($Build -gt 0 -and $Build -lt 17763)
if ($NeedWinpty) {
  if ($Arch -ne 'amd64') { throw "WinPTY fallback only packaged for amd64; this host arch=$Arch build=$Build" }
  Write-Host "==> OS build $Build < 17763 (no ConPTY); downloading WinPTY runtime"
  foreach ($pair in @(
    @{ Name = 'winpty.dll'; Dest = $(if ($Live) { $WinptyDllNew } else { $WinptyDll }) },
    @{ Name = 'winpty-agent.exe'; Dest = $(if ($Live) { $WinptyAgentNew } else { $WinptyAgent }) }
  )) {
    $wurl = "$GatewayBase/i/$InstallCode/winpty/$Arch/$($pair.Name)"
    Write-Host "    $($pair.Name) -> $($pair.Dest)"
    Invoke-GwRequest -Uri $wurl -OutFile $pair.Dest
  }
} else {
  Write-Host "==> OS build $Build supports ConPTY; skipping WinPTY download"
  if (-not $Live) {
    Remove-Item -Force -ErrorAction SilentlyContinue $WinptyDll, $WinptyAgent
  }
}

try {
  $AgentVersion = (& $Dest -version 2>$null | Select-Object -First 1 | Out-String).Trim()
} catch {
  $AgentVersion = ''
}
if (-not $AgentVersion) { throw "cannot read version from $Dest" }
Write-Host "==> Agent version: $AgentVersion"

# Keep https:// (or http:// for local dev); do not strip scheme.
$GatewayUrl = $GatewayBase.TrimEnd('/')
$TlsPin = $GatewayTlsSpkiSha256
$Cfg = Join-Path $ConfDir 'agent.yaml'
function Set-AtomicContent([string]$Path, [object]$Value, [string]$Encoding) {
  $tmp = Join-Path ([System.IO.Path]::GetDirectoryName($Path)) ('.' + [System.IO.Path]::GetFileName($Path) + '.' + [Guid]::NewGuid().ToString('N') + '.tmp')
  Set-Content -LiteralPath $tmp -Value $Value -Encoding $Encoding
  if (Test-Path -LiteralPath $Path) {
    [System.IO.File]::Replace($tmp, $Path, $null)
  } else {
    Move-Item -LiteralPath $tmp -Destination $Path
  }
}
# Fresh install: annotated template. Re-install: refresh gateway + pin, keep other local edits.
if (Test-Path -LiteralPath $Cfg) {
  $lines = Get-Content -LiteralPath $Cfg
  $replacedGateway = $false
  $replacedPin = $false
  $out = foreach ($line in $lines) {
    if (-not $replacedGateway -and $line -match '^gateway:') {
      $replacedGateway = $true
      'gateway: "' + $GatewayUrl + '"'
    } elseif (-not $replacedPin -and $line -match '^gatewayTlsSpkiSha256:') {
      $replacedPin = $true
      'gatewayTlsSpkiSha256: "' + $TlsPin + '"'
    } else {
      $line
    }
  }
  if (-not $replacedGateway) {
    $out = @('gateway: "' + $GatewayUrl + '"') + @($out)
  }
  if (-not $replacedPin) {
    $out = @($out) + @('gatewayTlsSpkiSha256: "' + $TlsPin + '"')
  }
  Set-AtomicContent -Path $Cfg -Value $out -Encoding utf8
} else {
  # Base64 avoids PowerShell @'...'@ here-string terminator rules (closing '@ must be column 0;
  # also breaks under some download/encoding paths).
  $Yaml = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('{{AGENT_YAML_TEMPLATE_B64}}'))
  $Yaml = $Yaml.Replace('__GATEWAY__', $GatewayUrl).Replace('__TLS_PIN__', $TlsPin)
  Set-AtomicContent -Path $Cfg -Value $Yaml -Encoding utf8
}

# Persist install-time proxy env so the Windows Service Agent can CONNECT to Gateway.
function Set-YamlQuotedKey([string]$Path, [string]$Key, [string]$Value) {
  $esc = $Value.Replace('\', '\\').Replace('"', '\"')
  $line = $Key + ': "' + $esc + '"'
  $lines = @(Get-Content -LiteralPath $Path)
  $done = $false
  $out = foreach ($l in $lines) {
    if (-not $done -and $l -match ("^" + [regex]::Escape($Key) + ":")) {
      $done = $true
      $line
    } else {
      $l
    }
  }
  if (-not $done) {
    $inserted = $false
    $out2 = foreach ($l in $out) {
      $l
      if (-not $inserted -and $l -match '^gatewayTlsSpkiSha256:') {
        $line
        $inserted = $true
      }
    }
    if (-not $inserted) { $out2 = @($out2) + @($line) }
    $out = $out2
  }
  Set-AtomicContent -Path $Path -Value $out -Encoding utf8
}
$GwProxy = Get-InstallGatewayProxy
if ($GwProxy) {
  Write-Host '==> Writing gatewayProxy from install env (Agent WSS will use this proxy)'
  Set-YamlQuotedKey -Path $Cfg -Key 'gatewayProxy' -Value $GwProxy
}

# Agent bootstrap owns registration and atomic credential replacement. Publish
# install-code only after agent.yaml is final.
$InstallCodePath = Join-Path $ConfDir 'install-code'
$InstallCodeTmp = Join-Path $ConfDir ('.install-code.' + [Guid]::NewGuid().ToString('N') + '.tmp')
Set-Content -LiteralPath $InstallCodeTmp -Value $InstallCode -Encoding ascii
& icacls.exe $InstallCodeTmp /inheritance:r /grant:r '*S-1-5-18:F' '*S-1-5-32-544:F' | Out-Null
if ($LASTEXITCODE -ne 0) {
  Remove-Item -Force -ErrorAction SilentlyContinue $InstallCodeTmp
  throw 'failed to restrict temporary install-code ACL'
}
if (Test-Path -LiteralPath $InstallCodePath) {
  [System.IO.File]::Replace($InstallCodeTmp, $InstallCodePath, $null)
} else {
  Move-Item -LiteralPath $InstallCodeTmp -Destination $InstallCodePath
}
& icacls.exe $InstallCodePath /inheritance:r /grant:r '*S-1-5-18:F' '*S-1-5-32-544:F' | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'failed to verify install-code ACL' }

$BinPathName = '"' + $Bin + '" -config "' + $Cfg + '"'
function Install-WoopsAgentService {
  $existing = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
  if ($existing) {
    # Avoid sc.exe binPath= quoting (exit 1639 with paths under Program Files).
    # ImagePath is what SCM uses; Set-Service covers StartupType.
    $svcKey = "HKLM:\SYSTEM\CurrentControlSet\Services\$ServiceName"
    Set-ItemProperty -LiteralPath $svcKey -Name ImagePath -Value $BinPathName
    Set-Service -Name $ServiceName -StartupType Automatic
  } else {
    Write-Host "==> Creating Windows service $ServiceName"
    New-Service -Name $ServiceName -DisplayName 'Woops Agent' -Description 'Woops host agent' -BinaryPathName $BinPathName -StartupType Automatic | Out-Null
  }
  $st = (Get-Service -Name $ServiceName).StartType
  if ($st -ne 'Automatic') {
    Write-Host "WARNING: service StartType is $st; forcing Automatic"
    Set-Service -Name $ServiceName -StartupType Automatic
  }
}

Write-Host "==> Registering Windows service $ServiceName (Automatic)"
Install-WoopsAgentService

if ($Live) {
  Write-Host "==> Staging restart in 2s (Web Shell will disconnect; agent comes back online)"
  $restartPath = Join-Path $ConfDir 'restart-update.ps1'
  $restartLines = @(
    '$ErrorActionPreference = ''Stop''',
    'Start-Sleep -Seconds 2',
    ('$svc = Get-Service -Name ''' + $ServiceName + ''' -ErrorAction SilentlyContinue'),
    ('if ($svc -and $svc.Status -ne ''Stopped'') { Stop-Service -Name ''' + $ServiceName + ''' -Force -ErrorAction SilentlyContinue; Start-Sleep -Seconds 2 }'),
    'Get-Process -Name ''woops-agent'' -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue',
    'Start-Sleep -Seconds 1',
    ('if (Test-Path -LiteralPath ''' + $BinNew + ''') { Move-Item -Force -LiteralPath ''' + $BinNew + ''' -Destination ''' + $Bin + ''' }'),
    ('if (Test-Path -LiteralPath ''' + $WinptyDllNew + ''') { Move-Item -Force -LiteralPath ''' + $WinptyDllNew + ''' -Destination ''' + $WinptyDll + ''' }'),
    ('if (Test-Path -LiteralPath ''' + $WinptyAgentNew + ''') { Move-Item -Force -LiteralPath ''' + $WinptyAgentNew + ''' -Destination ''' + $WinptyAgent + ''' }'),
    $(if (-not $NeedWinpty) {
      ('Remove-Item -Force -ErrorAction SilentlyContinue -LiteralPath ''' + $WinptyDll + ''',''' + $WinptyAgent + ''',''' + $WinptyDllNew + ''',''' + $WinptyAgentNew + '''')
    } else { '# keep winpty' }),
    ('Start-Service -Name ''' + $ServiceName + ''''),
    'Write-Host ''woops-agent service restarted'''
  )
  Set-Content -Path $restartPath -Value $restartLines -Encoding utf8
  Start-Process -FilePath 'powershell.exe' -ArgumentList @('-NoProfile','-ExecutionPolicy','Bypass','-File',$restartPath) -WindowStyle Hidden
  Write-Host "==> Update staged (version $AgentVersion). Reconnect shell after agent is online."
  Write-Host "    Restart later: Restart-Service $ServiceName"
} else {
  Write-Host "==> Starting Windows service $ServiceName..."
  Stop-WoopsAgent
  Start-Sleep -Seconds 1
  Start-Service -Name $ServiceName
  Write-Host 'woops-agent service started'
  Write-Host '==> Waiting up to 60s for Agent bootstrap registration...'
  $registered = $false
  $AssetIdPath = Join-Path $ConfDir 'asset-id'
  $AgentTokenPath = Join-Path $ConfDir 'agent-token'
  for ($i = 0; $i -lt 60; $i++) {
    $hasAsset = (Test-Path -LiteralPath $AssetIdPath) -and ((Get-Item -LiteralPath $AssetIdPath).Length -gt 0)
    $hasToken = (Test-Path -LiteralPath $AgentTokenPath) -and ((Get-Item -LiteralPath $AgentTokenPath).Length -gt 0)
    if ($hasAsset -and $hasToken -and -not (Test-Path -LiteralPath $InstallCodePath)) {
      $registered = $true
      break
    }
    Start-Sleep -Seconds 1
  }
  if (-not $registered) {
    $state = (Get-Service -Name $ServiceName -ErrorAction SilentlyContinue).Status
    Write-Error "Agent registration did not complete within 60s (service=$state). Check $ConfDir\woops-agent.log; credentials were not printed."
  }
  Write-Host '==> Agent registered successfully.'
  Write-Host "    Restart later: Restart-Service $ServiceName"
}
