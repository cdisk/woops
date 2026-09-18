@echo off
setlocal EnableExtensions EnableDelayedExpansion
REM install.bat for Win7 / Server 2012 (pure cmd, ASCII only)

set "GATEWAY={{GATEWAY_BASE}}"
set "INSTALL_CODE={{INSTALL_CODE}}"
set "TLS_PIN={{GATEWAY_TLS_SPKI_SHA256}}"
set "CURL_PIN={{CURL_PIN}}"
set "AGENT_SHA256={{AGENT_SHA256_AMD64}}"

set "CONF_DIR=%ProgramData%\woops-agent"
set "BIN_DIR=%ProgramFiles%\woops-agent"
set "BIN=%BIN_DIR%\woops-agent.exe"
set "BIN_NEW=%BIN_DIR%\woops-agent-new.exe"
set "CFG=%CONF_DIR%\agent.yaml"
set "ID_FILE=%CONF_DIR%\asset-id"
set "TOKEN_FILE=%CONF_DIR%\agent-token"
set "CODE_FILE=%CONF_DIR%\install-code"
set "SVC=woops-agent"
set "TEMP_DIR=%CONF_DIR%\install-temp"
set "AGENT_URL=%GATEWAY%/i/%INSTALL_CODE%/agent/windows/amd64"

net session >nul 2>&1
if errorlevel 1 (
  echo [ERROR] Run as Administrator
  exit /b 1
)

where curl.exe >nul 2>&1
if errorlevel 1 (
  echo [ERROR] curl.exe not found in PATH
  exit /b 1
)

if not exist "%TEMP_DIR%" mkdir "%TEMP_DIR%" >nul 2>&1
if not exist "%CONF_DIR%" mkdir "%CONF_DIR%" >nul 2>&1
if not exist "%BIN_DIR%" mkdir "%BIN_DIR%" >nul 2>&1

set "LIVE=0"
sc query "%SVC%" 2>nul | findstr /I "RUNNING" >nul 2>&1
if not errorlevel 1 set "LIVE=1"
if "%LIVE%"=="1" echo ==^> Live update: staging files before service restart

echo ==^> Gateway: %GATEWAY%
echo ==^> Download agent...
if "%TLS_PIN%"=="" (
  curl.exe -fsSL -o "%TEMP_DIR%\woops-agent.exe" "%AGENT_URL%"
) else (
  curl.exe -fsSL -k --pinnedpubkey "%CURL_PIN%" -o "%TEMP_DIR%\woops-agent.exe" "%AGENT_URL%"
)
if errorlevel 1 (
  echo [ERROR] Download agent failed
  exit /b 1
)

if not "%AGENT_SHA256%"=="" (
  set "FILE_HASH="
  for /f "skip=1 tokens=*" %%h in ('certutil -hashfile "%TEMP_DIR%\woops-agent.exe" SHA256 2^>nul') do (
    if not defined FILE_HASH set "FILE_HASH=%%h"
  )
  set "FILE_HASH=!FILE_HASH: =!"
  if /I not "!FILE_HASH!"=="%AGENT_SHA256%" (
    echo [ERROR] Agent sha256 mismatch
    echo expected: %AGENT_SHA256%
    echo got:      !FILE_HASH!
    exit /b 1
  )
  echo ==^> Agent sha256 ok
)

set "DEST_BIN=%BIN%"
if "%LIVE%"=="1" set "DEST_BIN=%BIN_NEW%"
copy /Y "%TEMP_DIR%\woops-agent.exe" "%DEST_BIN%" >nul
if errorlevel 1 (
  echo [ERROR] Copy agent failed
  exit /b 1
)

set "AGENT_VER="
for /f "usebackq delims=" %%v in (`"%DEST_BIN%" -version 2^>nul`) do set "AGENT_VER=%%v" & goto :got_ver
:got_ver
if not defined AGENT_VER (
  echo [ERROR] Cannot read agent version
  exit /b 1
)
echo ==^> Version: !AGENT_VER!

