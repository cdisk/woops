package com.ops.control.serverops;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;
import com.ops.control.access.AccessService;
import com.ops.control.asset.AssetEntity;
import com.ops.control.asset.AssetRepository;
import com.ops.control.common.OpsProperties;
import com.ops.control.common.PageSupport;
import com.ops.control.group.GroupService;
import com.ops.control.user.UserEntity;
import org.springframework.data.domain.Page;
import org.springframework.data.domain.PageRequest;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.time.Instant;
import java.time.format.DateTimeParseException;
import java.time.temporal.ChronoUnit;
import java.util.*;

/**
 * Ingests runtime activity envelopes (Gateway/Agent) into
 * {@code server_operation_records} and serves them back to the console.
 *
 * <p>Envelope shape:
 * <pre>
 * {
 *   "phase": "START|ACTION|ERROR|END",
 *   "operationId": "uuid",
 *   "eventId": "uuid",          // ACTION / ERROR
 *   "operationType": "SHELL",   // required on START
 *   "eventType": "COMMAND",     // ACTION / ERROR
 *   "assetId": "uuid",
 *   "userId": "uuid",
 *   "username": "alice",
 *   "occurredAt": "2026-08-03T10:00:00Z",
 *   "endedAt": "2026-08-03T10:05:00Z",  // END
 *   "status": "COMPLETED",
 *   "success": true,
 *   "detail": { … }
 * }
 * </pre>
 *
 * <p>Envelopes arrive as JSONL lines in the Gateway spool
 * ({@code ops.audit-dir}) and are replayed by {@link OpsAuditIngestScheduler}.
 * Delivery is at-least-once, so ingest is idempotent per operationId/eventId.
 */
@Service
public class ServerOperationAuditService {
    public static final String KIND_OPERATION = "OPERATION";
    public static final String KIND_EVENT = "EVENT";

    public static final String TYPE_SHELL = "SHELL";
    public static final String TYPE_FILE = "FILE";
    public static final String TYPE_RDP = "RDP";
    public static final String TYPE_VNC = "VNC";
    public static final String TYPE_EXEC = "EXEC";
    public static final String TYPE_PORTMAP_TCP = "PORTMAP_TCP";
    public static final String TYPE_PORTMAP_UDP = "PORTMAP_UDP";

    public static final String STATUS_RUNNING = "RUNNING";
    public static final String STATUS_COMPLETED = "COMPLETED";
    public static final String STATUS_FAILED = "FAILED";
    public static final String STATUS_INTERRUPTED = "INTERRUPTED";
    public static final String STATUS_PURGED = "PURGED";

    private final ServerOperationRecordRepository records;
    private final AssetRepository assets;
    private final AccessService access;
    private final GroupService groups;
    private final OpsProperties props;
    private final ObjectMapper json;

    public ServerOperationAuditService(
            ServerOperationRecordRepository records,
            AssetRepository assets,
            AccessService access,
            GroupService groups,
            OpsProperties props,
            ObjectMapper json) {
        this.records = records;
        this.assets = assets;
        this.access = access;
        this.groups = groups;
        this.props = props;
        this.json = json;
    }

    @Transactional
    public void ingestEnvelope(JsonNode envelope) {
        if (envelope == null || envelope.isNull()) {
            throw new IllegalArgumentException("envelope required");
        }
        String phase = upper(text(envelope, "phase"));
        UUID operationId = uuid(envelope, "operationId");
        if (operationId == null) {
            throw new IllegalArgumentException("operationId required");
        }
        switch (phase) {
            case "START" -> ingestStart(envelope, operationId);
            case "ACTION", "ERROR" -> ingestEvent(envelope, operationId, phase);
            case "END" -> ingestEnd(envelope, operationId);
            default -> throw new IllegalArgumentException("unknown phase: " + phase);
        }
    }

