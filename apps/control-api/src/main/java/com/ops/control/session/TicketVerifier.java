package com.ops.control.session;

import io.jsonwebtoken.Claims;
import io.jsonwebtoken.Jwts;
import org.springframework.stereotype.Component;

/** Parses and verifies tickets signed by {@link TicketSigner}. */
@Component
public class TicketVerifier {
    private final TicketSigner signer;

    public TicketVerifier(TicketSigner signer) {
        this.signer = signer;
    }

    public Claims verify(String ticket) {
        return Jwts.parser()
                .verifyWith(signer.key())
                .build()
                .parseSignedClaims(ticket)
                .getPayload();
    }
}
