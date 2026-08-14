package com.ops.control.auth;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.ops.control.access.ForbiddenException;
import com.ops.control.common.OpsProperties;
import com.ops.control.user.Roles;
import com.ops.control.user.UserEntity;
import com.ops.control.user.UserRepository;
import io.jsonwebtoken.Claims;
import io.jsonwebtoken.Jwts;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;
import org.springframework.web.util.UriComponentsBuilder;

import java.net.URI;
import java.net.URLEncoder;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.time.Instant;
import java.util.Date;
import java.util.Map;
import java.util.UUID;

@Service
public class GitLabOAuthService {
    private final OpsProperties props;
    private final AuthService authService;
    private final UserRepository users;
    private final ObjectMapper mapper;
    private final HttpClient http = HttpClient.newBuilder().connectTimeout(Duration.ofSeconds(15)).build();

    public GitLabOAuthService(OpsProperties props, AuthService authService, UserRepository users, ObjectMapper mapper) {
        this.props = props;
        this.authService = authService;
        this.users = users;
        this.mapper = mapper;
    }

    public void requireEnabled() {
        if (!props.gitlabEnabled()) {
            throw new ForbiddenException("GitLab login is not configured");
        }
    }

    public String authorizeUrl() {
        requireEnabled();
        String state = issueState();
        return UriComponentsBuilder
                .fromUriString(props.gitlabBaseUrl() + "/oauth/authorize")
                .queryParam("client_id", props.gitlab().clientId())
                .queryParam("redirect_uri", props.gitlabRedirectUri())
                .queryParam("response_type", "code")
                .queryParam("scope", "read_user")
                .queryParam("state", state)
                .encode()
                .build()
                .toUriString();
    }

    private String issueState() {
        Instant now = Instant.now();
        return Jwts.builder()
                .subject("gitlab-oauth")
                .id(UUID.randomUUID().toString())
                .issuedAt(Date.from(now))
                .expiration(Date.from(now.plusSeconds(600)))
                .signWith(authService.jwtKey())
                .compact();
    }

    private void verifyState(String state) {
        if (state == null || state.isBlank()) {
            throw new IllegalArgumentException("missing state");
        }
        Claims claims = authService.parse(state);
        if (!"gitlab-oauth".equals(claims.getSubject())) {
            throw new IllegalArgumentException("invalid state");
        }
    }

    @Transactional
    public Map<String, Object> handleCallback(String code, String state) {
        requireEnabled();
        verifyState(state);
        if (code == null || code.isBlank()) {
            throw new IllegalArgumentException("missing code");
        }
        String accessToken = exchangeCode(code);
        GitLabUser gu = fetchUser(accessToken);
        UserEntity user = findOrCreate(gu);
        if (!user.isEnabled()) {
            throw new ForbiddenException("user disabled");
        }
        return authService.tokenResponse(user);
    }

    public String consoleRedirectWithToken(String accessToken) {
        return props.consoleBase() + "/login?token=" + URLEncoder.encode(accessToken, StandardCharsets.UTF_8);
    }

    private String exchangeCode(String code) {
        String body = "client_id=" + enc(props.gitlab().clientId())
                + "&client_secret=" + enc(props.gitlab().clientSecret())
                + "&code=" + enc(code)
                + "&grant_type=authorization_code"
                + "&redirect_uri=" + enc(props.gitlabRedirectUri());
        try {
            HttpRequest req = HttpRequest.newBuilder()
                    .uri(URI.create(props.gitlabBaseUrl() + "/oauth/token"))
                    .timeout(Duration.ofSeconds(20))
                    .header("Content-Type", "application/x-www-form-urlencoded")
                    .header("Accept", "application/json")
                    .POST(HttpRequest.BodyPublishers.ofString(body))
                    .build();
            HttpResponse<String> resp = http.send(req, HttpResponse.BodyHandlers.ofString());
            if (resp.statusCode() >= 300) {
                throw new IllegalStateException("GitLab token exchange failed: HTTP " + resp.statusCode());
            }
            JsonNode node = mapper.readTree(resp.body());
            String token = node.path("access_token").asText(null);
            if (token == null || token.isBlank()) {
                throw new IllegalStateException("GitLab token missing in response");
            }
            return token;
        } catch (IllegalStateException e) {
            throw e;
        } catch (Exception e) {
            throw new IllegalStateException("GitLab token exchange error: " + e.getMessage(), e);
        }
    }

    private GitLabUser fetchUser(String accessToken) {
        try {
            HttpRequest req = HttpRequest.newBuilder()
                    .uri(URI.create(props.gitlabBaseUrl() + "/api/v4/user"))
                    .timeout(Duration.ofSeconds(20))
                    .header("Authorization", "Bearer " + accessToken)
                    .header("Accept", "application/json")
                    .GET()
                    .build();
            HttpResponse<String> resp = http.send(req, HttpResponse.BodyHandlers.ofString());
            if (resp.statusCode() >= 300) {
                throw new IllegalStateException("GitLab user fetch failed: HTTP " + resp.statusCode());
            }
            JsonNode node = mapper.readTree(resp.body());
            String username = node.path("username").asText(null);
            if (username == null || username.isBlank()) {
                throw new IllegalStateException("GitLab username missing");
            }
            return new GitLabUser(username.trim(), node.path("name").asText(""));
        } catch (IllegalStateException e) {
            throw e;
        } catch (Exception e) {
            throw new IllegalStateException("GitLab user fetch error: " + e.getMessage(), e);
        }
    }

    private UserEntity findOrCreate(GitLabUser gu) {
        boolean designatedAdmin = props.isGitlabAdminUser(gu.username());
        String nick = gu.name() == null ? "" : gu.name().trim();
        return users.findActiveByUsername(gu.username()).map(existing -> {
            boolean dirty = false;
            if (!"gitlab".equals(existing.getAuthSource())) {
                existing.setAuthSource("gitlab");
                dirty = true;
            }
            // OPS_GITLAB_ADMIN_USER is the source of super-admin when GitLab is on (promote only, no auto-demote).
            if (designatedAdmin && !Roles.isSuperAdmin(existing.getRole())) {
                existing.setRole(Roles.SUPER_ADMIN);
                dirty = true;
            }
            if ((existing.getNickname() == null || existing.getNickname().isBlank()) && !nick.isEmpty()) {
                existing.setNickname(nick);
                dirty = true;
            }
            // Do NOT re-enable a disabled user on GitLab login.
            return dirty ? users.save(existing) : existing;
        }).orElseGet(() -> {
            UserEntity u = new UserEntity();
            u.setUsername(gu.username());
            if (!nick.isEmpty()) {
                u.setNickname(nick);
            }
            u.setPasswordHash(null);
            u.setAuthSource("gitlab");
            u.setEnabled(true);
            u.setRole(designatedAdmin ? Roles.SUPER_ADMIN : Roles.MEMBER);
            return users.save(u);
        });
    }

    private static String enc(String s) {
        return URLEncoder.encode(s, StandardCharsets.UTF_8);
    }

    private record GitLabUser(String username, String name) {}
}
