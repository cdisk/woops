package com.ops.control.portmap;

import com.fasterxml.jackson.databind.JsonNode;
import com.ops.control.asset.AssetEntity;
import com.ops.control.asset.AssetRepository;
import com.ops.control.common.GatewayClient;
import com.ops.control.controlaudit.ControlAuditService;
import com.ops.control.group.GroupService;
import com.ops.control.serverops.ServerOperationAuditService;
import com.ops.control.session.SessionTicketService;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.net.InetAddress;
import java.time.Instant;
import java.util.*;
import java.util.concurrent.ThreadLocalRandom;
import java.util.stream.Collectors;

@Service
public class PortMappingService {
    private static final int PORT_MIN = 20000;
    private static final int PORT_MAX = 21000;
    private static final int ALLOC_ATTEMPTS = 40;

    private final PortMappingRepository mappings;
    private final AssetRepository assets;
    private final GroupService groups;
    private final GatewayClient gateway;
    private final SessionTicketService tickets;
    private final ControlAuditService audit;
    private final ServerOperationAuditService operations;

    public PortMappingService(
            PortMappingRepository mappings,
            AssetRepository assets,
            GroupService groups,
            GatewayClient gateway,
            SessionTicketService tickets,
            ControlAuditService audit,
            ServerOperationAuditService operations) {
        this.mappings = mappings;
        this.assets = assets;
        this.groups = groups;
        this.gateway = gateway;
        this.tickets = tickets;
        this.audit = audit;
        this.operations = operations;
    }

    @Transactional
    public Map<String, Object> create(
            UUID assetId,
            String direction,
            String protocol,
            String targetHost,
            int targetPort,
            String listenHost,
            Integer listenPort,
            String remark,
            UUID userId,
            String createdBy) {
        AssetEntity asset = assets.findById(assetId)
                .orElseThrow(() -> new IllegalArgumentException("asset not found"));
        if (asset.getAgentTokenHash() == null || asset.getAgentTokenHash().isBlank()) {
            throw new IllegalStateException("asset has no agent");
        }
        String dir = normalizeDirection(direction);
        String proto = protocol == null ? "" : protocol.trim().toLowerCase();
        if (!proto.equals("tcp") && !proto.equals("udp")) {
            throw new IllegalArgumentException("protocol must be tcp or udp");
        }
        if (targetHost == null || targetHost.isBlank()) {
            targetHost = "127.0.0.1";
        }
        targetHost = targetHost.trim();
        if (targetPort <= 0 || targetPort > 65535) {
            throw new IllegalArgumentException("invalid targetPort");
        }
        String bindHost = normalizeListenHost(dir, listenHost);
        String note = normalizeRemark(remark);
        boolean reverse = PortMappingEntity.DIR_ASSET_TO_GATEWAY.equals(dir);
        boolean autoPort = listenPort == null || listenPort <= 0;
        if (!autoPort && (listenPort < 1 || listenPort > 65535)) {
            throw new IllegalArgumentException("invalid listenPort");
        }
        if (!autoPort && !reverse && (listenPort < PORT_MIN || listenPort > PORT_MAX)) {
            throw new IllegalArgumentException("forward listenPort must be in " + PORT_MIN + "-" + PORT_MAX);
        }

        Set<Integer> occupied = occupiedPorts(dir, assetId, proto);
        if (!autoPort && occupied.contains(listenPort)) {
            throw new IllegalArgumentException("listenPort already in use: " + listenPort);
        }

        String lastError = null;
        PortMappingEntity saved = null;
        int attempts = autoPort ? ALLOC_ATTEMPTS : 1;
        for (int i = 0; i < attempts; i++) {
            int candidate = autoPort ? randomPort(occupied) : listenPort;
            occupied.add(candidate);

            PortMappingEntity row = new PortMappingEntity();
            row.setAssetId(asset.getId());
            row.setDirection(dir);
            row.setProtocol(proto);
            row.setTargetHost(targetHost);
            row.setTargetPort(targetPort);
            row.setListenHost(bindHost);
            row.setListenPort(candidate);
            row.setRemark(note);
            row.setCreatedBy(createdBy);
            row.setCreatedAt(Instant.now());
            saved = mappings.save(row);

            try {
                Map<String, Object> openResult = gateway.openPortMap(openPayload(saved));
                boolean listening = openResult.get("listening") == null
                        || Boolean.TRUE.equals(openResult.get("listening"));
                Object actualPort = openResult.get("listenPort");
                if (actualPort instanceof Number n && n.intValue() > 0 && n.intValue() != saved.getListenPort()) {
                    saved.setListenPort(n.intValue());
                }
                String reason = str(openResult.get("reason"));
                if (listening) {
                    saved.setLastError(null);
                } else if ("agent_offline".equals(reason)) {
                    // Expected until Agent reconnects and restore runs.
                    saved.setLastError(null);
                } else if (!reason.isBlank()) {
                    saved.setLastError(reason);
                } else {
                    saved.setLastError(null);
                }
                mappings.save(saved);
                audit.record(
                        ControlAuditService.CAT_PORTMAP,
                        ControlAuditService.ACT_CREATE,
                        userId,
                        createdBy,
                        asset.getId(),
                        null,
                        "{\"mappingId\":\"" + saved.getId()
                                + "\",\"direction\":\"" + dir
                                + "\",\"protocol\":\"" + proto
                                + "\",\"listenHost\":\"" + jsonEscape(saved.getListenHost())
                                + "\",\"listenPort\":" + saved.getListenPort()
                                + ",\"targetHost\":\"" + jsonEscape(saved.getTargetHost())
                                + "\",\"targetPort\":" + saved.getTargetPort()
                                + ",\"remark\":\"" + jsonEscape(note) + "\"}");
                Map<String, Map<String, Object>> runtime = runtimeByMappingId();
                return toView(saved, runtime.get(saved.getId().toString()));
            } catch (RuntimeException e) {
                lastError = e.getMessage();
                saved.setLastError(lastError);
                mappings.save(saved);
                if (i < attempts - 1) {
                    saved.setRemovedAt(Instant.now());
                    mappings.save(saved);
                    saved = null;
                    continue;
                }
            }
        }
        throw new IllegalStateException("无法分配监听端口: " + (lastError == null ? "unknown" : lastError));
    }

