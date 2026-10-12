package com.ops.control.metrics.notify;

import java.net.InetAddress;
import java.net.URI;
import java.net.UnknownHostException;
import java.util.Locale;

/** Rejects loopback / link-local / metadata targets for outbound alert webhooks. */
public final class WebhookUrlGuard {
    private WebhookUrlGuard() {}

    public static void assertSafe(String rawUrl) {
        if (rawUrl == null || rawUrl.isBlank()) {
            throw new IllegalArgumentException("url required");
        }
        URI uri;
        try {
            uri = URI.create(rawUrl.trim());
        } catch (Exception e) {
            throw new IllegalArgumentException("invalid url");
        }
        String scheme = uri.getScheme() == null ? "" : uri.getScheme().toLowerCase(Locale.ROOT);
        if (!"http".equals(scheme) && !"https".equals(scheme)) {
            throw new IllegalArgumentException("url must be http or https");
        }
        String host = uri.getHost();
        if (host == null || host.isBlank()) {
            throw new IllegalArgumentException("url host required");
        }
        String h = host.toLowerCase(Locale.ROOT);
        if ("localhost".equals(h) || h.endsWith(".localhost") || "metadata.google.internal".equals(h)) {
            throw new IllegalArgumentException("url host not allowed");
        }
        InetAddress[] addrs;
        try {
            addrs = InetAddress.getAllByName(host);
        } catch (UnknownHostException e) {
            throw new IllegalArgumentException("url host not resolvable");
        }
        if (addrs.length == 0) {
            throw new IllegalArgumentException("url host not resolvable");
        }
        for (InetAddress addr : addrs) {
            // Private RFC1918 (isSiteLocalAddress) is allowed so LAN WeCom bridges work.
            if (addr.isAnyLocalAddress()
                    || addr.isLoopbackAddress()
                    || addr.isLinkLocalAddress()
                    || addr.isMulticastAddress()) {
                throw new IllegalArgumentException("url host not allowed");
            }
        }
    }
}
