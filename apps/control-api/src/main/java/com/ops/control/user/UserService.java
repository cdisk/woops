package com.ops.control.user;

import com.ops.control.access.AccessService;
import com.ops.control.access.ForbiddenException;
import com.ops.control.asset.AssetEntity;
import com.ops.control.asset.AssetRepository;
import com.ops.control.controlaudit.ControlAuditService;
import com.ops.control.group.ServerGroupRepository;
import org.springframework.security.crypto.password.PasswordEncoder;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.Instant;
import java.util.*;
import java.util.stream.Collectors;

@Service
public class UserService {
    private final UserRepository users;
    private final UserScopeRepository scopes;
    private final ServerGroupRepository groups;
    private final AssetRepository assets;
    private final AccessService access;
    private final PasswordEncoder passwordEncoder;
    private final ControlAuditService audit;

    public UserService(
            UserRepository users,
            UserScopeRepository scopes,
            ServerGroupRepository groups,
            AssetRepository assets,
            AccessService access,
            PasswordEncoder passwordEncoder,
            ControlAuditService audit) {
        this.users = users;
        this.scopes = scopes;
        this.groups = groups;
        this.assets = assets;
        this.access = access;
        this.passwordEncoder = passwordEncoder;
        this.audit = audit;
    }

    @Transactional(readOnly = true)
    public List<Map<String, Object>> list(UserEntity actor) {
        if (!Roles.isSuperAdmin(actor.getRole()) && !Roles.isAdmin(actor.getRole())) {
            throw new ForbiddenException("cannot list users");
        }
        List<UserEntity> list = users.findAllActive();
        if (Roles.isAdmin(actor.getRole()) && !access.isSuperAdmin(actor)) {
            list = list.stream().filter(u -> Roles.isMember(u.getRole())).toList();
        }
        return list.stream().map(this::toView).toList();
    }

    @Transactional
    public Map<String, Object> create(UserEntity actor, String username, String nickname, String password, String role) {
        if (!Roles.isSuperAdmin(actor.getRole()) && !Roles.isAdmin(actor.getRole())) {
            throw new ForbiddenException("cannot create users");
        }
        String r = Roles.requireValid(role == null ? Roles.MEMBER : role);
        if (!access.isSuperAdmin(actor)) {
            if (!Roles.isMember(r)) {
                throw new ForbiddenException("admin can only create members");
            }
        }
        if (username == null || username.isBlank()) {
            throw new IllegalArgumentException("username required");
        }
        String name = username.trim();
        if (users.findActiveByUsername(name).isPresent()) {
            throw new IllegalArgumentException("username already exists");
        }
        if (password == null || password.isBlank()) {
            throw new IllegalArgumentException("password required for local user");
        }
        UserEntity u = new UserEntity();
        u.setUsername(name);
        u.setNickname(normalizeNickname(nickname));
        u.setPasswordHash(passwordEncoder.encode(password));
        u.setRole(r);
        u.setAuthSource("local");
        u.setEnabled(true);
        users.save(u);
        audit.record(
                ControlAuditService.CAT_USER,
                ControlAuditService.ACT_CREATE,
                actor.getId(),
                actor.getUsername(),
                null,
                null,
                ControlAuditService.jsonDetail(Map.of(
                        "targetUserId", u.getId().toString(),
                        "targetUsername", u.getUsername(),
                        "role", u.getRole()
                )));
        return toView(u);
    }