    @Transactional
    public Map<String, Object> updateRemark(UUID id, String remark, UUID userId, String username) {
        PortMappingEntity m = mappings.findById(id)
                .orElseThrow(() -> new IllegalArgumentException("mapping not found"));
        if (m.getRemovedAt() != null) {
            throw new IllegalStateException("mapping removed");
        }
        String note = normalizeRemark(remark);
        m.setRemark(note);
        mappings.save(m);
        audit.record(
                ControlAuditService.CAT_PORTMAP,
                ControlAuditService.ACT_UPDATE,
                userId,
                username,
                m.getAssetId(),
                null,
                "{\"mappingId\":\"" + m.getId()
                        + "\",\"remark\":\"" + jsonEscape(note) + "\"}");
        Map<String, Map<String, Object>> runtime = runtimeByMappingId();
        return toView(m, runtime.get(m.getId().toString()));
    }

    @Transactional
    public void remove(UUID id, UUID userId, String username) {
        PortMappingEntity m = mappings.findById(id)
                .orElseThrow(() -> new IllegalArgumentException("mapping not found"));
        if (m.getRemovedAt() != null) {
            return;
        }
        try {
            gateway.closePortMap(m.getId().toString());
        } catch (RuntimeException e) {
            m.setLastError(e.getMessage());
        }
        m.setRemovedAt(Instant.now());
        mappings.save(m);
        audit.record(
                ControlAuditService.CAT_PORTMAP,
                ControlAuditService.ACT_REMOVE,
                userId,
                username,
                m.getAssetId(),
                null,
                "{\"mappingId\":\"" + m.getId()
                        + "\",\"direction\":\"" + m.effectiveDirection()
                        + "\",\"protocol\":\"" + m.getProtocol()
                        + "\",\"listenHost\":\"" + jsonEscape(m.effectiveListenHost())
                        + "\",\"listenPort\":" + m.getListenPort()
                        + ",\"targetHost\":\"" + jsonEscape(m.getTargetHost())
                        + "\",\"targetPort\":" + m.getTargetPort() + "}");
    }

