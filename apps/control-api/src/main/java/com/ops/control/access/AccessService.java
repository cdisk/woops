package com.ops.control.access;

import com.ops.control.asset.AssetEntity;
import com.ops.control.asset.AssetRepository;
import com.ops.control.group.ServerGroupEntity;
import com.ops.control.group.ServerGroupRepository;
import com.ops.control.user.Roles;
import com.ops.control.user.ScopeTypes;
import com.ops.control.user.UserEntity;
import com.ops.control.user.UserRepository;
import com.ops.control.user.UserScopeEntity;
import com.ops.control.user.UserScopeRepository;
import org.springframework.security.core.Authentication;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.*;
import java.util.stream.Collectors;

@Service
public class AccessService {
    private final UserRepository users;
    private final UserScopeRepository scopes;
    private final ServerGroupRepository groups;
    private final AssetRepository assets;

    public AccessService(
            UserRepository users,
            UserScopeRepository scopes,
            ServerGroupRepository groups,
            AssetRepository assets) {
        this.users = users;
        this.scopes = scopes;
        this.groups = groups;
        this.assets = assets;
    }

    public UUID requireUserId(Authentication auth) {
        if (auth == null || auth.getName() == null || auth.getName().isBlank()) {
            throw new ForbiddenException("unauthorized");
        }
        return UUID.fromString(auth.getName());
    }

    @Transactional(readOnly = true)
    public UserEntity requireUser(Authentication auth) {
        UUID id = requireUserId(auth);
        UserEntity u = users.findById(id).orElseThrow(() -> new ForbiddenException("user not found"));
        if (u.isDeleted()) {
            throw new ForbiddenException("user not found");
        }
        if (!u.isEnabled()) {
            throw new ForbiddenException("user disabled");
        }
        return u;
    }

    public boolean isSuperAdmin(UserEntity u) {
        return Roles.isSuperAdmin(u.getRole());
    }

    public boolean canManageInventory(UserEntity u) {
        return Roles.canManageInventory(u.getRole());
    }

    @Transactional(readOnly = true)
    public List<UserScopeEntity> rawScopes(UserEntity u) {
        if (isSuperAdmin(u)) {
            return List.of();
        }
        return scopes.findByUserId(u.getId());
    }

    /** GROUP scope ids (not expanded). */
    @Transactional(readOnly = true)
    public List<UUID> groupScopeRoots(UserEntity u) {
        return rawScopes(u).stream()
                .filter(s -> ScopeTypes.GROUP.equals(s.getScopeType()))
                .map(UserScopeEntity::getScopeId)
                .toList();
    }

    /** ASSET scope ids. */
    @Transactional(readOnly = true)
    public Set<UUID> assetScopeIds(UserEntity u) {
        if (isSuperAdmin(u)) {
            return Set.of();
        }
        return rawScopes(u).stream()
                .filter(s -> ScopeTypes.ASSET.equals(s.getScopeType()))
                .map(UserScopeEntity::getScopeId)
                .collect(Collectors.toSet());
    }

    /**
     * Groups the user may manage (GROUP scopes + descendants). Super → all.
     */
    @Transactional(readOnly = true)
    public Set<UUID> manageableGroupIds(UserEntity u) {
        if (isSuperAdmin(u)) {
            return groups.findAll().stream().map(ServerGroupEntity::getId).collect(Collectors.toSet());
        }
        List<UUID> roots = groupScopeRoots(u);
        if (roots.isEmpty()) {
            return Set.of();
        }
        return expandDescendants(roots);
    }

