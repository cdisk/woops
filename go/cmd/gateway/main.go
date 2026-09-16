package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	gatewayapp "github.com/ops-bastion/ops/go/internal/gateway/app"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	publicListen := flag.String("public-listen", envOr("OPS_GATEWAY_PUBLIC_LISTEN", ":9200"), "public listen address (HTTPS when cert/key set)")
	internalListen := flag.String("internal-listen", envOr("OPS_GATEWAY_INTERNAL_LISTEN", ":9201"), "internal plain-HTTP listen for control-api")
	controlInternal := flag.String("control-internal-http", envOr("OPS_CONTROL_INTERNAL_HTTP", "http://127.0.0.1:9100"), "control-api internal HTTP base")
	publicHTTP := flag.String("gateway-public-http", envOr("OPS_GATEWAY_PUBLIC_HTTP", "https://127.0.0.1:9200"), "public Gateway HTTPS base embedded in install scripts")
	agentBinDir := flag.String("agent-bin-dir", envOr("OPS_AGENT_BIN_DIR", "bin"), "directory with woops-agent / woopsctl binaries")
	guacd := flag.String("guacd", envOr("OPS_GUACD_ADDR", "127.0.0.1:4822"), "guacd address for RDP/VNC")
	guacBridge := flag.String("guac-bridge-host", envOr("OPS_GUAC_BRIDGE_HOST", "host.docker.internal"), "hostname guacd uses to reach local RDP/VNC bridge")
	auditDir := flag.String("audit-dir", envOr("OPS_AUDIT_DIR", "./data/ops-audit"), "runtime activity JSONL spool root (empty disables)")
	auditInstance := flag.String("audit-instance-id", envOr("OPS_GATEWAY_INSTANCE_ID", ""), "Gateway id used in spool segment names (default hostname + random)")
	tlsPin := flag.String("tls-spki-sha256", envOr("OPS_GATEWAY_TLS_SPKI_SHA256", ""), "optional Gateway cert SPKI SHA-256 hex pin")
	tlsCert := flag.String("tls-cert", envOr("OPS_GATEWAY_TLS_CERT", ""), "TLS certificate file for public listen")
	tlsKey := flag.String("tls-key", envOr("OPS_GATEWAY_TLS_KEY", ""), "TLS private key file for public listen")
	flag.Parse()

	srv := gatewayapp.New(gatewayapp.Config{
		PublicListenAddr:    *publicListen,
		InternalListenAddr:  *internalListen,
		ControlInternalHTTP: *controlInternal,
		PublicHTTPBase:      *publicHTTP,
		AgentBinDir:         *agentBinDir,
		GuacdAddr:           *guacd,
		GuacBridgeHost:      *guacBridge,
		TLSSpkiSHA256:       strings.TrimSpace(*tlsPin),
		AuditDir:            strings.TrimSpace(*auditDir),
		AuditInstanceID:     strings.TrimSpace(*auditInstance),
	})
	// Force-kill cannot CloseAudit; seal leftover RUNNING rows on every start.
	srv.InterruptOrphanOperations()
	// Seal the active spool segment on shutdown so ingest can finalize it.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		if err := srv.CloseAudit(); err != nil {
			log.Printf("ops-audit close: %v", err)
		}
		os.Exit(0)
	}()

	handler := srv.Handler()
	internalHandler := srv.InternalHandler()
	cert := strings.TrimSpace(*tlsCert)
	key := strings.TrimSpace(*tlsKey)
	if (cert == "") != (key == "") {
		log.Fatal("OPS_GATEWAY_TLS_CERT and OPS_GATEWAY_TLS_KEY must be set together")
	}

	internalAddr := strings.TrimSpace(*internalListen)
	if internalAddr != "" {
		go func() {
			log.Printf("gateway INTERNAL http on %s (control-api→gateway)", internalAddr)
			if err := http.ListenAndServe(internalAddr, internalHandler); err != nil {
				log.Fatalf("internal listen: %v", err)
			}
		}()
	}

	publicAddr := strings.TrimSpace(*publicListen)
	if publicAddr == "" {
		log.Fatal("OPS_GATEWAY_PUBLIC_LISTEN is required")
	}
	if cert != "" {
		log.Printf("gateway PUBLIC https on %s (agents/opsctl/browser; pin=%v)", publicAddr, *tlsPin != "")
		if err := http.ListenAndServeTLS(publicAddr, cert, key, handler); err != nil {
			log.Fatal(err)
		}
		return
	}
	log.Printf("gateway PUBLIC http on %s (dev without TLS cert; set OPS_GATEWAY_TLS_CERT/KEY for https)", publicAddr)
	if err := http.ListenAndServe(publicAddr, handler); err != nil {
		log.Fatal(err)
	}
}
