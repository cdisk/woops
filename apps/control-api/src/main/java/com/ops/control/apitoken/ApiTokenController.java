package com.ops.control.apitoken;

import com.ops.control.access.AccessService;
import com.ops.control.user.UserEntity;
import org.springframework.security.core.Authentication;
import org.springframework.web.bind.annotation.*;

import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.UUID;

@RestController
@RequestMapping("/api/profile/api-tokens")
public class ApiTokenController {
    private final ApiTokenService tokens;
    private final AccessService access;

    public ApiTokenController(ApiTokenService tokens, AccessService access) {
        this.tokens = tokens;
        this.access = access;
    }

    public record CreateBody(String name, List<String> scopes, String expiresAt) {}

    @GetMapping
    public List<Map<String, Object>> list(Authentication auth) {
        UserEntity user = access.requireUser(auth);
        return tokens.list(user);
    }

    @PostMapping
    public Map<String, Object> create(@RequestBody CreateBody body, Authentication auth) {
        UserEntity user = access.requireUser(auth);
        Instant expiresAt = null;
        if (body != null && body.expiresAt() != null && !body.expiresAt().isBlank()) {
            try {
                expiresAt = Instant.parse(body.expiresAt());
            } catch (Exception e) {
                throw new IllegalArgumentException("invalid expiresAt");
            }
        }
        String name = body == null ? null : body.name();
        List<String> scopes = body == null ? null : body.scopes();
        return tokens.create(user, name, scopes, expiresAt);
    }

    @DeleteMapping("/{id}")
    public Map<String, String> delete(@PathVariable UUID id, Authentication auth) {
        UserEntity user = access.requireUser(auth);
        return tokens.delete(user, id);
    }
}
