package com.ops.control.metrics;

import jakarta.persistence.*;

import java.time.Instant;
import java.util.UUID;

@Entity
@Table(name = "asset_alert_ignores")
@IdClass(AssetAlertIgnoreId.class)
public class AssetAlertIgnoreEntity {
    @Id
    @Column(name = "asset_id", nullable = false)
    private UUID assetId;

    /** Monitor item id, or {@link AlertIssueLogic#ITEM_HOST_ONLINE} for expected offline. */
    @Id
    @Column(name = "item_id", nullable = false, length = 128)
    private String itemId;

    @Column(name = "ignored_at", nullable = false)
    private Instant ignoredAt = Instant.now();

    @Column(name = "ignored_by")
    private UUID ignoredBy;

    public UUID getAssetId() { return assetId; }
    public void setAssetId(UUID assetId) { this.assetId = assetId; }
    public String getItemId() { return itemId; }
    public void setItemId(String itemId) { this.itemId = itemId == null ? "" : itemId; }
    public Instant getIgnoredAt() { return ignoredAt; }
    public void setIgnoredAt(Instant ignoredAt) { this.ignoredAt = ignoredAt; }
    public UUID getIgnoredBy() { return ignoredBy; }
    public void setIgnoredBy(UUID ignoredBy) { this.ignoredBy = ignoredBy; }
}
