package com.ops.control.ci;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.ops.control.access.AccessService;
import com.ops.control.asset.AssetEntity;
import com.ops.control.asset.AssetRepository;
import com.ops.control.controlaudit.ControlAuditService;
import com.ops.control.common.OpsProperties;
import com.ops.control.common.PublicUrlResolver;
import com.ops.control.session.SessionTicketService;
import com.ops.control.user.UserEntity;
import jakarta.servlet.http.HttpServletRequest;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.SecureRandom;
import java.time.Instant;
import java.util.HexFormat;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.UUID;

@Service
public class DeployTokenService {
    private static final String TOKEN_PREFIX = "ops_";

    private final DeployTokenRepository tokens;
    private final AssetRepository assets;
    private final AccessService access;
    private final ControlAuditService audit;
    private final SessionTicketService tickets;
    private final OpsProperties props;
    private final ObjectMapper mapper;
    private final SecureRandom random = new SecureRandom();

    public DeployTokenService(
            DeployTokenRepository tokens,
            AssetRepository assets,
            AccessService access,
            ControlAuditService audit,
            SessionTicketService tickets,
            OpsProperties props,
            ObjectMapper mapper) {
        this.tokens = tokens;
        this.assets = assets;
        this.access = access;
        this.audit = audit;
        this.tickets = tickets;
        this.props = props;
        this.mapper = mapper;
    }

    @Transactional
    public Map<String, Object> create(
            UUID assetId,
            String remark,
            boolean allowUpload,
            boolean allowDownload,
            boolean allowExec,
            boolean allowForward,
            boolean allowReverse,
            Instant expiresAt,
            UserEntity user,
            HttpServletRequest request) {
        AssetEntity asset = requireAccessibleAsset(assetId, user);
        if (!allowUpload && !allowDownload && !allowExec && !allowForward && !allowReverse) {
            throw new IllegalArgumentException("至少选择一个 scope");
        }
        String trimmedRemark = remark == null ? "" : remark.trim();
        if (trimmedRemark.length() > 512) {
            throw new IllegalArgumentException("remark too long");
        }
        if (expiresAt != null && !expiresAt.isAfter(Instant.now())) {
            throw new IllegalArgumentException("expiresAt must be in the future");
        }

        DeployTokenEntity entity = new DeployTokenEntity();
        entity.setAssetId(asset.getId());
        entity.setRemark(trimmedRemark.isEmpty() ? null : trimmedRemark);
        entity.setAllowUpload(allowUpload);
        entity.setAllowDownload(allowDownload);
        entity.setAllowExec(allowExec);
        entity.setAllowForward(allowForward);
        entity.setAllowReverse(allowReverse);
        entity.setExpiresAt(expiresAt);
        entity.setCreatedBy(user.getId());
        entity.setCreatedAt(Instant.now());

        // Persist to assign UUID, then seal secret.
        entity.setSecretHash("pending");
        entity = tokens.save(entity);

        String secret = randomHex(32);
        entity.setSecretHash(sha256Hex(secret));
        tokens.save(entity);

        String plaintext = TOKEN_PREFIX + entity.getId() + "_" + secret;
        Map<String, Object> detail = new LinkedHashMap<>();
        detail.put("tokenId", entity.getId().toString());
        detail.put("allowUpload", allowUpload);
        detail.put("allowDownload", allowDownload);
        detail.put("allowExec", allowExec);
        detail.put("allowForward", allowForward);
        detail.put("allowReverse", allowReverse);
        detail.put("expiresAt", expiresAt == null ? "" : expiresAt.toString());
        if (entity.getRemark() != null) {
            detail.put("remark", entity.getRemark());
        }
        audit.record(
                ControlAuditService.CAT_CI,
                ControlAuditService.ACT_CREATE,
                user.getId(),
                user.getUsername(),
                asset.getId(),
                null,
                ControlAuditService.jsonDetail(detail));

        Map<String, Object> out = toView(entity);
        out.put("token", plaintext);
        out.put("opsctlConfig", buildOpsctlConfig(request, plaintext));
        out.put("opsctlConfigJson", writeOpsctlConfigJson(request, plaintext));
        return out;
    }

