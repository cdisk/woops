package com.ops.control.metrics;

import jakarta.persistence.*;
import java.time.Instant;
import java.util.UUID;

@Entity
@Table(name = "asset_alert_status")
public class AssetAlertStatusEntity {
    @Id
    @Column(name = "asset_id")
    private UUID assetId;

    @Column(nullable = false)
    private boolean firing = false;

    @Column(name = "summary", length = 1024)
    private String summary = "";

    @Column(name = "updated_at", nullable = false)
    private Instant updatedAt = Instant.now();

    public UUID getAssetId() { return assetId; }
    public void setAssetId(UUID assetId) { this.assetId = assetId; }
    public boolean isFiring() { return firing; }
    public void setFiring(boolean firing) { this.firing = firing; }
    public String getSummary() { return summary; }
    public void setSummary(String summary) { this.summary = summary; }
    public Instant getUpdatedAt() { return updatedAt; }
    public void setUpdatedAt(Instant updatedAt) { this.updatedAt = updatedAt; }
}
