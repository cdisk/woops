package com.ops.control.common;

import org.junit.jupiter.api.Test;
import org.springframework.mock.web.MockHttpServletRequest;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

class PublicUrlResolverTest {

    @Test
    void prefersConfiguredPublicGatewayOverLanOrigin() {
        OpsProperties props = props(
                "https://127.0.0.1:9100",
                "https://gateway.example.com:9200",
                "wss://gateway.example.com:9200",
                "http://gateway:9201"
        );
        MockHttpServletRequest req = new MockHttpServletRequest();
        req.addHeader("Origin", "https://192.168.1.10:5173");

        var ep = PublicUrlResolver.resolve(req, props);
        assertEquals("https://gateway.example.com:9200", ep.gatewayHttp());
        assertEquals("wss://gateway.example.com:9200", ep.gatewayWs());
    }

    @Test
    void rewritesFromOriginWhenConfigIsLoopback() {
        OpsProperties props = props(
                "https://127.0.0.1:9100",
                "https://127.0.0.1:9200",
                "wss://127.0.0.1:9200",
                "http://127.0.0.1:9201"
        );
        MockHttpServletRequest req = new MockHttpServletRequest();
        req.addHeader("Origin", "https://192.168.1.10:5173");

        var ep = PublicUrlResolver.resolve(req, props);
        assertEquals("https://192.168.1.10:9200", ep.gatewayHttp());
        assertEquals("wss://192.168.1.10:9200", ep.gatewayWs());
        assertEquals("https://192.168.1.10:9100", ep.apiBase());
    }

    @Test
    void loopbackHelpers() {
        assertTrue(PublicUrlResolver.isLoopbackBase("https://127.0.0.1:9200"));
        assertFalse(PublicUrlResolver.isLoopbackBase("https://gateway.example.com:9200"));
        assertEquals(9200, PublicUrlResolver.portOr("https://x:9200", 1));
    }

    private static OpsProperties props(String controlPublic, String gwPublicHttp, String gwPublicWs, String gwInternal) {
        return new OpsProperties(
                controlPublic,
                gwPublicHttp,
                gwPublicWs,
                gwInternal,
                "https://127.0.0.1:5173",
                "jwt",
                "ticket",
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
    }
}