    /** One-time CI blob for OPSCTL_CONFIG: Gateway HTTPS base + deploy token + SPKI pin. */
    private Map<String, Object> buildOpsctlConfig(HttpServletRequest request, String plaintextToken) {
        var endpoints = PublicUrlResolver.resolve(request, props);
        String server = endpoints.gatewayHttp();
        Map<String, Object> cfg = new LinkedHashMap<>();
        cfg.put("server", server);
        cfg.put("token", plaintextToken);
        cfg.put("pin", props.gatewayTlsSpkiSha256Normalized());
        return cfg;
    }

    private String writeOpsctlConfigJson(HttpServletRequest request, String plaintextToken) {
        try {
            return mapper.writeValueAsString(buildOpsctlConfig(request, plaintextToken));
        } catch (Exception e) {
            throw new IllegalStateException("encode OPSCTL_CONFIG", e);
        }
    }

    @Transactional(readOnly = true)
    public List<Map<String, Object>> list(UUID assetId, UserEntity user) {
        requireAccessibleAsset(assetId, user);
        return tokens.findByAssetIdOrderByCreatedAtDesc(assetId).stream().map(this::toView).toList();
    }

    @Transactional
    public Map<String, String> revoke(UUID assetId, UUID tokenId, UserEntity user) {
        AssetEntity asset = requireAccessibleAsset(assetId, user);
        DeployTokenEntity entity = tokens.findById(tokenId)
                .orElseThrow(() -> new IllegalArgumentException("token not found"));
        if (!entity.getAssetId().equals(asset.getId())) {
            throw new IllegalArgumentException("token not found");
        }
        if (entity.getRevokedAt() == null) {
            entity.setRevokedAt(Instant.now());
            tokens.save(entity);
            Map<String, Object> detail = new LinkedHashMap<>();
            detail.put("tokenId", entity.getId().toString());
            if (entity.getRemark() != null) {
                detail.put("remark", entity.getRemark());
            }
            audit.record(
                    ControlAuditService.CAT_CI,
                    ControlAuditService.ACT_REVOKE,
                    user.getId(),
                    user.getUsername(),
                    asset.getId(),
                    null,
                    ControlAuditService.jsonDetail(detail));
        }
        return Map.of("status", "revoked", "id", tokenId.toString());
    }

    /**
     * Authenticate deploy token and issue a short-lived file/exec session ticket.
     * Plaintext form: ops_&lt;tokenId&gt;_&lt;secret&gt;
     */
    @Transactional
    public Map<String, Object> createOpsctlTicket(String bearerToken, String action, Map<String, Object> meta) {
        DeployTokenEntity entity = authenticate(bearerToken);
        String act = action == null ? "" : action.trim().toLowerCase();
        if (!List.of(
                "upload",
                "download",
                "exec",
                "port-forward",
                "port-reverse",
                "port-reverse-connection").contains(act)) {
            throw new IllegalArgumentException(
                    "action must be upload, download, exec, port-forward, port-reverse or port-reverse-connection");
        }
        if ("upload".equals(act) && !entity.isAllowUpload()) {
            throw new IllegalArgumentException("token does not allow upload");
        }
        if ("download".equals(act) && !entity.isAllowDownload()) {
            throw new IllegalArgumentException("token does not allow download");
        }
        if ("exec".equals(act) && !entity.isAllowExec()) {
            throw new IllegalArgumentException("token does not allow exec");
        }
        if ("port-forward".equals(act) && !entity.isAllowForward()) {
            throw new IllegalArgumentException("token does not allow forward");
        }
        if (("port-reverse".equals(act) || "port-reverse-connection".equals(act)) && !entity.isAllowReverse()) {
            throw new IllegalArgumentException("token does not allow reverse");
        }

        AssetEntity asset = assets.findById(entity.getAssetId())
                .orElseThrow(() -> new IllegalStateException("asset not found"));
        if (asset.getAgentTokenHash() == null || asset.getAgentTokenHash().isBlank()) {
            throw new IllegalStateException("asset has no agent");
        }
        if (!asset.isOnline()) {
            throw new IllegalStateException("agent offline");
        }

        String actor = "token:" + entity.getId().toString().substring(0, 8);
        Map<String, Object> ticket = switch (act) {
            case "upload", "download" -> {
                String path = metaString(meta, "remotePath");
                if (path.isBlank()) {
                    path = metaString(meta, "path");
                }
                Long size = metaLong(meta, "bytes");
                if (size == null) {
                    size = metaLong(meta, "size");
                }
                String fingerprint = metaString(meta, "fingerprint");
                String transferId = metaString(meta, "transferId");
                Boolean abort = metaBool(meta, "abort");
                yield tickets.createCiFileTransferTicket(
                        entity.getId(),
                        actor,
                        asset,
                        act,
                        path,
                        size,
                        fingerprint.isBlank() ? null : fingerprint,
                        transferId.isBlank() ? null : transferId,
                        abort);
            }
            case "exec" -> tickets.createCiExecTicket(entity.getId(), actor, asset);
            case "port-forward", "port-reverse", "port-reverse-connection" -> {
                PortmapMeta portmap = requirePortmapMeta(meta);
                yield tickets.createCiPortmapTicket(
                        entity.getId(),
                        actor,
                        asset,
                        act,
                        portmap.protocol(),
                        portmap.ephemeralId(),
                        portmap.listenHost(),
                        portmap.listenPort(),
                        portmap.targetHost(),
                        portmap.targetPort(),
                        portmap.clientAddr());
            }
            default -> throw new IllegalArgumentException("unsupported action");
        };

        entity.setLastUsedAt(Instant.now());
        tokens.save(entity);
        return ticket;
    }

