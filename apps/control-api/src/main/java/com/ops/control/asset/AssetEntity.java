package com.ops.control.asset;

import jakarta.persistence.*;
import org.hibernate.annotations.JdbcTypeCode;
import org.hibernate.type.SqlTypes;

import java.time.Instant;
import java.util.List;
import java.util.UUID;

@Entity
@Table(name = "assets")
public class AssetEntity {
    /** Host identity (= disk asset-id); minted explicitly at register. */
    @Id
    private UUID id;

    @Column(name = "display_name", nullable = false, length = 256)
    private String displayName;

    /** Free-form operator note; empty string when unset. */
    @Column(length = 4096)
    private String remark = "";

    @Column(length = 256)
    private String hostname;

    /** Pretty OS string from install/register (e.g. "Windows Server 2016", "Debian GNU/Linux 13"). */
    @Column(length = 256)
    private String os;

    @Column(length = 32)
    private String arch;

    /** Agent binary version (e.g. yymmddhhMM), reported at register. */
    @Column(name = "agent_version", length = 32)
    private String agentVersion;

    /** Auto-updated public/WAN IP (read-only in UI). */
    @Column(name = "public_ip", length = 64)
    private String publicIp;

    /** Auto-updated private/LAN IPs, comma-separated (read-only in UI). */
    @Column(name = "private_ip", length = 512)
    private String privateIp;

    /** Null = asset belongs to the virtual root (alongside top-level groups). */
    @Column(name = "group_id")
    private UUID groupId;

    /** RDP/VNC listen port on the agent host (Windows 3389 / Linux 5900 by default). */
    @Column(name = "desktop_port", nullable = false)
    private int desktopPort = 5900;

    @Column(name = "desktop_username", length = 128)
    private String desktopUsername = "";

    @Column(name = "desktop_password")
    private String desktopPassword;

    /** RDP color depth bits for guacd: 8, 16, 24, or 32. Default 16 (low bandwidth). */
    @Column(name = "desktop_color_depth", nullable = false, columnDefinition = "integer not null default 16")
    private int desktopColorDepth = 16;

    /** RDP experience preset for guacd: low | medium | high. Default low. */
    @Column(name = "desktop_rdp_quality", nullable = false, length = 16, columnDefinition = "varchar(16) not null default 'low'")
    private String desktopRdpQuality = "low";

    @Column(name = "agent_token_hash", length = 128)
    private String agentTokenHash;

    @Column(name = "online", nullable = false)
    private boolean online = false;

    @Column(name = "last_seen_at")
    private Instant lastSeenAt;

    @Column(name = "created_at", nullable = false)
    private Instant createdAt = Instant.now();

    @Column(name = "updated_at", nullable = false)
    private Instant updatedAt = Instant.now();

    /** Shared Shell snippets for this asset (multi-line command strings). */
    @JdbcTypeCode(SqlTypes.JSON)
    @Column(name = "shell_commands", columnDefinition = "jsonb")
    private List<String> shellCommands;

    public UUID getId() { return id; }
    public void setId(UUID id) { this.id = id; }
    public String getDisplayName() { return displayName; }
    public void setDisplayName(String displayName) { this.displayName = displayName; }
    public String getRemark() { return remark; }
    public void setRemark(String remark) { this.remark = remark; }
    public String getHostname() { return hostname; }
    public void setHostname(String hostname) { this.hostname = hostname; }
    public String getOs() { return os; }
    public void setOs(String os) { this.os = os; }
    public String getArch() { return arch; }
    public void setArch(String arch) { this.arch = arch; }
    public String getAgentVersion() { return agentVersion; }
    public void setAgentVersion(String agentVersion) { this.agentVersion = agentVersion; }
    public String getPublicIp() { return publicIp; }
    public void setPublicIp(String publicIp) { this.publicIp = publicIp; }
    public String getPrivateIp() { return privateIp; }
    public void setPrivateIp(String privateIp) { this.privateIp = privateIp; }
    public UUID getGroupId() { return groupId; }
    public void setGroupId(UUID groupId) { this.groupId = groupId; }
    public int getDesktopPort() { return desktopPort; }
    public void setDesktopPort(int desktopPort) { this.desktopPort = desktopPort; }
    public String getDesktopUsername() { return desktopUsername; }
    public void setDesktopUsername(String desktopUsername) { this.desktopUsername = desktopUsername; }
    public String getDesktopPassword() { return desktopPassword; }
    public void setDesktopPassword(String desktopPassword) { this.desktopPassword = desktopPassword; }
    public int getDesktopColorDepth() { return desktopColorDepth; }
    public void setDesktopColorDepth(int desktopColorDepth) { this.desktopColorDepth = desktopColorDepth; }
    public String getDesktopRdpQuality() { return desktopRdpQuality; }
    public void setDesktopRdpQuality(String desktopRdpQuality) { this.desktopRdpQuality = desktopRdpQuality; }
    public String getAgentTokenHash() { return agentTokenHash; }
    public void setAgentTokenHash(String agentTokenHash) { this.agentTokenHash = agentTokenHash; }
    public boolean isOnline() { return online; }
    public void setOnline(boolean online) { this.online = online; }
    public Instant getLastSeenAt() { return lastSeenAt; }
    public void setLastSeenAt(Instant lastSeenAt) { this.lastSeenAt = lastSeenAt; }
    public Instant getCreatedAt() { return createdAt; }
    public void setCreatedAt(Instant createdAt) { this.createdAt = createdAt; }
    public Instant getUpdatedAt() { return updatedAt; }
    public void setUpdatedAt(Instant updatedAt) { this.updatedAt = updatedAt; }
    public List<String> getShellCommands() { return shellCommands; }
    public void setShellCommands(List<String> shellCommands) { this.shellCommands = shellCommands; }
}
