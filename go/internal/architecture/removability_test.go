package architecture

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestFeatureRegistrationRemovability(t *testing.T) {
	tests := []struct {
		name          string
		registrations []string
		commands      []string
	}{
		{
			name: "shell",
			registrations: []string{
				"internal/agent/app/register_session_shell.go",
				"internal/gateway/app/register_session_shell.go",
			},
			commands: []string{"agent", "gateway"},
		},
		{
			name:          "desktop",
			registrations: []string{"internal/gateway/app/register_session_desktop.go"},
			commands:      []string{"gateway"},
		},
		{
			name: "portmap",
			registrations: []string{
				"internal/agent/app/register_service_portmap.go",
				"internal/gateway/app/register_service_portmap.go",
			},
			commands: []string{"agent", "gateway"},
		},
		{
			name: "monitor",
			registrations: []string{
				"internal/agent/app/register_plugin_monitor.go",
				"internal/gateway/app/register_plugin_monitor.go",
			},
			commands: []string{"agent", "gateway"},
		},
	}

	root := moduleRoot(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			overlay := registrationOverlay(t, root, tc.registrations)
			for _, command := range tc.commands {
				t.Run(command, func(t *testing.T) {
					output := filepath.Join(t.TempDir(), command)
					cmd := exec.Command("go", "build", "-overlay="+overlay, "-o", output, "./cmd/"+command)
					cmd.Dir = root
					if combined, err := cmd.CombinedOutput(); err != nil {
						t.Fatalf("build without %s registration: %v\n%s", tc.name, err, combined)
					}
				})
			}
		})
	}
}

func registrationOverlay(t *testing.T, root string, registrations []string) string {
	t.Helper()
	temp := t.TempDir()
	replacement := filepath.Join(temp, "registration_removed.go")
	if err := os.WriteFile(replacement, []byte("package app\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	replace := make(map[string]string, len(registrations))
	for _, registration := range registrations {
		original := filepath.Join(root, filepath.FromSlash(registration))
		if _, err := os.Stat(original); err != nil {
			t.Fatalf("registration %s: %v", registration, err)
		}
		replace[original] = replacement
	}
	data, err := json.Marshal(struct {
		Replace map[string]string `json:"Replace"`
	}{Replace: replace})
	if err != nil {
		t.Fatal(err)
	}
	overlay := filepath.Join(temp, "overlay.json")
	if err := os.WriteFile(overlay, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return overlay
}