    @Transactional
    public Map<String, Object> update(
            UserEntity actor, UUID id, String nickname, String role, Boolean enabled, String password) {
        UserEntity target = requireActive(id);
        access.assertCanManageUser(actor, target);

        Map<String, Object> changes = new LinkedHashMap<>();
        changes.put("targetUserId", target.getId().toString());
        changes.put("targetUsername", target.getUsername());
        if (nickname != null) {
            String n = normalizeNickname(nickname);
            changes.put("nickname", n == null ? "" : n);
            target.setNickname(n);
        }
        if (role != null && !role.isBlank()) {
            String r = Roles.requireValid(role);
            if (!access.isSuperAdmin(actor)) {
                throw new ForbiddenException("only super admin can change roles");
            }
            if (access.isSuperAdmin(target) && !Roles.isSuperAdmin(r)
                    && users.countActiveEnabledByRole(Roles.SUPER_ADMIN) <= 1) {
                throw new IllegalArgumentException("cannot demote the last super admin");
            }
            changes.put("role", r);
            target.setRole(r);
        }
        if (enabled != null) {
            if (actor.getId().equals(target.getId()) && !enabled) {
                throw new IllegalArgumentException("cannot disable yourself");
            }
            if (access.isSuperAdmin(target) && !enabled
                    && users.countActiveEnabledByRole(Roles.SUPER_ADMIN) <= 1) {
                throw new IllegalArgumentException("cannot disable the last super admin");
            }
            changes.put("enabled", enabled);
            target.setEnabled(enabled);
        }
        if (password != null && !password.isBlank()) {
            if (!"local".equals(target.getAuthSource())) {
                throw new IllegalArgumentException("cannot set password for GitLab user");
            }
            changes.put("passwordChanged", true);
            target.setPasswordHash(passwordEncoder.encode(password));
        }
        users.save(target);
        if (changes.size() > 2) {
            audit.record(
                    ControlAuditService.CAT_USER,
                    ControlAuditService.ACT_UPDATE,
                    actor.getId(),
                    actor.getUsername(),
                    null,
                    null,
                    ControlAuditService.jsonDetail(changes));
        }
        return toView(target);
    }

    /** Clear TOTP so the next local password login must re-enroll (lockout recovery). */
    @Transactional
    public Map<String, Object> resetTotp(UserEntity actor, UUID id) {
        UserEntity target = requireActive(id);
        access.assertCanManageUser(actor, target);
        if (!"local".equals(target.getAuthSource())) {
            throw new IllegalArgumentException("totp only applies to local users");
        }
        if (!target.isTotpEnabled() && (target.getTotpSecret() == null || target.getTotpSecret().isBlank())) {
            return toView(target);
        }
        target.setTotpEnabled(false);
        target.setTotpSecret(null);
        target.setTotpLastStep(null);
        users.save(target);
        audit.record(
                ControlAuditService.CAT_USER,
                ControlAuditService.ACT_TOTP_RESET,
                actor.getId(),
                actor.getUsername(),
                null,
                null,
                ControlAuditService.jsonDetail(Map.of(
                        "targetUserId", target.getId().toString(),
                        "targetUsername", target.getUsername()
                )));
        return toView(target);
    }

    @Transactional
    public Map<String, Object> softDelete(UserEntity actor, UUID id) {
        UserEntity target = requireActive(id);
        access.assertCanManageUser(actor, target);
        if (actor.getId().equals(target.getId())) {
            throw new IllegalArgumentException("cannot delete yourself");
        }
        if (access.isSuperAdmin(target) && users.countActiveEnabledByRole(Roles.SUPER_ADMIN) <= 1) {
            throw new IllegalArgumentException("cannot delete the last super admin");
        }
        String originalUsername = target.getUsername();
        scopes.deleteByUserId(target.getId());
        target.setEnabled(false);
        target.setDeletedAt(Instant.now());
        target.setUsername(tombstoneUsername(originalUsername, target.getId()));
        users.save(target);
        audit.record(
                ControlAuditService.CAT_USER,
                ControlAuditService.ACT_REMOVE,
                actor.getId(),
                actor.getUsername(),
                null,
                null,
                ControlAuditService.jsonDetail(Map.of(
                        "targetUserId", target.getId().toString(),
                        "targetUsername", originalUsername,
                        "softDelete", true
                )));
        return Map.of("ok", true, "id", id.toString());
    }

    public record ScopeEntry(String type, UUID id) {}

