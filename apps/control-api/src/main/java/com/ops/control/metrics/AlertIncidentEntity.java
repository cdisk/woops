package com.ops.control.metrics;

import jakarta.persistence.*;

import java.time.Instant;
import java.util.UUID;

@Entity
@Table(name = "alert_incidents")
public class AlertIncidentEntity {
    public static final String REASON_NORMAL = "NORMAL";
    public static final String REASON_INSTANCE_GONE = "INSTANCE_GONE";
    public static final String REASON_ASSET_OFFLINE = "ASSET_OFFLINE";
    public static final String REASON_RULE_CHANGED = "RULE_CHANGED";

    @Id
    private UUID id;

    @Column(name = "asset_id", nullable = false)
    private UUID assetId;

    @Column(name = "rule_id")
    private UUID ruleId;

    @Column(name = "item_id", nullable = false, length = 128)
    private String itemId;

    @Column(nullable = false, length = 256)
    private String instance = "";

    @Column(nullable = false, length = 32)
    private String kind;

    @Column(name = "rule_name", length = 128)
    private String ruleName = "";

    @Column(length = 8)
    private String op = "";

    @Column
    private Double threshold;

    @Column(name = "start_value")
    private Double startValue;

    @Column(name = "latest_value")
    private Double latestValue;

    @Column(length = 1024)
    private String label = "";

    @Column(name = "started_at", nullable = false)
    private Instant startedAt = Instant.now();

    @Column(name = "last_seen_at", nullable = false)
    private Instant lastSeenAt = Instant.now();

    @Column(name = "resolved_at")
    private Instant resolvedAt;

    @Column(name = "resolution_reason", length = 32)
    private String resolutionReason;

    public UUID getId() { return id; }
    public void setId(UUID id) { this.id = id; }
    public UUID getAssetId() { return assetId; }
    public void setAssetId(UUID assetId) { this.assetId = assetId; }
    public UUID getRuleId() { return ruleId; }
    public void setRuleId(UUID ruleId) { this.ruleId = ruleId; }
    public String getItemId() { return itemId; }
    public void setItemId(String itemId) { this.itemId = itemId == null ? "" : itemId; }
    public String getInstance() { return instance; }
    public void setInstance(String instance) { this.instance = instance == null ? "" : instance; }
    public String getKind() { return kind; }
    public void setKind(String kind) { this.kind = kind; }
    public String getRuleName() { return ruleName; }
    public void setRuleName(String ruleName) { this.ruleName = ruleName == null ? "" : ruleName; }
    public String getOp() { return op; }
    public void setOp(String op) { this.op = op == null ? "" : op; }
    public Double getThreshold() { return threshold; }
    public void setThreshold(Double threshold) { this.threshold = threshold; }
    public Double getStartValue() { return startValue; }
    public void setStartValue(Double startValue) { this.startValue = startValue; }
    public Double getLatestValue() { return latestValue; }
    public void setLatestValue(Double latestValue) { this.latestValue = latestValue; }
    public String getLabel() { return label; }
    public void setLabel(String label) { this.label = label == null ? "" : label; }
    public Instant getStartedAt() { return startedAt; }
    public void setStartedAt(Instant startedAt) { this.startedAt = startedAt; }
    public Instant getLastSeenAt() { return lastSeenAt; }
    public void setLastSeenAt(Instant lastSeenAt) { this.lastSeenAt = lastSeenAt; }
    public Instant getResolvedAt() { return resolvedAt; }
    public void setResolvedAt(Instant resolvedAt) { this.resolvedAt = resolvedAt; }
    public String getResolutionReason() { return resolutionReason; }
    public void setResolutionReason(String resolutionReason) { this.resolutionReason = resolutionReason; }

    public boolean isOpen() {
        return resolvedAt == null;
    }
}