    public List<Map<String, Object>> listActive(UUID assetId) {
        List<PortMappingEntity> rows = assetId == null
                ? mappings.findByRemovedAtIsNullOrderByCreatedAtDesc()
                : mappings.findByAssetIdAndRemovedAtIsNullOrderByCreatedAtDesc(assetId);
        Map<String, Map<String, Object>> runtime = runtimeByMappingId();
        return rows.stream().map(m -> toView(m, runtime.get(m.getId().toString()))).toList();
    }

    public List<Map<String, Object>> runtime() {
        return gateway.runtimeList();
    }

    /**
     * Tunnel history from {@code server_operation_records} (Gateway JSONL ingest).
     * A mappingId narrows to that mapping and implies its asset.
     */
    public List<Map<String, Object>> history(UUID mappingId, UUID assetId) {
        UUID asset = assetId;
        if (mappingId != null) {
            PortMappingEntity m = mappings.findById(mappingId)
                    .orElseThrow(() -> new IllegalArgumentException("mapping not found"));
            asset = m.getAssetId();
        }
        return operations.listPortmapOperations(asset, mappingId).stream()
                .map(PortMappingService::toConnectionView)
                .toList();
    }

    private static Map<String, Object> toConnectionView(Map<String, Object> op) {
        JsonNode detail = op.get("detail") instanceof JsonNode n ? n : null;
        Map<String, Object> out = new LinkedHashMap<>();
        out.put("sessionId", op.get("operationId"));
        out.put("mappingId", detailText(detail, "mappingId"));
        out.put("assetId", op.get("assetId"));
        out.put("protocol", ServerOperationAuditService.TYPE_PORTMAP_UDP.equals(op.get("operationType"))
                ? "udp" : "tcp");
        out.put("direction", detailText(detail, "direction"));
        out.put("listenHost", detailText(detail, "listenHost"));
        out.put("listenPort", detailLong(detail, "listenPort"));
        out.put("clientAddr", detailText(detail, "clientAddr"));
        out.put("openedAt", op.get("occurredAt"));
        out.put("closedAt", op.get("endedAt"));
        out.put("status", op.get("status"));
        out.put("bytesIn", detailLong(detail, "bytesIn"));
        out.put("bytesOut", detailLong(detail, "bytesOut"));
        out.put("error", detailText(detail, "error"));
        out.put("assetDisplayName", op.get("assetDisplayName"));
        return out;
    }

    private static String detailText(JsonNode detail, String field) {
        JsonNode v = detail == null ? null : detail.get(field);
        return v == null || v.isNull() ? "" : v.asText();
    }

    private static long detailLong(JsonNode detail, String field) {
        JsonNode v = detail == null ? null : detail.get(field);
        return v == null || !v.isNumber() ? 0L : v.asLong();
    }

    public List<Map<String, Object>> listForAsset(UUID assetId) {
        return mappings.findByAssetIdAndRemovedAtIsNullOrderByCreatedAtDesc(assetId).stream()
                .map(m -> {
                    Map<String, Object> row = new LinkedHashMap<>();
                    row.put("mappingId", m.getId().toString());
                    row.put("assetId", m.getAssetId().toString());
                    row.put("direction", m.effectiveDirection());
                    row.put("protocol", m.getProtocol());
                    row.put("targetHost", m.getTargetHost());
                    row.put("targetPort", m.getTargetPort());
                    row.put("listenHost", m.effectiveListenHost());
                    row.put("listenPort", m.getListenPort());
                    return row;
                })
                .toList();
    }

