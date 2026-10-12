package com.ops.control.metrics.notify;

import jakarta.persistence.*;

import java.time.Instant;
import java.util.UUID;

@Entity
@Table(name = "alert_notify_webhooks")
public class AlertNotifyWebhookEntity {
    public static final String SCOPE_ALL = "ALL";
    public static final String SCOPE_GROUP = "GROUP";
    public static final String SCOPE_ASSET = "ASSET";

    @Id
    private UUID id;

    @Column(nullable = false, length = 128)
    private String name;

    @Column(nullable = false, length = 2048)
    private String url;

    @Column(name = "bearer_token", length = 512)
    private String bearerToken = "";

    @Column(nullable = false)
    private boolean enabled = true;

    /** ALL | GROUP | ASSET */
    @Column(name = "scope_type", nullable = false, length = 16)
    private String scopeType = SCOPE_ALL;

    @Column(name = "scope_id")
    private UUID scopeId;

    @Column(name = "created_at", nullable = false)
    private Instant createdAt = Instant.now();

    @Column(name = "updated_at", nullable = false)
    private Instant updatedAt = Instant.now();

    public UUID getId() { return id; }
    public void setId(UUID id) { this.id = id; }
    public String getName() { return name; }
    public void setName(String name) { this.name = name; }
    public String getUrl() { return url; }
    public void setUrl(String url) { this.url = url; }
    public String getBearerToken() { return bearerToken; }
    public void setBearerToken(String bearerToken) { this.bearerToken = bearerToken == null ? "" : bearerToken; }
    public boolean isEnabled() { return enabled; }
    public void setEnabled(boolean enabled) { this.enabled = enabled; }
    public String getScopeType() { return scopeType; }
    public void setScopeType(String scopeType) { this.scopeType = scopeType; }
    public UUID getScopeId() { return scopeId; }
    public void setScopeId(UUID scopeId) { this.scopeId = scopeId; }
    public Instant getCreatedAt() { return createdAt; }
    public void setCreatedAt(Instant createdAt) { this.createdAt = createdAt; }
    public Instant getUpdatedAt() { return updatedAt; }
    public void setUpdatedAt(Instant updatedAt) { this.updatedAt = updatedAt; }

    public boolean hasToken() {
        return bearerToken != null && !bearerToken.isBlank();
    }
}
