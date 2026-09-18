package com.ops.control.agent;

import com.ops.control.asset.AssetAccess;
import com.ops.control.asset.AssetEntity;
import com.ops.control.asset.AssetRepository;
import com.ops.control.assetevent.AssetEventService;
import com.ops.control.controlaudit.ControlAuditService;
import com.ops.control.common.OpsProperties;
import com.ops.control.common.PublicUrlResolver;
import com.ops.control.user.UserRepository;
import jakarta.servlet.http.HttpServletRequest;
import org.springframework.security.crypto.password.PasswordEncoder;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.security.SecureRandom;
import java.time.Instant;
import java.util.*;

@Service
public class AgentService {
    /** 16 hex chars; 15-min TTL makes collision negligible. */
    private static final int INSTALL_CODE_BYTES = 8;

    private final InstallCodeRepository installCodes;
    private final AssetRepository assets;
    private final OpsProperties props;
    private final PasswordEncoder passwordEncoder;
    private final ControlAuditService audit;
    private final AssetEventService assetEvents;
    private final UserRepository users;
    private final SecureRandom random = new SecureRandom();

    public AgentService(
            InstallCodeRepository installCodes,
            AssetRepository assets,
            OpsProperties props,
            PasswordEncoder passwordEncoder,
            ControlAuditService audit,
            AssetEventService assetEvents,
            UserRepository users) {
        this.installCodes = installCodes;
        this.assets = assets;
        this.props = props;
        this.passwordEncoder = passwordEncoder;
        this.audit = audit;
        this.assetEvents = assetEvents;
        this.users = users;
    }

    public Map<String, Object> createInstallCode(UUID userId, UUID groupId, HttpServletRequest request) {
        MintedInstallCode minted = mintInstallCode(userId, groupId, request);
        audit.record(
                ControlAuditService.CAT_ASSET,
                ControlAuditService.ACT_INSTALL_CODE,
                userId,
                usernameOf(userId),
                null,
                groupId,
                null,
                ControlAuditService.jsonDetail(Map.of(
                        "installCodeId", minted.installCodeId().toString(),
                        "expiresAt", minted.expiresAt().toString()
                )));
        return minted.response();
    }

    /**
     * One-click Agent update from console: mint an install code for the asset's
     * group and record control-audit UPDATE_AGENT (not a bare INSTALL_CODE).
     */
    public Map<String, Object> prepareAgentUpdate(UUID userId, AssetEntity asset, HttpServletRequest request) {
        if (asset == null) {
            throw new IllegalArgumentException("asset not found");
        }
        if (asset.getGroupId() == null) {
            throw new IllegalStateException("asset has no group; assign a group and save first");
        }
        if (asset.getAgentTokenHash() == null || asset.getAgentTokenHash().isBlank()) {
            throw new IllegalStateException("asset has no agent");
        }
        if (!asset.isOnline()) {
            throw new IllegalStateException("agent offline");
        }
        MintedInstallCode minted = mintInstallCode(userId, asset.getGroupId(), request);
        Map<String, Object> detail = new LinkedHashMap<>();
        detail.put("installCodeId", minted.installCodeId().toString());
        detail.put("fromAgentVersion", asset.getAgentVersion() == null ? "" : asset.getAgentVersion());
        detail.put("groupId", asset.getGroupId().toString());
        audit.record(
                ControlAuditService.CAT_ASSET,
                ControlAuditService.ACT_UPDATE_AGENT,
                userId,
                usernameOf(userId),
                asset.getId(),
                asset.getGroupId(),
                null,
                ControlAuditService.jsonDetail(detail));
        Map<String, Object> out = new LinkedHashMap<>(minted.response());
        String pin = props.gatewayTlsSpkiSha256Normalized();
        String gatewayBase = String.valueOf(out.get("gatewayBase"));
        String code = String.valueOf(out.get("code"));
        // Re-wrap with gatewayProxy export: exec inherits no proxy env on air-gapped hosts.
        out.put("curl", withAgentProxyLinux(buildLinuxInstallCurl(gatewayBase, code, pin)));
        out.put("powershell", withAgentProxyWindows(buildWindowsInstallCommand(gatewayBase, code, pin, true)));
        out.put("cmd", withAgentProxyWindows(buildWindowsInstallCommand(gatewayBase, code, pin, true)));
        out.put("assetId", asset.getId().toString());
        out.put("fromAgentVersion", asset.getAgentVersion() == null ? "" : asset.getAgentVersion());
        return out;
    }

