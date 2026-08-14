package com.ops.control.portmap;

import jakarta.persistence.*;
import java.time.Instant;
import java.util.UUID;

@Entity
@Table(name = "port_mappings")
public class PortMappingEntity {
    public static final String DIR_GATEWAY_TO_ASSET = "gateway_to_asset";
    public static final String DIR_ASSET_TO_GATEWAY = "asset_to_gateway";

    @Id
    @GeneratedValue(strategy = GenerationType.UUID)
    private UUID id;

    @Column(name = "asset_id", nullable = false)
    private UUID assetId;

    /**
     * {@link #DIR_GATEWAY_TO_ASSET} (default / legacy null) = Gateway listens, Agent dials target.
     * {@link #DIR_ASSET_TO_GATEWAY} = Agent listens, Gateway dials target.
     */
    @Column(length = 32)
    private String direction;

    @Column(nullable = false, length = 8)
    private String protocol; // tcp | udp

    @Column(name = "target_host", nullable = false, length = 256)
    private String targetHost;

    @Column(name = "target_port", nullable = false)
    private int targetPort;

    /** Bind address: Gateway 0.0.0.0 for forward; Agent 127.0.0.1 default for reverse. */
    @Column(name = "listen_host", length = 128)
    private String listenHost;

    @Column(name = "listen_port", nullable = false)
    private int listenPort;

    @Column(name = "created_by", length = 128)
    private String createdBy;

    /** Optional human note: what this mapping is for. */
    @Column(length = 256)
    private String remark;

    @Column(name = "created_at", nullable = false)
    private Instant createdAt = Instant.now();

    /** Non-null = soft-deleted; no longer restored on Agent online. */
    @Column(name = "removed_at")
    private Instant removedAt;

    @Column(name = "last_error", length = 1024)
    private String lastError;

    public UUID getId() { return id; }
    public void setId(UUID id) { this.id = id; }
    public UUID getAssetId() { return assetId; }
    public void setAssetId(UUID assetId) { this.assetId = assetId; }
    public String getDirection() { return direction; }
    public void setDirection(String direction) { this.direction = direction; }
    public String getProtocol() { return protocol; }
    public void setProtocol(String protocol) { this.protocol = protocol; }
    public String getTargetHost() { return targetHost; }
    public void setTargetHost(String targetHost) { this.targetHost = targetHost; }
    public int getTargetPort() { return targetPort; }
    public void setTargetPort(int targetPort) { this.targetPort = targetPort; }
    public String getListenHost() { return listenHost; }
    public void setListenHost(String listenHost) { this.listenHost = listenHost; }
    public int getListenPort() { return listenPort; }
    public void setListenPort(int listenPort) { this.listenPort = listenPort; }
    public String getCreatedBy() { return createdBy; }
    public void setCreatedBy(String createdBy) { this.createdBy = createdBy; }
    public String getRemark() { return remark; }
    public void setRemark(String remark) { this.remark = remark; }
    public Instant getCreatedAt() { return createdAt; }
    public void setCreatedAt(Instant createdAt) { this.createdAt = createdAt; }
    public Instant getRemovedAt() { return removedAt; }
    public void setRemovedAt(Instant removedAt) { this.removedAt = removedAt; }
    public String getLastError() { return lastError; }
    public void setLastError(String lastError) { this.lastError = lastError; }

    /** Resolve persisted/null direction to the canonical forward value. */
    public String effectiveDirection() {
        if (direction == null || direction.isBlank()) {
            return DIR_GATEWAY_TO_ASSET;
        }
        return direction.trim().toLowerCase();
    }

    public String effectiveListenHost() {
        if (listenHost != null && !listenHost.isBlank()) {
            return listenHost.trim();
        }
        return DIR_ASSET_TO_GATEWAY.equals(effectiveDirection()) ? "127.0.0.1" : "0.0.0.0";
    }

    public boolean isReverse() {
        return DIR_ASSET_TO_GATEWAY.equals(effectiveDirection());
    }
}
