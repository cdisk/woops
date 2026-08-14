package com.ops.control.auth;

import com.ops.control.access.ForbiddenException;
import com.ops.control.controlaudit.ControlAuditService;
import com.ops.control.common.OpsProperties;
import com.ops.control.user.Roles;
import com.ops.control.user.UserEntity;
import com.ops.control.user.UserRepository;
import io.jsonwebtoken.Claims;
import io.jsonwebtoken.Jwts;
import io.jsonwebtoken.security.Keys;
import jakarta.annotation.PostConstruct;
import org.springframework.security.crypto.password.PasswordEncoder;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import javax.crypto.SecretKey;
import java.nio.charset.StandardCharsets;
import java.time.Instant;
import java.util.Date;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.UUID;

@Service
public class AuthService {
    public static final String PURPOSE_ACCESS = "access";
    public static final String PURPOSE_TOTP_VERIFY = "totp_verify";
    public static final String PURPOSE_TOTP_SETUP = "totp_setup";

    private static final long PENDING_TTL_SECONDS = 10 * 60;
    private static final long ACCESS_TTL_SECONDS = 12 * 3600;

    private final UserRepository users;
    private final PasswordEncoder passwordEncoder;
    private final OpsProperties props;
    private final ControlAuditService audit;
    private final TotpService totp;
    private SecretKey jwtKey;

    public AuthService(
            UserRepository users,
            PasswordEncoder passwordEncoder,
            OpsProperties props,
            ControlAuditService audit,
            TotpService totp) {
        this.users = users;
        this.passwordEncoder = passwordEncoder;
        this.props = props;
        this.audit = audit;
        this.totp = totp;
    }

    @PostConstruct
    void bootstrapAdmin() {
        jwtKey = Keys.hmacShaKeyFor(props.jwtSecret().getBytes(StandardCharsets.UTF_8));
        var existing = users.findActiveByUsername(props.bootstrapAdminUsername());
        if (existing.isEmpty()) {
            UserEntity admin = new UserEntity();
            admin.setUsername(props.bootstrapAdminUsername());
            admin.setPasswordHash(passwordEncoder.encode(props.bootstrapAdminPassword()));
            admin.setRole(Roles.SUPER_ADMIN);
            admin.setAuthSource("local");
            users.save(admin);
        } else {
            UserEntity admin = existing.get();
            // Legacy break-glass role was "ADMIN"
            if (!Roles.isSuperAdmin(admin.getRole())) {
                admin.setRole(Roles.SUPER_ADMIN);
                users.save(admin);
            }
        }
    }

    public Map<String, Object> loginOptions() {
        Map<String, Object> out = new LinkedHashMap<>();
        out.put("localLoginEnabled", props.localLoginEnabled());
        out.put("gitlabEnabled", props.gitlabEnabled());
        if (props.gitlabEnabled()) {
            out.put("gitlabAuthorizePath", "/api/auth/gitlab/authorize");
        }
        return out;
    }

    /**
     * Local password login. When TOTP is not yet enrolled, returns setup payload;
     * when enrolled, returns a short-lived pending token for code verification.
     */
    @Transactional(readOnly = true)
    public Map<String, Object> login(String username, String password) {
        if (!props.localLoginEnabled()) {
            throw new ForbiddenException("local login disabled; use GitLab");
        }
        UserEntity user = users.findActiveByUsername(username)
                .orElseThrow(() -> new IllegalArgumentException("invalid username or password"));
        if (!user.isEnabled() || user.getPasswordHash() == null
                || !passwordEncoder.matches(password, user.getPasswordHash())) {
            throw new IllegalArgumentException("invalid username or password");
        }
        if (!"local".equals(user.getAuthSource())) {
            throw new IllegalArgumentException("invalid username or password");
        }

        if (user.isTotpEnabled() && user.getTotpSecret() != null && !user.getTotpSecret().isBlank()) {
            Map<String, Object> out = new LinkedHashMap<>();
            out.put("requiresTotp", true);
            out.put("pendingToken", issuePendingJwt(user, PURPOSE_TOTP_VERIFY, null));
            return out;
        }

        String secret = totp.generateSecret();
        Map<String, Object> out = new LinkedHashMap<>();
        out.put("requiresTotpSetup", true);
        out.put("pendingToken", issuePendingJwt(user, PURPOSE_TOTP_SETUP, secret));
        out.put("secret", secret);
        out.put("otpauthUri", totp.otpauthUri(user.getUsername(), secret));
        String qr = totp.qrDataUrl(user.getUsername(), secret);
        if (qr != null && !qr.isBlank()) {
            out.put("qrCodeDataUrl", qr);
        }
        return out;
    }

