package com.ops.control.metrics;

import jakarta.persistence.*;
import java.time.Instant;
import java.util.UUID;

@Entity
@Table(name = "monitor_trends", indexes = {
        @Index(name = "idx_monitor_trends_lookup", columnList = "asset_id,item_id,hour,instance")
})
@IdClass(MonitorTrendsId.class)
public class MonitorTrendsEntity {
    @Id
    @Column(name = "asset_id", nullable = false)
    private UUID assetId;

    @Id
    @Column(name = "item_id", nullable = false, length = 128)
    private String itemId;

    @Id
    @Column(name = "instance", nullable = false, length = 256)
    private String instance = "";

    @Id
    @Column(name = "hour", nullable = false)
    private Instant hour;

    @Column(name = "min_value", nullable = false)
    private double minValue;

    @Column(name = "max_value", nullable = false)
    private double maxValue;

    @Column(name = "avg_value", nullable = false)
    private double avgValue;

    @Column(name = "sample_count", nullable = false)
    private int sampleCount;

    public UUID getAssetId() { return assetId; }
    public void setAssetId(UUID assetId) { this.assetId = assetId; }
    public String getItemId() { return itemId; }
    public void setItemId(String itemId) { this.itemId = itemId; }
    public String getInstance() { return instance; }
    public void setInstance(String instance) { this.instance = instance == null ? "" : instance; }
    public Instant getHour() { return hour; }
    public void setHour(Instant hour) { this.hour = hour; }
    public double getMinValue() { return minValue; }
    public void setMinValue(double minValue) { this.minValue = minValue; }
    public double getMaxValue() { return maxValue; }
    public void setMaxValue(double maxValue) { this.maxValue = maxValue; }
    public double getAvgValue() { return avgValue; }
    public void setAvgValue(double avgValue) { this.avgValue = avgValue; }
    public int getSampleCount() { return sampleCount; }
    public void setSampleCount(int sampleCount) { this.sampleCount = sampleCount; }
}
