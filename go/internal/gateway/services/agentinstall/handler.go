package agentinstall

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ops-bastion/ops/go/internal/gateway/core"
)

type Deps struct {
	GetJSON        func(string, any) error
	PublicHTTPBase string
	AgentBinDir    string
	TLSSpkiSHA256  string
}

func Register(r core.Router, d Deps) {
	if d.GetJSON == nil {
		return
	}
	r.Register("/i/", Handler(d))
}

func Handler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/i/"), "/")
		if len(parts) < 2 {
			http.NotFound(w, r)
			return
		}
		code, action := parts[0], parts[1]
		var valid map[string]any
		if err := d.GetJSON("/api/install-codes/"+code+"/valid", &valid); err != nil || valid["valid"] != true {
			http.Error(w, "invalid or expired install code", http.StatusGone)
			return
		}

		switch action {
		case "install.sh", "install.ps1":
			if action == "install.sh" {
				w.Header().Set("Content-Type", "text/x-shellscript")
			} else {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			}
			osName := "linux"
			if action == "install.ps1" {
				osName = "windows"
			}
			body, err := Render(action, installParams(d, publicGatewayBase(r, d.PublicHTTPBase), code, osName))
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if action == "install.ps1" {
				// UTF-8 BOM so Windows PowerShell 5.1 parses as UTF-8 (not system ANSI).
				_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
			}
			_, _ = w.Write([]byte(body))
		case "agent":
			serveAgent(d.AgentBinDir, parts, w, r)
		case "winpty":
			serveWinpty(d.AgentBinDir, parts, w, r)
		default:
			http.NotFound(w, r)
		}
	}
}

func serveAgent(dir string, parts []string, w http.ResponseWriter, r *http.Request) {
	if len(parts) < 4 {
		http.Error(w, "usage: /i/{code}/agent/{os}/{arch}", http.StatusBadRequest)
		return
	}
	osName, arch := parts[2], parts[3]
	binPath, name := resolveAgentBinary(dir, osName, arch)
	if binPath == "" {
		http.Error(w, "agent binary not found: "+name, http.StatusNotFound)
		return
	}
	filename := "woops-agent"
	if osName == "windows" {
		filename = "woops-agent.exe"
	}
	if r.URL.Query().Get("format") == "gz" {
		gzPath := binPath + ".gz"
		if st, err := os.Stat(gzPath); err != nil || st.IsDir() {
			http.Error(w, "gzip agent not found: "+name+".gz", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Disposition", "attachment; filename="+filename+".gz")
		http.ServeFile(w, r, gzPath)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	http.ServeFile(w, r, binPath)
}

func serveWinpty(dir string, parts []string, w http.ResponseWriter, r *http.Request) {
	if len(parts) < 4 {
		http.Error(w, "usage: /i/{code}/winpty/{arch}/{filename}", http.StatusBadRequest)
		return
	}
	arch, filename := parts[2], parts[3]
	if filename != "winpty.dll" && filename != "winpty-agent.exe" {
		http.Error(w, "unsupported winpty file", http.StatusBadRequest)
		return
	}
	binPath := resolveWinptyFile(dir, arch, filename)
	if binPath == "" {
		http.Error(w, "winpty runtime not found: "+filename, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	http.ServeFile(w, r, binPath)
}

func installParams(d Deps, gwBase, code, osName string) Params {
	hashes := map[string]string{}
	for _, arch := range []string{"amd64", "arm64"} {
		if p, _ := resolveAgentBinary(d.AgentBinDir, osName, arch); p != "" {
			if sum, err := fileSHA256Hex(p); err == nil {
				hashes[arch] = sum
			}
		}
	}
	return Params{GatewayBase: gwBase, InstallCode: code, TLSSpkiSHA256: d.TLSSpkiSHA256, AgentSHA256ByArch: hashes}
}

func fileSHA256Hex(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func resolveAgentBinary(dir, osName, arch string) (path, displayName string) {
	base := fmt.Sprintf("woops-agent-%s-%s", osName, arch)
	candidates := []string{base}
	if osName == "windows" {
		candidates = []string{base + ".exe", base}
	}
	for _, name := range candidates {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p, name
		}
	}
	return "", base
}

func resolveWinptyFile(agentBinDir, arch, filename string) string {
	candidates := []string{filepath.Join(agentBinDir, "winpty", arch, filename)}
	if arch == "amd64" || arch == "x64" {
		candidates = append(candidates,
			filepath.Join(agentBinDir, "winpty", "x64", filename),
			filepath.Join(agentBinDir, "..", "third_party", "winpty", "x64", filename),
			filepath.Join("third_party", "winpty", "x64", filename))
	}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func publicGatewayBase(r *http.Request, fallback string) string {
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	} else {
		host = strings.TrimSpace(strings.Split(host, ",")[0])
	}
	if host == "" {
		return fallback
	}
	h, port, err := net.SplitHostPort(host)
	if err != nil {
		h, port = host, "9200"
	}
	if h == "localhost" || h == "127.0.0.1" {
		return fallback
	}
	scheme := "http"
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = strings.TrimSpace(strings.Split(proto, ",")[0])
	} else if r.TLS != nil {
		scheme = "https"
	}
	if port == "" {
		port = "9200"
	}
	return fmt.Sprintf("%s://%s:%s", scheme, h, port)
}
