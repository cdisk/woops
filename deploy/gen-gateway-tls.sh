#!/usr/bin/env bash
# Generate a self-signed Gateway TLS cert and print SPKI pin for woops-agent.
# Pin format matches agent.yaml gatewayTlsSpkiSha256 / OPS_GATEWAY_TLS_SPKI_SHA256 (hex).
#
# Usage:
#   ./deploy/gen-gateway-tls.sh
#   ./deploy/gen-gateway-tls.sh --host gateway.example.com --ip 10.0.0.5 --out deploy/tls
#   ./deploy/gen-gateway-tls.sh --cert deploy/tls/gateway.crt   # pin only
set -euo pipefail

OUT_DIR="deploy/tls"
HOST="gateway.local"
IP=""
DAYS=825
CERT=""
KEY=""
CN=""

usage() {
  cat <<'EOF'
Usage: gen-gateway-tls.sh [options]

  --out DIR       Output directory (default: deploy/tls)
  --host NAME     DNS SAN / default CN (default: gateway.local); repeatable
  --ip ADDR       IP SAN; repeatable
  --days N        Validity days (default: 825)
  --cn NAME       Certificate CN (default: first --host)
  --cert FILE     Only compute pin from existing cert (skip generate)
  --key FILE      Key path when generating (default: OUT/gateway.key)
  -h, --help      Show help

Prints:
  - gateway.crt / gateway.key paths
  - OPS_GATEWAY_TLS_SPKI_SHA256=<hex>
  - curl --pinnedpubkey value
EOF
}

HOSTS=()
IPS=()
while [ $# -gt 0 ]; do
  case "$1" in
    --out) OUT_DIR="${2:?}"; shift 2 ;;
    --host) HOSTS+=("${2:?}"); shift 2 ;;
    --ip) IPS+=("${2:?}"); shift 2 ;;
    --days) DAYS="${2:?}"; shift 2 ;;
    --cn) CN="${2:?}"; shift 2 ;;
    --cert) CERT="${2:?}"; shift 2 ;;
    --key) KEY="${2:?}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown arg: $1" >&2; usage >&2; exit 1 ;;
  esac
done

if [ ${#HOSTS[@]} -eq 0 ]; then
  HOSTS+=("$HOST")
fi
if [ -z "$CN" ]; then
  CN="${HOSTS[0]}"
fi

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || { echo "ERROR: need $1" >&2; exit 1; }
}
need_cmd openssl

spki_pin_hex() {
  local cert="$1"
  # SHA-256 of SubjectPublicKeyInfo DER — same as go/internal/agent SPKIPinHex
  openssl x509 -in "$cert" -pubkey -noout \
    | openssl pkey -pubin -outform der \
    | openssl dgst -sha256 -hex \
    | awk '{print $NF}'
}

spki_pin_curl() {
  local cert="$1"
  local b64
  b64=$(openssl x509 -in "$cert" -pubkey -noout \
    | openssl pkey -pubin -outform der \
    | openssl dgst -sha256 -binary \
    | openssl base64 -A)
  printf 'sha256//%s' "$b64"
}

if [ -n "$CERT" ]; then
  [ -f "$CERT" ] || { echo "ERROR: cert not found: $CERT" >&2; exit 1; }
else
  mkdir -p "$OUT_DIR"
  CERT="${CERT:-$OUT_DIR/gateway.crt}"
  KEY="${KEY:-$OUT_DIR/gateway.key}"
  CONF=$(mktemp)
  trap 'rm -f "$CONF"' EXIT

  {
    echo "[req]"
    echo "default_bits = 2048"
    echo "prompt = no"
    echo "default_md = sha256"
    echo "distinguished_name = dn"
    echo "x509_extensions = v3_req"
    echo "[dn]"
    echo "CN = $CN"
    echo "[v3_req]"
    echo "basicConstraints = CA:FALSE"
    echo "keyUsage = digitalSignature, keyEncipherment"
    echo "extendedKeyUsage = serverAuth"
    echo "subjectAltName = @alt"
    echo "[alt]"
    i=1
    for h in "${HOSTS[@]}"; do
      echo "DNS.$i = $h"
      i=$((i + 1))
    done
    j=1
    for a in "${IPS[@]}"; do
      echo "IP.$j = $a"
      j=$((j + 1))
    done
    # Always include loopback for local smoke tests.
    echo "IP.$j = 127.0.0.1"
  } > "$CONF"

  echo "==> Generating self-signed cert (CN=$CN, days=$DAYS)"
  openssl req -x509 -newkey rsa:2048 -nodes \
    -keyout "$KEY" \
    -out "$CERT" \
    -days "$DAYS" \
    -config "$CONF"
  chmod 600 "$KEY" 2>/dev/null || true
  echo "    cert: $CERT"
  echo "    key:  $KEY"
fi

PIN=$(spki_pin_hex "$CERT")
CURL_PIN=$(spki_pin_curl "$CERT")

echo
echo "==> SPKI pin (put on control-api + gateway)"
echo "OPS_GATEWAY_TLS_SPKI_SHA256=$PIN"
echo
echo "==> agent.yaml"
echo "gateway: \"https://<host>:9200\""
echo "gatewayTlsSpkiSha256: \"$PIN\""
echo
echo "==> Linux install (example)"
echo "curl -fsSL -k --pinnedpubkey $CURL_PIN --compressed -o /tmp/woops-agent \"https://<host>:9200/i/<code>/agent/linux/\$(uname -m | sed -e s/x86_64/amd64/ -e s/aarch64/arm64/)\" && chmod +x /tmp/woops-agent && /tmp/woops-agent install -gateway https://<host>:9200 -code <code> -pin $PIN"
echo
echo "==> Windows install (example; need curl.exe — Console shows install tip if missing)"
echo "curl.exe -fsSL -k --pinnedpubkey $CURL_PIN --compressed -o \"%TEMP%\\woops-agent.exe\" https://<host>:9200/i/<code>/agent/windows/amd64 && \"%TEMP%\\woops-agent.exe\" install -gateway https://<host>:9200 -code <code> -pin $PIN"
echo
echo "==> openssl verify leaf (self-signed will fail system trust; pin is the trust root)"
openssl x509 -in "$CERT" -noout -subject -dates -ext subjectAltName 2>/dev/null \
  || openssl x509 -in "$CERT" -noout -subject -dates
