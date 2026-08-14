package com.ops.control.assetevent;

import jakarta.persistence.*;
import java.time.Instant;
import java.util.UUID;

/**
 * System-observed asset lifecycle / health events (Agent online/offline, later
 * IP/version changes, monitoring loss, etc.). Not human control-plane actions
 * ({@code control_audit_events}) and not operator-on-server sessions
 * ({@code server_operation_records}).
 */
@Entity
@Table(name = "asset_events", indexes = {
        @Index(name = "idx_asset_events_occurred_at", columnList = "occurred_at"),
        @Index(name = "idx_asset_events_asset_id", columnList = "asset_id"),
        @Index(name = "idx_asset_events_category", columnList = "category"),
        @Index(name = "idx_asset_events_event_type", columnList = "event_type")
})
public class AssetEventEntity {
    @Id
    @GeneratedValue(strategy = GenerationType.UUID)
    private UUID id;

    @Column(name = "asset_id", nullable = false)
    private UUID assetId;

    /** CONNECTIVITY | AGENT | NETWORK | MONITORING | RUNTIME (extensible). */
    @Column(nullable = false, length = 32)
    private String category;

    /** ONLINE | OFFLINE | … (extensible). */
    @Column(name = "event_type", nullable = false, length = 64)
    private String eventType;

    /** INFO | WARNING | ERROR */
    @Column(nullable = false, length = 16)
    private String severity = "INFO";

    @Column(name = "occurred_at", nullable = false)
    private Instant occurredAt = Instant.now();

    /** Compact JSON: peerIp, connectionId, gatewayInstance, reason, … */
    @Column(length = 2000)
    private String detail = "";

    @Column(name = "source_instance", length = 128)
    private String sourceInstance = "";

    @Column(name = "connection_id", length = 64)
    private String connectionId = "";

    public UUID getId() { return id; }
    public void setId(UUID id) { this.id = id; }
    public UUID getAssetId() { return assetId; }
    public void setAssetId(UUID assetId) { this.assetId = assetId; }
    public String getCategory() { return category; }
    public void setCategory(String category) { this.category = category; }
    public String getEventType() { return eventType; }
    public void setEventType(String eventType) { this.eventType = eventType; }
    public String getSeverity() { return severity; }
    public void setSeverity(String severity) { this.severity = severity; }
    public Instant getOccurredAt() { return occurredAt; }
    public void setOccurredAt(Instant occurredAt) { this.occurredAt = occurredAt; }
    public String getDetail() { return detail; }
    public void setDetail(String detail) { this.detail = detail; }
    public String getSourceInstance() { return sourceInstance; }
    public void setSourceInstance(String sourceInstance) { this.sourceInstance = sourceInstance; }
    public String getConnectionId() { return connectionId; }
    public void setConnectionId(String connectionId) { this.connectionId = connectionId; }
}