    /**
     * Update-only prefix: hosts behind an air gap reach the Gateway solely through the
     * {@code gatewayProxy} in their own {@code agent.yaml}, while exec inherits the service
     * environment, which has no proxy — curl would fail to fetch the install script. The value is
     * read from disk on the target because the Agent that knows it is the one being replaced, so
     * already-deployed (old) Agents must work unchanged.
     */
    static String withAgentProxyLinux(String command) {
        return "OPS_CONF=; "
                + "if [ -r /etc/woops-agent/agent.yaml ]; then OPS_CONF=/etc/woops-agent/agent.yaml; fi; "
                + "if [ -n \"$OPS_CONF\" ]; then "
                + "OPS_PROXY=$(sed -n 's/^[[:space:]]*gatewayProxy:[[:space:]]*//p' \"$OPS_CONF\" "
                // Octal classes keep quote characters out of the generated one-liner.
                + "| head -n1 | tr -d '\\042\\047\\r '); "
                + "if [ -n \"$OPS_PROXY\" ]; then "
                + "export https_proxy=\"$OPS_PROXY\" http_proxy=\"$OPS_PROXY\" all_proxy=\"$OPS_PROXY\" "
                + "HTTPS_PROXY=\"$OPS_PROXY\" HTTP_PROXY=\"$OPS_PROXY\"; "
                // Never echo the value: gatewayProxy may embed user:pass.
                + "echo '==> using gatewayProxy from agent.yaml'; "
                + "fi; fi; "
                + command;
    }

    /** Windows counterpart of {@link #withAgentProxyLinux(String)}; config lives under ProgramData. */
    static String withAgentProxyWindows(String command) {
        return "$c=$null; $p=Join-Path $env:ProgramData 'woops-agent\\agent.yaml'; "
                + "if (Test-Path -LiteralPath $p) { $c=$p }; "
                + "if ($c) { "
                // ReadAllText works on PowerShell 2.0; Get-Content -Raw requires PowerShell 3.0.
                + "$m=[regex]::Match([System.IO.File]::ReadAllText($c), "
                + "'(?m)^\\s*gatewayProxy:\\s*(\\S+)\\s*$'); "
                + "if ($m.Success) { $p=$m.Groups[1].Value.Trim([char]34).Trim([char]39); "
                + "if ($p) { $env:HTTPS_PROXY=$p; $env:HTTP_PROXY=$p; $env:ALL_PROXY=$p; "
                + "Write-Host '==> using gatewayProxy from agent.yaml' } } }; "
                + command;
    }

    private record MintedInstallCode(
            UUID installCodeId,
            Instant expiresAt,
            Map<String, Object> response) {}

