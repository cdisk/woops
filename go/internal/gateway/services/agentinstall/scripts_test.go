package agentinstall

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestRenderLinux(t *testing.T) {
	out, err := Render("install.sh", Params{
		GatewayBase:   "https://gw.example",
		InstallCode:   "CODE123",
		TLSSpkiSHA256: "aabbcc",
		AgentSHA256ByArch: map[string]string{
			"amd64": "deadbeef",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "{{GATEWAY_BASE}}") || strings.Contains(out, "{{INSTALL_CODE}}") {
		t.Fatal("placeholders not replaced")
	}
	if strings.Contains(out, "{{API_BASE}}") || strings.Contains(out, "API_BASE=") {
		t.Fatal("API_BASE should be merged into GATEWAY_BASE")
	}
	if strings.Contains(out, "{{AGENT_YAML_TEMPLATE}}") {
		t.Fatal("agent.yaml template not injected")
	}
	if !strings.Contains(out, `GATEWAY_BASE="https://gw.example"`) {
		t.Fatalf("missing GATEWAY_BASE: %s", out[:160])
	}
	if !strings.Contains(out, `INSTALL_CODE="CODE123"`) {
		t.Fatal("missing INSTALL_CODE")
	}
	if !strings.Contains(out, `GATEWAY_TLS_SPKI_SHA256="aabbcc"`) {
		t.Fatal("missing TLS pin")
	}
	if !strings.Contains(out, `AGENT_SHA256_AMD64="deadbeef"`) {
		t.Fatal("missing agent sha256")
	}
	if strings.Contains(out, "/api/agent/register") || strings.Contains(out, "BODY=") || strings.Contains(out, "AGENT_TOKEN=") {
		t.Fatal("installer must delegate registration and credential writes to Agent bootstrap")
	}
	if !strings.Contains(out, "proxy:") || !strings.Contains(out, "allowGlobal") {
		t.Fatal("expected proxy comment block in injected agent.yaml template")
	}
	if !strings.Contains(out, "gatewayTlsSpkiSha256") {
		t.Fatal("expected pin field in agent.yaml template")
	}
	if !strings.Contains(out, `gateway: "__GATEWAY__"`) && !strings.Contains(out, "gateway:") {
		t.Fatal("expected gateway: in agent.yaml template")
	}
	if !strings.Contains(out, "gatewayProxy") || !strings.Contains(out, "detect_gateway_proxy_env") {
		t.Fatal("expected gatewayProxy persist from https_proxy env")
	}
	if !strings.Contains(out, "grep -qE '^gateway:'") || strings.Contains(out, "grep -qE '^[[:space:]]*gateway:'") {
		t.Fatal("Linux reinstall must update only top-level gateway and preserve nested proxyBridge keys")
	}
	if !strings.Contains(out, "/etc/woops-agent") || !strings.Contains(out, "woops-agent.service") {
		t.Fatal("expected woops-agent paths in install.sh")
	}
	if strings.Contains(out, "/etc/ops-agent") || strings.Contains(out, "remove_legacy") || strings.Contains(out, "disable --now ops-agent") || strings.Contains(out, "pkill -9 -x ops-agent") {
		t.Fatal("legacy ops-agent migration must be removed from install.sh")
	}
	if !strings.Contains(out, "need_staged_swap") || !strings.Contains(out, "final path was free") {
		t.Fatal("expected staged-swap only when woops-agent binary path is busy")
	}
	if !strings.Contains(out, "need_deferred_restart") || !strings.Contains(out, "online update session will disconnect") {
		t.Fatal("expected deferred restart whenever LIVE agent session must survive install")
	}
	if !strings.Contains(out, "systemd-run") || !strings.Contains(out, "--no-block") {
		t.Fatal("expected systemd-run --no-block so deferred restart survives exec session end")
	}
	if !strings.Contains(out, "Must not stop the live agent here") {
		t.Fatal("expected install path to avoid stopping live agent during online update")
	}
	if strings.Contains(out, "\nserver:") || strings.Contains(out, "__SERVER__") {
		t.Fatal("legacy server: / __SERVER__ should be gone from template")
	}
	if strings.Contains(out, "%%s") {
		t.Fatal("legacy Sprintf %% escapes should not remain")
	}
	configAt := strings.Index(out, "write_agent_yaml\n")
	codeAt := strings.Index(out, "write_install_code\n")
	startAt := strings.Index(out, `echo "==> Starting woops-agent`)
	if configAt < 0 || codeAt < 0 || startAt < 0 || !(configAt < codeAt && codeAt < startAt) {
		t.Fatal("agent.yaml must be published before install-code and cold start")
	}
	if !strings.Contains(out, `chmod 0600 "$CONF_DIR/install-code"`) {
		t.Fatal("Linux install-code must be mode 0600")
	}
	if !strings.Contains(out, "Waiting up to 60s for Agent bootstrap registration") ||
		!strings.Contains(out, `[ ! -e "$CONF_DIR/install-code" ]`) {
		t.Fatal("cold install must wait for credentials and consumed install-code")
	}
	if strings.Index(out, "Update staged (version") > strings.Index(out, "Waiting up to 60s for Agent bootstrap registration") {
		t.Fatal("live update must return without entering cold registration wait")
	}
}

func TestRenderWindows(t *testing.T) {
	out, err := Render("install.ps1", Params{
		GatewayBase: "https://gw.example",
		InstallCode: "CODE123",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "$ApiBase") || strings.Contains(out, "API_BASE") {
		t.Fatal("ApiBase/API_BASE should be gone")
	}
	if !strings.Contains(out, `$GatewayBase = 'https://gw.example'`) {
		t.Fatal("missing GatewayBase")
	}
	if !strings.Contains(out, `$InstallCode = 'CODE123'`) {
		t.Fatal("missing InstallCode")
	}
	if strings.Contains(out, "/api/agent/register") || strings.Contains(out, "Invoke-RestMethod -Method Post") ||
		strings.Contains(out, "$Resp.agentToken") || strings.Contains(out, "$Resp.assetId") ||
		strings.Contains(out, "Set-Content -Path (Join-Path $ConfDir 'agent-token')") {
		t.Fatal("PowerShell installer must delegate registration and credential writes to Agent bootstrap")
	}
	if strings.Contains(out, "{{AGENT_YAML_TEMPLATE}}") || strings.Contains(out, "{{AGENT_YAML_TEMPLATE_B64}}") {
		t.Fatal("agent.yaml template placeholders not replaced")
	}
	if !strings.Contains(out, "FromBase64String") {
		t.Fatal("expected base64 yaml decode in Windows install.ps1")
	}
	if strings.Contains(out, "$Yaml = @'") {
		t.Fatal("Windows install must not use @' here-string for yaml template")
	}
	// Decode embedded yaml and sanity-check contents.
	const marker = "FromBase64String('"
	i := strings.Index(out, marker)
	if i < 0 {
		t.Fatal("missing base64 literal")
	}
	rest := out[i+len(marker):]
	j := strings.Index(rest, "')")
	if j < 0 {
		t.Fatal("unclosed base64 literal")
	}
	raw, err := base64.StdEncoding.DecodeString(rest[:j])
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}
	yamlText := string(raw)
	if !strings.Contains(yamlText, "proxy:") || !strings.Contains(yamlText, "__GATEWAY__") {
		t.Fatal("expected agent.yaml template content inside base64")
	}
	if !strings.Contains(yamlText, "gateway:") || strings.Contains(yamlText, "\nserver:") {
		t.Fatal("agent.yaml should use gateway: not server:")
	}
	if !strings.Contains(out, "gatewayProxy") || !strings.Contains(out, "Get-InstallGatewayProxy") {
		t.Fatal("expected Windows gatewayProxy persist from https_proxy env")
	}
	if !strings.Contains(out, "-match '^gateway:'") || strings.Contains(out, "-match '^\\s*gateway:'") {
		t.Fatal("PowerShell reinstall must update only top-level gateway and preserve nested proxyBridge keys")
	}
	if !strings.Contains(out, "curl.exe") {
		t.Fatal("expected curl.exe for Windows TLS pin downloads")
	}
	if !strings.Contains(out, "woops-agent") || !strings.Contains(out, "Woops Agent") {
		t.Fatal("expected woops-agent branding in install.ps1")
	}
	if strings.Contains(out, "'ops-agent'") || strings.Contains(out, "LegacyServiceName") || strings.Contains(out, "Remove-LegacyOpsAgentService") {
		t.Fatal("legacy ops-agent migration must be removed from install.ps1")
	}
	if !strings.Contains(out, "Administrator") {
		t.Fatal("expected elevation check in install.ps1")
	}
	if strings.Contains(out, "throw @\"") || strings.Contains(out, "throw @'") {
		t.Fatal("install.ps1 must not use throw here-strings (PS 5.1 ANSI decode breaks UTF-8)")
	}
	configAt := strings.Index(out, "$GwProxy = Get-InstallGatewayProxy")
	codeAt := strings.Index(out, "$InstallCodePath = Join-Path $ConfDir 'install-code'")
	serviceAt := strings.Index(out, `$BinPathName = '"' + $Bin`)
	if configAt < 0 || codeAt < 0 || serviceAt < 0 || !(configAt < codeAt && codeAt < serviceAt) {
		t.Fatal("agent.yaml must be updated before install-code and service setup")
	}
	if !strings.Contains(out, "Set-AtomicContent") || !strings.Contains(out, "[System.IO.File]::Replace") {
		t.Fatal("PowerShell config/install-code writes must use same-volume atomic replacement")
	}
	if !strings.Contains(out, "/inheritance:r") || !strings.Contains(out, "*S-1-5-18:F") ||
		!strings.Contains(out, "*S-1-5-32-544:F") {
		t.Fatal("install-code ACL must be restricted to SYSTEM and Administrators")
	}
	if !strings.Contains(out, "Waiting up to 60s for Agent bootstrap registration") ||
		!strings.Contains(out, "-not (Test-Path -LiteralPath $InstallCodePath)") {
		t.Fatal("cold install must wait for credentials and consumed install-code")
	}
	if strings.Index(out, "Update staged (version") > strings.Index(out, "Waiting up to 60s for Agent bootstrap registration") {
		t.Fatal("registration wait must remain in the non-live branch")
	}
}

func TestRenderWindowsBat(t *testing.T) {
	hex := "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	out, err := Render("install.bat", Params{
		GatewayBase:       "https://gw.example",
		InstallCode:       "CODE123",
		TLSSpkiSHA256:     hex,
		AgentSHA256ByArch: map[string]string{"amd64": "deadbeef"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "{{") {
		t.Fatal("placeholders not replaced")
	}
	if !strings.Contains(out, `set "GATEWAY=https://gw.example"`) {
		t.Fatal("missing GATEWAY")
	}
	if !strings.Contains(out, `set "INSTALL_CODE=CODE123"`) {
		t.Fatal("missing INSTALL_CODE")
	}
	if !strings.Contains(out, "sha256//") {
		t.Fatal("missing CURL_PIN")
	}
	if !strings.Contains(out, "winpty.dll") {
		t.Fatal("expected winpty download for legacy OS")
	}
	if !strings.Contains(out, `set "TEMP_DIR=%CONF_DIR%\install-temp"`) {
		t.Fatal("legacy service update must not depend on a LocalSystem TEMP directory")
	}
	if !strings.Contains(out, `set "BIN_NEW=%BIN_DIR%\woops-agent-new.exe"`) ||
		!strings.Contains(out, `restart-update.bat`) {
		t.Fatal("install.bat must stage a detached live update")
	}
	if !strings.Contains(out, `if not exist "%CFG%" (`) ||
		!strings.Contains(out, "Existing agent.yaml preserved ^(including proxyBridge^)") {
		t.Fatal("legacy BAT must preserve an existing config, including proxyBridge")
	}
	if strings.Contains(out, `>"%CFG%" (`) || strings.Contains(out, `findstr /V /C:"gateway:"`) {
		t.Fatal("legacy BAT must not rewrite or merge an existing YAML file")
	}
	if !strings.Contains(out, `echo [ERROR] Service start failed`) {
		t.Fatal("manual install must report service start failures")
	}
	if strings.Contains(out, "powershell") {
		t.Fatal("install.bat must be pure cmd")
	}
	if !strings.Contains(out, "\r\n") {
		t.Fatal("install.bat must use CRLF line endings for legacy cmd")
	}
	if strings.Contains(out, "/api/agent/register") || strings.Contains(out, "register.json") ||
		strings.Contains(out, "ParseRegisterResp") || strings.Contains(out, `>"%TOKEN_FILE%"`) ||
		strings.Contains(out, `>"%ID_FILE%"`) {
		t.Fatal("legacy BAT must delegate registration and credential writes to Agent bootstrap")
	}
	configAt := strings.Index(out, `if not exist "%CFG%" (`)
	codeAt := strings.Index(out, `set "CODE_TMP=%CONF_DIR%\install-code.tmp"`)
	serviceAt := strings.Index(out, `set "BINPATH=`)
	if configAt < 0 || codeAt < 0 || serviceAt < 0 || !(configAt < codeAt && codeAt < serviceAt) {
		t.Fatal("BAT must publish config before install-code and service setup")
	}
	if !strings.Contains(out, `move /Y "!CODE_TMP!" "%CODE_FILE%"`) ||
		!strings.Contains(out, `icacls "%CODE_FILE%" /inheritance:r`) {
		t.Fatal("BAT must safely publish and restrict install-code")
	}
	if !strings.Contains(out, "Waiting up to 60s for Agent bootstrap registration") ||
		!strings.Contains(out, `if defined WAIT_ID if defined WAIT_TOKEN if not exist "%CODE_FILE%"`) {
		t.Fatal("cold BAT install must wait for credentials and consumed install-code")
	}
	if strings.Index(out, "Update staged; service restart scheduled") >
		strings.Index(out, "Waiting up to 60s for Agent bootstrap registration") {
		t.Fatal("live BAT update must return before the cold registration wait")
	}
}