    @Transactional
    public Map<String, Object> completeTotpLogin(String pendingToken, String code) {
        Claims claims = requirePending(pendingToken, PURPOSE_TOTP_VERIFY);
        UserEntity user = requireLocalUser(UUID.fromString(claims.getSubject()));
        if (!user.isTotpEnabled() || user.getTotpSecret() == null || user.getTotpSecret().isBlank()) {
            throw new IllegalArgumentException("totp not enabled");
        }
        consumeTotpCode(user, user.getTotpSecret(), code);
        users.save(user);
        return tokenResponse(user);
    }

    @Transactional
    public Map<String, Object> completeTotpSetup(String pendingToken, String code) {
        Claims claims = requirePending(pendingToken, PURPOSE_TOTP_SETUP);
        String secret = claims.get("totpSecret", String.class);
        if (secret == null || secret.isBlank()) {
            throw new IllegalArgumentException("invalid pending token");
        }
        UserEntity user = requireLocalUser(UUID.fromString(claims.getSubject()));
        if (user.isTotpEnabled()) {
            throw new IllegalStateException("totp already enabled");
        }
        consumeTotpCode(user, secret, code);
        user.setTotpSecret(secret);
        user.setTotpEnabled(true);
        users.save(user);
        audit.record(
                ControlAuditService.CAT_AUTH,
                ControlAuditService.ACT_TOTP_ENABLE,
                user.getId(),
                user.getUsername(),
                null,
                null,
                ControlAuditService.jsonDetail(Map.of("via", "login_setup")));
        return tokenResponse(user);
    }

    /** Validate code within skew window and reject replay of an already-used time-step. */
    private void consumeTotpCode(UserEntity user, String secret, String code) {
        var matched = totp.matchingStep(secret, code);
        if (matched.isEmpty()) {
            throw new IllegalArgumentException("invalid totp code");
        }
        long step = matched.getAsLong();
        Long last = user.getTotpLastStep();
        if (last != null && step <= last) {
            throw new IllegalArgumentException("invalid totp code");
        }
        user.setTotpLastStep(step);
    }

    public Map<String, Object> tokenResponse(UserEntity user) {
        audit.record(
                ControlAuditService.CAT_AUTH,
                ControlAuditService.ACT_LOGIN,
                user.getId(),
                user.getUsername(),
                null,
                null,
                ControlAuditService.jsonDetail(Map.of(
                        "authSource", user.getAuthSource() == null ? "local" : user.getAuthSource(),
                        "role", user.getRole() == null ? "" : user.getRole()
                )));
        String token = issueJwt(user);
        Map<String, Object> userView = new LinkedHashMap<>();
        userView.put("id", user.getId().toString());
        userView.put("username", user.getUsername());
        userView.put("nickname", user.getNickname() == null ? "" : user.getNickname());
        userView.put("role", user.getRole());
        userView.put("authSource", user.getAuthSource());
        userView.put("totpEnabled", user.isTotpEnabled());
        return Map.of(
                "accessToken", token,
                "tokenType", "Bearer",
                "user", userView
        );
    }

