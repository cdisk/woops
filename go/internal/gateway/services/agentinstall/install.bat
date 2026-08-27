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
set "SVC=woops-agent"
set "TEMP_DIR=%CONF_DIR%\install-temp"
set "BODY_FILE=%TEMP_DIR%\register.json"
set "RESP_FILE=%TEMP_DIR%\register.resp"
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
if exist "%RESP_FILE%" del /f /q "%RESP_FILE%" >nul 2>&1
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

set "HOSTNAME=%COMPUTERNAME%"
set "OS_LABEL=Windows"
for /f "tokens=1,* delims==" %%a in ('wmic os get Caption /value 2^>nul ^| findstr /B "Caption="') do set "OS_LABEL=%%b"
set "OS_LABEL=!OS_LABEL: =!"
if "!OS_LABEL!"=="" set "OS_LABEL=Windows"
echo ==^> OS: !OS_LABEL!

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

set "EXISTING_ID="
if exist "%ID_FILE%" (
  set /p "EXISTING_ID="<"%ID_FILE%"
  set "EXISTING_ID=!EXISTING_ID: =!"
  set "EXISTING_ID=!EXISTING_ID:"=!"
)
if defined EXISTING_ID (
  echo ==^> Re-register assetId=!EXISTING_ID!
) else (
  echo ==^> First-time register
)

set "REG_VER=!AGENT_VER!"
set "REG_HOST=!HOSTNAME!"
set "REG_OS=!OS_LABEL!"
set "REG_AID=!EXISTING_ID!"
setlocal DisableDelayedExpansion
if defined REG_AID (
  >"%BODY_FILE%" echo {"installCode":"%INSTALL_CODE%","assetId":"%REG_AID%","agentVersion":"%REG_VER%","hostname":"%REG_HOST%","os":"%REG_OS%","arch":"amd64"}
) else (
  >"%BODY_FILE%" echo {"installCode":"%INSTALL_CODE%","agentVersion":"%REG_VER%","hostname":"%REG_HOST%","os":"%REG_OS%","arch":"amd64"}
)
endlocal

echo ==^> Register...
if "%TLS_PIN%"=="" (
  curl.exe -fsSL -X POST -H "Content-Type: application/json" --data-binary "@%BODY_FILE%" -o "%RESP_FILE%" "%GATEWAY%/api/agent/register"
) else (
  curl.exe -fsSL -k --pinnedpubkey "%CURL_PIN%" -X POST -H "Content-Type: application/json" --data-binary "@%BODY_FILE%" -o "%RESP_FILE%" "%GATEWAY%/api/agent/register"
)
if errorlevel 1 (
  echo [ERROR] Register failed
  if exist "%RESP_FILE%" type "%RESP_FILE%"
  exit /b 1
)
type "%RESP_FILE%"
echo.

findstr /C:"assetId" "%RESP_FILE%" >nul 2>&1
if errorlevel 1 (
  echo [ERROR] Register response invalid
  exit /b 1
)

call :ParseRegisterResp
if errorlevel 1 (
  echo [ERROR] Parse register response failed
  exit /b 1
)
echo ==^> assetId=!ASSET_ID!

REM Always rewrite minimal agent.yaml (ASCII). Do not merge UTF-8/BOM content from install.ps1.
REM findstr append leaves a BOM on line 3 and breaks the Go yaml parser.
>"%CFG%" (
  echo gateway: "%GATEWAY%"
  echo gatewayTlsSpkiSha256: "%TLS_PIN%"
  echo.
  echo metrics:
  echo   enabled: false
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
echo [OK] Done. Log: %CONF_DIR%\woops-agent.log
exit /b 0

:ParseRegisterResp
set "LINE="
for /f "usebackq delims=" %%a in ("%RESP_FILE%") do set "LINE=%%a"
set "ASSET_ID="
set "AGENT_TOKEN="
set "TMP=!LINE:*"assetId":"=!"
if not "!TMP!"=="!LINE!" for /f "tokens=1 delims=," %%a in ("!TMP!") do set "ASSET_ID=%%~a"
set "ASSET_ID=!ASSET_ID:"=!"
set "ASSET_ID=!ASSET_ID:}=!"
set "TMP=!LINE:*"agentToken":"=!"
if not "!TMP!"=="!LINE!" for /f "tokens=1 delims=," %%a in ("!TMP!") do set "AGENT_TOKEN=%%~a"
set "AGENT_TOKEN=!AGENT_TOKEN:"=!"
set "AGENT_TOKEN=!AGENT_TOKEN:}=!"
if "!ASSET_ID!"=="" exit /b 1
if "!AGENT_TOKEN!"=="" exit /b 1
> "%ID_FILE%" echo:!ASSET_ID!
> "%TOKEN_FILE%" echo:!AGENT_TOKEN!
exit /b 0
