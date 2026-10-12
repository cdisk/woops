package com.ops.control.metrics.notify;

import com.ops.control.asset.AssetEntity;
import com.ops.control.group.ServerGroupEntity;
import com.ops.control.group.ServerGroupRepository;
import com.ops.control.user.ScopeTypes;
import com.ops.control.user.UserEntity;
import com.ops.control.user.UserRepository;
import com.ops.control.user.UserScopeEntity;
import com.ops.control.user.UserScopeRepository;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.*;

/**
 * Resolves alert recipients from explicit {@code user_scopes} only.
 * Super-admin implicit visibility does not make them recipients of every asset.
 */
@Service
public class AlertRecipientResolver {
    private final UserScopeRepository scopes;
    private final UserRepository users;
    private final ServerGroupRepository groups;

    public AlertRecipientResolver(
            UserScopeRepository scopes, UserRepository users, ServerGroupRepository groups) {
        this.scopes = scopes;
        this.users = users;
        this.groups = groups;
    }

    public record Recipient(UUID userId, String username, String nickname, String authSource) {}

    @Transactional(readOnly = true)
    public List<Recipient> resolve(AssetEntity asset) {
        if (asset == null) {
            return List.of();
        }
        Set<UUID> groupChain = groupAncestorIds(asset.getGroupId());
        Map<UUID, UserEntity> active = new HashMap<>();
        for (UserEntity u : users.findAllActive()) {
            if (u.isEnabled()) {
                active.put(u.getId(), u);
            }
        }
        LinkedHashMap<UUID, Recipient> out = new LinkedHashMap<>();
        for (UserScopeEntity s : scopes.findAll()) {
            UserEntity u = active.get(s.getUserId());
            if (u == null) {
                continue;
            }
            boolean match = false;
            if (ScopeTypes.ASSET.equals(s.getScopeType()) && asset.getId().equals(s.getScopeId())) {
                match = true;
            } else if (ScopeTypes.GROUP.equals(s.getScopeType()) && groupChain.contains(s.getScopeId())) {
                match = true;
            }
            if (!match) {
                continue;
            }
            out.putIfAbsent(
                    u.getId(),
                    new Recipient(
                            u.getId(),
                            u.getUsername(),
                            u.getNickname() == null ? "" : u.getNickname(),
                            u.getAuthSource() == null ? "" : u.getAuthSource()));
        }
        return List.copyOf(out.values());
    }

    private Set<UUID> groupAncestorIds(UUID groupId) {
        if (groupId == null) {
            return Set.of();
        }
        Map<UUID, ServerGroupEntity> byId = new HashMap<>();
        for (ServerGroupEntity g : groups.findAll()) {
            byId.put(g.getId(), g);
        }
        Set<UUID> out = new LinkedHashSet<>();
        UUID cur = groupId;
        while (cur != null && out.add(cur)) {
            ServerGroupEntity g = byId.get(cur);
            cur = g == null ? null : g.getParentId();
        }
        return out;
    }
}