    private void ingestStart(JsonNode envelope, UUID operationId) {
        UUID assetId = uuid(envelope, "assetId");
        if (assetId == null) {
            throw new IllegalArgumentException("assetId required");
        }
        ServerOperationRecordEntity row = records.findById(operationId)
                .orElseGet(ServerOperationRecordEntity::new);
        // Spool replay can re-deliver START after END; never reopen a closed row.
        boolean alreadyEnded = row.getEndedAt() != null;
        row.setId(operationId);
        row.setOperationId(operationId);
        row.setRecordKind(KIND_OPERATION);
        row.setOperationType(requireType(envelope));
        row.setEventType(null);
        row.setAssetId(assetId);
        row.setUserId(uuid(envelope, "userId"));
        row.setUsername(text(envelope, "username"));
        row.setOccurredAt(instant(envelope, "occurredAt", Instant.now()));
        if (!alreadyEnded) {
            String st = text(envelope, "status");
            row.setStatus(st == null || st.isBlank() ? STATUS_RUNNING : upper(st));
        }
        row.setDetail(mergeDetail(row.getDetail(), envelope.get("detail")));
        records.save(row);
    }

    private void ingestEvent(JsonNode envelope, UUID operationId, String phase) {
        ServerOperationRecordEntity parent = records.findById(operationId).orElse(null);
        UUID assetId = uuid(envelope, "assetId");
        if (assetId == null && parent != null) {
            assetId = parent.getAssetId();
        }
        if (assetId == null) {
            throw new IllegalArgumentException("assetId required");
        }
        UUID eventId = uuid(envelope, "eventId");
        if (eventId != null && records.existsById(eventId)) {
            // At-least-once spool delivery: the same eventId is a replay, not a new event.
            return;
        }

        ServerOperationRecordEntity row = new ServerOperationRecordEntity();
        row.setId(eventId == null ? UUID.randomUUID() : eventId);
        row.setOperationId(operationId);
        row.setRecordKind(KIND_EVENT);
        String type = text(envelope, "operationType");
        row.setOperationType(type != null ? type : (parent == null ? "UNKNOWN" : parent.getOperationType()));
        row.setEventType(orDefault(text(envelope, "eventType"), phase));
        row.setAssetId(assetId);
        row.setUserId(uuid(envelope, "userId"));
        row.setUsername(orDefault(text(envelope, "username"), parent == null ? null : parent.getUsername()));
        row.setOccurredAt(instant(envelope, "occurredAt", Instant.now()));
        row.setStatus(blankToNull(text(envelope, "status")));
        row.setSuccess(bool(envelope, "success", "ERROR".equals(phase) ? Boolean.FALSE : null));
        row.setDetail(mergeDetail(null, envelope.get("detail")));
        records.save(row);
    }

    private void ingestEnd(JsonNode envelope, UUID operationId) {
        ServerOperationRecordEntity row = records.findById(operationId).orElse(null);
        if (row == null) {
            // END without START (control-api restart, dropped envelope): keep the
            // tail rather than losing the operation entirely.
            ingestStart(envelope, operationId);
            row = records.findById(operationId).orElseThrow();
        }
        // First END wins: gateway shutdown may write INTERRUPTED, then a dying
        // bridge still emits COMPLETED/FAILED — do not reopen or flip status.
        if (row.getEndedAt() != null) {
            row.setDetail(mergeDetail(row.getDetail(), envelope.get("detail")));
            records.save(row);
            return;
        }
        row.setEndedAt(instant(envelope, "endedAt", Instant.now()));
        Boolean success = bool(envelope, "success", null);
        row.setSuccess(success);
        String status = text(envelope, "status");
        if (status == null || status.isBlank()) {
            status = Boolean.FALSE.equals(success) ? STATUS_FAILED : STATUS_COMPLETED;
        }
        row.setStatus(upper(status));
        row.setDetail(mergeDetail(row.getDetail(), envelope.get("detail")));
        records.save(row);
    }

    /**
     * Mark every OPERATION still RUNNING as INTERRUPTED. Used when Gateway
     * restarts: force-kill cannot write JSONL END, and live sessions are gone.
     * Returns how many rows were updated.
     */
    @Transactional
    public int interruptAllRunning(String reason) {
        String why = reason == null || reason.isBlank() ? "gateway_restart" : reason.trim();
        List<ServerOperationRecordEntity> rows = records.findByRecordKindAndStatus(KIND_OPERATION, STATUS_RUNNING);
        return sealInterrupted(rows, why, 0);
    }