    private MintedInstallCode mintInstallCode(UUID userId, UUID groupId, HttpServletRequest request) {
        if (groupId == null) {
            throw new IllegalArgumentException("groupId required");
        }
        InstallCodeEntity entity = new InstallCodeEntity();
        entity.setCode(randomToken(INSTALL_CODE_BYTES));
        entity.setExpiresAt(Instant.now().plusSeconds(props.installCodeTtlMinutes() * 60L));
        entity.setCreatedBy(userId);
        entity.setGroupId(groupId);
        installCodes.save(entity);

        var endpoints = PublicUrlResolver.resolve(request, props);
        String gatewayBase = endpoints.gatewayHttp().replaceAll("/+$", "");
        String base = gatewayBase + "/i/" + entity.getCode();
        String linuxUrl = base + "/agent/linux/amd64";
        String windowsUrl = base + "/agent/windows/amd64";
        String pin = props.gatewayTlsSpkiSha256Normalized();
        Map<String, Object> out = new LinkedHashMap<>();
        out.put("code", entity.getCode());
        out.put("installCodeId", entity.getId().toString());
        out.put("groupId", groupId.toString());
        out.put("expiresAt", entity.getExpiresAt().toString());
        out.put("installUrl", linuxUrl);
        out.put("installUrlWindows", windowsUrl);
        out.put("installUrlWindowsLegacy", windowsUrl);
        out.put("gatewayBase", gatewayBase);
        if (!pin.isBlank()) {
            out.put("gatewayTlsSpkiSha256", pin);
        }
        out.put("curl", buildLinuxInstallCurl(gatewayBase, entity.getCode(), pin));
        out.put("powershell", buildWindowsInstallCommand(gatewayBase, entity.getCode(), pin, false));
        out.put("cmd", buildWindowsLegacyInstallCommand(gatewayBase, entity.getCode(), pin));
        return new MintedInstallCode(entity.getId(), entity.getExpiresAt(), out);
    }

    public List<Map<String, Object>> listInstallCodes() {
        return installCodes.findAll().stream()
                .sorted(Comparator.comparing(InstallCodeEntity::getCreatedAt).reversed())
                .map(c -> {
                    Map<String, Object> m = new LinkedHashMap<>();
                    m.put("id", c.getId().toString());
                    m.put("code", c.getCode());
                    m.put("groupId", c.getGroupId() == null ? null : c.getGroupId().toString());
                    m.put("expiresAt", c.getExpiresAt().toString());
                    m.put("revoked", c.isRevoked());
                    m.put("usedCount", c.getUsedCount());
                    m.put("valid", !c.isRevoked() && c.getExpiresAt().isAfter(Instant.now()) && c.getGroupId() != null);
                    return m;
                })
                .toList();
    }

    @Transactional
    public void revokeInstallCode(UUID id, UUID actorId, String actorUsername) {
        InstallCodeEntity code = installCodes.findById(id)
                .orElseThrow(() -> new IllegalArgumentException("install code not found"));
        code.setRevoked(true);
        audit.record(
                ControlAuditService.CAT_ASSET,
                ControlAuditService.ACT_REVOKE_INSTALL,
                actorId,
                actorUsername,
                null,
                code.getGroupId(),
                null,
                ControlAuditService.jsonDetail(Map.of("installCodeId", code.getId().toString())));
    }

    public boolean isInstallCodeValid(String code) {
        return installCodes.findByCode(code)
                .filter(c -> !c.isRevoked())
                .filter(c -> c.getExpiresAt().isAfter(Instant.now()))
                .filter(c -> c.getGroupId() != null)
                .isPresent();
    }

