# Woops

**Remote ops for machines you can't reach into.** Agents dial out to the gateway — you never need an inbound path into the target network.

Shell, file transfer, RDP/VNC, port forwarding, lightweight monitoring, session recording and CI (`woopsctl`) all ride the *same* outbound agent channel.

[![Live demo](https://img.shields.io/badge/live%20demo-try%20it%20now-2F5D9F.svg)](https://woops-demo.tool4dev.net/demo/)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](../LICENSE)
[![Docker](https://img.shields.io/badge/docker-cdisk%2Fwoops-2496ed.svg)](https://hub.docker.com/r/cdisk/woops-console)

[中文文档](../README.md) · [Feature list & architecture](../FEATURES.md)

---

## ▶ Try the live demo

**[woops-demo.tool4dev.net](https://woops-demo.tool4dev.net/demo/)** — one click, no signup, no credentials.

You land in the console as an admin with four throwaway Linux hosts already online: open a shell, browse and edit files, forward a port, watch the metrics, replay your own session from the audit log. Everything resets on the hour, so feel free to break things — but the account is shared, so don't upload anything real.

---

## Why another bastion host?

Most bastion hosts assume the control plane can open a connection *to* the asset. The moment a firewall, a data diode, or three layers of NAT sit in between, you end up gluing on FRP tunnels, jump boxes, or third-party remote-desktop tools that leave no audit trail.

Woops inverts the direction. Every asset runs an agent that **only makes outbound HTTPS/WSS connections** to the gateway. If a machine can reach the gateway — directly or through an upstream hop — it can be managed. Nothing has to reach back in.

|  | Traditional bastion (e.g. JumpServer) | Woops |
|--|--------------------------------------|-------|
| Network assumption | Control plane dials the asset; needs FRP/jump host otherwise | **Asset agent dials out** to the gateway |
| Scope | Sessions and access control | Sessions + light monitoring + **CI over the same channel** |
| Target requirements | Usually needs `sshd` or similar | Agent implements shell and file transfer natively |

Teleport also uses an outbound tunnel model, but targets large zero-trust deployments. Woops aims at the other end: one compose file, a handful of binaries, and you can actually get work done.

## Screenshots

| Dashboard · unhealthy assets | Asset list · group tree |
|:---:|:---:|
| ![Dashboard](../docs/screenshots/home.png) | ![Assets](../docs/screenshots/assets.png) |

| Shell (agent-native, no `sshd`) | File manager |
|:---:|:---:|
| ![Shell](../docs/screenshots/shell.png) | ![Files](../docs/screenshots/files.png) |

| Metrics | Audit log · session replay |
|:---:|:---:|
| ![Monitoring](../docs/screenshots/monitor.png) | ![Audit](../docs/screenshots/audit.png) |

## What you get

- **Agent-native shell** — real PTY on Linux, ConPTY or WinPTY on Windows. No `sshd`, no OpenSSH for Windows, nothing to install on the target beyond the agent.
- **File manager and transfer** — resumable binary transfers with per-chunk SHA-256, in-browser text editing with encoding detection.
- **RDP and VNC in the browser** — the agent tunnels TCP to the local desktop service; Guacamole renders it. Clipboard passthrough included.
- **Port forwarding, both directions** — TCP and UDP, persistent mappings or ad-hoc tunnels from the CLI.
- **Monitoring** — CPU, memory, disk and network on a separate optional channel, stored in TimescaleDB with hourly rollups and threshold alerts.
- **Full session recording** — terminals as asciinema casts, desktops as Guacamole recordings, replayable from the audit log. Recording is mandatory, not a toggle.
- **CI from the same channel** — `woopsctl upload`/`download`/`exec`/`forward`/`reverse` with per-asset deploy tokens and per-verb scopes. Works from GitLab CI without opening anything inbound.
- **Legacy Windows that still runs your factory** — Windows 7 and Server 2012 are supported, not tolerated. The agent for those is pinned to a Go 1.20 toolchain so the binary actually starts.

## Quick start

You need Docker and Docker Compose. Images come from Docker Hub — no JDK, Go or Node required.

```bash
git clone https://github.com/cdisk/woops.git
cd woops

cp .env.example .env
# Point these at the address you will actually browse to;
# the self-signed cert's SAN is derived from them.
#   OPS_CONSOLE_PUBLIC_HTTP=https://<your-ip-or-domain>
#   OPS_GATEWAY_PUBLIC_HTTP=https://<same>:9200
#   OPS_GATEWAY_PUBLIC_WS=wss://<same>:9200
#   OPS_CONTROL_PUBLIC_HTTP=http://<same>:9100
# For anything real, also change OPS_JWT_SECRET, OPS_TICKET_SECRET
# and the bootstrap admin password.

docker compose --env-file .env --profile full --profile desktop up -d
```

Open `https://<your-address>` and trust the self-signed certificate once. Default login is `admin` / `admin123`, and you are required to bind a TOTP authenticator on first sign-in.

If `deploy/tls/` has no certificate, the `tls-init` container generates one from your `PUBLIC` URLs and writes the matching SPKI pin to `deploy/compose-pin.env`. Bringing your own CA-issued cert works too — drop it in `deploy/tls/` before the first `up`. Changing domains later? Delete `deploy/tls/gateway.*` and `up` again to re-sign.

Images: [`cdisk/woops-console`](https://hub.docker.com/r/cdisk/woops-console), [`cdisk/woops-control-api`](https://hub.docker.com/r/cdisk/woops-control-api), [`cdisk/woops-gateway`](https://hub.docker.com/r/cdisk/woops-gateway).

## Enrolling a host

In the console, go to **Assets**, select a group in the left tree, and click **Generate install link**. Copy the command for the target's OS and run it there:

| Target | Command |
|--------|---------|
| Linux (amd64, systemd) | `curl … \| bash` |
| Windows 10 1809+ / Server 2019+ | `curl.exe` the `install.ps1`, then `powershell -File …` as administrator |
| Windows 7 / Server 2012 / 2016 | `curl.exe` the `install.bat`, then run it from an elevated `cmd` |

Install codes are valid for about 15 minutes and can be reused within that window. The script downloads the agent, verifies it against an embedded SHA-256, registers with the gateway, and installs a service (`woops-agent`). Reinstalling keeps the asset's identity and rotates its token.

Offline, air-gapped and fully manual installs are covered in [`docs/agent-manual-install.md`](../docs/agent-manual-install.md).

## Behind air gaps and nested networks

You do **not** need Squid, nginx or FRP to reach deeper segments. The agent ships with an optional inbound HTTP forward proxy (`agent.yaml` → `proxy.*`), sharing the same binary. It is off by default, and when enabled it is deliberately restricted rather than an open egress:

| Guard | Behaviour |
|-------|-----------|
| Credentials required | `username` / `password` are mandatory — no anonymous proxying |
| Source allowlist | `allowCIDRs` restricts which subnets may connect at all |
| Ops-only by default | With `allowGlobal=false`, egress is limited to the gateway and ops hosts. It cannot be used as a general-purpose web proxy |
| Optional widening | `allowGlobal=true` permits broader targets, still refusing loopback and cloud metadata addresses |
| Loop prevention | If the host already has a `gatewayProxy`, inbound traffic egresses via that upstream and refuses to dial back into itself |

```text
  ┌─ Internet / DMZ ─┐      firewall      ┌──────── internal ────────┐
  │  Woops gateway   │  ←—— blocked ——→   │  Jump host A             │
  │  Console         │                    │  agent + restricted proxy│
  └──────────────────┘                    │        │                 │
                                          │        ▼ proxy           │
                                          │  Hosts B / C …           │
                                          │  agent (egress via A)    │
                                          └──────────────────────────┘
```

Install the agent normally on a host that can reach the gateway and enable `proxy.*` there. On deeper hosts, set `https_proxy` to that jump host before running the installer; the script persists it as `gatewayProxy`. Chaining works to arbitrary depth (`C → B → A → gateway`). Where a segment only allows connections in one direction, `proxyBridge.*` carries the same proxy over an agent-to-agent TCP session multiplexed with yamux.

See [`go/agent.example.yaml`](../go/agent.example.yaml) for the full configuration.

## Supported targets

Binaries are primarily amd64; the installers have arm64 placeholders that work once the gateway has matching artifacts.

| Platform | Notes |
|----------|-------|
| Linux, common amd64 distributions | systemd service, native PTY shell. VNC requires a desktop and VNC server on the target |
| Windows 10 1809+ / Server 2019+ | Build ≥ 17763, so ConPTY is available. PowerShell by default |
| Windows 7 / Server 2012 / 2016 | No ConPTY, so the agent uses WinPTY with `cmd`. Target needs `curl.exe` |

## Ports

| Port | Purpose |
|------|---------|
| 443 | Console (HTTPS) |
| 9100 | control-api — keep this internal |
| 9200 | Gateway **public** — agents, `woopsctl`, browsers |
| 9201 | Gateway **internal** — control-api only, never expose |
| 5432 | PostgreSQL / TimescaleDB |
| 4822 | guacd (`desktop` profile) |

In production, expose only 443 and 9200, plus your forward port-mapping range if you use it.

## Security

- Replace `OPS_JWT_SECRET`, `OPS_TICKET_SECRET` and the bootstrap admin password before exposing anything.
- The gateway must serve `https`/`wss`. For self-signed certificates or dynamic IPs, pin the key with `OPS_GATEWAY_TLS_SPKI_SHA256` — **do not** disable certificate verification.
- Never commit a real `.env`, the private keys under `deploy/tls/`, or any `agent-token`.
- Agent tokens are stored as BCrypt hashes, deploy tokens as SHA-256, and session tickets are short-lived (roughly 90 seconds).
- Local sign-in enforces TOTP, and both login and TOTP are rate-limited to 10 failures per 15 minutes per IP and identity.

Known gaps and hardening still in progress are tracked honestly in [`FEATURES.md`](../FEATURES.md) §9 — notably credential-at-rest encryption, a file path sandbox, and a denylist for port-mapping targets.

## Architecture

```text
Browser ──WSS──► Gateway ──WSS──► Agent
                    │
                    ├── shell           agent-native PTY, no sshd
                    ├── filemanager     directory JSON-RPC, metadata only
                    ├── filetransfer    dedicated binary WSS, resumable
                    ├── rdp / vnc       TCP tunnel → local desktop port + guacd
                    ├── portmap         forward and reverse TCP/UDP
                    ├── metrics         optional separate WSS → TimescaleDB
                    └── control plane   Java: auth, assets, tickets, RBAC, audit
```

Control plane is Java 21 + Spring Boot 3 against PostgreSQL/TimescaleDB. The data plane is Go — gateway, agent and `woopsctl`. Console is Vue 3, English and Chinese, defaulting to English. No Redis, no message queue, no object storage: the runtime writes JSONL to disk and the control plane tails it.

## Contributing

Issues and pull requests are welcome. Please don't paste secrets, install codes, or internal network topology into them.

## License and scope of use

Apache License 2.0. See [`LICENSE`](../LICENSE).

**This software is for lawful, authorised operations and asset management only.** Using it for unauthorised access or disruption is prohibited, and the consequences rest with whoever does it.

The software is provided **"as is"**, without warranty of merchantability, fitness for a particular purpose, security, or non-infringement. You, as the operator, are responsible for accounts and keys, TLS and network exposure, patching, backups, privilege separation, and compliance in your jurisdiction. The authors and contributors accept no liability for data loss, outages, or intrusion resulting from misconfiguration, weak credentials, unpatched deployments, third-party attacks, or misuse.
