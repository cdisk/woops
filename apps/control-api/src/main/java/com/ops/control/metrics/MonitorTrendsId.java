package com.ops.control.metrics;

import java.io.Serializable;
import java.time.Instant;
import java.util.Objects;
import java.util.UUID;

public class MonitorTrendsId implements Serializable {
    private UUID assetId;
    private String itemId;
    private String instance;
    private Instant hour;

    public MonitorTrendsId() {}

    public MonitorTrendsId(UUID assetId, String itemId, String instance, Instant hour) {
        this.assetId = assetId;
        this.itemId = itemId;
        this.instance = instance;
        this.hour = hour;
    }

    @Override
    public boolean equals(Object o) {
        if (this == o) return true;
        if (!(o instanceof MonitorTrendsId that)) return false;
        return Objects.equals(assetId, that.assetId)
                && Objects.equals(itemId, that.itemId)
                && Objects.equals(instance, that.instance)
                && Objects.equals(hour, that.hour);
    }

    @Override
    public int hashCode() {
        return Objects.hash(assetId, itemId, instance, hour);
    }
}
