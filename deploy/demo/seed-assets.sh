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
# 幂等：按 name 找，没有才建。sort_order 决定控制台里的排列顺序。
echo "==> 分组树"
psql_q "
INSERT INTO server_groups (id, name, parent_id, sort_order, created_at, updated_at)
SELECT gen_random_uuid(), '演示环境', NULL, 0, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM server_groups WHERE name = '演示环境');
" >/dev/null

ROOT_ID=$(psql_q "SELECT id FROM server_groups WHERE name='演示环境' LIMIT 1;")
[ -n "$ROOT_ID" ] || { echo "ERROR: 根分组创建失败" >&2; exit 1; }

i=0
for g in "Web 集群" "数据库" "跳板机"; do
  i=$((i + 1))
  psql_q "
  INSERT INTO server_groups (id, name, parent_id, sort_order, created_at, updated_at)
  SELECT gen_random_uuid(), '$g', '$ROOT_ID', $i, NOW(), NOW()
  WHERE NOT EXISTS (SELECT 1 FROM server_groups WHERE name = '$g' AND parent_id = '$ROOT_ID');
  " >/dev/null
done

GID_WEB=$(psql_q "SELECT id FROM server_groups WHERE name='Web 集群'  AND parent_id='$ROOT_ID' LIMIT 1;")
GID_DB=$(psql_q  "SELECT id FROM server_groups WHERE name='数据库'    AND parent_id='$ROOT_ID' LIMIT 1;")
GID_JMP=$(psql_q "SELECT id FROM server_groups WHERE name='跳板机'    AND parent_id='$ROOT_ID' LIMIT 1;")
echo "    Web=$GID_WEB  DB=$GID_DB  Jump=$GID_JMP"

# ---- 2. 临时安装码 ----
# 明文存库、16 位 hex，与 control-api 的 randomToken(8) 同格式。30 分钟够容器起来了。
echo "==> 临时安装码（30 分钟）"
mint() {
  local gid="$1" code
  code=$(openssl rand -hex 8)
  psql_q "
  INSERT INTO install_codes (id, code, expires_at, revoked, used_count, group_id, created_at)
  VALUES (gen_random_uuid(), '$code', NOW() + INTERVAL '30 minutes', false, 0, '$gid', NOW());
  " >/dev/null
  printf '%s' "$code"
}
CODE_WEB=$(mint "$GID_WEB")
CODE_DB=$(mint "$GID_DB")
CODE_JMP=$(mint "$GID_JMP")

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
