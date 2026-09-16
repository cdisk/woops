package com.ops.control.apitoken;

import com.ops.control.controlaudit.ControlAuditService;
import com.ops.control.user.UserEntity;
import com.ops.control.user.UserRepository;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.security.MessageDigest;
import java.security.SecureRandom;
import java.time.Duration;
import java.time.Instant;
import java.util.HexFormat;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.UUID;

@Service
public class ApiTokenService {
    public static final String TOKEN_PREFIX = "wpat_";
    private static final Duration LAST_USED_THROTTLE = Duration.ofHours(1);

    private final ApiTokenRepository tokens;
    private final UserRepository users;
    private final ControlAuditService audit;
    private final SecureRandom random = new SecureRandom();

    public ApiTokenService(ApiTokenRepository tokens, UserRepository users, ControlAuditService audit) {
        this.tokens = tokens;
        this.users = users;
        this.audit = audit;
    }

    @Transactional
    public Map<String, Object> create(UserEntity user, String name, List<String> scopes, Instant expiresAt) {
        String trimmed = name == null ? "" : name.trim();
        if (trimmed.isEmpty() || trimmed.length() > 128) {
            throw new IllegalArgumentException("name required (max 128)");
        }
        Set<String> scopeSet = ApiTokenScopes.parseAndValidate(scopes);
        Instant now = Instant.now();
        Instant exp = expiresAt;
        if (exp != null) {
            if (!exp.isAfter(now)) {
                throw new IllegalArgumentException("expiresAt must be in the future");
            }
        }

        ApiTokenEntity entity = new ApiTokenEntity();
        entity.setUserId(user.getId());
        entity.setName(trimmed);
        entity.setScopes(ApiTokenScopes.toStored(scopeSet));
        entity.setExpiresAt(exp);
        entity.setCreatedAt(now);
        entity.setSecretHash("pending");
        entity = tokens.save(entity);

        String secret = randomHex(32);
        entity.setSecretHash(sha256Hex(secret));
        tokens.save(entity);

        String plaintext = TOKEN_PREFIX + entity.getId() + "_" + secret;
        Map<String, Object> detail = new LinkedHashMap<>();
        detail.put("tokenId", entity.getId().toString());
        detail.put("name", entity.getName());
        detail.put("scopes", scopeSet);
        detail.put("expiresAt", exp == null ? "" : exp.toString());
        audit.record(
                ControlAuditService.CAT_API_TOKEN,
                ControlAuditService.ACT_CREATE,
                user.getId(),
                user.getUsername(),
                null,
                null,
                ControlAuditService.jsonDetail(detail));

        Map<String, Object> out = toView(entity);
        out.put("token", plaintext);
        return out;
    }

    @Transactional(readOnly = true)
    public List<Map<String, Object>> list(UserEntity user) {
        return tokens.findByUserIdOrderByCreatedAtDesc(user.getId()).stream().map(this::toView).toList();
    }

    @Transactional
    public Map<String, String> delete(UserEntity user, UUID tokenId) {
        ApiTokenEntity entity = tokens.findById(tokenId)
                .orElseThrow(() -> new IllegalArgumentException("token not found"));
        if (!entity.getUserId().equals(user.getId())) {
            throw new IllegalArgumentException("token not found");
        }
        Map<String, Object> detail = new LinkedHashMap<>();
        detail.put("tokenId", entity.getId().toString());
        detail.put("name", entity.getName());
        detail.put("scopes", ApiTokenScopes.fromStored(entity.getScopes()));
        tokens.delete(entity);
        audit.record(
                ControlAuditService.CAT_API_TOKEN,
                ControlAuditService.ACT_REMOVE,
                user.getId(),
                user.getUsername(),
                null,
                null,
                ControlAuditService.jsonDetail(detail));
        return Map.of("status", "deleted", "id", tokenId.toString());
    }

    /**
     * Resolve a plaintext Bearer token. Returns null if not an API token or invalid.
     * Does not update lastUsedAt (caller should touch asynchronously / throttled).
     */
    @Transactional(readOnly = true)
    public ResolvedToken resolve(String bearer) {
        if (bearer == null || !bearer.startsWith(TOKEN_PREFIX)) {
            return null;
        }
        String rest = bearer.substring(TOKEN_PREFIX.length());
        int us = rest.indexOf('_');
        if (us <= 0 || us >= rest.length() - 1) {
            return null;
        }
        UUID id;
        try {
            id = UUID.fromString(rest.substring(0, us));
        } catch (Exception e) {
            return null;
        }
        String secret = rest.substring(us + 1);
        ApiTokenEntity entity = tokens.findById(id).orElse(null);
        if (entity == null) {
            return null;
        }
        if (!constantTimeEquals(entity.getSecretHash(), sha256Hex(secret))) {
            return null;
        }
        Instant now = Instant.now();
        if (entity.getExpiresAt() != null && !entity.getExpiresAt().isAfter(now)) {
            return null;
        }
        UserEntity user = users.findById(entity.getUserId()).orElse(null);
        if (user == null || user.isDeleted() || !user.isEnabled()) {
            return null;
        }
        return new ResolvedToken(
                user,
                new ApiTokenAuth(entity.getId(), entity.getName(), ApiTokenScopes.fromStored(entity.getScopes())));
    }

    @Transactional
    public void touchLastUsed(UUID tokenId) {
        Instant now = Instant.now();
        Instant throttleBefore = now.minus(LAST_USED_THROTTLE);
        tokens.touchLastUsedIfStale(tokenId, now, throttleBefore);
    }

    public record ResolvedToken(UserEntity user, ApiTokenAuth auth) {}

    private Map<String, Object> toView(ApiTokenEntity e) {
        Map<String, Object> m = new LinkedHashMap<>();
        m.put("id", e.getId().toString());
        m.put("name", e.getName());
        m.put("scopes", List.copyOf(ApiTokenScopes.fromStored(e.getScopes())));
        m.put("expiresAt", e.getExpiresAt() == null ? null : e.getExpiresAt().toString());
        m.put("createdAt", e.getCreatedAt().toString());
        m.put("lastUsedAt", e.getLastUsedAt() == null ? null : e.getLastUsedAt().toString());
        boolean expired = e.getExpiresAt() != null && !e.getExpiresAt().isAfter(Instant.now());
        m.put("expired", expired);
        return m;
    }

    private String randomHex(int bytes) {
        byte[] buf = new byte[bytes];
        random.nextBytes(buf);
        return HexFormat.of().formatHex(buf);
    }

    static String sha256Hex(String secret) {
        try {
            MessageDigest md = MessageDigest.getInstance("SHA-256");
            byte[] dig = md.digest(secret.getBytes(java.nio.charset.StandardCharsets.UTF_8));
            return HexFormat.of().formatHex(dig);
        } catch (Exception e) {
            throw new IllegalStateException("sha256", e);
        }
    }

    private static boolean constantTimeEquals(String a, String b) {
        if (a == null || b == null) return false;
        byte[] x = a.getBytes(java.nio.charset.StandardCharsets.UTF_8);
        byte[] y = b.getBytes(java.nio.charset.StandardCharsets.UTF_8);
        if (x.length != y.length) return false;
        int r = 0;
        for (int i = 0; i < x.length; i++) r |= x[i] ^ y[i];
        return r == 0;
    }
}
