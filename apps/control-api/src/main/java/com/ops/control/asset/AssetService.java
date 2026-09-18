package com.ops.control.asset;

import com.ops.control.access.AccessService;
import com.ops.control.access.ForbiddenException;
import com.ops.control.controlaudit.ControlAuditService;
import com.ops.control.group.GroupService;
import com.ops.control.metrics.AssetAlertIgnoreRepository;
import com.ops.control.metrics.AssetAlertStatusRepository;
import com.ops.control.user.ScopeTypes;
import com.ops.control.user.UserEntity;
import com.ops.control.user.UserScopeRepository;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.ArrayList;
import java.util.Comparator;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.UUID;

@Service
public class AssetService {
    private static final int SHELL_CMD_MAX_COUNT = 30;
    private static final int SHELL_CMD_MAX_EACH = 8 * 1024;
    private static final int SHELL_CMD_MAX_TOTAL = 64 * 1024;
    private static final int REMARK_MAX = 4096;

    private static final Comparator<AssetEntity> ASSET_BY_NAME = Comparator
            .comparing((AssetEntity a) -> nullToEmpty(a.getDisplayName()), String.CASE_INSENSITIVE_ORDER)
            .thenComparing(a -> nullToEmpty(a.getHostname()), String.CASE_INSENSITIVE_ORDER)
            .thenComparing(a -> a.getId() == null ? "" : a.getId().toString());

    private final AssetRepository assets;
    private final GroupService groups;
    private final AccessService access;
    private final ControlAuditService audit;
    private final UserScopeRepository userScopes;
    private final AssetAlertIgnoreRepository alertIgnores;
    private final AssetAlertStatusRepository alertStatus;

    public AssetService(
            AssetRepository assets,
            GroupService groups,
            AccessService access,
            ControlAuditService audit,
            UserScopeRepository userScopes,
            AssetAlertIgnoreRepository alertIgnores,
            AssetAlertStatusRepository alertStatus) {
        this.assets = assets;
        this.groups = groups;
        this.access = access;
        this.audit = audit;
        this.userScopes = userScopes;
        this.alertIgnores = alertIgnores;
        this.alertStatus = alertStatus;
    }

    public record UpdateRequest(String displayName, String remark, UUID groupId, Boolean updateGroup,
                                Integer desktopPort, String desktopUsername, String desktopPassword,
                                Integer desktopColorDepth, String desktopRdpQuality) {}

    public record ShellCommandsRequest(List<String> items) {}

    @Transactional(readOnly = true)
    public List<Map<String, Object>> list(UserEntity user, UUID groupId, boolean rootOnly, boolean includeSubtree) {
        if (rootOnly) {
            if (!access.isSuperAdmin(user)) {
                return List.of();
            }
        } else if (groupId != null) {
            access.assertCanAccessGroup(user, groupId);
        }
        List<AssetEntity> list;
        if (rootOnly) {
            list = assets.findByGroupIdIsNullOrderByDisplayNameAscHostnameAsc();
        } else if (groupId != null) {
            if (includeSubtree) {
                Set<UUID> ids = groups.selfAndDescendantIds(groupId);
                list = ids.isEmpty()
                        ? List.of()
                        : assets.findByGroupIdInOrderByDisplayNameAscHostnameAsc(ids);
            } else {
                list = assets.findByGroupIdOrderByDisplayNameAscHostnameAsc(groupId);
            }
        } else {
            list = assets.findAllByOrderByDisplayNameAscHostnameAsc();
        }
        if (!access.isSuperAdmin(user)) {
            list = list.stream().filter(a -> access.canAccessAsset(user, a)).toList();
        }
        list = list.stream().sorted(ASSET_BY_NAME).toList();
        Map<UUID, String> names = groups.nameById();
        return list.stream().map(a -> toView(user, a, names)).toList();
    }

    @Transactional(readOnly = true)
    public Map<String, Object> get(UserEntity user, UUID id) {
        AssetEntity a = assets.findById(id).orElseThrow(() -> new IllegalArgumentException("asset not found"));
        access.assertCanAccessAsset(user, a);
        return toView(user, a, groups.nameById());
    }