    /**
     * First install: omit assetId → mint assets.id, return token.
     * Reinstall: send local asset-id → reuse asset, rotate token.
     */
    @Transactional
    public Map<String, Object> register(RegisterRequest req, String remoteIp) {
        InstallCodeEntity code = installCodes.findByCode(req.installCode())
                .orElseThrow(() -> new IllegalArgumentException("invalid install code"));
        if (code.isRevoked() || code.getExpiresAt().isBefore(Instant.now())) {
            throw new IllegalArgumentException("install code expired or revoked");
        }
        if (code.getGroupId() == null) {
            throw new IllegalArgumentException("install code has no group; create a new install link");
        }

        UUID requestedId = parseAssetId(req.assetId());
        Optional<AssetEntity> existing = requestedId == null ? Optional.empty() : assets.findById(requestedId);
        boolean isNew = existing.isEmpty();
        AssetEntity asset;
        if (isNew) {
            asset = new AssetEntity();
            asset.setId(requestedId != null ? requestedId : UUID.randomUUID());
            asset.setDisplayName(req.hostname() == null || req.hostname().isBlank() ? "unknown" : req.hostname());
        } else {
            asset = existing.get();
        }
        // Install code group is authoritative for both first install and reinstall (move asset).
        asset.setGroupId(code.getGroupId());
        asset.setHostname(req.hostname());
        asset.setOs(truncate(req.os(), 256));
        asset.setArch(truncate(req.arch(), 32));
        asset.setAgentVersion(truncate(req.agentVersion(), 32));
        applyNetInfo(asset, req.publicIp(), req.privateIp(), remoteIp);
        AssetAccess.applyRegisterDefaults(asset, isNew);
        asset.setOnline(false);
        asset.setUpdatedAt(Instant.now());

        String agentToken = "agt_" + randomToken(32);
        asset.setAgentTokenHash(passwordEncoder.encode(agentToken));
        assets.save(asset);

        code.setUsedCount(code.getUsedCount() + 1);

        audit.record(
                ControlAuditService.CAT_ASSET,
                isNew ? ControlAuditService.ACT_REGISTER : ControlAuditService.ACT_REREGISTER,
                code.getCreatedBy(),
                usernameOf(code.getCreatedBy()),
                asset.getId(),
                asset.getGroupId(),
                null,
                ControlAuditService.jsonDetail(Map.of(
                        "hostname", asset.getHostname() == null ? "" : asset.getHostname(),
                        "os", asset.getOs() == null ? "" : asset.getOs(),
                        "arch", asset.getArch() == null ? "" : asset.getArch(),
                        "reused", !isNew
                )));

        return Map.of(
                "assetId", asset.getId().toString(),
                "agentToken", agentToken,
                "reused", !isNew
        );
    }

    private String usernameOf(UUID userId) {
        if (userId == null) return "";
        return users.findById(userId).map(u -> u.getUsername() == null ? "" : u.getUsername()).orElse("");
    }

    private static UUID parseAssetId(String raw) {
        if (raw == null || raw.isBlank()) return null;
        try {
            return UUID.fromString(raw.trim());
        } catch (IllegalArgumentException e) {
            return null;
        }
    }

    public Optional<AssetEntity> authenticateAgent(String assetId, String token) {
        try {
            UUID id = UUID.fromString(assetId);
            return assets.findById(id)
                    .filter(a -> a.getAgentTokenHash() != null)
                    .filter(a -> passwordEncoder.matches(token, a.getAgentTokenHash()));
        } catch (Exception e) {
            return Optional.empty();
        }
    }

    public record PresenceMeta(
            String sourceIp,
            String reason,
            String connectionId,
            String gatewayInstance
    ) {}

    @Transactional
    public void markOnline(UUID assetId, boolean online, PresenceMeta meta) {
        PresenceMeta m = meta == null ? new PresenceMeta("", "", "", "") : meta;
        assets.findById(assetId).ifPresent(a -> {
            boolean changed = a.isOnline() != online;
            a.setOnline(online);
            a.setLastSeenAt(Instant.now());
            a.setUpdatedAt(Instant.now());
            // Public/source IP comes only from Gateway TCP peer on online; never on offline.
            if (online) {
                String pub = publicIfRoutable(m.sourceIp());
                if (pub != null && !pub.isBlank()) {
                    a.setPublicIp(pub);
                }
            }
            if (changed) {
                Map<String, Object> detail = new LinkedHashMap<>();
                detail.put("sourceIp", blankToEmpty(m.sourceIp()));
                detail.put("privateIp", blankToEmpty(a.getPrivateIp()));
                detail.put("agentVersion", blankToEmpty(a.getAgentVersion()));
                detail.put("reason", blankToEmpty(m.reason()));
                detail.put("gatewayInstance", blankToEmpty(m.gatewayInstance()));
                detail.put("connectionId", blankToEmpty(m.connectionId()));
                assetEvents.record(
                        assetId,
                        AssetEventService.CAT_CONNECTIVITY,
                        online ? AssetEventService.TYPE_ONLINE : AssetEventService.TYPE_OFFLINE,
                        AssetEventService.SEV_INFO,
                        AssetEventService.jsonDetail(detail),
                        blankToEmpty(m.gatewayInstance()),
                        blankToEmpty(m.connectionId()));
            }
        });
    }

