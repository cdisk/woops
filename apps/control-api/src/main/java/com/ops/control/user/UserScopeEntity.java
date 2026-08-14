package com.ops.control.user;

import jakarta.persistence.*;
import java.util.UUID;

@Entity
@Table(
        name = "user_scopes",
        uniqueConstraints = @UniqueConstraint(columnNames = {"user_id", "scope_type", "scope_id"}))
public class UserScopeEntity {
    @Id
    @GeneratedValue(strategy = GenerationType.UUID)
    private UUID id;

    @Column(name = "user_id", nullable = false)
    private UUID userId;

    /** GROUP or ASSET */
    @Column(name = "scope_type", nullable = false, length = 16)
    private String scopeType;

    @Column(name = "scope_id", nullable = false)
    private UUID scopeId;

    public UUID getId() { return id; }
    public void setId(UUID id) { this.id = id; }
    public UUID getUserId() { return userId; }
    public void setUserId(UUID userId) { this.userId = userId; }
    public String getScopeType() { return scopeType; }
    public void setScopeType(String scopeType) { this.scopeType = scopeType; }
    public UUID getScopeId() { return scopeId; }
    public void setScopeId(UUID scopeId) { this.scopeId = scopeId; }
}