    @Transactional
    public Map<String, Object> update(UserEntity user, UUID id, UpdateRequest req) {
        AssetEntity asset = assets.findById(id).orElseThrow(() -> new IllegalArgumentException("asset not found"));
        access.assertCanAccessAsset(user, asset);
        Map<String, Object> changes = new LinkedHashMap<>();
        if (req.displayName() != null && !req.displayName().isBlank()) {
            asset.setDisplayName(req.displayName());
            changes.put("displayName", req.displayName());
        }
        if (req.remark() != null) {
            String note = normalizeRemark(req.remark());
            asset.setRemark(note);
            changes.put("remark", note);
        }
        if (Boolean.TRUE.equals(req.updateGroup())) {
            if (req.groupId() == null) {
                if (!access.isSuperAdmin(user)) {
                    throw new ForbiddenException("only super admin can move asset to root");
                }
            } else {
                groups.find(req.groupId()).orElseThrow(() -> new IllegalArgumentException("group not found"));
                access.assertCanManageGroup(user, req.groupId());
            }
            asset.setGroupId(req.groupId());
            changes.put("groupId", req.groupId() == null ? null : req.groupId().toString());
        }
        if (req.desktopPort() != null && req.desktopPort() > 0) {
            asset.setDesktopPort(req.desktopPort());
            changes.put("desktopPort", req.desktopPort());
        }
        if (req.desktopUsername() != null) {
            asset.setDesktopUsername(req.desktopUsername().trim());
            changes.put("desktopUsername", asset.getDesktopUsername());
        }
        // Empty string means "leave unchanged"; only update when a non-empty password is provided.
        if (req.desktopPassword() != null && !req.desktopPassword().isBlank()) {
            asset.setDesktopPassword(req.desktopPassword());
            changes.put("passwordChanged", true);
        }
        if (req.desktopColorDepth() != null) {
            int depth = normalizeColorDepth(req.desktopColorDepth());
            asset.setDesktopColorDepth(depth);
            changes.put("desktopColorDepth", depth);
        }
        if (req.desktopRdpQuality() != null && !req.desktopRdpQuality().isBlank()) {
            String q = normalizeRdpQuality(req.desktopRdpQuality());
            asset.setDesktopRdpQuality(q);
            changes.put("desktopRdpQuality", q);
        }
        asset.setUpdatedAt(java.time.Instant.now());
        assets.save(asset);
        if (!changes.isEmpty()) {
            audit.record(
                    ControlAuditService.CAT_ASSET,
                    ControlAuditService.ACT_UPDATE,
                    user.getId(),
                    user.getUsername(),
                    asset.getId(),
                    asset.getGroupId(),
                    null,
                    ControlAuditService.jsonDetail(changes));
        }
        return toView(user, asset, groups.nameById());
    }

    @Transactional
    public Map<String, String> delete(UserEntity user, UUID id) {
        AssetEntity asset = assets.findById(id).orElseThrow(() -> new IllegalArgumentException("asset not found"));
        access.assertCanDeleteAsset(user, asset);
        audit.record(
                ControlAuditService.CAT_ASSET,
                ControlAuditService.ACT_REMOVE,
                user.getId(),
                user.getUsername(),
                asset.getId(),
                asset.getGroupId(),
                null,
                ControlAuditService.jsonDetail(Map.of(
                        "displayName", asset.getDisplayName() == null ? "" : asset.getDisplayName(),
                        "hostname", asset.getHostname() == null ? "" : asset.getHostname()
                )));
        // Clear ASSET scopes pointing at this asset
        userScopes.deleteByScopeTypeAndScopeId(ScopeTypes.ASSET, asset.getId());
        alertIgnores.deleteByAssetId(asset.getId());
        alertStatus.deleteById(asset.getId());
        assets.delete(asset);
        return Map.of("status", "deleted", "id", id.toString());
    }

    @Transactional(readOnly = true)
    public Map<String, Object> getShellCommands(UserEntity user, UUID id) {
        AssetEntity asset = assets.findById(id).orElseThrow(() -> new IllegalArgumentException("asset not found"));
        access.assertCanAccessAsset(user, asset);
        List<String> items = asset.getShellCommands();
        return Map.of("items", items == null ? List.of() : List.copyOf(items));
    }

    @Transactional
    public Map<String, Object> putShellCommands(UserEntity user, UUID id, ShellCommandsRequest req) {
        AssetEntity asset = assets.findById(id).orElseThrow(() -> new IllegalArgumentException("asset not found"));
        access.assertCanAccessAsset(user, asset);
        List<String> normalized = normalizeShellCommands(req == null ? null : req.items());
        asset.setShellCommands(normalized);
        asset.setUpdatedAt(java.time.Instant.now());
        assets.save(asset);
        audit.record(
                ControlAuditService.CAT_ASSET,
                ControlAuditService.ACT_UPDATE,
                user.getId(),
                user.getUsername(),
                asset.getId(),
                asset.getGroupId(),
                null,
                ControlAuditService.jsonDetail(Map.of(
                        "shellCommands", true,
                        "count", normalized.size())));
        return Map.of("items", normalized);
    }

