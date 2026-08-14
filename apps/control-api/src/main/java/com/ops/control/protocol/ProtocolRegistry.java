package com.ops.control.protocol;

import org.springframework.stereotype.Component;

import java.util.HashMap;
import java.util.List;
import java.util.Map;

/** Maps request protocol strings to {@link ProtocolTicketIssuer} beans. */
@Component
public class ProtocolRegistry {
    private final Map<String, ProtocolTicketIssuer> byProtocol;

    public ProtocolRegistry(List<ProtocolTicketIssuer> issuers) {
        Map<String, ProtocolTicketIssuer> map = new HashMap<>();
        for (ProtocolTicketIssuer issuer : issuers) {
            for (String protocol : issuer.requestProtocols()) {
                String key = protocol.toLowerCase();
                ProtocolTicketIssuer prev = map.put(key, issuer);
                if (prev != null) {
                    throw new IllegalStateException(
                            "duplicate ProtocolTicketIssuer for protocol '" + key + "': "
                                    + prev.getClass().getName() + " and " + issuer.getClass().getName());
                }
            }
        }
        this.byProtocol = Map.copyOf(map);
    }

    public ProtocolTicketIssuer resolve(String protocol) {
        if (protocol == null || protocol.isBlank()) {
            throw new IllegalArgumentException("unsupported protocol: " + protocol);
        }
        ProtocolTicketIssuer issuer = byProtocol.get(protocol.toLowerCase());
        if (issuer == null) {
            throw new IllegalArgumentException("unsupported protocol: " + protocol);
        }
        return issuer;
    }
}
