#!/usr/bin/env bash
# Issue and install a browser-trusted Console certificate with acme.sh.
# Gateway TLS is intentionally left untouched so existing SPKI-pinned Agents
# remain connected.
#
# Usage:
#   DOMAIN=console.example.com ACME_EMAIL=admin@example.com \
#     bash deploy/acme/issue-console-cert.sh
set -euo pipefail

DOMAIN="${DOMAIN:?set DOMAIN to the Console public hostname}"
OPS_DIR="${OPS_DIR:-$(cd "$(dirname "$0")/../.." && pwd)}"
ACME_EMAIL="${ACME_EMAIL:-admin@$DOMAIN}"
ACME_HOME="${ACME_HOME:-/root/.acme.sh}"
ACME="$ACME_HOME/acme.sh"
CERT_DIR="$OPS_DIR/deploy/acme"
COMPOSE_RELOAD="cd '$OPS_DIR' && docker compose -f docker-compose.yml -f docker-compose.acme.yml --env-file .env restart console"

case "$DOMAIN" in
  *[!A-Za-z0-9.-]* | .* | *.) echo "ERROR: invalid DOMAIN: $DOMAIN" >&2; exit 1 ;;
esac

for command_name in curl openssl socat docker ss crontab; do
  command -v "$command_name" >/dev/null 2>&1 || {
    echo "ERROR: need $command_name" >&2
    exit 1
  }
done
docker compose version >/dev/null

if ss -lnt "( sport = :80 )" 2>/dev/null | grep -q LISTEN; then
  echo "ERROR: TCP port 80 is already in use; acme.sh standalone HTTP-01 needs it." >&2
  ss -lntp "( sport = :80 )" >&2 || true
  exit 1
fi

if [ ! -x "$ACME" ]; then
  echo "==> Installing acme.sh"
  curl -fsSL https://get.acme.sh | sh -s email="$ACME_EMAIL"
fi
"$ACME" --set-default-ca --server letsencrypt

mkdir -p "$CERT_DIR"

if [ ! -d "$ACME_HOME/${DOMAIN}_ecc" ]; then
  echo "==> Issuing Let's Encrypt certificate for $DOMAIN"
  "$ACME" --issue -d "$DOMAIN" --standalone --keylength ec-256
else
  echo "==> Existing acme.sh order found for $DOMAIN; installing current certificate"
fi

echo "==> Installing Console certificate under $CERT_DIR"
"$ACME" --install-cert -d "$DOMAIN" --ecc \
  --fullchain-file "$CERT_DIR/console.crt" \
  --key-file "$CERT_DIR/console.key" \
  --reloadcmd "$COMPOSE_RELOAD"

chmod 644 "$CERT_DIR/console.crt"
chmod 600 "$CERT_DIR/console.key"

echo "==> Certificate"
openssl x509 -in "$CERT_DIR/console.crt" -noout -subject -issuer -dates
echo
echo "==> Activate the ACME overlay"
echo "cd '$OPS_DIR'"
echo "docker compose -f docker-compose.yml -f docker-compose.acme.yml --env-file .env --profile full --profile desktop up -d --force-recreate --no-deps console"
