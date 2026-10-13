package com.ops.control.metrics;

import jakarta.persistence.*;

import java.time.Instant;
import java.util.UUID;

@Entity
@Table(name = "alert_pending")
@IdClass(AlertPendingId.class)
public class AlertPendingEntity {
    public static final String PHASE_FIRE = "FIRE";
    public static final String PHASE_RECOVER = "RECOVER";

    @Id
    @Column(name = "asset_id", nullable = false)
    private UUID assetId;

    @Id
    @Column(name = "rule_id", nullable = false)
    private UUID ruleId;

    @Id
    @Column(name = "item_id", nullable = false, length = 128)
    private String itemId = "";

    @Id
    @Column(nullable = false, length = 256)
    private String instance = "";

    @Column(nullable = false, length = 16)
    private String phase;

    @Column(name = "since_at", nullable = false)
    private Instant sinceAt = Instant.now();

    @Column(name = "latest_value")
    private Double latestValue;

    @Column(length = 1024)
    private String label = "";

    @Column(name = "updated_at", nullable = false)
    private Instant updatedAt = Instant.now();

    public UUID getAssetId() { return assetId; }
    public void setAssetId(UUID assetId) { this.assetId = assetId; }
    public UUID getRuleId() { return ruleId; }
    public void setRuleId(UUID ruleId) { this.ruleId = ruleId; }
    public String getItemId() { return itemId; }
    public void setItemId(String itemId) { this.itemId = itemId == null ? "" : itemId; }
    public String getInstance() { return instance; }
    public void setInstance(String instance) { this.instance = instance == null ? "" : instance; }
    public String getPhase() { return phase; }
    public void setPhase(String phase) { this.phase = phase; }
    public Instant getSinceAt() { return sinceAt; }
    public void setSinceAt(Instant sinceAt) { this.sinceAt = sinceAt; }
    public Double getLatestValue() { return latestValue; }
    public void setLatestValue(Double latestValue) { this.latestValue = latestValue; }
    public String getLabel() { return label; }
    public void setLabel(String label) { this.label = label == null ? "" : label; }
    public Instant getUpdatedAt() { return updatedAt; }
    public void setUpdatedAt(Instant updatedAt) { this.updatedAt = updatedAt; }
}