    /**
     * Issue the short-lived tunnel ticket Gateway needs to dial the Agent (forward)
     * or that the Agent uses to open a data WSS for reverse tunnels.
     */
    @Transactional(readOnly = true)
    public Map<String, Object> openConnectionRecord(UUID mappingId, String clientAddr) {
        PortMappingEntity m = mappings.findById(mappingId)
                .orElseThrow(() -> new IllegalArgumentException("mapping not found"));
        if (m.getRemovedAt() != null) {
            throw new IllegalStateException("mapping removed");
        }
        UUID sessionId = UUID.randomUUID();
        String ticket = tickets.createPortmapTicket(
                sessionId,
                m.getAssetId(),
                m.getProtocol(),
                m.getTargetHost(),
                m.getTargetPort(),
                m.effectiveDirection());

        Map<String, Object> out = new LinkedHashMap<>();
        out.put("connectionId", sessionId.toString());
        out.put("sessionId", sessionId.toString());
        out.put("ticket", ticket);
        out.put("protocol", m.getProtocol());
        out.put("direction", m.effectiveDirection());
        out.put("targetHost", m.getTargetHost());
        out.put("targetPort", m.getTargetPort());
        out.put("listenHost", m.effectiveListenHost());
        out.put("listenPort", m.getListenPort());
        out.put("assetId", m.getAssetId().toString());
        out.put("mappingId", m.getId().toString());
        out.put("clientAddr", clientAddr == null ? "" : clientAddr);
        return out;
    }

    @Transactional
    public void setLastError(UUID mappingId, String error) {
        mappings.findById(mappingId).ifPresent(m -> {
            m.setLastError(error);
            mappings.save(m);
        });
    }

    private Map<String, Object> openPayload(PortMappingEntity m) {
        Map<String, Object> payload = new LinkedHashMap<>();
        payload.put("mappingId", m.getId().toString());
        payload.put("assetId", m.getAssetId().toString());
        payload.put("direction", m.effectiveDirection());
        payload.put("protocol", m.getProtocol());
        payload.put("targetHost", m.getTargetHost());
        payload.put("targetPort", m.getTargetPort());
        payload.put("listenHost", m.effectiveListenHost());
        payload.put("listenPort", m.getListenPort());
        return payload;
    }

    private Set<Integer> occupiedPorts(String direction, UUID assetId, String protocol) {
        Set<Integer> occupied = new HashSet<>();
        if (PortMappingEntity.DIR_ASSET_TO_GATEWAY.equals(direction)) {
            occupied.addAll(mappings.findActiveReverseListenPorts(assetId, protocol));
        } else {
            occupied.addAll(mappings.findActiveForwardListenPorts());
            occupied.add(9100);
            occupied.add(9200);
            occupied.add(9201);
            occupied.add(4822);
            try {
                occupied.addAll(gateway.listeningPorts());
            } catch (RuntimeException ignored) {
                // Gateway may be briefly down; still try bind via open.
            }
        }
        return occupied;
    }

    private Map<String, Map<String, Object>> runtimeByMappingId() {
        try {
            return gateway.runtimeList().stream()
                    .filter(r -> r.get("mappingId") != null)
                    .collect(Collectors.toMap(
                            r -> String.valueOf(r.get("mappingId")),
                            r -> r,
                            (a, b) -> a));
        } catch (RuntimeException e) {
            return Map.of();
        }
    }

