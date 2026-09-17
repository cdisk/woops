# 演示环境的分组树与安装码。被 seed-assets.sh（首次铺资产）和 reset.sh（整点重置）
# 共同 source，别在两处各写一份 SQL。
#
# 依赖调用方先定义 psql_q()：接一条 SQL、把裸结果打到 stdout。
#
# 分组名一律英文：分组名是**数据**，不走 i18n，切浏览器语言不会变，
# 而这是个面向全球访客的公开 Demo。

DEMO_ROOT_NAME='Demo environment'
DEMO_GROUPS=('Web cluster' 'Database' 'Bastion')

# 建出分组树并回填 GID_WEB / GID_DB / GID_JMP。
# 幂等：按 name 找、没有才建——每次重置都要调，因为 setup-demo.py 的清理
# 会把「当时没有资产挂着」的分组删掉（Database 尤其容易被删）。
ensure_demo_groups() {
  psql_q "
  INSERT INTO server_groups (id, name, parent_id, sort_order, created_at, updated_at)
  SELECT gen_random_uuid(), '$DEMO_ROOT_NAME', NULL, 0, NOW(), NOW()
  WHERE NOT EXISTS (SELECT 1 FROM server_groups WHERE name = '$DEMO_ROOT_NAME');
  " >/dev/null

  DEMO_ROOT_ID=$(psql_q "SELECT id FROM server_groups WHERE name='$DEMO_ROOT_NAME' LIMIT 1;")
  [ -n "$DEMO_ROOT_ID" ] || { echo "ERROR: 根分组创建失败" >&2; return 1; }

  local i=0 g
  for g in "${DEMO_GROUPS[@]}"; do
    i=$((i + 1))
    psql_q "
    INSERT INTO server_groups (id, name, parent_id, sort_order, created_at, updated_at)
    SELECT gen_random_uuid(), '$g', '$DEMO_ROOT_ID', $i, NOW(), NOW()
    WHERE NOT EXISTS (SELECT 1 FROM server_groups WHERE name = '$g' AND parent_id = '$DEMO_ROOT_ID');
    " >/dev/null
  done

  GID_WEB=$(psql_q "SELECT id FROM server_groups WHERE name='${DEMO_GROUPS[0]}' AND parent_id='$DEMO_ROOT_ID' LIMIT 1;")
  GID_DB=$(psql_q  "SELECT id FROM server_groups WHERE name='${DEMO_GROUPS[1]}' AND parent_id='$DEMO_ROOT_ID' LIMIT 1;")
  GID_JMP=$(psql_q "SELECT id FROM server_groups WHERE name='${DEMO_GROUPS[2]}' AND parent_id='$DEMO_ROOT_ID' LIMIT 1;")
  [ -n "$GID_WEB" ] && [ -n "$GID_DB" ] && [ -n "$GID_JMP" ] \
    || { echo "ERROR: 子分组 id 取不到" >&2; return 1; }
}

# 给某个分组签一个 30 分钟的安装码，打到 stdout。
# 明文存库、16 位 hex，与 control-api 的 randomToken(8) 同格式。
mint_install_code() {
  local gid="$1" code
  code=$(openssl rand -hex 8)
  psql_q "
  INSERT INTO install_codes (id, code, expires_at, revoked, used_count, group_id, created_at)
  VALUES (gen_random_uuid(), '$code', NOW() + INTERVAL '30 minutes', false, 0, '$gid', NOW());
  " >/dev/null
  printf '%s' "$code"
}
