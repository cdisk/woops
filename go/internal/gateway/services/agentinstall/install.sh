#!/usr/bin/env bash
# Served as GET /i/{code}/install.sh - placeholders filled by gateway.
set -euo pipefail
GATEWAY_BASE="{{GATEWAY_BASE}}"
INSTALL_CODE="{{INSTALL_CODE}}"
GATEWAY_TLS_SPKI_SHA256="{{GATEWAY_TLS_SPKI_SHA256}}"
AGENT_SHA256_AMD64="{{AGENT_SHA256_AMD64}}"
AGENT_SHA256_ARM64="{{AGENT_SHA256_ARM64}}"
# OS = binary download family (linux/darwin); OS_LABEL = inventory pretty name.
OS_KERNEL=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS_KERNEL" in
  linux*) OS=linux ;;
  darwin*) OS=darwin ;;
  *) OS=linux ;;
esac
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
esac

OS_LABEL=""
if [ -r /etc/os-release ]; then
  # shellcheck disable=SC1091
  . /etc/os-release
  OS_LABEL=${PRETTY_NAME:-}
fi
if [ -z "$OS_LABEL" ]; then
  OS_LABEL=$(uname -srm 2>/dev/null || echo "$OS")
fi
OS_LABEL_JSON=$(printf '%s' "$OS_LABEL" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g')

CONF_DIR=/etc/woops-agent
mkdir -p "$CONF_DIR" /usr/local/bin /var/log
install -d -m 0750 /var/log/woops-agent 2>/dev/null || mkdir -p /var/log/woops-agent

need_cmd() {
  local c="$1" hint="${2:-}"
  if ! command -v "$c" >/dev/null 2>&1; then
    echo "ERROR: missing command '$c'${hint:+ ($hint)}" >&2
    exit 1
  fi
}

# Fail fast with a clear message (install used to hide curl/gzip errors in /tmp).
need_cmd curl "apt install curl / yum install curl"
if [ -n "$GATEWAY_TLS_SPKI_SHA256" ]; then
  need_cmd xxd "apt install xxd / yum install vim-common - required for TLS pin"
fi
if ! command -v sha256sum >/dev/null 2>&1 && ! command -v shasum >/dev/null 2>&1; then
  echo "ERROR: need sha256sum (coreutils) or shasum to verify agent binary" >&2
  exit 1
fi

# curl with optional SPKI pin. Self-signed: -k + --pinnedpubkey (CA check runs first; pin still enforced).
curl_gw() {
  local args=(-fsSL)
  if [ -n "$GATEWAY_TLS_SPKI_SHA256" ]; then
    local hex="$GATEWAY_TLS_SPKI_SHA256"
    hex=${hex#sha256:}
    hex=${hex#sha256/}
    hex=${hex#sha256//}
    local b64
    b64=$(printf '%s' "$hex" | xxd -r -p | base64 | tr -d '\n\r')
    # -k disables CA trust only; wrong SPKI still fails with pinnedpubkey mismatch
    args+=(-k --pinnedpubkey "sha256//${b64}")
  fi
  curl "${args[@]}" "$@"
}

has_systemd() {
  command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]
}

agent_running() {
  (has_systemd && systemctl is-active --quiet woops-agent) \
    || pgrep -x woops-agent >/dev/null 2>&1 \
    || pgrep -f '/usr/local/bin/woops-agent' >/dev/null 2>&1 \
    || pgrep -f '/opt/woops-agent' >/dev/null 2>&1
}

LIVE=0
if agent_running; then
  LIVE=1
fi

if [ "$LIVE" = "1" ]; then
  echo "==> Live update: keep current agent until download/register finish"
else
  echo "==> Stopping previous woops-agent (if any)"
  if has_systemd; then
    systemctl stop woops-agent 2>/dev/null || true
  fi
  # Exact name only - avoid pkill -f matching this install script's cmdline.
  pkill -9 -x woops-agent >/dev/null 2>&1 || true
  sleep 1
fi

EXISTING_ASSET_ID=""
if [ -s "$CONF_DIR/asset-id" ]; then
  EXISTING_ASSET_ID=$(tr -d '[:space:]' < "$CONF_DIR/asset-id")
fi
rm -f "$CONF_DIR/agent-id" 2>/dev/null || true

verify_agent_sha256() {
  local file="$1"
  local expect=""
  case "$ARCH" in
    amd64) expect="$AGENT_SHA256_AMD64" ;;
    arm64) expect="$AGENT_SHA256_ARM64" ;;
  esac
  if [ -z "$expect" ]; then
    echo "==> WARNING: no embedded agent sha256 for $ARCH; skip integrity check"
    return 0
  fi
  local got
  if command -v sha256sum >/dev/null 2>&1; then
    got=$(sha256sum "$file" | awk '{print $1}')
  elif command -v shasum >/dev/null 2>&1; then
    got=$(shasum -a 256 "$file" | awk '{print $1}')
  else
    echo "ERROR: need sha256sum or shasum to verify agent binary" >&2
    exit 1
  fi
  if [ "$got" != "$expect" ]; then
    echo "ERROR: agent sha256 mismatch (got $got want $expect)" >&2
    exit 1
  fi
  echo "==> Agent sha256 ok"
}

echo "==> Downloading woops-agent ($OS/$ARCH)..."
download_agent() {
  local dest="$1"
  local base="$GATEWAY_BASE/i/$INSTALL_CODE/agent/$OS/$ARCH"
  local err
  err=$(mktemp)
  # Prefer gzip payload when gzip exists; else raw binary.
  if command -v gzip >/dev/null 2>&1; then
    if curl_gw "$base?format=gz" -o "$dest.gz" 2>"$err"; then
      if ! gzip -dc "$dest.gz" > "$dest" 2>>"$err"; then
        echo "ERROR: failed to decompress agent (gzip). Details:" >&2
        cat "$err" >&2 || true
        rm -f "$err" "$dest.gz"
        return 1
      fi
      rm -f "$dest.gz" "$err"
      verify_agent_sha256 "$dest"
      return 0
    fi
    echo "==> gzip download failed; trying uncompressed binary..." >&2
    cat "$err" >&2 || true
  fi
  if ! curl_gw "$base" -o "$dest" 2>"$err"; then
    echo "ERROR: agent download failed. Details:" >&2
    cat "$err" >&2 || true
    rm -f "$err"
    return 1
  fi
  rm -f "$err"
  verify_agent_sha256 "$dest"
}

# Prefer /usr/local/bin; only download to *-new when that exact path is the
# running woops-agent binary (Linux ETXTBSY).
# /opt/woops-agent is created only when that fallback path is actually used.
if [ -x /usr/local/bin/woops-agent ] || [ "$LIVE" = "0" ]; then
  BIN=/usr/local/bin/woops-agent
else
  BIN=/opt/woops-agent/woops-agent
  mkdir -p /opt/woops-agent
fi

# Stage to *-new only when overwriting a live woops-agent at $BIN.
need_staged_swap=0
if [ "$LIVE" = "1" ] && [ -x "$BIN" ] && pgrep -x woops-agent >/dev/null 2>&1; then
  need_staged_swap=1
fi

STAGE="$BIN"
if [ "$need_staged_swap" = "1" ]; then
  STAGE="${BIN}-new"
fi
if ! download_agent "$STAGE"; then
  echo "==> Retry install path under /opt/woops-agent ..." >&2
  BIN=/opt/woops-agent/woops-agent
  mkdir -p /opt/woops-agent
  need_staged_swap=0
  if [ "$LIVE" = "1" ] && [ -x "$BIN" ] && pgrep -x woops-agent >/dev/null 2>&1; then
    need_staged_swap=1
  fi
  STAGE="$BIN"
  if [ "$need_staged_swap" = "1" ]; then
    STAGE="${BIN}-new"
  fi
  download_agent "$STAGE"
fi
echo "==> Downloaded to $STAGE"
chmod +x "$STAGE"
# If we staged but final path is somehow free, promote immediately.
if [ "$STAGE" != "$BIN" ] && [ ! -e "$BIN" ]; then
  mv -f "$STAGE" "$BIN"
  STAGE="$BIN"
  need_staged_swap=0
  echo "==> Installed binary at $BIN (final path was free)"
fi
if [ ! -x "$BIN" ] && [ ! -x "${BIN}-new" ]; then
  echo "ERROR: agent binary missing after download ($BIN)" >&2
  exit 1
fi
# Online update (一键更新 / Web Shell) runs inside the agent session: never
# stop the live agent in-process or the install is killed before start.
# Always defer stop→start when LIVE=1.
need_deferred_restart=0
if [ "$LIVE" = "1" ]; then
  need_deferred_restart=1
fi

AGENT_VERSION=$("$STAGE" -version 2>/dev/null | head -n1 | tr -d '[:space:]' || true)
if [ -z "$AGENT_VERSION" ]; then
  echo "ERROR: cannot read version from $STAGE" >&2
  exit 1
fi
echo "==> Agent version: $AGENT_VERSION"

# Minimal images (busybox / stripped RHEL) often lack hostname(1).
detect_hostname() {
  local h=""
  if command -v hostname >/dev/null 2>&1; then
    h=$(hostname 2>/dev/null || true)
  fi
  if [ -z "$h" ] && [ -r /proc/sys/kernel/hostname ]; then
    h=$(tr -d '[:space:]' </proc/sys/kernel/hostname || true)
  fi
  if [ -z "$h" ] && [ -r /etc/hostname ]; then
    h=$(tr -d '[:space:]' </etc/hostname || true)
  fi
  if [ -z "$h" ]; then
    h=$(uname -n 2>/dev/null || true)
  fi
  if [ -z "$h" ] || [ "$h" = "(none)" ]; then
    h=unknown
  fi
  printf '%s' "$h"
}

detect_private_ips() {
  local ips=""
  if command -v hostname >/dev/null 2>&1; then
    ips=$(hostname -I 2>/dev/null | tr ' ' ',' | sed 's/,$//' || true)
  fi
  if [ -z "$ips" ] && command -v ip >/dev/null 2>&1; then
    ips=$(ip -4 -o addr show scope global 2>/dev/null \
      | awk '{gsub(/\/.*/, "", $4); print $4}' \
      | tr '\n' ',' | sed 's/,$//' || true)
  fi
  printf '%s' "$ips"
}

HOSTNAME=$(detect_hostname)
PRIVATE_IP=$(detect_private_ips)
echo "==> Detected OS: $OS_LABEL host=$HOSTNAME"
if [ -n "$EXISTING_ASSET_ID" ]; then
  BODY=$(printf '{"installCode":"%s","assetId":"%s","agentVersion":"%s","hostname":"%s","os":"%s","arch":"%s","privateIp":"%s"}' \
    "$INSTALL_CODE" "$EXISTING_ASSET_ID" "$AGENT_VERSION" "$HOSTNAME" "$OS_LABEL_JSON" "$ARCH" "$PRIVATE_IP")
  echo "==> Re-registering assetId=$EXISTING_ASSET_ID (refresh token)"
else
  BODY=$(printf '{"installCode":"%s","agentVersion":"%s","hostname":"%s","os":"%s","arch":"%s","privateIp":"%s"}' \
    "$INSTALL_CODE" "$AGENT_VERSION" "$HOSTNAME" "$OS_LABEL_JSON" "$ARCH" "$PRIVATE_IP")
  echo "==> First-time register"
fi
RESP=$(curl_gw -X POST "$GATEWAY_BASE/api/agent/register" -H 'Content-Type: application/json' -d "$BODY")
echo "$RESP"

if command -v jq >/dev/null 2>&1; then
  ASSET_ID=$(echo "$RESP" | jq -r .assetId)
  AGENT_TOKEN=$(echo "$RESP" | jq -r .agentToken)
  REUSED=$(echo "$RESP" | jq -r .reused)
else
  ASSET_ID=$(echo "$RESP" | sed -n 's/.*"assetId":"\([^"]*\)".*/\1/p')
  AGENT_TOKEN=$(echo "$RESP" | sed -n 's/.*"agentToken":"\([^"]*\)".*/\1/p')
  REUSED=$(echo "$RESP" | sed -n 's/.*"reused":\([^,}]*\).*/\1/p')
fi
if [ -z "$ASSET_ID" ] || [ "$ASSET_ID" = "null" ]; then
  echo "ERROR: register failed" >&2
  exit 1
fi
if [ "$REUSED" = "true" ]; then
  echo "==> Reused existing asset (token refreshed)"
else
  echo "==> Registered as new asset"
fi

# Keep https:// (or http:// for local dev); do not strip scheme.
GATEWAY_URL=$(printf '%s' "$GATEWAY_BASE" | sed -E 's#/$##')
TLS_PIN="$GATEWAY_TLS_SPKI_SHA256"

# Detect install-time curl proxy env - persist as gatewayProxy for Agent runtime WSS.
detect_gateway_proxy_env() {
  local v cand
  for v in https_proxy HTTPS_PROXY ALL_PROXY all_proxy http_proxy HTTP_PROXY; do
    eval "cand=\${$v-}"
    if [ -n "${cand}" ]; then
      printf '%s' "$cand"
      return 0
    fi
  done
  return 1
}

# Upsert a top-level YAML key with a double-quoted scalar (ENVIRON-safe for special chars).
upsert_yaml_quoted() {
  local file="$1" key="$2" value="$3"
  local esc line tmp
  esc=$(printf '%s' "$value" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g')
  line="${key}: \"${esc}\""
  tmp=$(mktemp)
  if grep -qE "^[[:space:]]*${key}:" "$file"; then
    LINE="$line" KEY="$key" awk '
      BEGIN { line=ENVIRON["LINE"]; key=ENVIRON["KEY"]; done=0 }
      $0 ~ "^[[:space:]]*" key ":" {
        if (!done) { print line; done=1; next }
      }
      { print }
      END { if (!done) print line }
    ' "$file" > "$tmp"
  else
    LINE="$line" awk '
      BEGIN { line=ENVIRON["LINE"]; done=0 }
      /^[[:space:]]*gatewayTlsSpkiSha256:/ { print; if (!done) { print line; done=1 }; next }
      { print }
      END { if (!done) print line }
    ' "$file" > "$tmp"
  fi
  mv -f "$tmp" "$file"
}

# Fresh install: annotated template. Re-install: refresh gateway + pin, keep other local edits.
write_agent_yaml() {
  if [ -f "$CONF_DIR/agent.yaml" ]; then
    tmp=$(mktemp)
    if grep -qE '^[[:space:]]*gateway:' "$CONF_DIR/agent.yaml"; then
      sed -E 's|^[[:space:]]*gateway:.*|gateway: "'"$GATEWAY_URL"'"|' "$CONF_DIR/agent.yaml" > "$tmp"
    else
      { printf 'gateway: "%s"\n' "$GATEWAY_URL"; cat "$CONF_DIR/agent.yaml"; } > "$tmp"
    fi
    if grep -qE '^[[:space:]]*gatewayTlsSpkiSha256:' "$tmp"; then
      sed -E 's|^[[:space:]]*gatewayTlsSpkiSha256:.*|gatewayTlsSpkiSha256: "'"$TLS_PIN"'"|' "$tmp" > "${tmp}.2"
      mv -f "${tmp}.2" "$tmp"
    else
      printf 'gatewayTlsSpkiSha256: "%s"\n' "$TLS_PIN" >> "$tmp"
    fi
    mv -f "$tmp" "$CONF_DIR/agent.yaml"
  else
    cat > "$CONF_DIR/agent.yaml" <<'YAML'
{{AGENT_YAML_TEMPLATE}}
YAML
    # Quoted heredoc keeps template literal; fill after write.
    sed -i.bak -e "s|__GATEWAY__|${GATEWAY_URL}|g" -e "s|__TLS_PIN__|${TLS_PIN}|g" "$CONF_DIR/agent.yaml"
    rm -f "$CONF_DIR/agent.yaml.bak"
  fi
  # Persist outbound proxy so systemd Agent (no shell env) can reach Gateway via CONNECT.
  if PROXY_FROM_ENV=$(detect_gateway_proxy_env); then
    echo "==> Writing gatewayProxy from install env (Agent WSS will use this proxy)"
    upsert_yaml_quoted "$CONF_DIR/agent.yaml" gatewayProxy "$PROXY_FROM_ENV"
  fi
}
write_agent_yaml

printf '%s\n' "$ASSET_ID" > "$CONF_DIR/asset-id"
printf '%s\n' "$AGENT_TOKEN" > "$CONF_DIR/agent-token"
chmod 600 "$CONF_DIR/asset-id" "$CONF_DIR/agent-token" 2>/dev/null || true

install_systemd_unit() {
  cat > /etc/systemd/system/woops-agent.service <<UNIT
[Unit]
Description=Woops Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
Environment=OPS_AGENT_CONFIG=$CONF_DIR/agent.yaml
ExecStart=$BIN -config $CONF_DIR/agent.yaml
# Do not use $CONF_DIR as WorkingDirectory: interactive shells override to home in Agent.
WorkingDirectory=/
Restart=always
RestartSec=3
KillMode=process
TimeoutStopSec=15

[Install]
WantedBy=multi-user.target
UNIT
  systemctl daemon-reload
  systemctl enable woops-agent >/dev/null 2>&1 || true
}

# Kill only the agent binary by exact comm name. Never use pkill -f with the
# install path: a bash -c / restart script whose cmdline contains that path
# would match and suicide before relaunch (left host offline after live update).
stop_agent_procs() {
  if has_systemd; then
    systemctl stop woops-agent 2>/dev/null || true
  fi
  pkill -9 -x woops-agent >/dev/null 2>&1 || true
}

start_agent() {
  if [ -f "${BIN}-new" ]; then
    mv -f "${BIN}-new" "$BIN"
  fi
  if [ ! -x "$BIN" ]; then
    echo "ERROR: cannot start - missing executable $BIN" >&2
    exit 1
  fi
  chmod +x "$BIN"
  if has_systemd; then
    install_systemd_unit
    systemctl restart woops-agent
    echo "woops-agent systemd service started (systemctl status woops-agent)"
  else
    nohup env OPS_AGENT_CONFIG="$CONF_DIR/agent.yaml" "$BIN" -config "$CONF_DIR/agent.yaml" >/dev/null 2>&1 &
    echo "woops-agent started (pid $!) - no systemd; using nohup (logs: /var/log/woops-agent/woops-agent.log)"
  fi
}

# Write/refresh unit before live restart so the deferred script can systemctl start.
# Must not stop the live agent here or the online-update exec session dies early.
if has_systemd; then
  echo "==> Installing systemd unit woops-agent.service (ExecStart=$BIN)"
  install_systemd_unit
fi

if [ "$need_deferred_restart" = "1" ]; then
  echo "==> Staging restart in 2s (online update session will disconnect; agent comes back online)"
  RESTART_SH="$CONF_DIR/restart-update.sh"
  cat > "$RESTART_SH" <<EOF
#!/bin/sh
set -eu
sleep 2
if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
  systemctl stop woops-agent 2>/dev/null || true
fi
pkill -9 -x woops-agent >/dev/null 2>&1 || true
sleep 1
if [ -f '${BIN}-new' ]; then
  mv -f '${BIN}-new' '$BIN'
fi
if [ ! -x '$BIN' ]; then
  echo "ERROR: missing agent binary at $BIN (deferred restart failed)" >&2
  echo "missing $BIN at \$(date -Is)" >>/var/log/woops-agent-restart.log
  exit 1
fi
chmod +x '$BIN'
if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
  systemctl daemon-reload
  systemctl enable woops-agent >/dev/null 2>&1 || true
  systemctl start woops-agent
  echo "restarted via systemd at \$(date -Is)" >>/var/log/woops-agent-restart.log
else
  nohup env OPS_AGENT_CONFIG='$CONF_DIR/agent.yaml' '$BIN' -config '$CONF_DIR/agent.yaml' >/dev/null 2>&1 &
  echo "restarted pid \$! at \$(date -Is)" >>/var/log/woops-agent-restart.log
fi
EOF
  chmod +x "$RESTART_SH"
  # Schedule outside the exec session. Prefer systemd-run --no-block so the timer/service
  # is owned by systemd (survives exec teardown / process-group kill). setsid alone can
  # still be reaped when the online-update shell exits.
  scheduled=0
  if command -v systemd-run >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
    unit="woops-agent-restart-$$"
    if systemd-run --no-block --collect --quiet --unit="$unit" /bin/sh "$RESTART_SH" \
        >>/var/log/woops-agent-restart.log 2>&1; then
      echo "==> Restart scheduled via systemd-run ($unit)"
      scheduled=1
    else
      echo "==> WARNING: systemd-run failed; falling back to setsid/nohup" >&2
    fi
  fi
  if [ "$scheduled" != "1" ]; then
    if command -v setsid >/dev/null 2>&1; then
      setsid "$RESTART_SH" >>/var/log/woops-agent-restart.log 2>&1 < /dev/null &
    else
      nohup "$RESTART_SH" >>/var/log/woops-agent-restart.log 2>&1 &
    fi
  fi
  echo "==> Update staged (version $AGENT_VERSION). Agent will restart shortly."
  if [ "$need_staged_swap" = "1" ]; then
    echo "    Binary swap: ${BIN}-new -> $BIN (after stop)"
  else
    echo "    Binary ready at $BIN (will restart woops-agent shortly)"
  fi
else
  echo "==> Starting woops-agent ($BIN)..."
  stop_agent_procs || true
  sleep 1
  start_agent
  if has_systemd; then
    echo "    Restart later: systemctl restart woops-agent"
  fi
fi
