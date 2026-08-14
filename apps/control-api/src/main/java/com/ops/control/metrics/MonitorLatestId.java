package com.ops.control.metrics;

import java.io.Serializable;
import java.util.Objects;
import java.util.UUID;

public class MonitorLatestId implements Serializable {
    private UUID assetId;
    private String itemId;
    private String instance;

    public MonitorLatestId() {}

    public MonitorLatestId(UUID assetId, String itemId, String instance) {
        this.assetId = assetId;
        this.itemId = itemId;
        this.instance = instance;
    }

    @Override
    public boolean equals(Object o) {
        if (this == o) return true;
        if (!(o instanceof MonitorLatestId that)) return false;
        return Objects.equals(assetId, that.assetId)
                && Objects.equals(itemId, that.itemId)
                && Objects.equals(instance, that.instance);
    }

    @Override
    public int hashCode() {
        return Objects.hash(assetId, itemId, instance);
    }
}
