package com.ops.control.user;

import jakarta.persistence.*;
import java.time.Instant;
import java.util.UUID;

@Entity
@Table(name = "users")
public class UserEntity {
    @Id
    @GeneratedValue(strategy = GenerationType.UUID)
    private UUID id;

    @Column(nullable = false, unique = true, length = 128)
    private String username;

    /** Display name; empty/null → UI falls back to username. */
    @Column(length = 128)
    private String nickname;

    @Column(name = "password_hash")
    private String passwordHash;

    /** Base32 TOTP secret; set when local user enrolls 2FA. */
    @Column(name = "totp_secret", length = 64)
    private String totpSecret;

    @Column(name = "totp_enabled", nullable = false, columnDefinition = "boolean not null default false")
    private boolean totpEnabled = false;

    /** Last consumed TOTP time-step (unix seconds / 30); blocks code replay. */
    @Column(name = "totp_last_step")
    private Long totpLastStep;

    @Column(nullable = false, length = 32)
    private String role = "MEMBER";

    @Column(name = "auth_source", nullable = false, length = 32)
    private String authSource = "local";

    @Column(name = "enabled", nullable = false)
    private boolean enabled = true;

    /** Soft-delete timestamp; null = active. Soft-deleted rows free username via rename. */
    @Column(name = "deleted_at")
    private Instant deletedAt;

    @Column(name = "created_at", nullable = false)
    private Instant createdAt = Instant.now();

    public UUID getId() { return id; }
    public void setId(UUID id) { this.id = id; }
    public String getUsername() { return username; }
    public void setUsername(String username) { this.username = username; }
    public String getNickname() { return nickname; }
    public void setNickname(String nickname) { this.nickname = nickname; }
    public String getPasswordHash() { return passwordHash; }
    public void setPasswordHash(String passwordHash) { this.passwordHash = passwordHash; }
    public String getTotpSecret() { return totpSecret; }
    public void setTotpSecret(String totpSecret) { this.totpSecret = totpSecret; }
    public boolean isTotpEnabled() { return totpEnabled; }
    public void setTotpEnabled(boolean totpEnabled) { this.totpEnabled = totpEnabled; }
    public Long getTotpLastStep() { return totpLastStep; }
    public void setTotpLastStep(Long totpLastStep) { this.totpLastStep = totpLastStep; }
    public String getRole() { return role; }
    public void setRole(String role) { this.role = role; }
    public String getAuthSource() { return authSource; }
    public void setAuthSource(String authSource) { this.authSource = authSource; }
    public boolean isEnabled() { return enabled; }
    public void setEnabled(boolean enabled) { this.enabled = enabled; }
    public Instant getDeletedAt() { return deletedAt; }
    public void setDeletedAt(Instant deletedAt) { this.deletedAt = deletedAt; }
    public Instant getCreatedAt() { return createdAt; }
    public void setCreatedAt(Instant createdAt) { this.createdAt = createdAt; }

    public boolean isDeleted() {
        return deletedAt != null;
    }
}
