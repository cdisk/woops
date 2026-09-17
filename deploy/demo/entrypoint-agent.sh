#!/bin/sh
# 演示 Agent 启动：每次重写 agent.yaml（配置不是身份），保留 asset-id / agent-token。
#
# 首次启动时 /etc/woops-agent 是空卷，需要一个安装码：
#   WOOPS_INSTALL_CODE=<控制台生成的安装码>  docker compose up -d demo-agent-xxx
# Agent 自注册成功后会自己删掉 install-code 并落下身份文件，之后重建容器不用再给码。
set -eu

CONF_DIR=/etc/woops-agent
mkdir -p "$CONF_DIR"

: "${WOOPS_GATEWAY:?需要 WOOPS_GATEWAY}"

# 不写 gatewayTlsSpkiSha256：证书由公有 CA 签发，走系统 CA 校验，
# 这样 acme.sh 续期（哪怕换了私钥）也不会让 Agent 掉线。
cat > "$CONF_DIR/agent.yaml" <<EOF
gateway: ${WOOPS_GATEWAY}
metrics:
  enabled: true
  intervalSec: 60
EOF

if [ ! -f "$CONF_DIR/asset-id" ]; then
  if [ -n "${WOOPS_INSTALL_CODE:-}" ]; then
    printf '%s\n' "$WOOPS_INSTALL_CODE" > "$CONF_DIR/install-code"
    chmod 600 "$CONF_DIR/install-code"
    echo "==> 首次上线，已写入安装码"
  elif [ ! -f "$CONF_DIR/install-code" ]; then
    echo "ERROR: $CONF_DIR 无身份文件，且未提供 WOOPS_INSTALL_CODE。" >&2
    echo "       到控制台「资产 → 选分组 → 生成安装链接」取一个安装码再启动。" >&2
    exit 1
  fi
fi

exec woops-agent -config "$CONF_DIR/agent.yaml"
