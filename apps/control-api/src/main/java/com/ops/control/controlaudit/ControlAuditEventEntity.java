package com.ops.control.controlaudit;

import jakarta.persistence.*;
import java.time.Instant;
import java.util.UUID;

/**
 * Control-plane audit: who changed what in the bastion itself (login, users,
 * assets, groups, port-mapping CRUD, deploy tokens). Runtime server activity
 * lives in {@code server_operation_records}.
 */
@Entity
@Table(name = "control_audit_events", indexes = {
        @Index(name = "idx_control_audit_occurred_at", columnList = "occurred_at"),
        @Index(name = "idx_control_audit_asset_id", columnList = "asset_id"),
        @Index(name = "idx_control_audit_group_id", columnList = "group_id"),
        @Index(name = "idx_control_audit_category", columnList = "category")
})
public class ControlAuditEventEntity {
    @Id
    @GeneratedValue(strategy = GenerationType.UUID)
    private UUID id;

    @Column(name = "occurred_at", nullable = false)
    private Instant occurredAt = Instant.now();

    /** AUTH | USER | ASSET | GROUP | PORTMAP | CI */
    @Column(nullable = false, length = 32)
    private String category;

    /** LOGIN | CREATE | UPDATE | REMOVE | SET_SCOPES | REGISTER … */
    @Column(nullable = false, length = 64)
    private String action;

    @Column(name = "user_id")
    private UUID userId;

    @Column(length = 128)
    private String username = "";

    @Column(name = "asset_id")
    private UUID assetId;

    @Column(name = "group_id")
    private UUID groupId;

    @Column(name = "session_id")
    private UUID sessionId;

    /** Compact JSON / free text for changed fields, ports, token ids, etc. */
    @Column(length = 2000)
    private String detail = "";

    public UUID getId() { return id; }
    public void setId(UUID id) { this.id = id; }
    public Instant getOccurredAt() { return occurredAt; }
    public void setOccurredAt(Instant occurredAt) { this.occurredAt = occurredAt; }
    public String getCategory() { return category; }
    public void setCategory(String category) { this.category = category; }
    public String getAction() { return action; }
    public void setAction(String action) { this.action = action; }
    public UUID getUserId() { return userId; }
    public void setUserId(UUID userId) { this.userId = userId; }
    public String getUsername() { return username; }
    public void setUsername(String username) { this.username = username; }
    public UUID getAssetId() { return assetId; }
    public void setAssetId(UUID assetId) { this.assetId = assetId; }
    public UUID getGroupId() { return groupId; }
    public void setGroupId(UUID groupId) { this.groupId = groupId; }
    public UUID getSessionId() { return sessionId; }
    public void setSessionId(UUID sessionId) { this.sessionId = sessionId; }
    public String getDetail() { return detail; }
    public void setDetail(String detail) { this.detail = detail; }
}
