package main

import (
	"flag"
	"fmt"
	"log"
	"os"
)

// Version is injected at build time: -ldflags "-X main.Version=yymmddhhMM"
var Version = "dev"

func main() {
	cfgPath := flag.String("config", "", "path to agent.yaml")
	showVersion := flag.Bool("version", false, "print agent version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(Version)
		return
	}
	path := absolutizeConfigPath(resolveConfigPath(*cfgPath))
	if err := maybeRunService(path); err != nil {
		log.Println(err)
		os.Exit(1)
	}
}