    private DeployTokenEntity authenticate(String bearerToken) {
        if (bearerToken == null || bearerToken.isBlank()) {
            throw new IllegalArgumentException("token required");
        }
        String raw = bearerToken.trim();
        if (!raw.startsWith(TOKEN_PREFIX)) {
            throw new IllegalArgumentException("invalid token");
        }
        String rest = raw.substring(TOKEN_PREFIX.length());
        int sep = rest.indexOf('_');
        if (sep <= 0 || sep >= rest.length() - 1) {
            throw new IllegalArgumentException("invalid token");
        }
        UUID tokenId;
        try {
            tokenId = UUID.fromString(rest.substring(0, sep));
        } catch (IllegalArgumentException e) {
            throw new IllegalArgumentException("invalid token");
        }
        String secret = rest.substring(sep + 1);
        DeployTokenEntity entity = tokens.findById(tokenId)
                .orElseThrow(() -> new IllegalArgumentException("invalid token"));
        if (!MessageDigest.isEqual(
                sha256Hex(secret).getBytes(StandardCharsets.UTF_8),
                entity.getSecretHash().getBytes(StandardCharsets.UTF_8))) {
            throw new IllegalArgumentException("invalid token");
        }
        if (entity.getRevokedAt() != null) {
            throw new IllegalArgumentException("token revoked");
        }
        if (entity.getExpiresAt() != null && !entity.getExpiresAt().isAfter(Instant.now())) {
            throw new IllegalArgumentException("token expired");
        }
        return entity;
    }

    private AssetEntity requireAccessibleAsset(UUID assetId, UserEntity user) {
        AssetEntity asset = assets.findById(assetId)
                .orElseThrow(() -> new IllegalArgumentException("asset not found"));
        access.assertCanAccessAsset(user, asset);
        return asset;
    }

    private Map<String, Object> toView(DeployTokenEntity e) {
        Map<String, Object> m = new LinkedHashMap<>();
        m.put("id", e.getId().toString());
        m.put("assetId", e.getAssetId().toString());
        m.put("remark", e.getRemark() == null ? "" : e.getRemark());
        m.put("allowUpload", e.isAllowUpload());
        m.put("allowDownload", e.isAllowDownload());
        m.put("allowExec", e.isAllowExec());
        m.put("allowForward", e.isAllowForward());
        m.put("allowReverse", e.isAllowReverse());
        m.put("expiresAt", e.getExpiresAt() == null ? null : e.getExpiresAt().toString());
        m.put("revokedAt", e.getRevokedAt() == null ? null : e.getRevokedAt().toString());
        m.put("createdAt", e.getCreatedAt().toString());
        m.put("lastUsedAt", e.getLastUsedAt() == null ? null : e.getLastUsedAt().toString());
        m.put("active", e.getRevokedAt() == null
                && (e.getExpiresAt() == null || e.getExpiresAt().isAfter(Instant.now())));
        return m;
    }

