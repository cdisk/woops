package com.ops.control.asset;

import org.junit.jupiter.api.Test;

import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

class AssetAccessTest {

    @Test
    void protocolOfIsDesktopByOs() {
        assertEquals("rdp", AssetAccess.protocolOf(asset("Windows Server 2016")));
        assertEquals("vnc", AssetAccess.protocolOf(asset("Debian GNU/Linux 13")));
    }

    @Test
    void actionsExcludeSsh() {
        assertFalse(AssetAccess.actions(asset("Windows")).contains("ssh"));
        assertFalse(AssetAccess.actions(asset("Linux")).contains("ssh"));
        assertTrue(AssetAccess.allows(asset("Windows"), "rdp"));
        assertTrue(AssetAccess.allows(asset("Linux"), "vnc"));
        assertFalse(AssetAccess.allows(asset("Linux"), "ssh"));
        assertFalse(AssetAccess.allows(asset("Windows"), "ssh"));
    }

    @Test
    void registerDefaultsSetDesktopPortAndUsername() {
        AssetEntity win = asset("Windows Server 2019");
        AssetAccess.applyRegisterDefaults(win, true);
        assertEquals(3389, win.getDesktopPort());
        assertEquals("Administrator", win.getDesktopUsername());

        AssetEntity linux = asset("Ubuntu 22.04");
        AssetAccess.applyRegisterDefaults(linux, true);
        assertEquals(5900, linux.getDesktopPort());
        assertEquals("", linux.getDesktopUsername());
    }

    @Test
    void reinstallDoesNotOverwriteDesktopConfig() {
        AssetEntity win = asset("Windows");
        win.setDesktopPort(3390);
        win.setDesktopUsername("ops");
        win.setDesktopPassword("secret");
        AssetAccess.applyRegisterDefaults(win, false);
        assertEquals(3390, win.getDesktopPort());
        assertEquals("ops", win.getDesktopUsername());
        assertEquals("secret", win.getDesktopPassword());
    }

    @Test
    void defaultDesktopPortByOs() {
        assertEquals(3389, AssetAccess.defaultDesktopPort("Windows"));
        assertEquals(5900, AssetAccess.defaultDesktopPort("Linux"));
    }

    private static AssetEntity asset(String os) {
        AssetEntity a = new AssetEntity();
        a.setId(UUID.randomUUID());
        a.setDisplayName("t");
        a.setOs(os);
        return a;
    }
}