    /**
     * Mark RUNNING operations whose START is older than {@code hours}.
     * Safety net when Gateway never comes back (or never calls interrupt-all).
     */
    @Transactional
    public int interruptStaleRunning(int hours, String reason) {
        if (hours <= 0) {
            return 0;
        }
        Instant cutoff = Instant.now().minus(hours, ChronoUnit.HOURS);
        List<ServerOperationRecordEntity> rows = records.findByRecordKindAndStatusAndOccurredAtBefore(
                KIND_OPERATION, STATUS_RUNNING, cutoff);
        String why = reason == null || reason.isBlank() ? "stale_running" : reason.trim();
        return sealInterrupted(rows, why, hours);
    }

    private int sealInterrupted(List<ServerOperationRecordEntity> rows, String reason, int staleAfterHours) {
        if (rows == null || rows.isEmpty()) {
            return 0;
        }
        Instant now = Instant.now();
        int n = 0;
        for (ServerOperationRecordEntity row : rows) {
            if (row.getEndedAt() != null) {
                continue;
            }
            // Already sealed by a concurrent END — skip status flip.
            if (row.getStatus() != null && !STATUS_RUNNING.equalsIgnoreCase(row.getStatus())) {
                continue;
            }
            row.setEndedAt(now);
            row.setSuccess(false);
            row.setStatus(STATUS_INTERRUPTED);
            ObjectNode detail = row.getDetail() != null && row.getDetail().isObject()
                    ? ((ObjectNode) row.getDetail()).deepCopy()
                    : json.createObjectNode();
            detail.put("reason", reason);
            if (staleAfterHours > 0) {
                detail.put("staleAfterHours", staleAfterHours);
            }
            row.setDetail(detail);
            records.save(row);
            n++;
        }
        return n;
    }

    @Transactional(readOnly = true)
    public Map<String, Object> pageOperations(
            UserEntity user,
            UUID assetId,
            String operationType,
            String scope,
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
        String typ = blankToNull(operationType);
        String mode = normalizeScope(scope);

        // Type filter that contradicts the tab scope → empty page.
        if (typ != null) {
            boolean portmapType = TYPE_PORTMAP_TCP.equals(typ) || TYPE_PORTMAP_UDP.equals(typ);
            if ("SESSION".equals(mode) && portmapType) {
                return PageSupport.emptyPage(pageIndex + 1, size);
            }
            if ("PORTMAP".equals(mode) && !portmapType) {
                return PageSupport.emptyPage(pageIndex + 1, size);
            }
        }

        Page<ServerOperationRecordEntity> result;
        if (access.isSuperAdmin(user)) {
            result = records.searchOperations(assetId, typ, mode, fromBound, toBound, pr);
        } else {
            Set<UUID> visible = access.visibleAssetIds(user);
            if (assetId != null) {
                if (!visible.contains(assetId)) {
                    return PageSupport.emptyPage(pageIndex + 1, size);
                }
                result = records.searchOperations(assetId, typ, mode, fromBound, toBound, pr);
            } else if (visible.isEmpty()) {
                return PageSupport.emptyPage(pageIndex + 1, size);
            } else {
                result = records.searchInAssets(visible, typ, mode, fromBound, toBound, pr);
            }
        }

        return PageSupport.pageResult(toViews(result.getContent()), result.getTotalElements(), pageIndex + 1, size);
    }

    /** ALL = no scope filter; SESSION = exclude tunnels; PORTMAP = only tunnels. */
    static String normalizeScope(String scope) {
        if (scope == null || scope.isBlank()) {
            return "ALL";
        }
        return switch (scope.trim().toLowerCase()) {
            case "session" -> "SESSION";
            case "portmap", "port-map", "tunnel" -> "PORTMAP";
            default -> "ALL";
        };
    }