set "BUILD=0"
for /f "tokens=1,* delims==" %%a in ('wmic os get BuildNumber /value 2^>nul ^| findstr /B "BuildNumber="') do set "BUILD=%%b"
if defined BUILD if !BUILD! LSS 17763 (
  echo ==^> OS build !BUILD! ^(no ConPTY^); downloading WinPTY
  set "WINPTY_DLL=%BIN_DIR%\winpty.dll"
  set "WINPTY_AGENT=%BIN_DIR%\winpty-agent.exe"
  if "%LIVE%"=="1" set "WINPTY_DLL=%BIN_DIR%\winpty-new.dll"
  if "%LIVE%"=="1" set "WINPTY_AGENT=%BIN_DIR%\winpty-agent-new.exe"
  if "%TLS_PIN%"=="" (
    curl.exe -fsSL -o "!WINPTY_DLL!" "%GATEWAY%/i/%INSTALL_CODE%/winpty/amd64/winpty.dll"
    curl.exe -fsSL -o "!WINPTY_AGENT!" "%GATEWAY%/i/%INSTALL_CODE%/winpty/amd64/winpty-agent.exe"
  ) else (
    curl.exe -fsSL -k --pinnedpubkey "%CURL_PIN%" -o "!WINPTY_DLL!" "%GATEWAY%/i/%INSTALL_CODE%/winpty/amd64/winpty.dll"
    curl.exe -fsSL -k --pinnedpubkey "%CURL_PIN%" -o "!WINPTY_AGENT!" "%GATEWAY%/i/%INSTALL_CODE%/winpty/amd64/winpty-agent.exe"
  )
)

REM Connection keys (gateway / pin) must always follow the install link, or the Agent
REM sends the new install-code to the old Gateway and gets rejected forever. Host-local
REM settings (gatewayProxy, proxy, proxyBridge, metrics) must survive untouched, so drop
REM only the two column-0 keys via findstr and re-append them; findstr passes blank
REM lines, indentation and UTF-8 bytes through unchanged.
REM mode: keep = gateway already correct, rewrite = surgical edit, fresh = minimal config.
REM A UTF-8 BOM glues itself to line 1 and defeats findstr /B there, so the old
REM gateway: would survive and collide with the appended one; when the key is not
REM matchable at column 0, fall back to a config that is guaranteed to parse.
set "CFG_MODE=fresh"
if exist "%CFG%" (
  set "CFG_MODE=rewrite"
  findstr /B /I /C:"gateway:" "%CFG%" | findstr /I /C:"%GATEWAY%" >nul 2>&1
  if not errorlevel 1 set "CFG_MODE=keep"
)
if "!CFG_MODE!"=="rewrite" (
  findstr /B /I /C:"gateway:" "%CFG%" >nul 2>&1
  if errorlevel 1 set "CFG_MODE=fresh"
)

set "CFG_TMP=%CONF_DIR%\agent.yaml.tmp"
if "!CFG_MODE!"=="keep" echo ==^> Existing agent.yaml preserved ^(gateway unchanged^)
if "!CFG_MODE!"=="rewrite" (
  echo ==^> Gateway changed; refreshing gateway + pin, keeping local proxy settings ^(backup: %CFG%.bak^)
  copy /Y "%CFG%" "%CFG%.bak" >nul
  findstr /V /B /I /C:"gateway:" /C:"gatewayTlsSpkiSha256:" "%CFG%" >"!CFG_TMP!" 2>nul
  >>"!CFG_TMP!" echo.
  >>"!CFG_TMP!" echo gateway: "%GATEWAY%"
  >>"!CFG_TMP!" echo gatewayTlsSpkiSha256: "%TLS_PIN%"
)
if "!CFG_MODE!"=="fresh" (
  if exist "%CFG%" (
    echo ==^> agent.yaml not editable by legacy cmd; writing minimal config ^(backup: %CFG%.bak^)
    echo [WARN] re-add proxy / proxyBridge from the .bak copy if the Agent cannot reach the new Gateway
    copy /Y "%CFG%" "%CFG%.bak" >nul
  )
  >"!CFG_TMP!" (
    echo gateway: "%GATEWAY%"
    echo gatewayTlsSpkiSha256: "%TLS_PIN%"
    echo.
    echo metrics:
    echo   enabled: false
  )
)
if not "!CFG_MODE!"=="keep" (
  move /Y "!CFG_TMP!" "%CFG%" >nul
  if errorlevel 1 (
    echo [ERROR] Cannot publish agent.yaml
    exit /b 1
  )
)

