#!/usr/bin/env bash
# 演示环境定时重置。由 cron 每小时整点调用（见文件末尾注释）。
#
# 思路：只持久化 Agent 身份（asset-id / agent-token 在具名卷里），别的全可丢。
# 所以重置不需要重新发安装码——重建容器后 Agent 用原凭据自己接回来。
#
#   1. 重建 Agent 容器：访客装的包、改的文件、起的进程一并消失。
#   2. 清审计、操作记录、会话录制：既省磁盘，也不把上一位访客的操作留给下一位看。
#   3. 清掉访客建的端口映射与部署 Token：这两个是滥用面最大的。
#
# 不动的东西：资产表、分组、用户、监控历史（留着让 Demo 有数据可看）。
#
# 用法：bash reset.sh   /   DRY_RUN=1 bash reset.sh
set -euo pipefail

OPS_DIR="${OPS_DIR:-/opt/ops}"
COMPOSE=(docker compose -f docker-compose.yml -f deploy/demo/docker-compose.demo.yml
         --env-file .env --profile full --profile desktop)
AGENTS=(demo-agent-web01 demo-agent-web02 demo-agent-db01 demo-agent-jump01)
DRY_RUN="${DRY_RUN:-0}"

cd "$OPS_DIR"

run() {
  if [ "$DRY_RUN" = "1" ]; then
    echo "[dry-run] $*"
  else
    "$@"
  fi
}

echo "===== $(date -Is) 开始重置 ====="

# ---- 1. 重建 Agent 容器（身份卷保留，容器文件系统丢弃）----
echo "==> 重建 Agent 容器"
run "${COMPOSE[@]}" up -d --force-recreate "${AGENTS[@]}"

# ---- 2. 清审计与录制 ----
# 表名用 to_regclass 判断，不存在就跳过，避免脚本随 schema 演进而失效。
echo "==> 清审计表"
SQL=$(cat <<'EOSQL'
DO $$
DECLARE
  t text;
BEGIN
  FOREACH t IN ARRAY ARRAY[
    'server_operation_records',
    'control_audit_events',
    'asset_events',
    'port_mappings',
    'deploy_tokens'
  ] LOOP
    IF to_regclass(t) IS NOT NULL THEN
      EXECUTE format('TRUNCATE TABLE %I', t);
      RAISE NOTICE 'truncated %', t;
    ELSE
      RAISE NOTICE 'skip (absent) %', t;
    END IF;
  END LOOP;
END $$;
EOSQL
)
if [ "$DRY_RUN" = "1" ]; then
  echo "[dry-run] psql <<< 上面的 DO 块"
else
  "${COMPOSE[@]}" exec -T postgres psql -U ops -d ops -v ON_ERROR_STOP=1 <<<"$SQL"
fi

echo "==> 清会话录制与 JSONL spool"
# data/ops-audit 同时挂给 control-api / gateway / guacd；只删内容不删目录本身。
run find ./data/ops-audit -mindepth 1 -maxdepth 1 -exec rm -rf {} +

# ---- 3. 重启 gateway，让它重新登记 spool 与端口监听 ----
# 上一步把 spool 目录清空了，顺手重启避免它握着已删除的 .open 文件句柄。
echo "==> 重启 gateway"
run "${COMPOSE[@]}" restart gateway

echo "==> 状态"
run "${COMPOSE[@]}" ps

echo "===== $(date -Is) 重置完成 ====="

# 安装到 cron（每小时整点）：
#   ( crontab -l 2>/dev/null | grep -v 'demo/reset.sh' ;
#     echo '0 * * * * /usr/bin/flock -n /tmp/woops-demo-reset.lock bash /opt/ops/deploy/demo/reset.sh >> /var/log/woops-demo-reset.log 2>&1' 
#   ) | crontab -
