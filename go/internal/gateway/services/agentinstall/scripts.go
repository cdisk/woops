package agentinstall

import (
	"embed"
	"encoding/base64"
	"strings"
)

//go:embed install.sh install.ps1 agent.yaml.tmpl
var files embed.FS

// Params fills install script placeholders.
type Params struct {
	GatewayBase string
	InstallCode string
	// TLSSpkiSHA256 is lowercase hex (or empty for system-CA / plaintext-dev).
	TLSSpkiSHA256 string
	// AgentSHA256ByArch maps arch -> hex sha256 of woops-agent binary (optional).
	AgentSHA256ByArch map[string]string
}

// Render returns an install script with gateway/install-code placeholders filled.
func Render(name string, p Params) (string, error) {
	b, err := files.ReadFile(name)
	if err != nil {
		return "", err
	}
	tmpl, err := files.ReadFile("agent.yaml.tmpl")
	if err != nil {
		return "", err
	}
	// Normalize CRLF so Windows checkouts don't break `set -o pipefail` on Linux.
	src := strings.ReplaceAll(string(b), "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")
	yamlTmpl := strings.ReplaceAll(string(tmpl), "\r\n", "\n")
	yamlTmpl = strings.ReplaceAll(yamlTmpl, "\r", "\n")
	yamlTmpl = strings.TrimRight(yamlTmpl, "\n")

	// Linux install.sh still uses a literal heredoc placeholder.
	src = strings.ReplaceAll(src, "{{AGENT_YAML_TEMPLATE}}", yamlTmpl)
	// Windows install.ps1 uses base64 to avoid PowerShell here-string terminator issues.
	src = strings.ReplaceAll(src, "{{AGENT_YAML_TEMPLATE_B64}}", base64.StdEncoding.EncodeToString([]byte(yamlTmpl)))

	amd := ""
	arm := ""
	if p.AgentSHA256ByArch != nil {
		amd = p.AgentSHA256ByArch["amd64"]
		arm = p.AgentSHA256ByArch["arm64"]
	}
	r := strings.NewReplacer(
		"{{GATEWAY_BASE}}", p.GatewayBase,
		"{{INSTALL_CODE}}", p.InstallCode,
		"{{GATEWAY_TLS_SPKI_SHA256}}", p.TLSSpkiSHA256,
		"{{AGENT_SHA256_AMD64}}", amd,
		"{{AGENT_SHA256_ARM64}}", arm,
	)
	return r.Replace(src), nil
}
