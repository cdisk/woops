# -*- coding: utf-8 -*-
"""演示环境登录代理（只用标准库，无第三方依赖）。

为什么需要它：本地账号强制绑定 TOTP，且 AuthService.consumeTotpCode 按
`step <= last` 拒绝任何不比上次更新的时间步。公开演示如果让访客自己填验证码，
同一个 30 秒窗口里只有第一个人能登进去，后面的人会收到 "invalid totp code"。

做法：本服务用演示账号完成一次两步登录，把 12 小时有效的 JWT 缓存下来，
之后所有访客共用同一个 token。整个有效期内只消耗一次 TOTP，并发不再冲突。
"""
import base64
import hashlib
import hmac
import json
import os
import struct
import threading
import time
import urllib.error
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

CONTROL_API = os.environ.get("CONTROL_API", "http://control-api:9100").rstrip("/")
# 首次 up 时 setup-demo.py 还没生成 demo.env，这里不能直接抛，否则容器反复重启。
# 真正需要凭据是在处理请求的时候，那时校验。
USERNAME = os.environ.get("DEMO_USERNAME", "")
PASSWORD = os.environ.get("DEMO_PASSWORD", "")
TOTP_SECRET = os.environ.get("DEMO_TOTP_SECRET", "")
RESET_CRON = os.environ.get("DEMO_RESET_HINT", "每小时整点 / hourly")
LISTEN_PORT = int(os.environ.get("PORT", "8088"))

# JWT 有效期是 12h；提前一小时换新的，避免访客拿到一个马上过期的 token。
TOKEN_TTL = 11 * 3600
TOTP_PERIOD = 30


