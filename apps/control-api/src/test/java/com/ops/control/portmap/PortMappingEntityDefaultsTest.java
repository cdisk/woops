package com.ops.control.portmap;

import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.*;

class PortMappingEntityDefaultsTest {

    @Test
    void legacyNullDirectionIsForward() {
        PortMappingEntity m = new PortMappingEntity();
        assertEquals(PortMappingEntity.DIR_GATEWAY_TO_ASSET, m.effectiveDirection());
        assertFalse(m.isReverse());
        assertEquals("0.0.0.0", m.effectiveListenHost());
    }

    @Test
    void reverseDefaultsListenHostToLoopback() {
        PortMappingEntity m = new PortMappingEntity();
        m.setDirection(PortMappingEntity.DIR_ASSET_TO_GATEWAY);
        assertTrue(m.isReverse());
        assertEquals("127.0.0.1", m.effectiveListenHost());
    }

    @Test
    void explicitListenHostWins() {
        PortMappingEntity m = new PortMappingEntity();
        m.setDirection(PortMappingEntity.DIR_ASSET_TO_GATEWAY);
        m.setListenHost("0.0.0.0");
        assertEquals("0.0.0.0", m.effectiveListenHost());
    }
}
