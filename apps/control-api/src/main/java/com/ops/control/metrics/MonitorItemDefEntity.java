package com.ops.control.metrics;

import jakarta.persistence.*;

@Entity
@Table(name = "monitor_item_def")
public class MonitorItemDefEntity {
    @Id
    @Column(name = "item_id", length = 128)
    private String itemId;

    @Column(nullable = false, length = 128)
    private String name;

    @Column(nullable = false, length = 32)
    private String unit;

    @Column(name = "chart_default", nullable = false)
    private boolean chartDefault;

    public String getItemId() { return itemId; }
    public void setItemId(String itemId) { this.itemId = itemId; }
    public String getName() { return name; }
    public void setName(String name) { this.name = name; }
    public String getUnit() { return unit; }
    public void setUnit(String unit) { this.unit = unit; }
    public boolean isChartDefault() { return chartDefault; }
    public void setChartDefault(boolean chartDefault) { this.chartDefault = chartDefault; }
}
