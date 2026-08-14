package guac

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"
)

// ConnParams are values supplied during guacd connect (by arg name).
type ConnParams struct {
	Protocol string // rdp | vnc
	Hostname string
	Port     int
	Username string
	Password string
	Domain   string // optional Windows domain; empty for local accounts
	// Security overrides RDP security mode: nla|tls|rdp|any (empty = nla for rdp).
	Security string
	Width    int
	Height   int
	DPI      int
	Timezone string
	Name     string
	// RecordingPath enables guacd's native session recording. It must be an
	// absolute path as guacd sees it; empty disables recording entirely.
	RecordingPath string
	// RecordingName is the file guacd creates inside RecordingPath (default "session").
	RecordingName string
	// ColorDepth is RDP bits-per-pixel for guacd (8|16|24|32). Zero → 16.
	ColorDepth int
	// RdpQuality is low|medium|high experience preset. Empty → low.
	RdpQuality string
}

// Connection is a configured guacd connection. Reader must be used for all
// subsequent reads because it may already contain post-handshake bytes.
type Connection struct {
	net.Conn
	Reader          *bufio.Reader
	ID              string
	ProtocolVersion string
}

// DialAndHandshake connects to guacd and completes its configured handshake.
func DialAndHandshake(guacdAddr string, p ConnParams) (*Connection, error) {
	raw, err := net.DialTimeout("tcp", guacdAddr, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("dial guacd %s: %w", guacdAddr, err)
	}
	conn, err := Handshake(raw, p)
	if err != nil {
		_ = raw.Close()
		return nil, err
	}
	return conn, nil
}

// Handshake selects a protocol on an existing guacd TCP connection and connects.
func Handshake(conn net.Conn, p ConnParams) (*Connection, error) {
	if p.Protocol == "" {
		return nil, fmt.Errorf("guac: protocol required")
	}
	if p.Width <= 0 {
		p.Width = 1280
	}
	if p.Height <= 0 {
		p.Height = 800
	}
	if p.DPI <= 0 {
		p.DPI = 96
	}
	if p.Timezone == "" {
		p.Timezone = "UTC"
	}
	if p.Name == "" {
		p.Name = "woops-console"
	}

	br := bufio.NewReader(conn)
	if err := WriteInstruction(conn, "select", p.Protocol); err != nil {
		return nil, err
	}
	op, argNames, err := ReadInstruction(br)
	if err != nil {
		return nil, fmt.Errorf("guac select: %w", err)
	}
	if op != "args" {
		return nil, fmt.Errorf("guac: expected args, got %s", op)
	}

	values := make([]string, len(argNames))
	protocolVersion := "VERSION_1_0_0"
	for i, name := range argNames {
		if i == 0 && strings.HasPrefix(name, "VERSION_") {
			protocolVersion = negotiateVersion(name)
			values[i] = protocolVersion
			continue
		}
		values[i] = paramValue(name, p)
	}
	if p.Protocol == "rdp" {
		// Help diagnose "wrong security type" / quality without dumping the password.
		sec, ign, depth, wall, qual := "", "", "", "", p.RdpQuality
		for i, name := range argNames {
			switch name {
			case "security":
				sec = values[i]
			case "ignore-cert":
				ign = values[i]
			case "color-depth":
				depth = values[i]
			case "enable-wallpaper":
				wall = values[i]
			}
		}
		log.Printf("guac rdp connect host=%s port=%d user=%q domain=%q security=%q ignore-cert=%q color-depth=%q wallpaper=%q quality=%q size=%dx%d args=%d",
			p.Hostname, p.Port, p.Username, p.Domain, sec, ign, depth, wall, qual, p.Width, p.Height, len(argNames))
	}

	if err := WriteInstruction(conn, "size", strconv.Itoa(p.Width), strconv.Itoa(p.Height), strconv.Itoa(p.DPI)); err != nil {
		return nil, fmt.Errorf("guac size: %w", err)
	}
	if err := WriteInstruction(conn, "audio"); err != nil {
		return nil, fmt.Errorf("guac audio: %w", err)
	}
	if err := WriteInstruction(conn, "video"); err != nil {
		return nil, fmt.Errorf("guac video: %w", err)
	}
	if err := WriteInstruction(conn, "image", "image/png", "image/jpeg"); err != nil {
		return nil, fmt.Errorf("guac image: %w", err)
	}
	if versionAtLeast(protocolVersion, 1, 1, 0) {
		if err := WriteInstruction(conn, "timezone", p.Timezone); err != nil {
			return nil, fmt.Errorf("guac timezone: %w", err)
		}
	}
	if versionAtLeast(protocolVersion, 1, 5, 0) {
		if err := WriteInstruction(conn, "name", p.Name); err != nil {
			return nil, fmt.Errorf("guac name: %w", err)
		}
	}
	if err := WriteInstruction(conn, "connect", values...); err != nil {
		return nil, err
	}

	op, restArgs, err := ReadInstruction(br)
	if err != nil {
		return nil, fmt.Errorf("guac connect: %w", err)
	}
	if op == "error" {
		msg := ""
		if len(restArgs) > 0 {
			msg = restArgs[0]
		}
		return nil, fmt.Errorf("guacd: %s", msg)
	}
	if op != "ready" {
		return nil, fmt.Errorf("guac connect: expected ready, got %s %v", op, restArgs)
	}
	if len(restArgs) == 0 || restArgs[0] == "" {
		return nil, fmt.Errorf("guac connect: ready missing connection ID")
	}
	log.Printf("guac connect ready id=%s version=%s", restArgs[0], protocolVersion)
	return &Connection{
		Conn:            conn,
		Reader:          br,
		ID:              restArgs[0],
		ProtocolVersion: protocolVersion,
	}, nil
}