    private static String blankToEmpty(String s) {
        return s == null ? "" : s.trim();
    }

    /**
     * Private IPs from Agent netinfo only. Empty reports do not clear existing values.
     * Writes {@code PRIVATE_IP_CHANGED} when the value actually changes.
     */
    @Transactional
    public void updatePrivateIp(UUID assetId, String privateIp) {
        if (privateIp == null || privateIp.isBlank()) {
            return;
        }
        String next = privateIp.trim();
        assets.findById(assetId).ifPresent(a -> {
            String prev = a.getPrivateIp() == null ? "" : a.getPrivateIp().trim();
            a.setLastSeenAt(Instant.now());
            a.setUpdatedAt(Instant.now());
            if (prev.equals(next)) {
                return;
            }
            a.setPrivateIp(next);
            // First non-empty fill still counts as a change (empty → value).
            assetEvents.record(
                    assetId,
                    AssetEventService.CAT_NETWORK,
                    AssetEventService.TYPE_PRIVATE_IP_CHANGED,
                    AssetEventService.SEV_INFO,
                    AssetEventService.jsonDetail(Map.of("from", prev, "to", next)),
                    "",
                    "");
        });
    }

    private void applyNetInfo(AssetEntity asset, String publicIp, String privateIp, String observedPeerIp) {
        // Register path only: optional private from install script; public from Gateway remote if routable.
        if (privateIp != null && !privateIp.isBlank()) {
            asset.setPrivateIp(privateIp.trim());
        }
        String pub = firstNonBlank(publicIp, publicIfRoutable(observedPeerIp));
        if (pub != null && !pub.isBlank()) {
            asset.setPublicIp(pub);
        }
        // Do not fall back observed peer into privateIp — private comes from Agent netinfo only.
    }

    private static String truncate(String s, int max) {
        if (s == null) return null;
        String t = s.trim();
        if (t.isEmpty()) return t;
        return t.length() <= max ? t : t.substring(0, max);
    }

    private static String firstNonBlank(String a, String b) {
        if (a != null && !a.isBlank()) return a.trim();
        if (b != null && !b.isBlank()) return b.trim();
        return null;
    }

    private static String publicIfRoutable(String ip) {
        if (ip == null || ip.isBlank() || isPrivateIp(ip) || isLoopback(ip)) return null;
        return ip.trim();
    }

    private static boolean isLoopback(String ip) {
        return "127.0.0.1".equals(ip) || "::1".equals(ip) || ip.startsWith("127.");
    }

    private static boolean isPrivateIp(String ip) {
        if (ip == null) return true;
        String s = ip.trim();
        return s.startsWith("10.")
                || s.startsWith("192.168.")
                || s.matches("172\\.(1[6-9]|2[0-9]|3[0-1])\\..*")
                || s.startsWith("169.254.")
                || isLoopback(s);
    }

    private String randomToken(int bytes) {
        byte[] buf = new byte[bytes];
        random.nextBytes(buf);
        return HexFormat.of().formatHex(buf);
    }

