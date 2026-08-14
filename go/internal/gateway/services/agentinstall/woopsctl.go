package agentinstall

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ops-bastion/ops/go/internal/gateway/core"
)

// RegisterWoopsctlBin serves public woopsctl downloads (no install code).
// Path: GET /bin/woopsctl/{os}/{arch}
func RegisterWoopsctlBin(r core.Router, binDir string) {
	if strings.TrimSpace(binDir) == "" {
		return
	}
	r.Register("/bin/woopsctl/", serveWoopsctl(binDir))
}

func serveWoopsctl(dir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/bin/woopsctl/"), "/")
		parts := strings.Split(rest, "/")
		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			http.Error(w, "usage: /bin/woopsctl/{os}/{arch}", http.StatusBadRequest)
			return
		}
		osName, arch := parts[0], parts[1]
		if !validWoopsctlPlatform(osName, arch) {
			http.Error(w, "unsupported os/arch", http.StatusBadRequest)
			return
		}
		binPath, name := resolveWoopsctlBinary(dir, osName, arch)
		if binPath == "" {
			http.Error(w, "woopsctl binary not found: "+name, http.StatusNotFound)
			return
		}
		filename := "woopsctl"
		if osName == "windows" {
			filename = "woopsctl.exe"
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment; filename="+filename)
		http.ServeFile(w, r, binPath)
	}
}

func validWoopsctlPlatform(osName, arch string) bool {
	switch osName {
	case "linux", "windows", "darwin":
	default:
		return false
	}
	switch arch {
	case "amd64", "arm64":
		return true
	default:
		return false
	}
}

func resolveWoopsctlBinary(dir, osName, arch string) (path, displayName string) {
	base := fmt.Sprintf("woopsctl-%s-%s", osName, arch)
	candidates := []string{base}
	if osName == "windows" {
		candidates = []string{base + ".exe", base}
	}
	for _, name := range candidates {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, name
		}
	}
	if osName == "windows" {
		return "", base + ".exe"
	}
	return "", base
}