REM Publish install-code only after agent.yaml. Agent performs registration and
REM atomically replaces credentials; existing asset-id and agent-token stay intact.
set "CODE_TMP=%CONF_DIR%\install-code.tmp"
>"!CODE_TMP!" echo %INSTALL_CODE%
icacls "!CODE_TMP!" /inheritance:r /grant:r "*S-1-5-18:F" "*S-1-5-32-544:F" >nul 2>&1
if errorlevel 1 (
  del /f /q "!CODE_TMP!" >nul 2>&1
  echo [ERROR] Cannot restrict temporary install-code ACL
  exit /b 1
)
move /Y "!CODE_TMP!" "%CODE_FILE%" >nul
if errorlevel 1 (
  echo [ERROR] Cannot publish install-code
  exit /b 1
)
icacls "%CODE_FILE%" /inheritance:r /grant:r "*S-1-5-18:F" "*S-1-5-32-544:F" >nul 2>&1
if errorlevel 1 (
  echo [ERROR] Cannot restrict install-code ACL
  exit /b 1
)

set "BINPATH=\"%BIN%\" -config \"%CFG%\""
sc query "%SVC%" >nul 2>&1
if errorlevel 1 (
  sc create "%SVC%" binPath= "%BINPATH%" start= auto DisplayName= "Woops Agent"
) else (
  reg add "HKLM\SYSTEM\CurrentControlSet\Services\%SVC%" /v ImagePath /t REG_EXPAND_SZ /d "%BINPATH%" /f >nul
  sc config "%SVC%" start= auto >nul
)

if "%LIVE%"=="1" (
  set "RESTART_BAT=%CONF_DIR%\restart-update.bat"
  >"!RESTART_BAT!" echo @echo off
  >>"!RESTART_BAT!" echo ping -n 3 127.0.0.1 ^>nul
  >>"!RESTART_BAT!" echo sc stop "%SVC%" ^>nul 2^>^&1
  >>"!RESTART_BAT!" echo ping -n 4 127.0.0.1 ^>nul
  >>"!RESTART_BAT!" echo taskkill /F /IM woops-agent.exe ^>nul 2^>^&1
  >>"!RESTART_BAT!" echo ping -n 2 127.0.0.1 ^>nul
  >>"!RESTART_BAT!" echo move /Y "%BIN_NEW%" "%BIN%" ^>nul
  >>"!RESTART_BAT!" echo if exist "%BIN_DIR%\winpty-new.dll" move /Y "%BIN_DIR%\winpty-new.dll" "%BIN_DIR%\winpty.dll" ^>nul
  >>"!RESTART_BAT!" echo if exist "%BIN_DIR%\winpty-agent-new.exe" move /Y "%BIN_DIR%\winpty-agent-new.exe" "%BIN_DIR%\winpty-agent.exe" ^>nul
  >>"!RESTART_BAT!" echo sc start "%SVC%" ^>nul 2^>^&1
  start "" /min cmd.exe /d /c call "!RESTART_BAT!"
  echo ==^> Update staged; service restart scheduled
  exit /b 0
)

sc start "%SVC%" >nul 2>&1
if errorlevel 1 (
  echo [ERROR] Service start failed
  sc query "%SVC%"
  exit /b 1
)
echo ==^> Service started
echo ==^> Waiting up to 60s for Agent bootstrap registration...
set /a WAIT_COUNT=0
:wait_register
set "WAIT_ID="
set "WAIT_TOKEN="
if exist "%ID_FILE%" set /p "WAIT_ID="<"%ID_FILE%"
if exist "%TOKEN_FILE%" set /p "WAIT_TOKEN="<"%TOKEN_FILE%"
if defined WAIT_ID if defined WAIT_TOKEN if not exist "%CODE_FILE%" goto :registered
set /a WAIT_COUNT+=1
if !WAIT_COUNT! GEQ 60 goto :register_timeout
ping -n 2 127.0.0.1 >nul
goto :wait_register

:registered
echo [OK] Agent registered successfully. Log: %CONF_DIR%\woops-agent.log
exit /b 0

:register_timeout
echo [ERROR] Agent registration did not complete within 60s.
echo Check service state and %CONF_DIR%\woops-agent.log. Credentials were not printed.
sc query "%SVC%"
exit /b 1