    @Transactional(readOnly = true)
    public Map<String, Object> getOperation(UserEntity user, UUID operationId) {
        ServerOperationRecordEntity op = requireVisibleOperation(user, operationId);
        AssetEntity asset = assets.findById(op.getAssetId()).orElseThrow();

        List<ServerOperationRecordEntity> events = records
                .findByOperationIdOrderByOccurredAtAsc(operationId).stream()
                .filter(r -> KIND_EVENT.equals(r.getRecordKind()))
                .toList();

        Map<String, Object> out = new LinkedHashMap<>();
        Map<UUID, String> groupNames = groups.nameById();
        out.put("operation", toView(op, asset, groupNames));
        out.put("events", events.stream().map(e -> toView(e, asset, groupNames)).toList());
        return out;
    }

    private ServerOperationRecordEntity requireVisibleOperation(UserEntity user, UUID operationId) {
        ServerOperationRecordEntity op = records.findById(operationId)
                .orElseThrow(() -> new IllegalArgumentException("operation not found"));
        AssetEntity asset = assets.findById(op.getAssetId())
                .orElseThrow(() -> new IllegalArgumentException("asset not found"));
        access.assertCanAccessAsset(user, asset);
        return op;
    }

    /** An operation's session recording on local disk. */
    public record Recording(Path file, String format, long size) {}

    /**
     * Resolves the recording an operation's END envelope points at, after the
     * same ACL check as {@link #getOperation}. {@code detail.recordingPath} is
     * spool-relative and comes from the Gateway, so the resolved file must stay
     * under the configured audit directory.
     */
    @Transactional(readOnly = true)
    public Recording openRecording(UserEntity user, UUID operationId) {
        ServerOperationRecordEntity op = requireVisibleOperation(user, operationId);
        String rel = detailText(op, "recordingPath");
        if (rel == null) {
            throw new IllegalArgumentException("operation has no recording");
        }
        String dir = props.auditDir();
        if (dir == null || dir.isBlank()) {
            throw new IllegalStateException("recording storage not configured (OPS_AUDIT_DIR)");
        }
        Path root = Path.of(dir.trim()).toAbsolutePath().normalize();
        Path file = root.resolve(rel).normalize();
        if (!file.startsWith(root)) {
            throw new IllegalArgumentException("recording path outside the audit directory");
        }
        if (!Files.isRegularFile(file)) {
            throw new IllegalArgumentException("recording file missing");
        }
        long size;
        try {
            size = Files.size(file);
        } catch (IOException e) {
            throw new IllegalStateException("recording unreadable: " + e.getMessage());
        }
        return new Recording(file, orDefault(detailText(op, "format"), "unknown"), size);
    }

    /**
     * Tunnel history from the Gateway spool. Gateway stores the mapping id in
     * {@code detail.mappingId}, so filtering happens here rather than in SQL.
     */
    @Transactional(readOnly = true)
    public List<Map<String, Object>> listPortmapOperations(UUID assetId, UUID mappingId) {
        Collection<String> types = List.of(TYPE_PORTMAP_TCP, TYPE_PORTMAP_UDP);
        List<ServerOperationRecordEntity> rows = assetId == null
                ? records.findByRecordKindAndOperationTypeInOrderByOccurredAtDesc(KIND_OPERATION, types)
                : records.findByRecordKindAndOperationTypeInAndAssetIdOrderByOccurredAtDesc(
                        KIND_OPERATION, types, assetId);
        if (mappingId != null) {
            String needle = mappingId.toString();
            rows = rows.stream().filter(r -> needle.equals(detailText(r, "mappingId"))).toList();
        }
        return toViews(rows);
    }

    private static String detailText(ServerOperationRecordEntity row, String field) {
        JsonNode detail = row.getDetail();
        if (detail == null || !detail.isObject()) {
            return null;
        }
        JsonNode v = detail.get(field);
        return v == null || v.isNull() ? null : v.asText();
    }

