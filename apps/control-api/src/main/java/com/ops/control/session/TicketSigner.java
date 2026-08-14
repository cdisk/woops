package com.ops.control.session;

import com.ops.control.common.OpsProperties;
import io.jsonwebtoken.Jwts;
import io.jsonwebtoken.security.Keys;
import jakarta.annotation.PostConstruct;
import org.springframework.stereotype.Component;

import javax.crypto.SecretKey;
import java.nio.charset.StandardCharsets;
import java.time.Instant;
import java.util.Date;
import java.util.Map;

/** Signs short-lived session / portmap JWTs with {@link OpsProperties#ticketSecret()}. */
@Component
public class TicketSigner {
    private final OpsProperties props;
    private SecretKey ticketKey;

    public TicketSigner(OpsProperties props) {
        this.props = props;
    }

    @PostConstruct
    void init() {
        ticketKey = Keys.hmacShaKeyFor(props.ticketSecret().getBytes(StandardCharsets.UTF_8));
    }

    /** Package-visible for tests that construct the signer without Spring. */
    void initForTests() {
        init();
    }

    SecretKey key() {
        return ticketKey;
    }

    public String sign(String subject, Map<String, Object> claims, Instant issuedAt, Instant expiresAt) {
        return Jwts.builder()
                .subject(subject)
                .claims(claims)
                .issuedAt(Date.from(issuedAt))
                .expiration(Date.from(expiresAt))
                .signWith(ticketKey)
                .compact();
    }
}
