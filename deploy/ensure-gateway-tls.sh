#!/usr/bin/env sh
# Ensure deploy/tls/gateway.{crt,key} exist; always refresh compose-pin.env with matching SPKI pin.
# Used by root docker-compose.yml service tls-init (runs inside Alpine with openssl).
#
# Env (optional):
#   TLS_OUT_DIR       cert dir (default /tls)
#   TLS_PIN_ENV       pin env file to write (default /out/pin.env)
#   OPS_GATEWAY_PUBLIC_HTTP / OPS_CONSOLE_PUBLIC_HTTP — used to pick DNS/IP SANs when generating
#   OPS_TLS_EXTRA_HOSTS / OPS_TLS_EXTRA_IPS — comma-separated extra SANs
set -eu

OUT_DIR="${TLS_OUT_DIR:-/tls}"
PIN_ENV="${TLS_PIN_ENV:-/out/pin.env}"
CERT="$OUT_DIR/gateway.crt"
KEY="$OUT_DIR/gateway.key"
DAYS="${TLS_DAYS:-825}"

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || { echo "ERROR: need $1" >&2; exit 1; }
}
need_cmd openssl

url_host() {
  # https://a.b:9200/x -> a.b ; strip brackets for IPv6 literals if any
  echo "$1" | sed -e 's|^[A-Za-z][A-Za-z0-9+.-]*://||' -e 's|/.*||' -e 's|^\[[^\]]*\]||' -e 's|:.*||'
}

is_ipv4() {
  echo "$1" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$'
}

spki_pin_hex() {
  openssl x509 -in "$1" -pubkey -noout \
    | openssl pkey -pubin -outform der \
    | openssl dgst -sha256 -hex \
    | awk '{print $NF}'
}

mkdir -p "$OUT_DIR"
mkdir -p "$(dirname "$PIN_ENV")"

if [ -f "$CERT" ] && [ -f "$KEY" ]; then
  echo "==> TLS already present: $CERT"
else
  echo "==> Generating self-signed Gateway/Console TLS under $OUT_DIR"

  HOSTS=""
  IPS="127.0.0.1"

  add_host_or_ip() {
    h="$1"
    [ -n "$h" ] || return 0
    case " $HOSTS $IPS " in
      *" $h "*) return 0 ;;
    esac
    if is_ipv4 "$h"; then
      IPS="$IPS $h"
    else
      HOSTS="$HOSTS $h"
    fi
  }

  add_host_or_ip "$(url_host "${OPS_GATEWAY_PUBLIC_HTTP:-}")"
  add_host_or_ip "$(url_host "${OPS_CONSOLE_PUBLIC_HTTP:-}")"
  add_host_or_ip "$(url_host "${OPS_GATEWAY_PUBLIC_WS:-}")"

  if [ -n "${OPS_TLS_EXTRA_HOSTS:-}" ]; then
    oldifs=$IFS
    IFS=,
    for h in $OPS_TLS_EXTRA_HOSTS; do
      IFS=$oldifs
      add_host_or_ip "$(echo "$h" | tr -d ' ')"
      IFS=,
    done
    IFS=$oldifs
  fi
  if [ -n "${OPS_TLS_EXTRA_IPS:-}" ]; then
    oldifs=$IFS
    IFS=,
    for a in $OPS_TLS_EXTRA_IPS; do
      IFS=$oldifs
      add_host_or_ip "$(echo "$a" | tr -d ' ')"
      IFS=,
    done
    IFS=$oldifs
  fi

  # Default when PUBLIC still points at loopback / empty
  set -- $HOSTS
  if [ $# -eq 0 ]; then
    HOSTS="gateway.local"
  fi
  set -- $HOSTS
  CN="$1"

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
    for h in $HOSTS; do
      echo "DNS.$i = $h"
      i=$((i + 1))
    done
    j=1
    for a in $IPS; do
      echo "IP.$j = $a"
      j=$((j + 1))
    done
  } > "$CONF"

  openssl req -x509 -newkey rsa:2048 -nodes \
    -keyout "$KEY" \
    -out "$CERT" \
    -days "$DAYS" \
    -config "$CONF"
  chmod 600 "$KEY" 2>/dev/null || true
  echo "    cert: $CERT"
  echo "    key:  $KEY"
  echo "    SAN hosts:$HOSTS  ips:$IPS"
fi

PIN=$(spki_pin_hex "$CERT")
printf 'OPS_GATEWAY_TLS_SPKI_SHA256=%s\n' "$PIN" > "$PIN_ENV"
echo "==> Wrote $PIN_ENV"
echo "OPS_GATEWAY_TLS_SPKI_SHA256=$PIN"