func negotiateVersion(offered string) string {
	major, minor, patch, ok := parseVersion(offered)
	if !ok {
		return "VERSION_1_0_0"
	}
	if major > 1 || (major == 1 && minor > 5) {
		return "VERSION_1_5_0"
	}
	return fmt.Sprintf("VERSION_%d_%d_%d", major, minor, patch)
}

func versionAtLeast(version string, wantMajor, wantMinor, wantPatch int) bool {
	major, minor, patch, ok := parseVersion(version)
	if !ok {
		return false
	}
	if major != wantMajor {
		return major > wantMajor
	}
	if minor != wantMinor {
		return minor > wantMinor
	}
	return patch >= wantPatch
}

func parseVersion(version string) (major, minor, patch int, ok bool) {
	if _, err := fmt.Sscanf(version, "VERSION_%d_%d_%d", &major, &minor, &patch); err != nil {
		return 0, 0, 0, false
	}
	return major, minor, patch, true
}

func paramValue(name string, p ConnParams) string {
	switch name {
	case "hostname":
		return p.Hostname
	case "port":
		if p.Port <= 0 {
			return ""
		}
		return strconv.Itoa(p.Port)
	case "username":
		return p.Username
	case "password":
		return p.Password
	case "width":
		return strconv.Itoa(p.Width)
	case "height":
		return strconv.Itoa(p.Height)
	case "dpi":
		return strconv.Itoa(p.DPI)
	case "security":
		// Prefer NLA for passworded Windows RDP. "any" frequently fails with
		// "Server refused connection (wrong security type?)" on Server 2016+.
		if p.Protocol == "rdp" {
			if p.Security != "" {
				return p.Security
			}
			return "nla"
		}
		return "any"
	case "ignore-cert":
		return "true"
	case "disable-auth":
		return "false"
	case "disable-copy", "disable-paste":
		// Clipboard redirection is enabled in both directions. guacd 1.5.x's
		// RDP implementation exposes Unicode/text clipboard data only.
		if p.Protocol == "rdp" {
			return "false"
		}
		return ""
	case "normalize-clipboard":
		if p.Protocol == "rdp" {
			return "preserve"
		}
		return ""
	case "domain":
		return p.Domain
	case "color-depth":
		if p.Protocol != "rdp" {
			return ""
		}
		switch p.ColorDepth {
		case 8, 16, 24, 32:
			return strconv.Itoa(p.ColorDepth)
		default:
			return "16"
		}
	case "enable-wallpaper", "enable-theming", "enable-font-smoothing",
		"enable-full-window-drag", "enable-desktop-composition", "enable-menu-animations":
		if p.Protocol != "rdp" {
			return ""
		}
		return rdpExperienceFlag(name, p.RdpQuality)
	case "resize-method":
		if p.Protocol == "rdp" {
			return "display-update"
		}
		return ""
	case "recording-path":
		return p.RecordingPath
	case "recording-name":
		if p.RecordingPath == "" {
			return ""
		}
		if p.RecordingName != "" {
			return p.RecordingName
		}
		return "session"
	case "create-recording-path":
		// guacd only creates the per-operation directory when asked; a failure
		// there is logged by guacd and never fails the connection.
		if p.RecordingPath == "" {
			return ""
		}
		return "true"
	default:
		return ""
	}
}

// rdpExperienceFlag maps asset quality preset to guacd RDP experience booleans.
// Default (empty/unknown) is low — prioritize bandwidth over chrome.
func rdpExperienceFlag(name, quality string) string {
	q := strings.ToLower(strings.TrimSpace(quality))
	if q == "" {
		q = "low"
	}
	switch q {
	case "high":
		return "true"
	case "medium":
		switch name {
		case "enable-theming":
			return "true"
		default:
			return "false"
		}
	default: // low
		return "false"
	}
}
