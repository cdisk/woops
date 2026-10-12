# Alert webhook reference

Woops `POST`s JSON to each admin-configured webhook URL when a monitor issue **starts** (`firing`) or **clears** (`resolved`). Adapters for Telegram, Slack, etc. belong in your bridge: this endpoint only delivers a generic payload plus owners resolved from asset scopes.

## Request

| Item | Detail |
|------|--------|
| Method | `POST` |
| `Content-Type` | `application/json` |
| Auth | If a bearer token is set: `Authorization: Bearer <token>` |
| Timeout | ~10s; non-2xx responses are retried via the outbox |
| URL | Prefer HTTPS; loopback / link-local targets are rejected |

The console **Test** button immediately POSTs a sample with `event: "test"` for bridge bring-up.

## Payload fields

| Field | Type | Meaning |
|-------|------|---------|
| `event` | string | `firing` / `resolved` / `test` |
| `assetId` | string | Asset UUID (may be empty for `test`) |
| `assetName` | string | Display name |
| `groupName` | string | Group name, or empty if ungrouped |
| `itemId` | string | Metric id, e.g. `cpu.usage_percent`; offline uses `host.online` |
| `instance` | string | Instance (mount, NIC, …); empty if none |
| `kind` | string | `metric` or `offline` |
| `label` | string | Human summary, e.g. `CPU high=95` |
| `time` | string | ISO-8601 UTC |
| `incidentId` | string | Optional; `alert_incidents` row id |
| `recipients` | array | Owners (see below) |

### recipients[]

Resolved from explicit `user_scopes` (GROUP / ASSET). **SUPER_ADMIN implicit full access does not add them to every alert.**

| Field | Meaning |
|-------|---------|
| `userId` | Woops user UUID |
| `username` | Login name (often matches GitLab; good key for Telegram/Slack user maps) |
| `nickname` | May be empty |
| `authSource` | `local` / `gitlab` / … |

An empty `recipients` array means no explicit owner; the bridge should fall back to a default on-call recipient so alerts are not dropped silently.

## Example: firing

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
      "nickname": "Zhang San",
      "authSource": "gitlab"
    }
  ]
}
```

## Example: resolved

Same fields as `firing`, with `event` set to `"resolved"` (metric recovered, disk/NIC instance gone, metric alerts closed because the asset went offline, etc.).

## Example: test

```json
{
  "event": "test",
  "assetId": "",
  "assetName": "test",
  "groupName": "",
  "itemId": "test",
  "instance": "",
  "kind": "metric",
  "label": "webhook test",
  "time": "2026-10-12T10:00:00Z",
  "recipients": []
}
```

## Behaviour notes

- **Ignore**: a global ignore on asset+item keeps the incident open but **does not** deliver firing/resolved; unignore while still abnormal re-sends firing once.
- **Scope**: each webhook can target all assets, a group (with descendants), or a single asset.
- **Dedup / retry**: one delivery per endpoint per incident event; failures use exponential backoff then stop.
- **Telegram / Slack**: keep a `username → chat id` (or Slack user id) map in the bridge and send DMs via the Bot API; channel/group webhooks usually cannot target individuals.
