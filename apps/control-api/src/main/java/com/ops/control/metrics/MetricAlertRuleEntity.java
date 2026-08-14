package com.ops.control.metrics;

import jakarta.persistence.*;
import java.time.Instant;
import java.util.UUID;

@Entity
@Table(name = "metric_alert_rules")
public class MetricAlertRuleEntity {
    @Id
    private UUID id;

    @Column(nullable = false, length = 128)
    private String name;

    @Column(name = "item_id", nullable = false, length = 128)
    private String itemId;

    /** Empty = all instances; otherwise exact match. */
    @Column(nullable = false, length = 256)
    private String instance = "";

    /** gt | gte | lt | lte */
    @Column(nullable = false, length = 8)
    private String op = "gt";

    @Column(nullable = false)
    private double threshold;

    @Column(nullable = false)
    private boolean enabled = true;

    @Column(name = "created_at", nullable = false)
    private Instant createdAt = Instant.now();

    @Column(name = "updated_at", nullable = false)
    private Instant updatedAt = Instant.now();

    public UUID getId() { return id; }
    public void setId(UUID id) { this.id = id; }
    public String getName() { return name; }
    public void setName(String name) { this.name = name; }
    public String getItemId() { return itemId; }
    public void setItemId(String itemId) { this.itemId = itemId; }
    public String getInstance() { return instance; }
    public void setInstance(String instance) { this.instance = instance == null ? "" : instance; }
    public String getOp() { return op; }
    public void setOp(String op) { this.op = op; }
    public double getThreshold() { return threshold; }
    public void setThreshold(double threshold) { this.threshold = threshold; }
    public boolean isEnabled() { return enabled; }
    public void setEnabled(boolean enabled) { this.enabled = enabled; }
    public Instant getCreatedAt() { return createdAt; }
    public void setCreatedAt(Instant createdAt) { this.createdAt = createdAt; }
    public Instant getUpdatedAt() { return updatedAt; }
    public void setUpdatedAt(Instant updatedAt) { this.updatedAt = updatedAt; }
}
