package com.ops.control.session;

import com.ops.control.access.AccessService;
import com.ops.control.asset.AssetEntity;
import com.ops.control.asset.AssetRepository;
import com.ops.control.common.OpsProperties;
import com.ops.control.group.GroupService;
import com.ops.control.protocol.ProtocolRegistry;
import com.ops.control.protocol.desktop.DesktopTicketIssuer;
import com.ops.control.protocol.exec.ExecTicketIssuer;
import com.ops.control.protocol.filemanager.FileManagerTicketIssuer;
import com.ops.control.protocol.filetransfer.FileTransferTicketIssuer;
import com.ops.control.protocol.portmap.PortmapTicketIssuer;
import com.ops.control.protocol.shell.ShellTicketIssuer;
import com.ops.control.user.UserEntity;
import com.ops.control.user.UserRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.doNothing;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class SessionTicketDesktopClaimsTest {

    @Mock AssetRepository assets;
    @Mock UserRepository users;
    @Mock AccessService access;
    @Mock GroupService groups;

    SessionTicketService tickets;
    UUID userId;
    UUID assetId;
    UserEntity user;
    AssetEntity asset;

    @BeforeEach
    void setUp() {
        OpsProperties props = new OpsProperties(
                "https://127.0.0.1:9100",
                "https://127.0.0.1:9200",
                "wss://127.0.0.1:9200",
                "http://127.0.0.1:9201",
                "https://127.0.0.1:5173",
                "jwt-secret-must-be-long-enough-0123456789abcd",
                "ticket-secret-must-be-long-enough-0123456789ab",
                "admin",
                "admin123",
                15,
                "",
                "./data/ops-audit",
                5,
                90,
                24,
                null
        );
        TicketSigner signer = new TicketSigner(props);
        signer.initForTests();
        TicketVerifier verifier = new TicketVerifier(signer);
        TicketResponseBuilder responses = new TicketResponseBuilder(props, groups);
        ProtocolRegistry registry = new ProtocolRegistry(List.of(
                new ShellTicketIssuer(),
                new FileManagerTicketIssuer(),
                new FileTransferTicketIssuer(),
                new ExecTicketIssuer(),
                new DesktopTicketIssuer()));
        PortmapTicketIssuer portmap = new PortmapTicketIssuer(signer);
        tickets = new SessionTicketService(
                assets, users, access, registry, signer, verifier, responses, portmap);

        userId = UUID.randomUUID();
        assetId = UUID.randomUUID();
        user = new UserEntity();
        user.setId(userId);
        user.setUsername("alice");

        asset = new AssetEntity();
        asset.setId(assetId);
        asset.setDisplayName("host-a");
        asset.setHostname("HOST-A");
        asset.setOs("Debian GNU/Linux 13");
        asset.setOnline(true);
        asset.setAgentTokenHash("hash");
        asset.setDesktopPort(5901);
        asset.setDesktopUsername("");
        asset.setDesktopPassword("vnc-secret");
        asset.setPrivateIp("10.0.0.8");
    }

    @Test
    void vncTicketUsesConfiguredDesktopPortAndDesktopClaimsOnly() {
        stubAccess();
        Map<String, Object> out = tickets.createTicket(userId, "alice", assetId, "vnc");
        assertEquals(5901, out.get("targetPort"));
        assertTrueBrowserDesktop(out);

        Map<String, Object> claims = tickets.verifyForGateway((String) out.get("ticket"));
        assertEquals("vnc", claims.get("type"));
        assertEquals(5901, claims.get("targetPort"));
        assertEquals("vnc-secret", claims.get("desktopPassword"));
        assertFalse(claims.containsKey("sshUsername"));
        assertFalse(claims.containsKey("sshPassword"));
    }

    @Test
    void rdpTicketUsesRfc1918LanIp() {
        asset.setOs("Windows Server 2016");
        asset.setDesktopPort(3390);
        asset.setDesktopUsername("Administrator");
        asset.setDesktopPassword("rdp-secret");
        stubAccess();

        Map<String, Object> out = tickets.createTicket(userId, "alice", assetId, "rdp");
        assertEquals(3390, out.get("targetPort"));
        assertEquals("10.0.0.8", out.get("targetHost"));

        Map<String, Object> claims = tickets.verifyForGateway((String) out.get("ticket"));
        assertEquals("Administrator", claims.get("desktopUsername"));
        assertEquals("rdp-secret", claims.get("desktopPassword"));
        assertFalse(claims.containsKey("sshPassword"));
    }

    @Test
    void rdpTicketSkipsVirtualNicAndNonRfc1918() {
        asset.setOs("Windows Server 2012");
        asset.setDesktopPort(3389);
        asset.setDesktopUsername("Administrator");
        asset.setDesktopPassword("rdp-secret");
        // Virtual NIC first, enterprise intranet second — neither is RFC1918.
        asset.setPrivateIp("172.80.0.81,188.188.185.250");
        stubAccess();

        Map<String, Object> out = tickets.createTicket(userId, "alice", assetId, "rdp");
        assertEquals("127.0.0.1", out.get("targetHost"));
    }

    @Test
    void rdpTicketPrefersRfc1918InCsv() {
        asset.setOs("Windows Server 2016");
        asset.setDesktopPort(3389);
        asset.setDesktopUsername("Administrator");
        asset.setDesktopPassword("rdp-secret");
        asset.setPrivateIp("172.80.0.81,10.1.2.3,188.188.185.250");
        stubAccess();

        Map<String, Object> out = tickets.createTicket(userId, "alice", assetId, "rdp");
        assertEquals("10.1.2.3", out.get("targetHost"));
    }

    @Test
    void preferRdpDialHostHelpers() {
        assertEquals("127.0.0.1", DesktopTicketIssuer.preferRdpDialHost(null));
        assertEquals("127.0.0.1", DesktopTicketIssuer.preferRdpDialHost("172.80.0.81,188.188.185.250"));
        assertEquals("10.0.0.1", DesktopTicketIssuer.preferRdpDialHost("172.80.0.81,10.0.0.1"));
        assertEquals("192.168.1.9", DesktopTicketIssuer.preferRdpDialHost("192.168.1.9"));
        assertFalse(DesktopTicketIssuer.isRfc1918Ipv4("172.80.0.81"));
        assertTrue(DesktopTicketIssuer.isRfc1918Ipv4("172.16.0.1"));
        // Wrappers on SessionTicketService remain for callers/tests.
        assertEquals("127.0.0.1", SessionTicketService.preferRdpDialHost(null));
        assertTrue(SessionTicketService.isRfc1918Ipv4("172.16.0.1"));
    }

    @Test
    void sshProtocolRejected() {
        stubAccess();
        assertThrows(IllegalArgumentException.class,
                () -> tickets.createTicket(userId, "alice", assetId, "ssh"));
    }

    private void stubAccess() {
        when(assets.findById(assetId)).thenReturn(Optional.of(asset));
        when(users.findById(userId)).thenReturn(Optional.of(user));
        doNothing().when(access).assertCanAccessAsset(any(), any());
    }

    private static void assertTrueBrowserDesktop(Map<String, Object> out) {
        String ws = String.valueOf(out.get("browserWs"));
        if (!ws.contains("/ws/desktop")) {
            throw new AssertionError("expected /ws/desktop in browserWs, got " + ws);
        }
    }
}