def totp_now(secret, at=None):
    """RFC 6238，SHA1 / 6 位 / 30 秒，与 control-api 的 TotpService 一致。"""
    key = base64.b32decode(secret.strip().replace(" ", "").upper())
    step = int((at if at is not None else time.time()) // TOTP_PERIOD)
    digest = hmac.new(key, struct.pack(">Q", step), hashlib.sha1).digest()
    offset = digest[-1] & 0x0F
    code = struct.unpack(">I", digest[offset:offset + 4])[0] & 0x7FFFFFFF
    return "%06d" % (code % 1000000), step


def api_post(path, payload, token=None):
    data = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(CONTROL_API + path, data=data, method="POST")
    req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", "Bearer " + token)
    with urllib.request.urlopen(req, timeout=15) as resp:
        return json.loads(resp.read().decode("utf-8"))


def api_get(path, token=None):
    req = urllib.request.Request(CONTROL_API + path, method="GET")
    if token:
        req.add_header("Authorization", "Bearer " + token)
    with urllib.request.urlopen(req, timeout=15) as resp:
        return json.loads(resp.read().decode("utf-8"))


class TokenCache(object):
    def __init__(self):
        self._lock = threading.Lock()
        self._session = None
        self._fetched_at = 0.0

    def get(self, force=False):
        with self._lock:
            fresh = self._session and (time.time() - self._fetched_at) < TOKEN_TTL
            if fresh and not force:
                return self._session
            self._session = self._login()
            self._fetched_at = time.time()
            return self._session

    def _login(self):
        if not (USERNAME and PASSWORD and TOTP_SECRET):
            raise RuntimeError("演示账号未配置：先在主机上跑 deploy/demo/setup-demo.py "
                               "生成 demo.env，再重启 demo-broker。")
        first = api_post("/api/auth/login", {"username": USERNAME, "password": PASSWORD})

        if first.get("requiresTotpSetup"):
            raise RuntimeError(
                "演示账号还没绑定 TOTP。请先用 seed-demo-user.sql 写入已知密钥，"
                "或手工绑定后把密钥填进 DEMO_TOTP_SECRET。")
        if not first.get("requiresTotp"):
            # 理论上不会走到：本地账号强制 2FA。
            return self._session_from(first)

        pending = first["pendingToken"]

        # 撞上 step <= last 时等到下一个时间步再试。
        last_err = None
        for _ in range(3):
            code, step = totp_now(TOTP_SECRET)
            try:
                return self._session_from(api_post(
                    "/api/auth/login/totp", {"pendingToken": pending, "code": code}))
            except urllib.error.HTTPError as e:
                last_err = e
                body = e.read().decode("utf-8", "replace")
                if "totp" not in body.lower():
                    raise
                # 睡到下一个 30 秒窗口的开头，多给 1 秒余量。
                time.sleep(max(1.0, (step + 1) * TOTP_PERIOD - time.time() + 1.0))
        raise RuntimeError("TOTP 登录连续失败：%s" % last_err)

    def _session_from(self, login_resp):
        token = login_resp["accessToken"]
        user = login_resp.get("user") or {}
        capabilities = None
        try:
            me = api_get("/api/auth/me", token=token)
            capabilities = me.get("capabilities")
        except Exception:
            pass  # 控制台自己也会补，拿不到不致命
        return {
            "token": token,
            "username": user.get("username", USERNAME),
            "nickname": user.get("nickname", ""),
            "role": user.get("role", ""),
            "capabilities": capabilities,
        }


CACHE = TokenCache()

PAGE = u"""<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Woops \u00b7 Live Demo</title>
<style>
  :root { color-scheme: light dark; }
  body { margin:0; min-height:100vh; display:flex; align-items:center; justify-content:center;
         font-family: "IBM Plex Sans", -apple-system, "Segoe UI", "Noto Sans SC", sans-serif;
         background:#0f172a; color:#e2e8f0; }
  .card { max-width:620px; padding:44px 48px; background:#1e293b; border-radius:14px;
          box-shadow:0 20px 60px rgba(0,0,0,.45); }
  h1 { margin:0 0 6px; font-size:26px; color:#fff; }
  .sub { margin:0 0 26px; color:#94a3b8; font-size:14px; }
  ul { margin:0 0 28px; padding-left:20px; color:#cbd5e1; font-size:14px; line-height:1.9; }
  code { background:#0f172a; padding:1px 6px; border-radius:4px; font-size:13px; }
  button { width:100%; padding:14px; font-size:16px; font-weight:600; color:#fff; cursor:pointer;
           background:#2F5D9F; border:0; border-radius:8px; }
  button:hover { background:#3a6fba; }
  button:disabled { opacity:.6; cursor:default; }
  .err { margin-top:16px; color:#fca5a5; font-size:13px; white-space:pre-wrap; }
  .foot { margin-top:26px; font-size:12px; color:#64748b; }
  a { color:#7dd3fc; }
</style>
</head>
<body>
<div class="card">
  <h1>Woops Live Demo</h1>
  <p class="sub">\u51fa\u7ad9 Agent \u7edf\u4e00\u8fd0\u7ef4\u901a\u9053 \u00b7 Outbound-only remote ops</p>
  <ul>
    <li>\u70b9\u4e0b\u9762\u7684\u6309\u94ae\u76f4\u63a5\u8fdb\u5165\uff0c\u4e0d\u7528\u8d26\u53f7\u5bc6\u7801\u3002<br>
        Click below to enter \u2014 no signup, no credentials.</li>
    <li>\u6f14\u793a\u8d44\u4ea7\u90fd\u662f\u4e00\u6b21\u6027\u5bb9\u5668\uff0c\u73af\u5883 <b>__RESET__</b> \u81ea\u52a8\u91cd\u7f6e\u3002<br>
        All hosts are throwaway containers; the environment resets <b>__RESET__</b>.</li>
    <li>\u6240\u6709\u4eba\u5171\u7528\u540c\u4e00\u4e2a\u6f14\u793a\u8d26\u53f7\uff0c<b>\u8bf7\u52ff\u4e0a\u4f20\u771f\u5b9e\u6570\u636e\u6216\u5bc6\u94a5</b>\u3002<br>
        The demo account is shared \u2014 <b>do not upload real data or secrets</b>.</li>
  </ul>
  <button id="go">\u8fdb\u5165\u6f14\u793a \u00b7 Enter demo</button>
  <div class="err" id="err"></div>
  <p class="foot">
    \u6e90\u7801 / Source: <a href="https://github.com/cdisk/woops">github.com/cdisk/woops</a>
  </p>
</div>
<script>
document.getElementById('go').addEventListener('click', async function () {
  var btn = this, err = document.getElementById('err');
  btn.disabled = true; err.textContent = ''; btn.textContent = '\u767b\u5f55\u4e2d\u2026 Signing in\u2026';
  try {
    var r = await fetch('/demo/token', { cache: 'no-store' });
    if (!r.ok) throw new Error('HTTP ' + r.status + ' ' + (await r.text()));
    var d = await r.json();
    localStorage.setItem('token', d.token);
    localStorage.setItem('username', d.username || '');
    localStorage.setItem('nickname', d.nickname || '');
    localStorage.setItem('role', d.role || '');
    if (d.capabilities) localStorage.setItem('capabilities', JSON.stringify(d.capabilities));
    location.href = '/';
  } catch (e) {
    err.textContent = '\u767b\u5f55\u5931\u8d25 / Sign-in failed:\\n' + e.message;
    btn.disabled = false; btn.textContent = '\u91cd\u8bd5 \u00b7 Retry';
  }
});
</script>
</body>
</html>
"""


class Handler(BaseHTTPRequestHandler):
    server_version = "woops-demo-broker"

    def _send(self, code, body, ctype="application/json; charset=utf-8"):
        raw = body.encode("utf-8") if isinstance(body, str) else body
        self.send_response(code)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(raw)))
        self.send_header("Cache-Control", "no-store")
        self.end_headers()
        self.wfile.write(raw)

    def do_GET(self):
        path = self.path.split("?", 1)[0].rstrip("/") or "/"
        if path in ("/", "/demo"):
            self._send(200, PAGE.replace("__RESET__", RESET_CRON), "text/html; charset=utf-8")
        elif path in ("/token", "/demo/token"):
            try:
                self._send(200, json.dumps(CACHE.get()))
            except Exception as e:
                # token 可能是被重置流程作废的，强制重登一次再报错。
                try:
                    self._send(200, json.dumps(CACHE.get(force=True)))
                except Exception as e2:
                    self._send(503, json.dumps({"error": str(e2) or str(e)}))
        elif path in ("/healthz", "/demo/healthz"):
            self._send(200, json.dumps({"status": "ok"}))
        else:
            self._send(404, json.dumps({"error": "not found"}))

    def log_message(self, fmt, *args):
        print("%s - %s" % (self.address_string(), fmt % args), flush=True)


if __name__ == "__main__":
    print("demo broker on :%d -> %s (user=%s)" % (LISTEN_PORT, CONTROL_API, USERNAME or "<未配置>"),
          flush=True)
    ThreadingHTTPServer(("0.0.0.0", LISTEN_PORT), Handler).serve_forever()
