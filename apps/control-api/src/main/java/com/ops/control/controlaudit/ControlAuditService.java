package com.ops.control.controlaudit;

import com.ops.control.access.AccessService;
import com.ops.control.asset.AssetEntity;
import com.ops.control.asset.AssetRepository;
import com.ops.control.common.PageSupport;
import com.ops.control.group.GroupService;
import com.ops.control.user.Roles;
import com.ops.control.user.UserEntity;
import org.springframework.data.domain.Page;
import org.springframework.data.domain.PageRequest;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.Instant;
import java.util.*;

/**
 * Control-plane audit only. Runtime activity on managed servers (shell, file,
 * desktop, port-mapping tunnels) is recorded as server operations instead.
 */
@Service
public class ControlAuditService {
    public static final String CAT_AUTH = "AUTH";
    public static final String CAT_USER = "USER";
    public static final String CAT_ASSET = "ASSET";
    public static final String CAT_GROUP = "GROUP";
    public static final String CAT_PORTMAP = "PORTMAP";
    /** Deploy token lifecycle (create / remove), not opsctl runtime usage. */
    public static final String CAT_CI = "CI";
    /** User API token lifecycle (create / remove), personal center. */
    public static final String CAT_API_TOKEN = "API_TOKEN";
    public static final String CAT_MONITOR = "MONITOR";

    public static final String ACT_LOGIN = "LOGIN";
    public static final String ACT_TOTP_ENABLE = "TOTP_ENABLE";
    public static final String ACT_TOTP_RESET = "TOTP_RESET";
    public static final String ACT_CREATE = "CREATE";
    public static final String ACT_UPDATE = "UPDATE";
    public static final String ACT_REMOVE = "REMOVE";
    public static final String ACT_SET_SCOPES = "SET_SCOPES";
    public static final String ACT_REGISTER = "REGISTER";
    public static final String ACT_REREGISTER = "REREGISTER";
    public static final String ACT_INSTALL_CODE = "INSTALL_CODE";
    public static final String ACT_REVOKE_INSTALL = "REVOKE_INSTALL";
    public static final String ACT_REVOKE = "REVOKE";
    /** Console one-click Agent update (install command via exec). */
    public static final String ACT_UPDATE_AGENT = "UPDATE_AGENT";
    public static final String ACT_IGNORE = "IGNORE";
    public static final String ACT_UNIGNORE = "UNIGNORE";
    public static final String ACT_REPORT_READ = "REPORT_READ";

    private final ControlAuditEventRepository events;
    private final AssetRepository assets;
    private final AccessService access;
    private final GroupService groups;

    public ControlAuditService(
            ControlAuditEventRepository events,
            AssetRepository assets,
            AccessService access,
            GroupService groups) {
        this.events = events;
        this.assets = assets;
        this.access = access;
        this.groups = groups;
    }

    @Transactional
    public void record(
            String category,
            String action,
            UUID userId,
            String username,
            UUID assetId,
            UUID sessionId,
            String detail) {
        record(category, action, userId, username, assetId, null, sessionId, detail);
    }

    @Transactional
    public void record(
            String category,
            String action,
            UUID userId,
            String username,
            UUID assetId,
            UUID groupId,
            UUID sessionId,
            String detail) {
        ControlAuditEventEntity e = new ControlAuditEventEntity();
        e.setOccurredAt(Instant.now());
        e.setCategory(category == null ? "" : category);
        e.setAction(action == null ? "" : action);
        e.setUserId(userId);
        e.setUsername(username == null ? "" : username);
        e.setAssetId(assetId);
        e.setGroupId(groupId);
        e.setSessionId(sessionId);
        e.setDetail(detail == null ? "" : trim(detail, 2000));
        events.save(e);
    }

