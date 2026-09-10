package com.ops.control.metrics;

import jakarta.persistence.*;
import java.time.Instant;
import java.util.UUID;

@Entity
@Table(name = "monitor_history")
@IdClass(MonitorDataId.class)
public class MonitorDataEntity {
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
    @Column(name = "time", nullable = false)
    private Instant time;

    @Column(name = "metric_value", nullable = false)
    private double value;

    public UUID getAssetId() { return assetId; }
    public void setAssetId(UUID assetId) { this.assetId = assetId; }
    public String getItemId() { return itemId; }
    public void setItemId(String itemId) { this.itemId = itemId; }
    public String getInstance() { return instance; }
    public void setInstance(String instance) { this.instance = instance == null ? "" : instance; }
    public Instant getTime() { return time; }
    public void setTime(Instant time) { this.time = time; }
    public double getValue() { return value; }
    public void setValue(double value) { this.value = value; }
}
