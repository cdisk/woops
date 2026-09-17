# User API tokens and read-only endpoints

This reference is meant for scripts, automation and other AI assistants. The **user API token** created in your profile (`wpat_<id>_<secret>`) is separate from the asset-level deploy token (`ops_…` / woopsctl).

{{API_REQUEST_PREFIX}}

## Authentication

Every endpoint below uses:

```http
Authorization: Bearer wpat_<tokenId>_<secret>
```

- **Never** put the token in a URL or a log.
- A token belongs to a user. Every request re-checks that the user is still enabled and filters assets by that user's GROUP/ASSET visibility, exactly as the console does.
- Management is create and delete only. Deleting the token, disabling the user or soft-deleting the user takes effect immediately.
- Expiry is optional; **leave it empty for a token that never expires**. The secret is shown once, at creation.

## Scopes

| Scope | Grants |
|-------|--------|
| `assets:read` | List visible assets and read a single asset (plus the group list, so you can filter by group) |
| `metrics:read` | Monitor item definitions, latest values, series, dashboard summary, metrics report |

Not granted: modifying assets, sessions, files, exec, writing alert rules, or managing tokens themselves.

When you need to "discover assets first, then pull metrics", select **`assets:read` + `metrics:read`** together: list assets to get an `assetId`, then call the report endpoint.

## Time conventions

- Ranges are half-open **`[from, to)`**, ISO-8601, stored and transferred in **UTC**.

## 1. List assets (needs `assets:read`)

```http
GET /api/assets
GET /api/assets?groupId=<uuid>&includeSubtree=true
GET /api/assets/{assetId}
GET /api/groups
```

Example:

```bash
TOKEN='wpat_…'
BASE='https://ops.example.com'

curl -fsS -H "Authorization: Bearer ${TOKEN}" "${BASE}/api/assets" | jq .
# Grab one asset id:
ASSET_ID=$(curl -fsS -H "Authorization: Bearer ${TOKEN}" "${BASE}/api/assets" \
  | jq -r '.[0].id')
```

A list item typically carries `id`, `displayName`, `hostname`, `online`, `groupId` / `groupName`, private and public IPs, and so on — the actual JSON is authoritative. You only ever see assets the current user may access.

## 2. Monitor metadata and series (needs `metrics:read`)

```http
GET /api/monitor/items
GET /api/assets/{assetId}/metrics/latest
GET /api/assets/{assetId}/metrics/series?keys=cpu.usage_percent,mem.used_percent&from=…&to=…&grain=hour
GET /api/dashboard/summary
```

- `grain`: `minute` | `hour` | `day` | `month`. When the trends source has no minute data the server falls back to `hour`.
- A `series` response carries `series` (the trend points) and **`summaries`** (`min`/`avg`/`max`/`sampleCount` over the whole range). Statistics count only real samples — gaps from a disconnected agent are not padded with zeros — and **do not change with the display grain**.
- Disks are split per mount point and NICs per interface (`itemId|instance`); instances are never averaged together.

## 3. Metrics report (needs `metrics:read`)

Returns range statistics plus trend `points` in one call, so you can plot them yourself with any charting or reporting tool.

```http
POST /api/reports/metrics
Content-Type: application/json
```

Request body:

```json
{
  "assetId": "<uuid>",
  "keys": ["cpu.usage_percent", "mem.used_percent", "disk.used_percent"],
  "from": "2026-09-01T00:00:00Z",
  "to": "2026-09-08T00:00:00Z",
  "grain": "hour"
}
```

Fields:

| Field | Notes |
|-------|-------|
| `assetId` | Required; must be visible to the user |
| `keys` | Monitor item ids (see `/api/monitor/items`); the count is capped |
| `from` / `to` | `[from,to)` |
| `grain` | `hour` or `day` recommended |
| `instance` | Optional; when set, only that instance is returned |

Example:

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

# Inspect the points of one series
jq '.results[] | select(.itemId=="cpu.usage_percent" and (.instance=="" or .instance==null)) | .points' \
  report.json
```

Each `results[]` entry: `itemId`, `instance`, `name`, `unit`, `status` (`ok` / `no_data`), `min`/`avg`/`max`/`sampleCount`, `points`.

Each `points[]` entry: `time` (ISO-8601 UTC), `value` (bucket average), `min`, `max`.

## 4. How this differs from a deploy token

| | User API token | Deploy token |
|--|--|--|
| Prefix | `wpat_` | `ops_` |
| Belongs to | A user | A single asset |
| Used for | Read-only asset list / read-only monitoring / metrics report | woopsctl upload/download/exec/portmap |
| Managed in | Your profile | Asset detail |

## 5. Shortest possible prompt for an AI

1. Call `GET /api/assets` with Bearer `wpat_…` and pick an `id` (needs scope `assets:read`).
2. Call `POST /api/reports/metrics` with `assetId`, `keys`, `from`, `to` in the body (needs `metrics:read`).
3. Use UTC `[from,to)`; plot `results[].points` (`time`/`value`/`min`/`max`). Do not rely on the removed `svg` field.
