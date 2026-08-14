package com.ops.control.metrics;

import java.io.Serializable;
import java.time.Instant;
import java.util.Objects;
import java.util.UUID;

public class MonitorDataId implements Serializable {
    private UUID assetId;
    private String itemId;
    private String instance;
    private Instant time;

    public MonitorDataId() {}

    public MonitorDataId(UUID assetId, String itemId, String instance, Instant time) {
        this.assetId = assetId;
        this.itemId = itemId;
        this.instance = instance;
        this.time = time;
    }

    @Override
    public boolean equals(Object o) {
        if (this == o) return true;
        if (!(o instanceof MonitorDataId that)) return false;
        return Objects.equals(assetId, that.assetId)
                && Objects.equals(itemId, that.itemId)
                && Objects.equals(instance, that.instance)
                && Objects.equals(time, that.time);
    }

    @Override
    public int hashCode() {
        return Objects.hash(assetId, itemId, instance, time);
    }
}
