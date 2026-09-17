#!/usr/bin/env bash
# 首次填充演示资产：建分组树 → 发临时安装码 → 起 Agent 容器 → 等注册 → 吊销安装码。
#
# 只需跑一次。Agent 的 asset-id / agent-token 落在具名卷里，之后 reset.sh 重建容器
# 时会用原凭据接回来，不再需要安装码。
#
# 为什么用完立刻吊销：/api/install-codes/{code}/valid 是匿名接口，且经 Gateway
# 挂在公网 9200 上。留一个长期有效的码，等于允许任何人往演示环境里注册自己的
# Agent，资产列表会被灌垃圾。
#
# 重新来过（连身份一起丢弃）：
#   docker compose … down -v   # 注意这会删掉所有卷，包括 Postgres
#   或只删 Agent 身份：docker volume rm ops_agent_id_web01 …
set -euo pipefail

OPS_DIR="${OPS_DIR:-/opt/ops}"
cd "$OPS_DIR"

COMPOSE=(docker compose -f docker-compose.yml -f deploy/demo/docker-compose.demo.yml
         --env-file .env --profile full --profile desktop)
AGENTS=(demo-agent-web01 demo-agent-web02 demo-agent-db01 demo-agent-jump01)

psql_q() { "${COMPOSE[@]}" exec -T postgres psql -U ops -d ops -qtAX -v ON_ERROR_STOP=1 -c "$1"; }

# ---- 1. 分组树 ----
# shellcheck source=deploy/demo/demo-tree.sh
source deploy/demo/demo-tree.sh

echo "==> 分组树"
ensure_demo_groups
echo "    Web=$GID_WEB  DB=$GID_DB  Jump=$GID_JMP"

# ---- 2. 临时安装码 ----
# 一个分组一个码，这样 Agent 注册就落在自己该在的分组里。30 分钟够容器起来了。
echo "==> 临时安装码（30 分钟）"
CODE_WEB=$(mint_install_code "$GID_WEB")
CODE_DB=$(mint_install_code "$GID_DB")
CODE_JMP=$(mint_install_code "$GID_JMP")

# ---- 3. 起 Agent ----
echo "==> 构建并启动 Agent 容器"
WOOPS_CODE_WEB="$CODE_WEB" WOOPS_CODE_DB="$CODE_DB" WOOPS_CODE_JUMP="$CODE_JMP" \
  "${COMPOSE[@]}" up -d --build "${AGENTS[@]}"

# ---- 4. 等注册 ----
echo "==> 等待 4 台资产注册上线"
for attempt in $(seq 1 30); do
  n=$(psql_q "SELECT count(*) FROM assets;")
  online=$(psql_q "SELECT count(*) FROM assets WHERE online;")
  echo "    [$attempt/30] 已注册 $n，在线 $online"
  [ "$online" -ge 4 ] && break
  sleep 5
done

# ---- 5. 吊销安装码 ----
echo "==> 吊销本次安装码"
psql_q "UPDATE install_codes SET revoked = true WHERE code IN ('$CODE_WEB','$CODE_DB','$CODE_JMP');" >/dev/null

echo
echo "==> 资产清单"
"${COMPOSE[@]}" exec -T postgres psql -U ops -d ops -c \
  "SELECT a.hostname, a.os, a.arch, a.online, g.name AS grp
     FROM assets a LEFT JOIN server_groups g ON g.id = a.group_id
    ORDER BY a.hostname;"
