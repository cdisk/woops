package com.ops.control.serverops;

import com.fasterxml.jackson.databind.JsonNode;
import jakarta.persistence.*;
import org.hibernate.annotations.JdbcTypeCode;
import org.hibernate.type.SqlTypes;

import java.time.Instant;
import java.util.UUID;

/**
 * Runtime activity on a managed server, ingested from Gateway/Agent envelopes.
 *
 * <p>Two kinds share one table: {@code OPERATION} is the long-lived row for a
 * session/tunnel (one per {@code operationId}, id == operationId), {@code EVENT}
 * rows are the actions and errors that happened inside it.
 */
@Entity
@Table(name = "server_operation_records", indexes = {
        @Index(name = "idx_server_ops_kind_time", columnList = "record_kind,occurred_at"),
        @Index(name = "idx_server_ops_asset_time", columnList = "asset_id,occurred_at"),
        @Index(name = "idx_server_ops_operation_time", columnList = "operation_id,occurred_at"),
        @Index(name = "idx_server_ops_type_time", columnList = "operation_type,occurred_at"),
        @Index(name = "idx_server_ops_status_time", columnList = "status,occurred_at")
})
public class ServerOperationRecordEntity {
    /** Always assigned by the service: operationId for OPERATION, eventId for EVENT. */
    @Id
    private UUID id;

    @Column(name = "operation_id", nullable = false)
    private UUID operationId;

    /** OPERATION | EVENT */
    @Column(name = "record_kind", nullable = false, length = 16)
    private String recordKind;

    /** SHELL | FILE | RDP | VNC | EXEC | PORTMAP_TCP | PORTMAP_UDP … */
    @Column(name = "operation_type", nullable = false, length = 32)
    private String operationType;

    /** Only for EVENT rows: COMMAND | UPLOAD | DOWNLOAD | ERROR … */
    @Column(name = "event_type", length = 64)
    private String eventType;

    @Column(name = "asset_id", nullable = false)
    private UUID assetId;

    @Column(name = "user_id")
    private UUID userId;

    @Column(length = 128)
    private String username;

    @Column(name = "occurred_at", nullable = false)
    private Instant occurredAt = Instant.now();

    /** Only for OPERATION rows, set when the END envelope arrives. */
    @Column(name = "ended_at")
    private Instant endedAt;

    /** RUNNING | COMPLETED | FAILED | INTERRUPTED */
    @Column(length = 32)
    private String status;

    private Boolean success;

    @JdbcTypeCode(SqlTypes.JSON)
    @Column(nullable = false, columnDefinition = "jsonb")
    private JsonNode detail;

    public UUID getId() { return id; }
    public void setId(UUID id) { this.id = id; }
    public UUID getOperationId() { return operationId; }
    public void setOperationId(UUID operationId) { this.operationId = operationId; }
    public String getRecordKind() { return recordKind; }
    public void setRecordKind(String recordKind) { this.recordKind = recordKind; }
    public String getOperationType() { return operationType; }
    public void setOperationType(String operationType) { this.operationType = operationType; }
    public String getEventType() { return eventType; }
    public void setEventType(String eventType) { this.eventType = eventType; }
    public UUID getAssetId() { return assetId; }
    public void setAssetId(UUID assetId) { this.assetId = assetId; }
    public UUID getUserId() { return userId; }
    public void setUserId(UUID userId) { this.userId = userId; }
    public String getUsername() { return username; }
    public void setUsername(String username) { this.username = username; }
    public Instant getOccurredAt() { return occurredAt; }
    public void setOccurredAt(Instant occurredAt) { this.occurredAt = occurredAt; }
    public Instant getEndedAt() { return endedAt; }
    public void setEndedAt(Instant endedAt) { this.endedAt = endedAt; }
    public String getStatus() { return status; }
    public void setStatus(String status) { this.status = status; }
    public Boolean getSuccess() { return success; }
    public void setSuccess(Boolean success) { this.success = success; }
    public JsonNode getDetail() { return detail; }
    public void setDetail(JsonNode detail) { this.detail = detail; }
}