    /**
     * Groups shown in navigation: manageable ∪ ancestors of ASSET-scoped assets.
     */
    @Transactional(readOnly = true)
    public Set<UUID> displayGroupIds(UserEntity u) {
        if (isSuperAdmin(u)) {
            return manageableGroupIds(u);
        }
        Set<UUID> out = new HashSet<>(manageableGroupIds(u));
        Set<UUID> assetIds = assetScopeIds(u);
        if (assetIds.isEmpty()) {
            return out;
        }
        Map<UUID, ServerGroupEntity> byId = groups.findAll().stream()
                .collect(Collectors.toMap(ServerGroupEntity::getId, g -> g, (a, b) -> a));
        for (AssetEntity a : assets.findAllById(assetIds)) {
            UUID gid = a.getGroupId();
            while (gid != null) {
                if (!out.add(gid)) break;
                ServerGroupEntity g = byId.get(gid);
                gid = g == null ? null : g.getParentId();
            }
        }
        return out;
    }

    private Set<UUID> expandDescendants(Collection<UUID> roots) {
        Map<UUID, List<UUID>> children = childrenMap();
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

    private Map<UUID, List<UUID>> childrenMap() {
        Map<UUID, List<UUID>> map = new HashMap<>();
        for (ServerGroupEntity g : groups.findAll()) {
            map.computeIfAbsent(g.getParentId(), k -> new ArrayList<>()).add(g.getId());
        }
        return map;
    }

    /** See group in nav / list under path (display). */
    public boolean canSeeGroup(UserEntity u, UUID groupId) {
        if (groupId == null) {
            return isSuperAdmin(u);
        }
        return displayGroupIds(u).contains(groupId);
    }

    /** Manage group (CRUD). */
    public boolean canManageGroup(UserEntity u, UUID groupId) {
        if (groupId == null) {
            return isSuperAdmin(u);
        }
        return manageableGroupIds(u).contains(groupId);
    }

    public boolean canAccessAsset(UserEntity u, AssetEntity asset) {
        if (asset == null) return false;
        if (isSuperAdmin(u)) return true;
        if (assetScopeIds(u).contains(asset.getId())) return true;
        UUID gid = asset.getGroupId();
        if (gid == null) return false;
        return manageableGroupIds(u).contains(gid);
    }

    /**
     * Asset ids the user can access (scan all assets + {@link #canAccessAsset}).
     * Super-admin returns every asset id.
     */
    @Transactional(readOnly = true)
    public Set<UUID> visibleAssetIds(UserEntity user) {
        Set<UUID> out = new HashSet<>();
        for (AssetEntity a : assets.findAll()) {
            if (canAccessAsset(user, a)) {
                out.add(a.getId());
            }
        }
        return out;
    }

    /**
     * Delete / inventory-manage this asset: inventory role + group manageable.
     * ASSET-only grant → false.
     */
    public boolean canDeleteAsset(UserEntity u, AssetEntity asset) {
        if (asset == null) return false;
        if (!canManageInventory(u)) return false;
        if (isSuperAdmin(u)) return true;
        UUID gid = asset.getGroupId();
        if (gid == null) return false;
        return manageableGroupIds(u).contains(gid);
    }

    public boolean canManageAsset(UserEntity u, AssetEntity asset) {
        return canDeleteAsset(u, asset);
    }

    public void assertCanAccessGroup(UserEntity u, UUID groupId) {
        if (!canSeeGroup(u, groupId)) {
            throw new ForbiddenException("group not in scope");
        }
    }

    public void assertCanManageGroup(UserEntity u, UUID groupId) {
        if (!canManageGroup(u, groupId)) {
            throw new ForbiddenException("group not manageable");
        }
    }

    public void assertCanAccessAsset(UserEntity u, AssetEntity asset) {
        if (!canAccessAsset(u, asset)) {
            throw new ForbiddenException("asset not in scope");
        }
    }

    public void assertCanDeleteAsset(UserEntity u, AssetEntity asset) {
        if (!canDeleteAsset(u, asset)) {
            throw new ForbiddenException("cannot delete asset");
        }
    }

    public void assertCanManageInventory(UserEntity u) {
        if (!canManageInventory(u)) {
            throw new ForbiddenException("requires admin");
        }
    }

    public void assertCanManageAlertRules(UserEntity u) {
        if (!isSuperAdmin(u)) {
            throw new ForbiddenException("requires super admin");
        }
    }

    /** Super can manage anyone; admin can only manage MEMBER. */
    public void assertCanManageUser(UserEntity actor, UserEntity target) {
        if (isSuperAdmin(actor)) {
            return;
        }
        if (!Roles.isAdmin(actor.getRole())) {
            throw new ForbiddenException("cannot manage users");
        }
        if (!Roles.isMember(target.getRole())) {
            throw new ForbiddenException("admin cannot modify this user");
        }
    }

    public void assertCanAssignGroupScopes(UserEntity actor, Collection<UUID> groupIds) {
        if (isSuperAdmin(actor)) {
            for (UUID gid : groupIds) {
                groups.findById(gid).orElseThrow(() -> new IllegalArgumentException("group not found: " + gid));
            }
            return;
        }
        Set<UUID> manageable = manageableGroupIds(actor);
        for (UUID gid : groupIds) {
            if (!manageable.contains(gid)) {
                throw new ForbiddenException("cannot assign group outside your manageable scope");
            }
        }
    }

    public void assertCanAssignAssetScopes(UserEntity actor, Collection<UUID> assetIds) {
        for (UUID aid : assetIds) {
            AssetEntity a = assets.findById(aid).orElseThrow(() -> new IllegalArgumentException("asset not found: " + aid));
            if (isSuperAdmin(actor)) {
                continue;
            }
            if (!canAccessAsset(actor, a)) {
                throw new ForbiddenException("cannot assign asset outside your scope");
            }
        }
    }

    /**
     * Prune tree to display groups; each kept node gets {@code manageable} boolean.
     */
    @Transactional(readOnly = true)
    public List<Map<String, Object>> pruneGroupTree(UserEntity u, List<Map<String, Object>> fullTree) {
        if (isSuperAdmin(u)) {
            return markManageable(fullTree, manageableGroupIds(u));
        }
        Set<UUID> display = displayGroupIds(u);
        if (display.isEmpty()) {
            return List.of();
        }
        Set<UUID> manageable = manageableGroupIds(u);
        List<Map<String, Object>> out = new ArrayList<>();
        collectVisible(fullTree, display, manageable, out);
        return out;
    }

    @SuppressWarnings("unchecked")
    private List<Map<String, Object>> markManageable(List<Map<String, Object>> nodes, Set<UUID> manageable) {
        if (nodes == null) return List.of();
        List<Map<String, Object>> out = new ArrayList<>();
        for (Map<String, Object> node : nodes) {
            Map<String, Object> copy = new LinkedHashMap<>(node);
            UUID id = UUID.fromString(String.valueOf(node.get("id")));
            copy.put("manageable", manageable.contains(id));
            List<Map<String, Object>> children = (List<Map<String, Object>>) node.get("children");
            copy.put("children", markManageable(children, manageable));
            out.add(copy);
        }
        return out;
    }

    @SuppressWarnings("unchecked")
    private void collectVisible(
            List<Map<String, Object>> nodes,
            Set<UUID> display,
            Set<UUID> manageable,
            List<Map<String, Object>> out) {
        if (nodes == null) return;
        for (Map<String, Object> node : nodes) {
            UUID id = UUID.fromString(String.valueOf(node.get("id")));
            List<Map<String, Object>> children = (List<Map<String, Object>>) node.get("children");
            List<Map<String, Object>> keptChildren = new ArrayList<>();
            collectVisible(children, display, manageable, keptChildren);
            if (display.contains(id)) {
                Map<String, Object> copy = new LinkedHashMap<>(node);
                copy.put("manageable", manageable.contains(id));
                copy.put("children", keptChildren);
                out.add(copy);
            } else {
                out.addAll(keptChildren);
            }
        }
    }
}
