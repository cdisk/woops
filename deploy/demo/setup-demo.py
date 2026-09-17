#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""幂等初始化公开 Demo 的账号 / 分组 / 安装码。在 Demo 主机上跑，只需 python3 + docker。

- API 走 Console 的 https://<域名>/api/（control-api 的 9100 在 Demo 里不对外发布）
- 改 TOTP 字段走 `docker compose exec postgres psql`（这些列没有对外接口）

产物：
  deploy/demo/.demo-secrets.env  管理员与演示账号的口令、TOTP 密钥（0600，不入库）
  deploy/demo/demo.env           broker 与 agent 容器要用的环境变量
"""
import base64
import hashlib
import hmac
import json
import os
import re
import secrets
import ssl
import struct
import subprocess
import sys
import time
import urllib.error
import urllib.request

HERE = os.path.dirname(os.path.abspath(__file__))
OPS_DIR = os.path.dirname(os.path.dirname(HERE))
SECRETS_FILE = os.path.join(HERE, ".demo-secrets.env")
DEMO_ENV_FILE = os.path.join(HERE, "demo.env")



def _env_value(key, path):
    """只为了在设置常量前读一个值，不依赖下面的 load_env_file。"""
    if os.path.exists(path):
        with open(path, "r", encoding="utf-8", errors="replace") as f:
            for line in f:
                if line.startswith(key + "="):
                    return line.split("=", 1)[1].strip()
    return ""


DOMAIN = (os.environ.get("OPS_DEMO_DOMAIN")
          or _env_value("OPS_DEMO_DOMAIN", os.path.join(OPS_DIR, ".env")))
if not DOMAIN:
    sys.exit("拿不到域名：设置 OPS_DEMO_DOMAIN，或在 %s/.env 里配好" % OPS_DIR)
BASE = "https://%s" % DOMAIN
DEMO_USER = os.environ.get("DEMO_USERNAME", "demo")
# 与 demo-tree.sh 的 DEMO_ROOT_NAME 一致：本脚本签的备用安装码挂在这棵树的根上，
# 免得多出一个空的根分组。逐分组的安装码由 reset.sh 用 demo-tree.sh 签发。
DEMO_GROUP = os.environ.get("DEMO_GROUP", "Demo environment")
B32 = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"

CTX = ssl.create_default_context()


# ---------- 小工具 ----------

def gen_base32(n=32):
    """与 control-api 的 DefaultSecretGenerator(32) 同形：32 个 Base32 字符。"""
    return "".join(secrets.choice(B32) for _ in range(n))


def totp(secret):
    key = base64.b32decode(secret.upper())
    step = int(time.time() // 30)
    d = hmac.new(key, struct.pack(">Q", step), hashlib.sha1).digest()
    o = d[-1] & 0x0F
    return "%06d" % ((struct.unpack(">I", d[o:o + 4])[0] & 0x7FFFFFFF) % 1000000), step


def call(path, payload=None, token=None, method=None):
    data = json.dumps(payload).encode() if payload is not None else None
    req = urllib.request.Request(BASE + path, data=data,
                                 method=method or ("POST" if data else "GET"))
    if data:
        req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", "Bearer " + token)
    with urllib.request.urlopen(req, context=CTX, timeout=30) as r:
        body = r.read().decode("utf-8")
        return json.loads(body) if body.strip() else {}


def psql(sql):
    out = subprocess.run(
        ["docker", "compose", "--env-file", ".env", "exec", "-T", "postgres",
         "psql", "-U", "ops", "-d", "ops", "-tAc", sql],
        cwd=OPS_DIR, capture_output=True, text=True)
    if out.returncode != 0:
        raise RuntimeError("psql 失败: %s" % (out.stderr.strip() or out.stdout.strip()))
    return out.stdout.strip()


def load_env_file(path):
    vals = {}
    if os.path.exists(path):
        with open(path, "r", encoding="utf-8") as f:
            for line in f:
                m = re.match(r"^\s*([A-Za-z_][A-Za-z0-9_]*)=(.*)$", line)
                if m:
                    vals[m.group(1)] = m.group(2).strip()
    return vals


def write_env_file(path, vals, mode=0o600):
    with open(path, "w", encoding="utf-8", newline="\n") as f:
        for k, v in vals.items():
            f.write("%s=%s\n" % (k, v))
    os.chmod(path, mode)


# ---------- 步骤 ----------

def wait_api():
    for _ in range(60):
        try:
            call("/api/health")
            return
        except Exception:
            time.sleep(3)
    sys.exit("control-api 一直没就绪：%s/api/health" % BASE)


def admin_login(store):
    """管理员登录。首次会触发 TOTP 绑定，这里直接用返回的 secret 完成绑定并记下来。"""
    root_env = load_env_file(os.path.join(OPS_DIR, ".env"))
    user = root_env.get("OPS_BOOTSTRAP_ADMIN_USERNAME", "admin")
    pwd = store.get("ADMIN_PASSWORD") or root_env.get("OPS_BOOTSTRAP_ADMIN_PASSWORD")
    if not pwd:
        sys.exit("拿不到管理员密码：.env 里没有 OPS_BOOTSTRAP_ADMIN_PASSWORD")

    first = call("/api/auth/login", {"username": user, "password": pwd})

    if first.get("requiresTotpSetup"):
        secret = first["secret"]
        code, _ = totp(secret)
        done = call("/api/auth/login/totp-setup",
                    {"pendingToken": first["pendingToken"], "code": code})
        store["ADMIN_USERNAME"], store["ADMIN_PASSWORD"] = user, pwd
        store["ADMIN_TOTP_SECRET"] = secret
        print("  管理员 TOTP 已绑定，密钥存入 .demo-secrets.env")
        return done["accessToken"]

    if first.get("requiresTotp"):
        secret = store.get("ADMIN_TOTP_SECRET")
        if not secret:
            sys.exit("管理员已绑定 TOTP，但 .demo-secrets.env 里没有密钥；"
                     "请在控制台里重置该账号 2FA 后重跑。")
        # 撞上 step <= last 就等下一个窗口。
        for _ in range(3):
            code, step = totp(secret)
            try:
                return call("/api/auth/login/totp",
                            {"pendingToken": first["pendingToken"], "code": code})["accessToken"]
            except urllib.error.HTTPError as e:
                if "totp" not in e.read().decode("utf-8", "replace").lower():
                    raise
                time.sleep(max(1.0, (step + 1) * 30 - time.time() + 1.0))
        sys.exit("管理员 TOTP 登录连续失败")

    return first["accessToken"]


def as_list(resp):
    """列表接口有的直接返回数组，有的包一层，统一成 list。"""
    if isinstance(resp, list):
        return resp
    if isinstance(resp, dict):
        for k in ("items", "content", "data", "list"):
            if isinstance(resp.get(k), list):
                return resp[k]
    return []


def cleanup(token, keep_asset_ids, admin_user):
    """清掉访客留下的用户 / 分组 / 非演示资产。

    走接口而不是裸 SQL：assets 被 user_scopes、告警表等外键引用，
    AssetService.delete 会按顺序先清这些，手写 DELETE 会撞约束。
    """
    removed = {"assets": 0, "users": 0, "groups": 0}

    for a in as_list(call("/api/assets", token=token)):
        aid = a.get("id")
        if aid and aid not in keep_asset_ids:
            try:
                call("/api/assets/%s" % aid, token=token, method="DELETE")
                removed["assets"] += 1
            except Exception:
                pass

    for u in as_list(call("/api/users", token=token)):
        if u.get("username") not in (admin_user, DEMO_USER):
            try:
                call("/api/users/%s" % u["id"], token=token, method="DELETE")
                removed["users"] += 1
            except Exception:
                pass

    # 分组只删「已经没有资产占用」的：演示资产落在 seed 阶段自动建的分组里
    # （名字不是 DEMO_GROUP），按名字删会把它们连带拖走。
    rows = psql("select distinct group_id from assets where group_id is not null")
    in_use = set(r.strip() for r in rows.splitlines() if r.strip())
    for gid in collect_group_ids(as_list(call("/api/groups", token=token))):
        if gid in in_use:
            continue
        try:
            call("/api/groups/%s" % gid, token=token, method="DELETE")
            removed["groups"] += 1
        except Exception:
            pass  # 有子分组时删不掉，下一轮再说

    print("  清理：资产 %d、用户 %d、分组 %d" % (removed["assets"], removed["users"], removed["groups"]))


def ensure_group(token):
    for g in call("/api/groups", token=token) or []:
        if g.get("name") == DEMO_GROUP:
            return g["id"]
    return call("/api/groups", {"name": DEMO_GROUP, "parentId": None}, token=token)["id"]


def ensure_demo_user(token, store):
    """建演示账号。角色给 ADMIN（不是 SUPER_ADMIN），TOTP 密钥用我们自己生成的。"""
    pwd = store.get("DEMO_PASSWORD") or secrets.token_urlsafe(18)
    secret = store.get("DEMO_TOTP_SECRET") or gen_base32()

    exists = psql("select count(1) from users where username='%s' and deleted_at is null" % DEMO_USER)
    if exists == "0":
        call("/api/users", {
            "username": DEMO_USER,
            "nickname": "Demo User",
            "password": pwd,
            "role": "ADMIN",
        }, token=token)
        print("  演示账号 %s 已创建（ADMIN）" % DEMO_USER)
    else:
        # 重跑时把密码改回我们记录的那个，免得上一轮访客改过。
        uid = psql("select id from users where username='%s' and deleted_at is null" % DEMO_USER)
        call("/api/users/%s" % uid, {"password": pwd, "role": "ADMIN", "enabled": True},
             token=token, method="PATCH")
        print("  演示账号 %s 已复位" % DEMO_USER)

    # 直接写 TOTP 列：让 broker 能用已知密钥登录，且不必走绑定流程。
    # totp_last_step 清空，避免上一轮消耗过的时间步把 broker 卡住。
    psql("update users set totp_secret='%s', totp_enabled=true, totp_last_step=null, "
         "role='ADMIN', enabled=true where username='%s' and deleted_at is null"
         % (secret, DEMO_USER))

    store["DEMO_USERNAME"], store["DEMO_PASSWORD"], store["DEMO_TOTP_SECRET"] = DEMO_USER, pwd, secret
    return psql("select id from users where username='%s' and deleted_at is null" % DEMO_USER)


def collect_group_ids(nodes, out=None):
    """分组接口可能返回树，递归把 id 收齐。"""
    out = [] if out is None else out
    for n in nodes:
        if n.get("id"):
            out.append(n["id"])
        kids = n.get("children") or n.get("items") or []
        if isinstance(kids, list):
            collect_group_ids(kids, out)
    return out


def ensure_scopes(token, demo_user_id):
    """ADMIN 只看得见授权范围内的资产，范围为空则资产列表是空的。
    演示环境里把所有分组都授权给演示账号。"""
    gids = collect_group_ids(as_list(call("/api/groups", token=token)))
    call("/api/users/%s/scopes" % demo_user_id,
         {"scopes": [{"type": "GROUP", "id": g} for g in gids]},
         token=token, method="PUT")
    print("  已授权 %d 个分组给 %s" % (len(gids), DEMO_USER))


def new_install_code(token, group_id):
    resp = call("/api/install-codes", {"groupId": group_id}, token=token)
    for k in ("code", "installCode", "value"):
        if resp.get(k):
            return resp[k]
    raise RuntimeError("install-codes 返回里找不到安装码：%s" % resp)


def main():
    store = load_env_file(SECRETS_FILE)
    print("等待 control-api …")
    wait_api()
    print("管理员登录 …")
    token = admin_login(store)

    # 重置时由 reset.sh 传入「身份卷里仍然有效的资产」，其余都是访客留下的。
    keep = set(filter(None, (os.environ.get("DEMO_KEEP_ASSET_IDS") or "").split(",")))
    if os.environ.get("DEMO_CLEANUP") == "1":
        print("清理访客残留 …")
        root_env = load_env_file(os.path.join(OPS_DIR, ".env"))
        cleanup(token, keep, root_env.get("OPS_BOOTSTRAP_ADMIN_USERNAME", "admin"))

    print("准备分组 …")
    group_id = ensure_group(token)
    print("准备演示账号 …")
    demo_uid = ensure_demo_user(token, store)
    print("授权可见范围 …")
    ensure_scopes(token, demo_uid)
    print("签发 agent 安装码 …")
    code = new_install_code(token, group_id)

    write_env_file(SECRETS_FILE, store)
    write_env_file(DEMO_ENV_FILE, {
        "DEMO_USERNAME": store["DEMO_USERNAME"],
        "DEMO_PASSWORD": store["DEMO_PASSWORD"],
        "DEMO_TOTP_SECRET": store["DEMO_TOTP_SECRET"],
        # Agent 的 gateway 地址由 compose 从 .env 的 OPS_GATEWAY_PUBLIC_HTTP 取，
        # 这里只放重置流程要用的安装码，不重复一份容易写歪的地址。
        "WOOPS_INSTALL_CODE": code,
        "DEMO_GROUP_ID": group_id,
    })
    print("\n完成。demo.env 与 .demo-secrets.env 已写入 %s" % HERE)
    print("演示入口：%s/demo/" % BASE)


if __name__ == "__main__":
    main()
