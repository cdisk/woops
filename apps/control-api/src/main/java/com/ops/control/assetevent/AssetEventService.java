package com.ops.control.assetevent;

import com.ops.control.access.AccessService;
import com.ops.control.asset.AssetEntity;
import com.ops.control.asset.AssetRepository;
import com.ops.control.common.PageSupport;
import com.ops.control.group.GroupService;
import com.ops.control.user.UserEntity;
import org.springframework.data.domain.Page;
import org.springframework.data.domain.PageRequest;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.Instant;
import java.util.*;

@Service
public class AssetEventService {
    public static final String CAT_CONNECTIVITY = "CONNECTIVITY";
    public static final String CAT_NETWORK = "NETWORK";

    public static final String TYPE_ONLINE = "ONLINE";
    public static final String TYPE_OFFLINE = "OFFLINE";
    public static final String TYPE_PRIVATE_IP_CHANGED = "PRIVATE_IP_CHANGED";

    public static final String SEV_INFO = "INFO";
    public static final String SEV_WARNING = "WARNING";
    public static final String SEV_ERROR = "ERROR";

    private final AssetEventRepository events;
    private final AssetRepository assets;
    private final AccessService access;
    private final GroupService groups;

    public AssetEventService(
            AssetEventRepository events,
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
            UUID assetId,
            String category,
            String eventType,
            String severity,
            String detail,
            String sourceInstance,
            String connectionId) {
        AssetEventEntity e = new AssetEventEntity();
        e.setAssetId(assetId);
        e.setCategory(category == null || category.isBlank() ? CAT_CONNECTIVITY : category.trim());
        e.setEventType(eventType == null ? "" : eventType.trim());
        e.setSeverity(severity == null || severity.isBlank() ? SEV_INFO : severity.trim());
        e.setOccurredAt(Instant.now());
        e.setDetail(detail == null ? "" : trim(detail, 2000));
        e.setSourceInstance(sourceInstance == null ? "" : trim(sourceInstance, 128));
        e.setConnectionId(connectionId == null ? "" : trim(connectionId, 64));
        events.save(e);
    }

    @Transactional(readOnly = true)
    public Map<String, Object> page(
            UserEntity user,
            UUID assetId,
            String category,
            String eventType,
            Instant from,
            Instant to,
            int page,
            int pageSize) {
        if (assetId != null) {
            AssetEntity asset = assets.findById(assetId)
                    .orElseThrow(() -> new IllegalArgumentException("asset not found"));
            access.assertCanAccessAsset(user, asset);
        }

        Instant fromBound = from != null ? from : Instant.EPOCH;
        Instant toBound = to != null ? to : Instant.parse("9999-12-31T23:59:59Z");
        int size = PageSupport.clampPageSize(pageSize);
        int pageIndex = Math.max(page, 1) - 1;
        PageRequest pr = PageRequest.of(pageIndex, size);
        String cat = blankToNull(category);
        String typ = blankToNull(eventType);

        Page<AssetEventEntity> result;
        if (access.isSuperAdmin(user)) {
            result = events.search(assetId, cat, typ, fromBound, toBound, pr);
        } else {
            Set<UUID> visible = access.visibleAssetIds(user);
            if (assetId != null) {
                if (!visible.contains(assetId)) {
                    return PageSupport.emptyPage(page, size);
                }
                result = events.search(assetId, cat, typ, fromBound, toBound, pr);
            } else if (visible.isEmpty()) {
                return PageSupport.emptyPage(page, size);
            } else {
                result = events.searchInAssets(visible, cat, typ, fromBound, toBound, pr);
            }
        }

        Map<UUID, AssetEntity> assetById = new HashMap<>();
        List<UUID> assetIds = result.getContent().stream()
                .map(AssetEventEntity::getAssetId)
                .distinct()
                .toList();
        for (AssetEntity a : assets.findAllById(assetIds)) {
            assetById.put(a.getId(), a);
        }
        Map<UUID, String> groupNames = groups.nameById();

        List<Map<String, Object>> items = new ArrayList<>(result.getNumberOfElements());
        for (AssetEventEntity e : result.getContent()) {
            items.add(toView(e, assetById.get(e.getAssetId()), groupNames));
        }

        return PageSupport.pageResult(items, result.getTotalElements(), pageIndex + 1, size);
    }

    private Map<String, Object> toView(
            AssetEventEntity e, AssetEntity asset, Map<UUID, String> groupNames) {
        Map<String, Object> m = new LinkedHashMap<>();
        m.put("id", e.getId().toString());
        m.put("occurredAt", e.getOccurredAt().toString());
        m.put("assetId", e.getAssetId().toString());
        m.put("category", e.getCategory());
        m.put("eventType", e.getEventType());
        m.put("severity", e.getSeverity());
        m.put("detail", e.getDetail() == null ? "" : e.getDetail());
        m.put("sourceInstance", e.getSourceInstance() == null ? "" : e.getSourceInstance());
        m.put("connectionId", e.getConnectionId() == null ? "" : e.getConnectionId());
        if (asset != null) {
            m.put("assetDisplayName", asset.getDisplayName());
            m.put("hostname", asset.getHostname() == null ? "" : asset.getHostname());
            UUID gid = asset.getGroupId();
            m.put("groupId", gid == null ? null : gid.toString());
            m.put("groupName", gid == null ? "" : groupNames.getOrDefault(gid, ""));
        } else {
            m.put("assetDisplayName", "");
            m.put("hostname", "");
            m.put("groupId", null);
            m.put("groupName", "");
        }
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
