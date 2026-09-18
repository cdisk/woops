package install

import (
	_ "embed"
	"bytes"
	"fmt"
	"os"
	"strings"
)

//go:embed agent.yaml.tmpl
var agentYAMLTemplate []byte

// MergeAgentYAML line-level merges connection keys into an existing agent.yaml.
// Top-level gateway: and gatewayTlsSpkiSha256: are always rewritten; every other
// line (including indented proxyBridge.gateway) is preserved byte-for-byte.
// Empty/missing input yields a fresh template. UTF-8 BOM and CRLF are preserved.
func MergeAgentYAML(old []byte, gateway, pin string) []byte {
	gateway = strings.TrimRight(strings.TrimSpace(gateway), "/")
	pin = strings.TrimSpace(pin)

	if len(bytes.TrimSpace(old)) == 0 {
		return renderTemplate(gateway, pin)
	}

	bom := []byte(nil)
	body := old
	if bytes.HasPrefix(old, []byte{0xEF, 0xBB, 0xBF}) {
		bom = old[:3]
		body = old[3:]
	}

	nl := "\n"
	if bytes.Contains(body, []byte("\r\n")) {
		nl = "\r\n"
	}

	// Split keeping empty trailing segment so a missing final newline is detectable.
	raw := string(body)
	endsWithNL := strings.HasSuffix(raw, "\n") || strings.HasSuffix(raw, "\r\n")
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(normalized, "\n")
	// Split always yields a trailing empty element when the file ends with \n;
	// drop it so we can re-emit the original trailing-newline policy.
	if endsWithNL && len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	gwLine := fmt.Sprintf(`gateway: "%s"`, gateway)
	pinLine := fmt.Sprintf(`gatewayTlsSpkiSha256: "%s"`, pin)

	replacedGW, replacedPin := false, false
	out := make([]string, 0, len(lines)+2)
	for _, line := range lines {
		switch {
		case !replacedGW && isTopLevelKey(line, "gateway"):
			out = append(out, gwLine)
			replacedGW = true
		case !replacedPin && isTopLevelKey(line, "gatewayTlsSpkiSha256"):
			out = append(out, pinLine)
			replacedPin = true
		default:
			out = append(out, line)
		}
	}
	if !replacedGW {
		out = append([]string{gwLine}, out...)
	}
	if !replacedPin {
		// Insert after gateway: when present; otherwise append.
		inserted := false
		withPin := make([]string, 0, len(out)+1)
		for _, line := range out {
			withPin = append(withPin, line)
			if !inserted && isTopLevelKey(line, "gateway") {
				withPin = append(withPin, pinLine)
				inserted = true
			}
		}
		if !inserted {
			withPin = append(withPin, pinLine)
		}
		out = withPin
	}

	joined := strings.Join(out, nl)
	if endsWithNL || len(out) > 0 {
		joined += nl
	}
	result := append(append([]byte{}, bom...), []byte(joined)...)
	return result
}

// UpsertQuotedKey sets a top-level quoted scalar key, preserving other lines.
func UpsertQuotedKey(doc []byte, key, value string) []byte {
	bom := []byte(nil)
	body := doc
	if bytes.HasPrefix(doc, []byte{0xEF, 0xBB, 0xBF}) {
		bom = doc[:3]
		body = doc[3:]
	}
	nl := "\n"
	if bytes.Contains(body, []byte("\r\n")) {
		nl = "\r\n"
	}
	raw := string(body)
	endsWithNL := strings.HasSuffix(raw, "\n") || strings.HasSuffix(raw, "\r\n")
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(normalized, "\n")
	if endsWithNL && len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	esc := strings.ReplaceAll(value, `\`, `\\`)
	esc = strings.ReplaceAll(esc, `"`, `\"`)
	newLine := key + `: "` + esc + `"`

	done := false
	out := make([]string, 0, len(lines)+1)
	for _, line := range lines {
		if !done && isTopLevelKey(line, key) {
			out = append(out, newLine)
			done = true
			continue
		}
		out = append(out, line)
	}
	if !done {
		inserted := false
		with := make([]string, 0, len(out)+1)
		for _, line := range out {
			with = append(with, line)
			if !inserted && isTopLevelKey(line, "gatewayTlsSpkiSha256") {
				with = append(with, newLine)
				inserted = true
			}
		}
		if !inserted {
			with = append(with, newLine)
		}
		out = with
	}

	joined := strings.Join(out, nl)
	if endsWithNL || len(out) > 0 {
		joined += nl
	}
	return append(append([]byte{}, bom...), []byte(joined)...)
}

// DetectGatewayProxyEnv reads install-time proxy env (same order as legacy scripts).
func DetectGatewayProxyEnv() string {
	for _, name := range []string{
		"https_proxy", "HTTPS_PROXY", "ALL_PROXY", "all_proxy", "http_proxy", "HTTP_PROXY",
	} {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			return v
		}
	}
	return ""
}

func renderTemplate(gateway, pin string) []byte {
	s := string(agentYAMLTemplate)
	s = strings.ReplaceAll(s, "__GATEWAY__", gateway)
	s = strings.ReplaceAll(s, "__TLS_PIN__", pin)
	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	return []byte(s)
}

func isTopLevelKey(line, key string) bool {
	if strings.TrimSpace(line) == "" {
		return false
	}
	// Column-0 only: leading whitespace means a nested key (e.g. proxyBridge.gateway).
	if line[0] == ' ' || line[0] == '\t' {
		return false
	}
	prefix := key + ":"
	if !strings.HasPrefix(line, prefix) {
		return false
	}
	// Accept "key:" or "key: value" or "key:".
	rest := line[len(prefix):]
	return rest == "" || rest[0] == ' ' || rest[0] == '\t'
}

// WriteAgentYAML merges and atomically writes agent.yaml.
func WriteAgentYAML(path, gateway, pin string) error {
	old, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	doc := MergeAgentYAML(old, gateway, pin)
	if proxy := DetectGatewayProxyEnv(); proxy != "" {
		doc = UpsertQuotedKey(doc, "gatewayProxy", proxy)
	}
	return atomicWriteFile(path, doc, 0o644)
}
