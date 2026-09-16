package com.ops.control.ci;

import jakarta.persistence.*;
import java.time.Instant;
import java.util.UUID;

@Entity
@Table(name = "deploy_tokens")
public class DeployTokenEntity {
    @Id
    @GeneratedValue(strategy = GenerationType.UUID)
    private UUID id;

    @Column(name = "asset_id", nullable = false)
    private UUID assetId;

    /** Optional human note. */
    @Column(length = 512)
    private String remark;

    /** SHA-256 hex of the secret segment (not the full plaintext token). */
    @Column(name = "secret_hash", nullable = false, length = 64)
    private String secretHash;

    @Column(name = "allow_upload", nullable = false)
    private boolean allowUpload;

    @Column(name = "allow_download", nullable = false, columnDefinition = "boolean not null default false")
    private boolean allowDownload;

    @Column(name = "allow_exec", nullable = false)
    private boolean allowExec;

    @Column(name = "allow_forward", nullable = false, columnDefinition = "boolean not null default false")
    private boolean allowForward;

    @Column(name = "allow_reverse", nullable = false, columnDefinition = "boolean not null default false")
    private boolean allowReverse;

    /** Null = never expires. */
    @Column(name = "expires_at")
    private Instant expiresAt;

    @Column(name = "created_by")
    private UUID createdBy;

    @Column(name = "created_at", nullable = false)
    private Instant createdAt = Instant.now();

    @Column(name = "last_used_at")
    private Instant lastUsedAt;

    public UUID getId() { return id; }
    public void setId(UUID id) { this.id = id; }
    public UUID getAssetId() { return assetId; }
    public void setAssetId(UUID assetId) { this.assetId = assetId; }
    public String getRemark() { return remark; }
    public void setRemark(String remark) { this.remark = remark; }
    public String getSecretHash() { return secretHash; }
    public void setSecretHash(String secretHash) { this.secretHash = secretHash; }
    public boolean isAllowUpload() { return allowUpload; }
    public void setAllowUpload(boolean allowUpload) { this.allowUpload = allowUpload; }
    public boolean isAllowDownload() { return allowDownload; }
    public void setAllowDownload(boolean allowDownload) { this.allowDownload = allowDownload; }
    public boolean isAllowExec() { return allowExec; }
    public void setAllowExec(boolean allowExec) { this.allowExec = allowExec; }
    public boolean isAllowForward() { return allowForward; }
    public void setAllowForward(boolean allowForward) { this.allowForward = allowForward; }
    public boolean isAllowReverse() { return allowReverse; }
    public void setAllowReverse(boolean allowReverse) { this.allowReverse = allowReverse; }
    public Instant getExpiresAt() { return expiresAt; }
    public void setExpiresAt(Instant expiresAt) { this.expiresAt = expiresAt; }
    public UUID getCreatedBy() { return createdBy; }
    public void setCreatedBy(UUID createdBy) { this.createdBy = createdBy; }
    public Instant getCreatedAt() { return createdAt; }
    public void setCreatedAt(Instant createdAt) { this.createdAt = createdAt; }
    public Instant getLastUsedAt() { return lastUsedAt; }
    public void setLastUsedAt(Instant lastUsedAt) { this.lastUsedAt = lastUsedAt; }
}
