package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/ops-bastion/ops/go/internal/agent/install"
)

func maybeInstallCommand() bool {
	if len(os.Args) < 2 {
		return false
	}
	switch os.Args[1] {
	case "install", "uninstall":
		return true
	default:
		return false
	}
}

func runInstallCommand() {
	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	gateway := fs.String("gateway", "", "Gateway base URL (https://host:9200)")
	code := fs.String("code", "", "install code")
	pin := fs.String("pin", "", "gatewayTlsSpkiSha256 hex pin")
	phase := fs.String("phase", "", "internal: restart")
	purge := fs.Bool("purge", false, "uninstall: also remove config directory")
	_ = fs.Parse(os.Args[2:])

	opts := install.Options{
		Gateway: *gateway,
		Code:    *code,
		Pin:     *pin,
		Phase:   *phase,
		Purge:   *purge,
	}
	var err error
	switch cmd {
	case "install":
		err = install.Run(opts)
	case "uninstall":
		err = install.Uninstall(opts)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
}