    private static String metaString(Map<String, Object> meta, String key) {
        if (meta == null || key == null) {
            return "";
        }
        Object v = meta.get(key);
        return v == null ? "" : String.valueOf(v).trim();
    }

    private static Long metaLong(Map<String, Object> meta, String key) {
        if (meta == null || key == null) {
            return null;
        }
        Object v = meta.get(key);
        if (v instanceof Number n) {
            return n.longValue();
        }
        if (v instanceof String s && !s.isBlank()) {
            try {
                return Long.parseLong(s.trim());
            } catch (NumberFormatException ignored) {
                return null;
            }
        }
        return null;
    }

    private static Boolean metaBool(Map<String, Object> meta, String key) {
        if (meta == null || key == null) {
            return null;
        }
        Object v = meta.get(key);
        if (v instanceof Boolean b) {
            return b;
        }
        if (v instanceof String s) {
            if ("true".equalsIgnoreCase(s.trim())) {
                return true;
            }
            if ("false".equalsIgnoreCase(s.trim())) {
                return false;
            }
        }
        return null;
    }

    private record PortmapMeta(
            String protocol,
            String ephemeralId,
            String listenHost,
            int listenPort,
            String targetHost,
            int targetPort,
            String clientAddr) {}

    private static PortmapMeta requirePortmapMeta(Map<String, Object> meta) {
        String protocol = requiredMetaString(meta, "protocol").toLowerCase();
        if (!"tcp".equals(protocol) && !"udp".equals(protocol)) {
            throw new IllegalArgumentException("meta.protocol must be tcp or udp");
        }
        String ephemeralId = requiredMetaString(meta, "ephemeralId");
        if (!ephemeralId.startsWith("opsctl:")) {
            throw new IllegalArgumentException("meta.ephemeralId must use opsctl namespace");
        }
        String clientAddr = metaString(meta, "clientAddr");
        return new PortmapMeta(
                protocol,
                ephemeralId,
                requiredMetaString(meta, "listenHost"),
                requiredMetaPort(meta, "listenPort"),
                requiredMetaString(meta, "targetHost"),
                requiredMetaPort(meta, "targetPort"),
                clientAddr.isBlank() ? null : clientAddr);
    }

    private static String requiredMetaString(Map<String, Object> meta, String key) {
        String value = metaString(meta, key);
        if (value.isBlank()) {
            throw new IllegalArgumentException("meta." + key + " is required");
        }
        return value;
    }

    private static int requiredMetaPort(Map<String, Object> meta, String key) {
        if (meta == null || !meta.containsKey(key) || meta.get(key) == null) {
            throw new IllegalArgumentException("meta." + key + " is required");
        }
        Object value = meta.get(key);
        long port;
        if (value instanceof Number number) {
            double decimal = number.doubleValue();
            port = number.longValue();
            if (!Double.isFinite(decimal) || decimal != port) {
                throw new IllegalArgumentException("meta." + key + " must be an integer port");
            }
        } else {
            try {
                port = Long.parseLong(String.valueOf(value).trim());
            } catch (NumberFormatException e) {
                throw new IllegalArgumentException("meta." + key + " must be an integer port");
            }
        }
        if (port < 1 || port > 65535) {
            throw new IllegalArgumentException("meta." + key + " must be between 1 and 65535");
        }
        return (int) port;
    }

    private String randomHex(int bytes) {
        byte[] buf = new byte[bytes];
        random.nextBytes(buf);
        return HexFormat.of().formatHex(buf);
    }

    private static String sha256Hex(String secret) {
        try {
            MessageDigest md = MessageDigest.getInstance("SHA-256");
            return HexFormat.of().formatHex(md.digest(secret.getBytes(StandardCharsets.UTF_8)));
        } catch (Exception e) {
            throw new IllegalStateException("sha256 unavailable", e);
        }
    }
}