    private List<Map<String, Object>> toViews(List<ServerOperationRecordEntity> rows) {
        Map<UUID, AssetEntity> assetById = new HashMap<>();
        List<UUID> assetIds = rows.stream()
                .map(ServerOperationRecordEntity::getAssetId)
                .filter(Objects::nonNull)
                .distinct()
                .toList();
        for (AssetEntity a : assets.findAllById(assetIds)) {
            assetById.put(a.getId(), a);
        }
        Map<UUID, String> groupNames = groups.nameById();
        List<Map<String, Object>> out = new ArrayList<>(rows.size());
        for (ServerOperationRecordEntity r : rows) {
            out.add(toView(r, assetById.get(r.getAssetId()), groupNames));
        }
        return out;
    }

    private Map<String, Object> toView(
            ServerOperationRecordEntity r, AssetEntity asset, Map<UUID, String> groupNames) {
        Map<String, Object> m = new LinkedHashMap<>();
        m.put("id", r.getId().toString());
        m.put("operationId", r.getOperationId().toString());
        m.put("recordKind", r.getRecordKind());
        m.put("operationType", r.getOperationType());
        m.put("eventType", r.getEventType());
        m.put("assetId", r.getAssetId().toString());
        m.put("userId", r.getUserId() == null ? null : r.getUserId().toString());
        m.put("username", r.getUsername() == null ? "" : r.getUsername());
        m.put("occurredAt", r.getOccurredAt().toString());
        m.put("endedAt", r.getEndedAt() == null ? null : r.getEndedAt().toString());
        m.put("status", r.getStatus());
        m.put("success", r.getSuccess());
        m.put("detail", r.getDetail());
        if (asset != null) {
            m.put("assetDisplayName", asset.getDisplayName());
            m.put("hostname", asset.getHostname() == null ? "" : asset.getHostname());
            m.put("groupName", asset.getGroupId() == null
                    ? "" : groupNames.getOrDefault(asset.getGroupId(), ""));
        } else {
            m.put("assetDisplayName", "");
            m.put("hostname", "");
            m.put("groupName", "");
        }
        return m;
    }

    private JsonNode mergeDetail(JsonNode current, JsonNode incoming) {
        ObjectNode merged = current instanceof ObjectNode obj
                ? obj.deepCopy()
                : json.createObjectNode();
        if (incoming != null && incoming.isObject()) {
            merged.setAll((ObjectNode) incoming);
        } else if (incoming != null && !incoming.isNull()) {
            merged.set("value", incoming);
        }
        return merged;
    }

    private static String requireType(JsonNode envelope) {
        String type = text(envelope, "operationType");
        if (type == null) {
            throw new IllegalArgumentException("operationType required");
        }
        return upper(type);
    }

    private static String text(JsonNode node, String field) {
        JsonNode v = node.get(field);
        if (v == null || v.isNull()) {
            return null;
        }
        String s = v.asText().trim();
        return s.isEmpty() ? null : s;
    }

    private static UUID uuid(JsonNode node, String field) {
        String s = text(node, field);
        if (s == null) {
            return null;
        }
        try {
            return UUID.fromString(s);
        } catch (IllegalArgumentException e) {
            throw new IllegalArgumentException("invalid uuid for " + field + ": " + s);
        }
    }

    private static Instant instant(JsonNode node, String field, Instant fallback) {
        String s = text(node, field);
        if (s == null) {
            return fallback;
        }
        try {
            return Instant.parse(s);
        } catch (DateTimeParseException e) {
            return fallback;
        }
    }

    private static Boolean bool(JsonNode node, String field, Boolean fallback) {
        JsonNode v = node.get(field);
        if (v == null || v.isNull()) {
            return fallback;
        }
        return v.asBoolean();
    }

    private static String orDefault(String value, String fallback) {
        return value == null ? fallback : value;
    }

    private static String upper(String s) {
        return s == null ? "" : s.toUpperCase(Locale.ROOT);
    }

    private static String blankToNull(String s) {
        return s == null || s.isBlank() ? null : s.trim().toUpperCase(Locale.ROOT);
    }
}
