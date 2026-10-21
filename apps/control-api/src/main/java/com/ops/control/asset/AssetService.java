package com.ops.control.asset;

import com.ops.control.access.AccessService;
import com.ops.control.access.ForbiddenException;
import com.ops.control.controlaudit.ControlAuditService;
import com.ops.control.group.GroupService;
import com.ops.control.metrics.AlertIncidentRepository;
import com.ops.control.metrics.AlertPendingRepository;
import com.ops.control.metrics.AssetAlertIgnoreRepository;
import com.ops.control.metrics.AssetAlertStatusRepository;
import com.ops.control.user.ScopeTypes;
import com.ops.control.user.UserEntity;
import com.ops.control.user.UserScopeRepository;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.ArrayList;
import java.util.Comparator;
import java.util.HashMap;
import java.util.HashSet;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Objects;
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
    private final AlertIncidentRepository alertIncidents;
    private final AlertPendingRepository alertPendings;

    public AssetService(
            AssetRepository assets,
            GroupService groups,
            AccessService access,
            ControlAuditService audit,
            UserScopeRepository userScopes,
            AssetAlertIgnoreRepository alertIgnores,
            AssetAlertStatusRepository alertStatus,
            AlertIncidentRepository alertIncidents,
            AlertPendingRepository alertPendings) {
        this.assets = assets;
        this.groups = groups;
        this.access = access;
        this.audit = audit;
        this.userScopes = userScopes;
        this.alertIgnores = alertIgnores;
        this.alertStatus = alertStatus;
        this.alertPendings = alertPendings;
        this.alertIncidents = alertIncidents;
    }

    public record UpdateRequest(String displayName, String remark, UUID groupId, Boolean updateGroup,
                                Integer desktopPort, String desktopUsername, String desktopPassword,
                                Integer desktopColorDepth, String desktopRdpQuality) {}

    public record ShellCommandsRequest(List<String> items) {}

    public record UpdateUpstreamRequest(List<UUID> assetIds, UUID upstreamAssetId, Boolean clear) {}

    public record UpdatePlanRequest(List<UUID> assetIds) {}

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
        Map<UUID, AssetEntity> upstreams = indexUpstreams(list);
        return list.stream().map(a -> toView(user, a, names, upstreams)).toList();
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
        alertPendings.deleteByAssetId(asset.getId());
        alertIncidents.deleteByAssetId(asset.getId());
        assets.clearUpdateUpstream(asset.getId());
        assets.delete(asset);
        return Map.of("status", "deleted", "id", id.toString());
    }

    @Transactional
    public Map<String, Object> setUpdateUpstream(UserEntity user, UpdateUpstreamRequest req) {
        if (req == null || req.assetIds() == null || req.assetIds().isEmpty()) {
            throw new IllegalArgumentException("assetIds required");
        }
        boolean clear = Boolean.TRUE.equals(req.clear());
        UUID upstreamId = clear ? null : req.upstreamAssetId();
        if (!clear && upstreamId == null) {
            throw new IllegalArgumentException("upstreamAssetId required");
        }
        List<AssetEntity> all = assets.findAll();
        Map<UUID, AssetEntity> byId = new HashMap<>();
        Map<UUID, UUID> graph = new HashMap<>();
        for (AssetEntity a : all) {
            byId.put(a.getId(), a);
            if (a.getUpdateUpstreamAssetId() != null) {
                graph.put(a.getId(), a.getUpdateUpstreamAssetId());
            }
        }
        List<UUID> ids = req.assetIds().stream().filter(Objects::nonNull).distinct().toList();
        if (ids.isEmpty()) {
            throw new IllegalArgumentException("assetIds required");
        }
        for (UUID id : ids) {
            AssetEntity asset = byId.get(id);
            if (asset == null) {
                throw new IllegalArgumentException("asset not found");
            }
            access.assertCanAccessAsset(user, asset);
        }
        if (upstreamId != null) {
            AssetEntity upstream = byId.get(upstreamId);
            if (upstream == null) {
                throw new IllegalArgumentException("upstream asset not found");
            }
            access.assertCanAccessAsset(user, upstream);
        }
        UpdateTopology.withUpstream(graph, ids, upstreamId);
        String upstreamText = upstreamId == null ? "" : upstreamId.toString();
        for (UUID id : ids) {
            AssetEntity asset = byId.get(id);
            asset.setUpdateUpstreamAssetId(upstreamId);
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
                            "updateUpstreamAssetId", upstreamText,
                            "cleared", upstreamId == null)));
        }
        Map<String, Object> out = new LinkedHashMap<>();
        out.put("updated", ids.size());
        out.put("updateUpstreamAssetId", upstreamText);
        return out;
    }

    @Transactional(readOnly = true)
    public Map<String, Object> updatePlan(UserEntity user, List<UUID> assetIds) {
        LoadedGraph loaded = loadGraph();
        List<UUID> ids = assetIds == null ? List.of() : assetIds.stream().filter(Objects::nonNull).distinct().toList();
        for (UUID id : ids) {
            AssetEntity asset = loaded.byId().get(id);
            if (asset == null) {
                throw new IllegalArgumentException("asset not found");
            }
            access.assertCanAccessAsset(user, asset);
        }
        List<UpdateTopology.PlanLayer> layers = UpdateTopology.plan(ids, loaded.upstreamOf());
        List<Map<String, Object>> outLayers = new ArrayList<>();
        for (UpdateTopology.PlanLayer layer : layers) {
            List<Map<String, Object>> assetsOut = new ArrayList<>();
            for (UpdateTopology.PlanNode node : layer.assets()) {
                assetsOut.add(planAsset(loaded.byId().get(node.id()), node.blockIds()));
            }
            outLayers.add(Map.of("assets", assetsOut));
        }
        return Map.of("layers", outLayers);
    }

    @Transactional(readOnly = true)
    public Map<String, Object> updateTopology(UserEntity user, List<UUID> focusIds) {
        LoadedGraph loaded = loadGraph();
        Map<UUID, String> groupNames = groups.nameById();
        Set<UUID> visible = new HashSet<>();
        for (AssetEntity asset : loaded.byId().values()) {
            if (access.canAccessAsset(user, asset)) {
                visible.add(asset.getId());
            }
        }
        Set<UUID> shown = UpdateTopology.focus(visible, loaded.upstreamOf(), focusIds);
        UpdateTopology.TopoSketch sketch = UpdateTopology.sketch(shown, visible, loaded.upstreamOf());
        Set<UUID> selected = new HashSet<>();
        if (focusIds != null) {
            for (UUID id : focusIds) {
                if (id != null && shown.contains(id)) {
                    selected.add(id);
                }
            }
        }
        List<Map<String, Object>> nodes = new ArrayList<>();
        if (sketch.gateway()) {
            nodes.add(topoGateway());
        }
        List<AssetEntity> shownAssets = new ArrayList<>();
        for (UUID id : shown) {
            AssetEntity asset = loaded.byId().get(id);
            if (asset != null) {
                shownAssets.add(asset);
            }
        }
        shownAssets.sort(ASSET_BY_NAME);
        for (AssetEntity asset : shownAssets) {
            nodes.add(topoAsset(asset, selected.contains(asset.getId()), groupNames));
        }
        for (String hiddenId : sketch.hiddenIds()) {
            nodes.add(topoHidden(hiddenId));
        }
        List<Map<String, String>> edges = new ArrayList<>();
        for (UpdateTopology.TopoEdge edge : sketch.edges()) {
            edges.add(Map.of("from", edge.from(), "to", edge.to()));
        }
        Map<String, Object> out = new LinkedHashMap<>();
        out.put("nodes", nodes);
        out.put("edges", edges);
        return out;
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

    private LoadedGraph loadGraph() {
        List<AssetEntity> all = assets.findAll();
        Map<UUID, AssetEntity> byId = new HashMap<>();
        Map<UUID, UUID> upstreamOf = new HashMap<>();
        for (AssetEntity asset : all) {
            byId.put(asset.getId(), asset);
            if (asset.getUpdateUpstreamAssetId() != null) {
                upstreamOf.put(asset.getId(), asset.getUpdateUpstreamAssetId());
            }
        }
        return new LoadedGraph(byId, upstreamOf);
    }

    private Map<UUID, AssetEntity> indexUpstreams(List<AssetEntity> list) {
        Set<UUID> ids = new HashSet<>();
        for (AssetEntity asset : list) {
            if (asset.getUpdateUpstreamAssetId() != null) {
                ids.add(asset.getUpdateUpstreamAssetId());
            }
        }
        Map<UUID, AssetEntity> out = new HashMap<>();
        if (!ids.isEmpty()) {
            for (AssetEntity asset : assets.findAllById(ids)) {
                out.put(asset.getId(), asset);
            }
        }
        return out;
    }

    private Map<String, Object> planAsset(AssetEntity asset, List<UUID> blockIds) {
        Map<String, Object> m = new LinkedHashMap<>();
        m.put("id", asset.getId().toString());
        m.put("displayName", asset.getDisplayName() == null ? "" : asset.getDisplayName());
        m.put("hostname", asset.getHostname() == null ? "" : asset.getHostname());
        m.put("os", asset.getOs() == null ? "" : asset.getOs());
        m.put("privateIp", UpdateTopology.primaryPrivateIp(asset.getPrivateIp()));
        m.put("online", asset.isOnline());
        m.put("groupId", asset.getGroupId() == null ? null : asset.getGroupId().toString());
        m.put("agentVersion", asset.getAgentVersion() == null ? "" : asset.getAgentVersion());
        m.put("blockIds", blockIds.stream().map(UUID::toString).toList());
        return m;
    }

    private static Map<String, Object> topoGateway() {
        Map<String, Object> n = new LinkedHashMap<>();
        n.put("id", UpdateTopology.GATEWAY_ID);
        n.put("kind", "gateway");
        n.put("name", "Gateway");
        n.put("privateIp", "");
        n.put("online", true);
        n.put("agentVersion", "");
        n.put("hidden", false);
        n.put("selected", false);
        return n;
    }

    private static Map<String, Object> topoHidden(String id) {
        Map<String, Object> n = new LinkedHashMap<>();
        n.put("id", id);
        n.put("kind", "hidden");
        n.put("name", "");
        n.put("privateIp", "");
        n.put("online", false);
        n.put("agentVersion", "");
        n.put("hidden", true);
        n.put("selected", false);
        return n;
    }

    private static Map<String, Object> topoAsset(
            AssetEntity asset,
            boolean selected,
            Map<UUID, String> groupNames) {
        Map<String, Object> n = new LinkedHashMap<>();
        n.put("id", asset.getId().toString());
        n.put("kind", "asset");
        n.put("name", asset.getDisplayName() == null ? "" : asset.getDisplayName());
        n.put("privateIp", UpdateTopology.primaryPrivateIp(asset.getPrivateIp()));
        n.put("online", asset.isOnline());
        n.put("agentVersion", asset.getAgentVersion() == null ? "" : asset.getAgentVersion());
        n.put("groupId", asset.getGroupId() == null ? null : asset.getGroupId().toString());
        n.put("groupName", asset.getGroupId() == null
                ? ""
                : groupNames.getOrDefault(asset.getGroupId(), ""));
        n.put("hidden", false);
        n.put("selected", selected);
        return n;
    }

    private record LoadedGraph(Map<UUID, AssetEntity> byId, Map<UUID, UUID> upstreamOf) {}

    private Map<String, Object> toView(UserEntity user, AssetEntity a, Map<UUID, String> names) {
        return toView(user, a, names, null);
    }

    private Map<String, Object> toView(UserEntity user, AssetEntity a, Map<UUID, String> names, Map<UUID, AssetEntity> upstreamById) {
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
        putUpstream(user, a, m, upstreamById);
        return m;
    }

    private void putUpstream(UserEntity user, AssetEntity a, Map<String, Object> m, Map<UUID, AssetEntity> upstreamById) {
        m.put("updateUpstreamAssetId", null);
        m.put("updateUpstreamName", "");
        m.put("updateUpstreamHidden", false);
        UUID upstreamId = a.getUpdateUpstreamAssetId();
        if (upstreamId == null) {
            return;
        }
        AssetEntity upstream = upstreamById != null
                ? upstreamById.get(upstreamId)
                : assets.findById(upstreamId).orElse(null);
        if (upstream != null && access.canAccessAsset(user, upstream)) {
            m.put("updateUpstreamAssetId", upstreamId.toString());
            m.put("updateUpstreamName", upstream.getDisplayName() == null ? "" : upstream.getDisplayName());
            return;
        }
        m.put("updateUpstreamHidden", true);
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