    @Transactional(readOnly = true)
    public Map<String, Object> page(
            UserEntity user,
            String category,
            UUID assetId,
            Instant from,
            Instant to,
            int page,
            int pageSize) {
        if (assetId != null) {
            AssetEntity asset = assets.findById(assetId)
                    .orElseThrow(() -> new IllegalArgumentException("asset not found"));
            access.assertCanAccessAsset(user, asset);
        }

        String cat = blankToNull(category);
        Instant fromBound = from != null ? from : Instant.EPOCH;
        Instant toBound = to != null ? to : Instant.parse("9999-12-31T23:59:59Z");
        int size = PageSupport.clampPageSize(pageSize);
        int pageIndex = Math.max(page, 1) - 1;
        PageRequest pr = PageRequest.of(pageIndex, size);

        Page<ControlAuditEventEntity> result;
        if (access.isSuperAdmin(user) || assetId != null) {
            result = events.search(cat, assetId, fromBound, toBound, pr);
        } else {
            Set<UUID> visibleAssets = access.visibleAssetIds(user);
            Set<UUID> visibleGroups = access.displayGroupIds(user);
            boolean manageUsers = Roles.isSuperAdmin(user.getRole()) || Roles.isAdmin(user.getRole());
            result = events.searchScoped(
                    cat,
                    fromBound,
                    toBound,
                    nonEmpty(visibleAssets),
                    nonEmpty(visibleGroups),
                    manageUsers,
                    user.getId(),
                    pr);
        }

        Map<UUID, AssetEntity> assetById = new HashMap<>();
        List<UUID> assetIds = result.getContent().stream()
                .map(ControlAuditEventEntity::getAssetId)
                .filter(Objects::nonNull)
                .distinct()
                .toList();
        for (AssetEntity a : assets.findAllById(assetIds)) {
            assetById.put(a.getId(), a);
        }
        Map<UUID, String> groupNames = groups.nameById();

        List<Map<String, Object>> items = new ArrayList<>(result.getNumberOfElements());
        for (ControlAuditEventEntity e : result.getContent()) {
            items.add(toView(e, assetById.get(e.getAssetId()), groupNames));
        }

        return PageSupport.pageResult(items, result.getTotalElements(), pageIndex + 1, size);
    }

    /** Hibernate {@code IN :xs} rejects empty collections; use a never-matching id. */
    private static Collection<UUID> nonEmpty(Collection<UUID> ids) {
        if (ids == null || ids.isEmpty()) {
            return List.of(UUID.fromString("00000000-0000-0000-0000-000000000000"));
        }
        return ids;
    }

    private Map<String, Object> toView(
            ControlAuditEventEntity e, AssetEntity asset, Map<UUID, String> groupNames) {
        Map<String, Object> m = new LinkedHashMap<>();
        m.put("id", e.getId().toString());
        m.put("occurredAt", e.getOccurredAt().toString());
        m.put("category", e.getCategory());
        m.put("action", e.getAction());
        m.put("userId", e.getUserId() == null ? null : e.getUserId().toString());
        m.put("username", e.getUsername() == null ? "" : e.getUsername());
        m.put("assetId", e.getAssetId() == null ? null : e.getAssetId().toString());
        m.put("groupId", e.getGroupId() == null ? null : e.getGroupId().toString());
        m.put("sessionId", e.getSessionId() == null ? null : e.getSessionId().toString());
        m.put("detail", e.getDetail() == null ? "" : e.getDetail());
        UUID gid = e.getGroupId();
        if (asset != null) {
            m.put("assetDisplayName", asset.getDisplayName());
            m.put("hostname", asset.getHostname() == null ? "" : asset.getHostname());
            if (gid == null) {
                gid = asset.getGroupId();
            }
        } else {
            m.put("assetDisplayName", "");
            m.put("hostname", "");
        }
        m.put("groupName", gid == null ? "" : groupNames.getOrDefault(gid, ""));
        return m;
    }

    public static String jsonDetail(Map<String, ?> fields) {
        if (fields == null || fields.isEmpty()) {
            return "{}";
        }
        StringBuilder sb = new StringBuilder("{");
        boolean first = true;
        for (Map.Entry<String, ?> e : fields.entrySet()) {
            if (!first) sb.append(',');
            first = false;
            sb.append('"').append(escape(e.getKey())).append("\":");
            Object v = e.getValue();
            if (v == null) {
                sb.append("null");
            } else if (v instanceof Number || v instanceof Boolean) {
                sb.append(v);
            } else if (v instanceof Collection<?> col) {
                sb.append('[');
                boolean cf = true;
                for (Object item : col) {
                    if (!cf) sb.append(',');
                    cf = false;
                    if (item instanceof Number || item instanceof Boolean) {
                        sb.append(item);
                    } else {
                        sb.append('"').append(escape(String.valueOf(item))).append('"');
                    }
                }
                sb.append(']');
            } else {
                sb.append('"').append(escape(String.valueOf(v))).append('"');
            }
        }
        return sb.append('}').toString();
    }

    private static String escape(String s) {
        if (s == null) return "";
        return s.replace("\\", "\\\\").replace("\"", "\\\"");
    }

    private static String blankToNull(String s) {
        return s == null || s.isBlank() ? null : s.trim();
    }

    private static String trim(String s, int max) {
        if (s.length() <= max) return s;
        return s.substring(0, max);
    }
}
