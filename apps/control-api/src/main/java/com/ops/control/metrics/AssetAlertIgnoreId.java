package com.ops.control.metrics;

import java.io.Serializable;
import java.util.Objects;
import java.util.UUID;

public class AssetAlertIgnoreId implements Serializable {
    private UUID assetId;
    private String itemId;

    public AssetAlertIgnoreId() {}

    public AssetAlertIgnoreId(UUID assetId, String itemId) {
        this.assetId = assetId;
        this.itemId = itemId;
    }

    public UUID getAssetId() { return assetId; }
    public void setAssetId(UUID assetId) { this.assetId = assetId; }
    public String getItemId() { return itemId; }
    public void setItemId(String itemId) { this.itemId = itemId; }

    @Override
    public boolean equals(Object o) {
        if (this == o) return true;
        if (!(o instanceof AssetAlertIgnoreId that)) return false;
        return Objects.equals(assetId, that.assetId) && Objects.equals(itemId, that.itemId);
    }

    @Override
    public int hashCode() {
        return Objects.hash(assetId, itemId);
    }
}