    private Map<String, Object> toView(PortMappingEntity m, Map<String, Object> runtime) {
        AssetEntity asset = assets.findById(m.getAssetId()).orElse(null);
        Map<UUID, String> groupNames = groups.nameById();
        Map<String, Object> out = new LinkedHashMap<>();
        out.put("id", m.getId().toString());
        out.put("assetId", m.getAssetId().toString());
        out.put("direction", m.effectiveDirection());
        out.put("protocol", m.getProtocol());
        out.put("targetHost", m.getTargetHost());
        out.put("targetPort", m.getTargetPort());
        out.put("listenHost", m.effectiveListenHost());
        out.put("listenPort", m.getListenPort());
        out.put("remark", m.getRemark() == null ? "" : m.getRemark());
        out.put("createdBy", m.getCreatedBy() == null ? "" : m.getCreatedBy());
        out.put("createdAt", m.getCreatedAt().toString());
        out.put("removedAt", m.getRemovedAt() == null ? null : m.getRemovedAt().toString());
        out.put("lastError", m.getLastError() == null ? "" : m.getLastError());
        if (runtime != null) {
            Object listening = runtime.get("listening");
            out.put("listening", !(listening instanceof Boolean b) || b);
            out.put("bytesIn", runtime.getOrDefault("bytesIn", 0));
            out.put("bytesOut", runtime.getOrDefault("bytesOut", 0));
            out.put("activeConns", runtime.getOrDefault("activeConns", 0));
            out.put("runtimeCreatedAt", runtime.get("createdAt"));
            out.put("runtimeError", runtime.getOrDefault("lastError", ""));
        } else {
            out.put("listening", false);
            out.put("bytesIn", 0);
            out.put("bytesOut", 0);
            out.put("activeConns", 0);
            out.put("runtimeError", "");
        }
        if (asset != null) {
            out.put("assetDisplayName", asset.getDisplayName());
            out.put("hostname", asset.getHostname() == null ? "" : asset.getHostname());
            out.put("online", asset.isOnline());
            out.put("publicIp", asset.getPublicIp() == null ? "" : asset.getPublicIp());
            out.put("privateIp", asset.getPrivateIp() == null ? "" : asset.getPrivateIp());
            out.put("groupId", asset.getGroupId() == null ? null : asset.getGroupId().toString());
            out.put("groupName", asset.getGroupId() == null ? "根" : groupNames.getOrDefault(asset.getGroupId(), ""));
            out.put("os", asset.getOs() == null ? "" : asset.getOs());
        }
        return out;
    }

    private static String normalizeDirection(String direction) {
        if (direction == null || direction.isBlank()) {
            return PortMappingEntity.DIR_GATEWAY_TO_ASSET;
        }
        String d = direction.trim().toLowerCase();
        if (d.equals("forward") || d.equals("gateway_to_asset")) {
            return PortMappingEntity.DIR_GATEWAY_TO_ASSET;
        }
        if (d.equals("reverse") || d.equals("asset_to_gateway")) {
            return PortMappingEntity.DIR_ASSET_TO_GATEWAY;
        }
        throw new IllegalArgumentException("direction must be gateway_to_asset or asset_to_gateway");
    }

    private static String normalizeListenHost(String direction, String listenHost) {
        String def = PortMappingEntity.DIR_ASSET_TO_GATEWAY.equals(direction) ? "127.0.0.1" : "0.0.0.0";
        if (listenHost == null || listenHost.isBlank()) {
            return def;
        }
        String host = listenHost.trim();
        if ("*".equals(host) || "any".equalsIgnoreCase(host)) {
            return "0.0.0.0";
        }
        try {
            InetAddress.getByName(host);
        } catch (Exception e) {
            throw new IllegalArgumentException("invalid listenHost: " + host);
        }
        return host;
    }

    private static int randomPort(Set<Integer> occupied) {
        ThreadLocalRandom rnd = ThreadLocalRandom.current();
        for (int i = 0; i < 200; i++) {
            int p = rnd.nextInt(PORT_MIN, PORT_MAX + 1);
            if (!occupied.contains(p)) {
                return p;
            }
        }
        throw new IllegalStateException("no free listen port in range");
    }

    private static String normalizeRemark(String remark) {
        if (remark == null) {
            return "";
        }
        String t = remark.trim();
        if (t.length() > 256) {
            throw new IllegalArgumentException("remark too long (max 256)");
        }
        return t;
    }

    private static String jsonEscape(String s) {
        if (s == null || s.isEmpty()) {
            return "";
        }
        return s.replace("\\", "\\\\").replace("\"", "\\\"");
    }

    private static String str(Object v) {
        return v == null ? "" : String.valueOf(v);
    }
}
