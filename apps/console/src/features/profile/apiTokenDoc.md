# 用户 API Token 与只读接口

本文档面向脚本、自动化与其他 AI 助手调用。个人中心创建的 **用户 API Token**（`wpat_<id>_<secret>`）与资产级 Deploy Token（`ops_…` / woopsctl）隔离。

{{API_REQUEST_PREFIX}}

## 鉴权

所有下述接口使用：

```http
Authorization: Bearer wpat_<tokenId>_<secret>
```

- **不要**把 Token 放进 URL 或日志。
- Token 归属用户；每次请求重新校验用户是否启用，并按该用户的 GROUP/ASSET 可见范围过滤资产（与控制台一致）。
- 管理：仅「新建」「删除」；删除 / 用户禁用 / 软删后立即失效。
- 过期时间可选；**留空 = 永久有效**。明文只在创建时展示一次。

## Scopes

| Scope | 能力 |
|-------|------|
| `assets:read` | 读取可见资产列表与单个资产详情（及分组列表，便于按组筛选） |
| `metrics:read` | 监控项定义、最新值、曲线、首页摘要、指标统计报表 |

不授予：资产修改、会话、文件、exec、预警规则写操作、Token 自身管理等。

需要「先发现资产再拉指标」时，通常同时勾选 **`assets:read` + `metrics:read`**：先列资产拿到 `assetId`，再调统计报表接口。

## 时间约定

- 区间为半开 **`[from, to)`**，ISO-8601，**UTC** 存储与传输。

## 1. 列出资产（需 `assets:read`）

```http
GET /api/assets
GET /api/assets?groupId=<uuid>&includeSubtree=true
GET /api/assets/{assetId}
GET /api/groups
```

示例：

```bash
TOKEN='wpat_…'
BASE='https://ops.example.com'

curl -fsS -H "Authorization: Bearer ${TOKEN}" "${BASE}/api/assets" | jq .
# 取某台 id：
ASSET_ID=$(curl -fsS -H "Authorization: Bearer ${TOKEN}" "${BASE}/api/assets" \
  | jq -r '.[0].id')
```

列表项通常含：`id`、`displayName`、`hostname`、`online`、`groupId` / `groupName`、内网/公网 IP 等（以实际 JSON 为准）。只能看到当前用户有权限的资产。

## 2. 监控元数据与曲线（需 `metrics:read`）

```http
GET /api/monitor/items
GET /api/assets/{assetId}/metrics/latest
GET /api/assets/{assetId}/metrics/series?keys=cpu.usage_percent,mem.used_percent&from=…&to=…&grain=hour
GET /api/dashboard/summary
```

- `grain`：`minute` | `hour` | `day` | `month`（trends 源无 minute 时服务端会改为 hour）。
- `series` 响应含 `series`（趋势点）与 **`summaries`**（全区间 `min`/`avg`/`max`/`sampleCount`）。统计按有效采样计分母，断线缺口不补 0；**不随显示粒度变化**。
- 磁盘按挂载点、网卡按网卡拆分 key（`itemId|instance`），不跨 instance 混算。

## 3. 指标统计报表（需 `metrics:read`）

一次返回区间统计与趋势 `points`，由调用方自行绘图（图表库 / 报表工具均可）。

```http
POST /api/reports/metrics
Content-Type: application/json
```

请求体：

```json
{
  "assetId": "<uuid>",
  "keys": ["cpu.usage_percent", "mem.used_percent", "disk.used_percent"],
  "from": "2026-09-01T00:00:00Z",
  "to": "2026-09-08T00:00:00Z",
  "grain": "hour"
}
```

字段说明：

| 字段 | 说明 |
|------|------|
| `assetId` | 必填；须对该用户可见 |
| `keys` | 监控项 id 列表（见 `/api/monitor/items`），有数量上限 |
| `from` / `to` | `[from,to)` |
| `grain` | 建议 `hour` 或 `day` |
| `instance` | 可选；非空时只返回该 instance |

示例：

```bash
TOKEN='wpat_…'
BASE='https://ops.example.com'
ASSET_ID='…'

curl -fsS -X POST "${BASE}/api/reports/metrics" \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{
    \"assetId\": \"${ASSET_ID}\",
    \"keys\": [\"cpu.usage_percent\", \"mem.used_percent\", \"disk.used_percent\"],
    \"from\": \"2026-09-01T00:00:00Z\",
    \"to\": \"2026-09-08T00:00:00Z\",
    \"grain\": \"hour\"
  }" -o report.json

# 查看某条曲线的 points
jq '.results[] | select(.itemId=="cpu.usage_percent" and (.instance=="" or .instance==null)) | .points' \
  report.json
```

每个 `results[]`：`itemId`、`instance`、`name`、`unit`、`status`（`ok` / `no_data`）、`min`/`avg`/`max`/`sampleCount`、`points`。

`points[]` 每项：`time`（ISO-8601 UTC）、`value`（桶均值）、`min`、`max`。

## 4. 与 Deploy Token 的区别

| | 用户 API Token | Deploy Token |
|--|--|--|
| 前缀 | `wpat_` | `ops_` |
| 归属 | 用户 | 单资产 |
| 用途 | 资产只读列表 / 监控只读 / 指标统计报表 | woopsctl upload/download/exec/portmap |
| 管理入口 | 个人中心 | 资产详情 |

## 5. 给 AI 的最短提示

1. 用 Bearer `wpat_…` 调 `GET /api/assets` 选 `id`（需 scope `assets:read`）。
2. 调 `POST /api/reports/metrics`，body 含 `assetId`、`keys`、`from`、`to`（需 `metrics:read`）。
3. 时间用 UTC 的 `[from,to)`；画图用 `results[].points`（`time`/`value`/`min`/`max`），不要依赖已移除的 `svg` 字段。
