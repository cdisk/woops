package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ops-bastion/ops/go/internal/opsctl"
)

// Exit codes: remote exec uses remote exit code; local failures use 2+.
const (
	exitUsage  = 2
	exitConfig = 3
	exitTicket = 4
	exitIO     = 5
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(exitUsage)
	}
	cmd := os.Args[1]
	switch cmd {
	case "upload":
		os.Exit(runUpload(os.Args[2:]))
	case "download":
		os.Exit(runDownload(os.Args[2:]))
	case "exec":
		os.Exit(runExec(os.Args[2:]))
	case "forward":
		os.Exit(runTunnel("forward", os.Args[2:]))
	case "reverse":
		os.Exit(runTunnel("reverse", os.Args[2:]))
	case "-h", "--help", "help":
		printUsage()
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		printUsage()
		os.Exit(exitUsage)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `woopsctl - CI and ephemeral tunnels via deploy token

Usage:
  woopsctl upload <local> <remote>
  woopsctl download <remote> <local>
  woopsctl exec [--cwd DIR] [--timeout DURATION] -- <command>
  woopsctl forward --protocol tcp|udp --listen host:port --target host:port
  woopsctl reverse --protocol tcp|udp --listen host:port --target host:port

Env:
  OPSCTL_CONFIG   JSON: {"server":"https://gateway:9200","token":"ops_…","pin":"<spki-hex>"}
                  (pin may be empty when using a public CA cert)

`)
}

func runUpload(args []string) int {
	fs := flag.NewFlagSet("upload", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	rest := fs.Args()
	if len(rest) != 2 {
		fmt.Fprintln(os.Stderr, "usage: woopsctl upload <local> <remote>")
		return exitUsage
	}
	local, remote := rest[0], rest[1]

	cfg, err := opsctl.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitConfig
	}
	client := opsctl.NewClient(cfg)

	progress := newTransferProgressPrinter("upload")
	err = opsctl.UploadBinary(cfg, func(transferID, fingerprint string, size int64, abort bool) (*opsctl.TransferTicket, error) {
		meta := map[string]any{
			"remotePath":  remote,
			"bytes":       size,
			"fingerprint": fingerprint,
		}
		if transferID != "" {
			meta["transferId"] = transferID
		}
		if abort {
			meta["abort"] = true
		}
		ticket, err := client.CreateTicket("upload", meta)
		if err != nil {
			return nil, err
		}
		return ticket.AsTransfer(), nil
	}, local, remote, progress.Handle)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitIO
	}
	fmt.Fprintf(os.Stderr, "uploaded %s -> %s\n", local, remote)
	return 0
}

func runDownload(args []string) int {
	fs := flag.NewFlagSet("download", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	rest := fs.Args()
	if len(rest) != 2 {
		fmt.Fprintln(os.Stderr, "usage: woopsctl download <remote> <local>")
		return exitUsage
	}
	remote, local := rest[0], rest[1]

	cfg, err := opsctl.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitConfig
	}
	client := opsctl.NewClient(cfg)

	progress := newTransferProgressPrinter("download")
	err = opsctl.DownloadBinary(cfg, func(transferID string, abort bool) (*opsctl.TransferTicket, error) {
		meta := map[string]any{"remotePath": remote}
		if transferID != "" {
			meta["transferId"] = transferID
		}
		if abort {
			meta["abort"] = true
		}
		ticket, err := client.CreateTicket("download", meta)
		if err != nil {
			return nil, err
		}
		return ticket.AsTransfer(), nil
	}, remote, local, progress.Handle)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitIO
	}
	fmt.Fprintf(os.Stderr, "downloaded %s -> %s\n", remote, local)
	return 0
}

func runExec(args []string) int {
	fs := flag.NewFlagSet("exec", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	cwd := fs.String("cwd", "", "remote working directory")
	timeoutStr := fs.String("timeout", "5m", "command timeout (e.g. 30s, 5m)")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	rest := fs.Args()
	if len(rest) > 0 && rest[0] == "--" {
		rest = rest[1:]
	}
	if len(rest) == 0 {
		fmt.Fprintln(os.Stderr, "usage: woopsctl exec [--cwd DIR] [--timeout DURATION] -- <command>")
		return exitUsage
	}
	command := strings.Join(rest, " ")
	timeoutSec, err := parseTimeoutSec(*timeoutStr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitUsage
	}

	cfg, err := opsctl.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitConfig
	}
	client := opsctl.NewClient(cfg)
	ticket, err := client.CreateTicket("exec", map[string]any{
		"command":    command,
		"cwd":        *cwd,
		"timeoutSec": timeoutSec,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitTicket
	}
	res, err := opsctl.Exec(cfg, ticket.BrowserWS, command, *cwd, timeoutSec, os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitIO
	}
	return res.ExitCode
}

func runTunnel(direction string, args []string) int {
	fs := flag.NewFlagSet(direction, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	protocol := fs.String("protocol", "", "tunnel protocol: tcp or udp")
	listen := fs.String("listen", "", "local/listener address (for example :8080 or [::1]:8080)")
	target := fs.String("target", "", "target address (IPv6 must use [addr]:port)")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 0 || *listen == "" || *target == "" {
		fmt.Fprintf(os.Stderr, "usage: woopsctl %s --protocol tcp|udp --listen host:port --target host:port\n", direction)
		return exitUsage
	}
	opts, err := opsctl.ParseTunnelOptions(*protocol, *listen, *target)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitUsage
	}
	opts.EphemeralID, err = opsctl.NewEphemeralID()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitIO
	}
	cfg, err := opsctl.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitConfig
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logger := log.New(os.Stderr, "", log.LstdFlags)
	client := opsctl.NewClient(cfg)
	if direction == "forward" {
		err = opsctl.RunForward(ctx, client, opts, logger)
	} else {
		err = opsctl.RunReverse(ctx, client, opts, logger)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		if opsctl.IsPermanent(err) {
			return exitTicket
		}
		return exitIO
	}
	return 0
}

func parseTimeoutSec(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 300, nil
	}
	if n, err := strconv.Atoi(s); err == nil {
		if n <= 0 {
			return 0, fmt.Errorf("timeout must be positive")
		}
		return n, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid timeout: %w", err)
	}
	sec := int(d.Seconds())
	if sec <= 0 {
		return 0, fmt.Errorf("timeout must be positive")
	}
	return sec, nil
}
