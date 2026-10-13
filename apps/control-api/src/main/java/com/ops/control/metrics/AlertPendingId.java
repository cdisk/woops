package com.ops.control.metrics;

import java.io.Serializable;
import java.util.Objects;
import java.util.UUID;

public class AlertPendingId implements Serializable {
    private UUID assetId;
    private UUID ruleId;
    private String itemId;
    private String instance;

    public AlertPendingId() {}

    public AlertPendingId(UUID assetId, UUID ruleId, String itemId, String instance) {
        this.assetId = assetId;
        this.ruleId = ruleId;
        this.itemId = itemId == null ? "" : itemId;
        this.instance = instance == null ? "" : instance;
    }

    public UUID getAssetId() { return assetId; }
    public void setAssetId(UUID assetId) { this.assetId = assetId; }
    public UUID getRuleId() { return ruleId; }
    public void setRuleId(UUID ruleId) { this.ruleId = ruleId; }
    public String getItemId() { return itemId; }
    public void setItemId(String itemId) { this.itemId = itemId == null ? "" : itemId; }
    public String getInstance() { return instance; }
    public void setInstance(String instance) { this.instance = instance == null ? "" : instance; }

    @Override
    public boolean equals(Object o) {
        if (this == o) return true;
        if (!(o instanceof AlertPendingId that)) return false;
        return Objects.equals(assetId, that.assetId)
                && Objects.equals(ruleId, that.ruleId)
                && Objects.equals(itemId, that.itemId)
                && Objects.equals(instance, that.instance);
    }

    @Override
    public int hashCode() {
        return Objects.hash(assetId, ruleId, itemId, instance);
    }
}
