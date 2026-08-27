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
	if !strings.Contains(out, "$GATEWAY_BASE/api/agent/register") {
		t.Fatal("register should use GATEWAY_BASE")
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
	if !strings.Contains(out, "/etc/woops-agent") || !strings.Contains(out, "woops-agent.service") {
		t.Fatal("expected woops-agent paths in install.sh")
	}
	if strings.Contains(out, "/etc/ops-agent") || strings.Contains(out, "remove_legacy") || strings.Contains(out, "disable --now ops-agent") || strings.Contains(out, "pkill -9 -x ops-agent") {
		t.Fatal("legacy ops-agent migration must be removed from install.sh")
	}
	if !strings.Contains(out, "detect_hostname") || !strings.Contains(out, "/proc/sys/kernel/hostname") {
		t.Fatal("expected hostname fallbacks for minimal Linux (no hostname(1))")
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
	if !strings.Contains(out, `"$GatewayBase/api/agent/register"`) {
		t.Fatal("register should use GatewayBase")
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
	if !strings.Contains(out, `echo gateway: "%GATEWAY%"`) ||
		strings.Contains(out, `findstr /V /C:"gateway:"`) {
		t.Fatal("install.bat must rewrite agent.yaml in ASCII without merging UTF-8 content")
	}
	if !strings.Contains(out, `set /p "EXISTING_ID="<"%ID_FILE%"`) {
		t.Fatal("reinstall must reliably read and reuse the persisted asset-id")
	}
	if strings.Contains(out, `EXISTING_ID:~36`) {
		t.Fatal("installer must not silently discard an existing identity using fragile length checks")
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
	if strings.Contains(out, `echo:!AGENT_TOKEN!"`) {
		t.Fatal("install.bat must not append a quote to agent-token")
	}
	if !strings.Contains(out, `echo:!AGENT_TOKEN!`) {
		t.Fatal("install.bat must persist agent-token")
	}
}
