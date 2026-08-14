package com.ops.control.user;

import com.ops.control.access.AccessService;
import org.springframework.security.core.Authentication;
import org.springframework.web.bind.annotation.*;

import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.UUID;

@RestController
@RequestMapping("/api/users")
public class UserController {
    private final UserService users;
    private final AccessService access;

    public UserController(UserService users, AccessService access) {
        this.users = users;
        this.access = access;
    }

    @GetMapping
    public List<Map<String, Object>> list(Authentication auth) {
        return users.list(access.requireUser(auth));
    }

    public record CreateRequest(String username, String nickname, String password, String role) {}

    @PostMapping
    public Map<String, Object> create(@RequestBody CreateRequest req, Authentication auth) {
        return users.create(access.requireUser(auth), req.username(), req.nickname(), req.password(), req.role());
    }

    public record UpdateRequest(String nickname, String role, Boolean enabled, String password) {}

    @PatchMapping("/{id}")
    public Map<String, Object> update(@PathVariable UUID id, @RequestBody UpdateRequest req, Authentication auth) {
        return users.update(access.requireUser(auth), id, req.nickname(), req.role(), req.enabled(), req.password());
    }

    @PostMapping("/{id}/totp/reset")
    public Map<String, Object> resetTotp(@PathVariable UUID id, Authentication auth) {
        return users.resetTotp(access.requireUser(auth), id);
    }

    public record ScopeItem(String type, UUID id) {}

    public record ScopesRequest(List<ScopeItem> scopes) {}

    @PutMapping("/{id}/scopes")
    public Map<String, Object> setScopes(@PathVariable UUID id, @RequestBody ScopesRequest req, Authentication auth) {
        List<UserService.ScopeEntry> entries = req.scopes() == null
                ? List.of()
                : req.scopes().stream()
                        .filter(Objects::nonNull)
                        .map(s -> new UserService.ScopeEntry(s.type(), s.id()))
                        .toList();
        return users.setScopes(access.requireUser(auth), id, entries);
    }

    @DeleteMapping("/{id}")
    public Map<String, Object> delete(@PathVariable UUID id, Authentication auth) {
        return users.softDelete(access.requireUser(auth), id);
    }
}