    /**
     * Console one-liner: download the agent binary (TLS pin on the binary itself), then run
     * {@code woops-agent install}. Architecture is probed inline so amd64/arm64 share one command.
     * With pin: prefer {@code -k --pinnedpubkey}; CentOS 6 / curl &lt; 7.39 lack that flag, so the
     * command falls back to {@code -k} only after detecting support (still better than failing).
     */
    static String buildLinuxInstallCurl(String gatewayBase, String code, String pinHex) {
        String url = gatewayBase + "/i/" + code
                + "/agent/linux/$(uname -m | sed -e s/x86_64/amd64/ -e s/aarch64/arm64/)";
        String download;
        if (pinHex == null || pinHex.isBlank()) {
            download = "curl -fsSL --compressed -o /tmp/woops-agent \"" + url + "\"";
        } else {
            String pinned = "sha256//" + Base64.getEncoder().encodeToString(HexFormat.of().parseHex(pinHex));
            // Old curl (CentOS 6) rejects unknown --pinnedpubkey before any transfer.
            download = "if curl --help 2>&1 | grep -q -- '--pinnedpubkey'; then "
                    + "curl -fsSL -k --pinnedpubkey " + pinned + " --compressed -o /tmp/woops-agent \"" + url + "\"; "
                    + "else "
                    + "echo '==> WARNING: curl has no --pinnedpubkey (need curl>=7.39); downloading with -k only'; "
                    + "curl -fsSL -k --compressed -o /tmp/woops-agent \"" + url + "\"; "
                    + "fi";
        }
        return download + " && chmod +x /tmp/woops-agent && /tmp/woops-agent " + installArgs(gatewayBase, code, pinHex);
    }

    /**
     * Windows PowerShell one-liner. {@code useProgramData} is for one-click update under LocalSystem
     * (no reliable TEMP); manual install uses {@code $env:TEMP}.
     */
    static String buildWindowsInstallCommand(String gatewayBase, String code, String pinHex, boolean useProgramData) {
        String url = gatewayBase + "/i/" + code + "/agent/windows/amd64";
        String destSetup = useProgramData
                ? "$d=Join-Path $env:ProgramData 'woops-agent'; New-Item -ItemType Directory -Force -Path $d | Out-Null; $f=Join-Path $d 'woops-agent-setup.exe'; "
                : "$f=Join-Path $env:TEMP woops-agent.exe; ";
        String curl;
        if (pinHex == null || pinHex.isBlank()) {
            curl = "curl.exe -fsSL --compressed -o $f " + url;
        } else {
            String pinned = "sha256//" + Base64.getEncoder().encodeToString(HexFormat.of().parseHex(pinHex));
            curl = "curl.exe -fsSL -k --pinnedpubkey " + pinned + " --compressed -o $f " + url;
        }
        return destSetup + curl + "; if ($LASTEXITCODE -ne 0) { throw 'curl failed' }; & $f "
                + installArgs(gatewayBase, code, pinHex);
    }

    /**
     * Win7 / Server 2012 one-liner: pure cmd (no PowerShell {@code &&} / {@code Join-Path}).
     */
    static String buildWindowsLegacyInstallCommand(String gatewayBase, String code, String pinHex) {
        String url = gatewayBase + "/i/" + code + "/agent/windows/amd64";
        String exe = "\"%TEMP%\\woops-agent.exe\"";
        String curl;
        if (pinHex == null || pinHex.isBlank()) {
            curl = "curl.exe -fsSL --compressed -o " + exe + " " + url;
        } else {
            String pinned = "sha256//" + Base64.getEncoder().encodeToString(HexFormat.of().parseHex(pinHex));
            curl = "curl.exe -fsSL -k --pinnedpubkey " + pinned + " --compressed -o " + exe + " " + url;
        }
        return curl + " && " + exe + " " + installArgs(gatewayBase, code, pinHex);
    }

    private static String installArgs(String gatewayBase, String code, String pinHex) {
        String args = "install -gateway " + gatewayBase + " -code " + code;
        if (pinHex != null && !pinHex.isBlank()) {
            args += " -pin " + pinHex;
        }
        return args;
    }

    public record RegisterRequest(
            String installCode,
            String assetId,
            String agentVersion,
            String hostname,
            String os,
            String arch,
            String publicIp,
            String privateIp
    ) {}
}