    public Map<String, Object> me(UserEntity user) {
        Map<String, Object> out = new LinkedHashMap<>();
        out.put("id", user.getId().toString());
        out.put("username", user.getUsername());
        out.put("nickname", user.getNickname() == null ? "" : user.getNickname());
        out.put("role", user.getRole());
        out.put("authSource", user.getAuthSource());
        out.put("enabled", user.isEnabled());
        out.put("totpEnabled", user.isTotpEnabled());
        out.put("capabilities", capabilities(user));
        return out;
    }

    public Map<String, Object> capabilities(UserEntity user) {
        Map<String, Object> c = new LinkedHashMap<>();
        c.put("manageUsers", Roles.isSuperAdmin(user.getRole()) || Roles.isAdmin(user.getRole()));
        c.put("manageInventory", Roles.canManageInventory(user.getRole()));
        c.put("manageAlertRules", Roles.isSuperAdmin(user.getRole()));
        c.put("superAdmin", Roles.isSuperAdmin(user.getRole()));
        return c;
    }

    public String issueJwt(UserEntity user) {
        Instant now = Instant.now();
        return Jwts.builder()
                .subject(user.getId().toString())
                .claims(Map.of(
                        "username", user.getUsername(),
                        "role", user.getRole(),
                        "authSource", user.getAuthSource() == null ? "local" : user.getAuthSource(),
                        "purpose", PURPOSE_ACCESS
                ))
                .issuedAt(Date.from(now))
                .expiration(Date.from(now.plusSeconds(ACCESS_TTL_SECONDS)))
                .signWith(jwtKey)
                .compact();
    }

    private String issuePendingJwt(UserEntity user, String purpose, String totpSecret) {
        Instant now = Instant.now();
        var builder = Jwts.builder()
                .subject(user.getId().toString())
                .claim("username", user.getUsername())
                .claim("role", user.getRole())
                .claim("authSource", user.getAuthSource() == null ? "local" : user.getAuthSource())
                .claim("purpose", purpose)
                .issuedAt(Date.from(now))
                .expiration(Date.from(now.plusSeconds(PENDING_TTL_SECONDS)))
                .signWith(jwtKey);
        if (totpSecret != null && !totpSecret.isBlank()) {
            builder.claim("totpSecret", totpSecret);
        }
        return builder.compact();
    }

    private Claims requirePending(String token, String expectedPurpose) {
        if (token == null || token.isBlank()) {
            throw new IllegalArgumentException("pending token required");
        }
        Claims claims;
        try {
            claims = parse(token);
        } catch (Exception e) {
            throw new IllegalArgumentException("invalid or expired pending token");
        }
        String purpose = claims.get("purpose", String.class);
        if (!expectedPurpose.equals(purpose)) {
            throw new IllegalArgumentException("invalid pending token");
        }
        return claims;
    }

    private UserEntity requireLocalUser(UUID id) {
        UserEntity user = users.findById(id)
                .orElseThrow(() -> new IllegalArgumentException("invalid pending token"));
        if (user.isDeleted() || !user.isEnabled()) {
            throw new IllegalArgumentException("invalid pending token");
        }
        if (!"local".equals(user.getAuthSource())) {
            throw new IllegalArgumentException("invalid pending token");
        }
        return user;
    }

    public Claims parse(String token) {
        return Jwts.parser().verifyWith(jwtKey).build().parseSignedClaims(token).getPayload();
    }

    /** True if this JWT is a full session token (not a TOTP pending challenge). */
    public boolean isAccessToken(Claims claims) {
        if (claims == null) {
            return false;
        }
        String purpose = claims.get("purpose", String.class);
        // Legacy tokens issued before purpose claim are treated as access.
        return purpose == null || PURPOSE_ACCESS.equals(purpose);
    }

    public UUID requireUserId(String token) {
        return UUID.fromString(parse(token).getSubject());
    }

    public SecretKey jwtKey() {
        return jwtKey;
    }
}