    @Transactional
    public Map<String, Object> setScopes(UserEntity actor, UUID id, List<ScopeEntry> entries) {
        UserEntity target = requireActive(id);
        access.assertCanManageUser(actor, target);
        if (access.isSuperAdmin(target)) {
            throw new IllegalArgumentException("super admin has unrestricted scope");
        }
        List<ScopeEntry> normalized = normalizeEntries(entries);
        List<UUID> groupIds = normalized.stream()
                .filter(e -> ScopeTypes.GROUP.equals(e.type()))
                .map(ScopeEntry::id)
                .toList();
        List<UUID> assetIds = normalized.stream()
                .filter(e -> ScopeTypes.ASSET.equals(e.type()))
                .map(ScopeEntry::id)
                .toList();
        access.assertCanAssignGroupScopes(actor, groupIds);
        access.assertCanAssignAssetScopes(actor, assetIds);
        for (UUID gid : groupIds) {
            groups.findById(gid).orElseThrow(() -> new IllegalArgumentException("group not found: " + gid));
        }

        // Drop ASSET rows already covered by a GROUP scope subtree.
        Set<UUID> coveredGroups = expandGroupIds(groupIds);
        List<ScopeEntry> stored = new ArrayList<>();
        for (ScopeEntry e : normalized) {
            if (ScopeTypes.GROUP.equals(e.type())) {
                stored.add(e);
                continue;
            }
            AssetEntity a = assets.findById(e.id()).orElseThrow();
            if (a.getGroupId() != null && coveredGroups.contains(a.getGroupId())) {
                continue;
            }
            stored.add(e);
        }

        scopes.deleteByUserId(target.getId());
        for (ScopeEntry e : stored) {
            UserScopeEntity s = new UserScopeEntity();
            s.setUserId(target.getId());
            s.setScopeType(e.type());
            s.setScopeId(e.id());
            scopes.save(s);
        }
        audit.record(
                ControlAuditService.CAT_USER,
                ControlAuditService.ACT_SET_SCOPES,
                actor.getId(),
                actor.getUsername(),
                null,
                null,
                ControlAuditService.jsonDetail(Map.of(
                        "targetUserId", target.getId().toString(),
                        "targetUsername", target.getUsername(),
                        "scopes", stored.stream()
                                .map(e -> Map.of("type", e.type(), "id", e.id().toString()))
                                .toList()
                )));
        return toView(target);
    }

    private List<ScopeEntry> normalizeEntries(List<ScopeEntry> entries) {
        if (entries == null || entries.isEmpty()) {
            return List.of();
        }
        LinkedHashMap<String, ScopeEntry> uniq = new LinkedHashMap<>();
        for (ScopeEntry e : entries) {
            if (e == null || e.id() == null) continue;
            String type = ScopeTypes.requireValid(e.type());
            uniq.put(type + ":" + e.id(), new ScopeEntry(type, e.id()));
        }
        return List.copyOf(uniq.values());
    }

    private Set<UUID> expandGroupIds(Collection<UUID> roots) {
        if (roots == null || roots.isEmpty()) return Set.of();
        Map<UUID, List<UUID>> children = new HashMap<>();
        for (var g : groups.findAll()) {
            children.computeIfAbsent(g.getParentId(), k -> new ArrayList<>()).add(g.getId());
        }
        Set<UUID> out = new HashSet<>();
        Deque<UUID> q = new ArrayDeque<>(roots);
        while (!q.isEmpty()) {
            UUID id = q.poll();
            if (!out.add(id)) continue;
            for (UUID c : children.getOrDefault(id, List.of())) {
                q.add(c);
            }
        }
        return out;
    }

    private Map<String, Object> toView(UserEntity u) {
        Map<String, Object> m = new LinkedHashMap<>();
        m.put("id", u.getId().toString());
        m.put("username", u.getUsername());
        m.put("nickname", u.getNickname() == null ? "" : u.getNickname());
        m.put("role", u.getRole());
        m.put("authSource", u.getAuthSource());
        m.put("enabled", u.isEnabled());
        m.put("totpEnabled", u.isTotpEnabled());
        m.put("createdAt", u.getCreatedAt() == null ? "" : u.getCreatedAt().toString());
        List<Map<String, String>> scopeViews = scopes.findByUserId(u.getId()).stream()
                .sorted(Comparator
                        .comparing(UserScopeEntity::getScopeType)
                        .thenComparing(s -> s.getScopeId().toString()))
                .map(s -> {
                    Map<String, String> row = new LinkedHashMap<>();
                    row.put("type", s.getScopeType());
                    row.put("id", s.getScopeId().toString());
                    return row;
                })
                .collect(Collectors.toList());
        m.put("scopes", scopeViews);
        return m;
    }

    private UserEntity requireActive(UUID id) {
        UserEntity u = users.findById(id).orElseThrow(() -> new IllegalArgumentException("user not found"));
        if (u.isDeleted()) {
            throw new IllegalArgumentException("user not found");
        }
        return u;
    }

    static String tombstoneUsername(String original, UUID id) {
        String suffix = "#d#" + id.toString().replace("-", "");
        String base = original == null ? "user" : original;
        int maxBase = Math.max(1, 128 - suffix.length());
        if (base.length() > maxBase) {
            base = base.substring(0, maxBase);
        }
        return base + suffix;
    }

    static String normalizeNickname(String nickname) {
        if (nickname == null) return null;
        String t = nickname.trim();
        return t.isEmpty() ? null : t;
    }
}
