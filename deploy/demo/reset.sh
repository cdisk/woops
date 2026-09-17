#!/usr/bin/env bash
# 公开 Demo 的定时重置。由 cron 每小时整点调用，见 deploy/demo/install-cron.sh。
#
# 访客拿的是 ADMIN，能删资产、改演示账号密码、建用户建分组。所以重置要能自愈：
#   - 身份卷里的 asset-id 在库里已经没有了 → 清空该卷，用新安装码重新上线
#   - 演示账号的密码 / 角色 / TOTP → 由 setup-demo.py 复位
#   - 访客建的用户、分组、以及非演示资产 → 清掉
set -euo pipefail

OPS_DIR=${OPS_DIR:-/opt/ops}
cd "$OPS_DIR"

COMPOSE=(docker compose -f docker-compose.yml -f deploy/demo/docker-compose.demo.yml
         --env-file .env --profile full --profile desktop)
PROJECT=$(basename "$OPS_DIR")
AGENTS=(web01 web02 db01 jump01)

log() { printf '[%s] %s\n' "$(date '+%F %T')" "$*"; }

psql_q() {
  docker compose --env-file .env exec -T postgres \
    psql -U ops -d ops -tAc "$1"
}

# shellcheck disable=SC1091
source <(grep -E '^OPS_DEMO_DOMAIN=' .env)
export OPS_DEMO_DOMAIN

log "=== 重置开始 ==="

# 0) 清掉 compose 重建留下的改名残壳（形如 <12位id>_ops-demo-broker-1）。
#    本脚本用 flock 自保，但手动 compose 操作不拿这把锁；两者撞上会让重建
#    半途而废，残壳继续占着容器名，之后每个整点都会 "name is already in use"
#    而失败。故每次重置先扫一遍，让碰撞只坏一个小时。
log "清理重建残壳"
#    两种命名都要管：compose 默认名 <project>-<svc>-1，以及 Agent 的 container_name。
#    grep 无匹配时返回 1，而「没有残壳」正是常态；套在 pipefail 的管道里会
#    连带 set -e 把整个重置掐死在这一步（静默不重置，日志只剩这一行）。
mapfile -t stale_shells < <(
  docker ps -a --format '{{.Names}}' \
    | grep -E "^[0-9a-f]{12}_(${PROJECT}-[a-z-]+-[0-9]+|woops-demo-[a-z0-9]+)$" || true
)
for stale in "${stale_shells[@]}"; do
  [ -n "$stale" ] || continue
  log "  删除残壳 $stale"
  docker rm -f "$stale" >/dev/null 2>&1 || true
done

# 1) 先停 Agent，避免它们在库被清理时反复重连。
log "停止演示 Agent"
for a in "${AGENTS[@]}"; do
  "${COMPOSE[@]}" stop "demo-agent-$a" >/dev/null 2>&1 || true
done

# 2) 自愈检查：身份卷里的 asset-id 是否还在库里。
#    访客把资产删了的话，agent-token 也跟着失效，必须清卷重新注册。
log "校验 Agent 身份"
KEEP_IDS=""
for a in "${AGENTS[@]}"; do
  vol="${PROJECT}_agent_id_${a}"
  docker volume inspect "$vol" >/dev/null 2>&1 || continue

  asset_id=$(docker run --rm -v "$vol:/id:ro" alpine:3.20 \
    sh -c 'cat /id/asset-id 2>/dev/null || true' | tr -d '[:space:]')

  if [ -z "$asset_id" ]; then
    log "  $a: 尚无身份，等新安装码"
    continue
  fi

  alive=$(psql_q "select count(1) from assets where id='${asset_id}'" || echo 0)
  if [ "$alive" = "1" ]; then
    log "  $a: 身份有效（$asset_id）"
    KEEP_IDS="${KEEP_IDS:+$KEEP_IDS,}${asset_id}"
  else
    log "  $a: 资产已被删除，清空身份卷重新注册"
    docker run --rm -v "$vol:/id" alpine:3.20 sh -c 'rm -f /id/*' || true
  fi
done

# 3) 清录制与文件审计产物（库里的审计行由 OPS_AUDIT_RETENTION_DAYS 自己过期）。
log "清理录制文件"
find ./data/ops-audit -mindepth 1 -maxdepth 2 -mtime +0 -exec rm -rf {} + 2>/dev/null || true

# 4) 复位演示账号（密码 / ADMIN 角色 / TOTP / totp_last_step）、清访客残留、签发新安装码。
#    KEEP 列表 = 身份卷里仍然有效的资产，其余资产一律视为访客留下的。
log "复位演示账号并清理残留"
DEMO_CLEANUP=1 DEMO_KEEP_ASSET_IDS="$KEEP_IDS" python3 deploy/demo/setup-demo.py

# 4.5) 一个分组一个安装码。
#      以前这里把 setup-demo.py 签的那一个码发给全部四台，于是任何需要重新注册的
#      Agent 都落进那个码指向的分组（Demo Servers），原分组空了又被上面的清理删掉
#      ——分组树会一小时一小时地烂掉。docker-compose.demo.yml 本来就按
#      WOOPS_CODE_WEB / _DB / _JUMP 分派，喂三个不同的码即可。
#      必须在清理**之后**建树：清理会删掉当时没有资产挂着的分组。
# shellcheck source=deploy/demo/demo-tree.sh
source deploy/demo/demo-tree.sh
log "确保分组树存在并签发分组安装码"
ensure_demo_groups
export WOOPS_CODE_WEB WOOPS_CODE_DB WOOPS_CODE_JUMP
WOOPS_CODE_WEB=$(mint_install_code "$GID_WEB")
WOOPS_CODE_DB=$(mint_install_code "$GID_DB")
WOOPS_CODE_JUMP=$(mint_install_code "$GID_JMP")

# 5) 重建 Agent 容器：文件系统回到镜像初始状态，身份卷按上面的判断保留或重来。
log "重建演示 Agent"
for a in "${AGENTS[@]}"; do
  "${COMPOSE[@]}" up -d --force-recreate --no-deps "demo-agent-$a"
done

# 6) broker 重新登录（密码刚被复位，缓存的 token 要换）。
log "重启登录代理"
"${COMPOSE[@]}" up -d --force-recreate --no-deps demo-broker

log "=== 重置完成 ==="
