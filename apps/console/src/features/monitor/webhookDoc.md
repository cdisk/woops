# 告警 Webhook 接口说明

Woops 在监控异常 **出现**（`firing`）或 **恢复**（`resolved`）时，向超管配置的 Webhook URL 发送一次 `POST`。企业微信、钉钉等适配应由你的桥接程序完成：本接口只推送通用 JSON，并带上按资产权限解析出的负责人列表。

## 请求约定

| 项 | 说明 |
|----|------|
| 方法 | `POST` |
| `Content-Type` | `application/json` |
| 鉴权 | 若配置了 Bearer Token：`Authorization: Bearer <token>` |
| 超时 | 约 10 秒；非 2xx 会进入 outbox 重试 |
| URL | 建议 HTTPS；拒绝 loopback / link-local 等地址 |

控制台「测试」按钮会立刻 POST 一条与 `firing` 同形、字段齐全的 `event: "test"` 样例，便于联调桥接程序。

## Payload 字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `event` | string | `firing` / `resolved` / `test` |
| `assetId` | string | 资产 UUID（`test` 时可能为空） |
| `assetName` | string | 显示名 |
| `groupName` | string | 所属分组名，未分组为空串 |
| `privateIp` | string | 内网 IP（与资产一致，多地址时逗号分隔；未知为空串） |
| `itemId` | string | 监控项，如 `cpu.usage_percent`；离线为 `host.online` |
| `instance` | string | 实例（磁盘挂载点、网卡名等）；无则空串 |
| `kind` | string | `metric` 或 `offline` |
| `label` | string | 可读摘要，如 `CPU high=95` |
| `time` | string | ISO-8601 UTC |
| `incidentId` | string | 可选；对应 `alert_incidents` 记录 ID |
| `recipients` | array | 负责人列表（见下） |

### recipients[]

按显式 `user_scopes`（GROUP / ASSET）解析；**SUPER_ADMIN 的隐式全量可见不会自动进入列表**。

| 字段 | 说明 |
|------|------|
| `userId` | Woops 用户 UUID |
| `username` | 登录名（GitLab 用户通常与此一致，便于映射企微 userid） |
| `nickname` | 昵称，可能为空 |
| `authSource` | `local` / `gitlab` 等 |

`recipients` 为空时表示该资产没有显式分配负责人；桥接程序宜发给默认运维接收人，避免静默丢警。

## 示例：firing

```http
POST /hooks/woops-alerts HTTP/1.1
Host: bridge.example.com
Authorization: Bearer your-token
Content-Type: application/json
```

```json
{
  "event": "firing",
  "assetId": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
  "assetName": "web-01",
  "groupName": "prod",
  "privateIp": "10.0.0.5",
  "itemId": "disk.used_percent",
  "instance": "/",
  "kind": "metric",
  "label": "Disk high (/)=99",
  "time": "2026-10-12T10:00:00Z",
  "incidentId": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
  "recipients": [
    {
      "userId": "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d",
      "username": "zhangsan",
      "nickname": "张三",
      "authSource": "gitlab"
    }
  ]
}
```

## 示例：resolved

字段同 `firing`，仅 `event` 为 `"resolved"`（例如指标回落、磁盘实例消失、资产离线导致指标告警关闭等）。

## 示例：test

控制台「测试」会推送与 `firing` **同形、字段齐全**的样例（`event` 为 `"test"`）；资产/告警字段为固定示例，`recipients` 含当前操作者（便于桥接映射真实用户名），例如：

```json
{
  "event": "test",
  "assetId": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
  "assetName": "web-01",
  "groupName": "prod",
  "privateIp": "10.0.0.5",
  "itemId": "disk.used_percent",
  "instance": "/",
  "kind": "metric",
  "label": "Disk high (/)=99",
  "time": "2026-10-12T10:00:00Z",
  "incidentId": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
  "recipients": [
    {
      "userId": "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d",
      "username": "admin",
      "nickname": "",
      "authSource": "local"
    }
  ]
}
```

## 行为说明

- **忽略**：全局忽略某资产+监控项后，该 issue 仍记在 incident 中，但 **不会** 再投递 firing/resolved；取消忽略且仍异常时会补发一次 firing。
- **范围**：每个 Webhook 可设 `全部资产` / `分组（含子组）` / `单台资产`。
- **去重与重试**：同一 incident 的同一次事件按端点去重；失败指数退避，多次后标记失败结束。
- **企业微信**：请用桥接程序维护 `username → 企微 userid`，并调用企微自建应用消息 API；群机器人 Webhook 通常无法按人私发。