    static List<String> normalizeShellCommands(List<String> raw) {
        if (raw == null || raw.isEmpty()) {
            return List.of();
        }
        if (raw.size() > SHELL_CMD_MAX_COUNT) {
            throw new IllegalArgumentException("too many shell commands (max " + SHELL_CMD_MAX_COUNT + ")");
        }
        List<String> out = new ArrayList<>(raw.size());
        int total = 0;
        for (String item : raw) {
            if (item == null) {
                throw new IllegalArgumentException("shell command must not be blank");
            }
            String s = item.replace("\r\n", "\n").replace('\r', '\n');
            if (s.isBlank()) {
                throw new IllegalArgumentException("shell command must not be blank");
            }
            if (s.length() > SHELL_CMD_MAX_EACH) {
                throw new IllegalArgumentException("shell command too long (max " + SHELL_CMD_MAX_EACH + " chars)");
            }
            total += s.length();
            if (total > SHELL_CMD_MAX_TOTAL) {
                throw new IllegalArgumentException("shell commands total too large (max " + SHELL_CMD_MAX_TOTAL + " chars)");
            }
            out.add(s);
        }
        return List.copyOf(out);
    }

    private Map<String, Object> toView(UserEntity user, AssetEntity a, Map<UUID, String> names) {
        Map<String, Object> m = new LinkedHashMap<>();
        m.put("id", a.getId().toString());
        m.put("displayName", a.getDisplayName());
        m.put("remark", a.getRemark() == null ? "" : a.getRemark());
        m.put("hostname", a.getHostname() == null ? "" : a.getHostname());
        m.put("os", a.getOs() == null ? "" : a.getOs());
        m.put("arch", a.getArch() == null ? "" : a.getArch());
        m.put("agentVersion", a.getAgentVersion() == null ? "" : a.getAgentVersion());
        m.put("publicIp", a.getPublicIp() == null ? "" : a.getPublicIp());
        m.put("privateIp", a.getPrivateIp() == null ? "" : a.getPrivateIp());
        m.put("groupId", a.getGroupId() == null ? null : a.getGroupId().toString());
        m.put("groupName", a.getGroupId() == null ? "" : names.getOrDefault(a.getGroupId(), ""));
        m.put("accessProtocol", AssetAccess.protocolOf(a));
        m.put("actions", AssetAccess.actions(a));
        m.put("desktopPort", a.getDesktopPort());
        m.put("desktopUsername", a.getDesktopUsername() == null ? "" : a.getDesktopUsername());
        m.put("hasDesktopPassword", a.getDesktopPassword() != null && !a.getDesktopPassword().isBlank());
        m.put("desktopColorDepth", normalizeColorDepth(a.getDesktopColorDepth()));
        m.put("desktopRdpQuality", normalizeRdpQuality(a.getDesktopRdpQuality()));
        m.put("online", a.isOnline());
        m.put("lastSeenAt", a.getLastSeenAt() == null ? "" : a.getLastSeenAt().toString());
        m.put("createdAt", a.getCreatedAt().toString());
        m.put("canDelete", access.canDeleteAsset(user, a));
        return m;
    }

    /** Guacamole RDP color-depth: 8, 16, 24, or 32 (no 15-bit). */
    static int normalizeColorDepth(Integer depth) {
        if (depth == null) {
            return 16;
        }
        return switch (depth) {
            case 8, 16, 24, 32 -> depth;
            default -> 16;
        };
    }

    /** low = min experience flags; medium = some chrome; high = wallpaper/fonts on. */
    static String normalizeRdpQuality(String quality) {
        if (quality == null || quality.isBlank()) {
            return "low";
        }
        return switch (quality.trim().toLowerCase()) {
            case "low", "medium", "high" -> quality.trim().toLowerCase();
            default -> "low";
        };
    }

    private static String normalizeRemark(String remark) {
        String t = remark.replace("\r\n", "\n").replace('\r', '\n');
        if (t.length() > REMARK_MAX) {
            throw new IllegalArgumentException("remark too long (max " + REMARK_MAX + ")");
        }
        return t;
    }

    private static String nullToEmpty(String s) {
        return s == null ? "" : s;
    }
}
