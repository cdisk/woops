package com.ops.control.session;

import org.springframework.security.core.Authentication;
import org.springframework.web.bind.annotation.*;

import java.util.Map;
import java.util.UUID;

@RestController
@RequestMapping("/api/sessions")
public class SessionController {
    private final SessionTicketService tickets;

    public SessionController(SessionTicketService tickets) {
        this.tickets = tickets;
    }

    public record TicketRequest(
            UUID assetId,
            String protocol,
            String direction,
            String path,
            Long size,
            String fingerprint,
            String transferId,
            Boolean abort) {}

    @PostMapping("/ticket")
    public Map<String, Object> ticket(@RequestBody TicketRequest req, Authentication auth) {
        var claims = (io.jsonwebtoken.Claims) auth.getDetails();
        String username = claims.get("username", String.class);
        return tickets.createTicket(
                UUID.fromString(auth.getName()),
                username,
                req.assetId(),
                req.protocol(),
                req.direction(),
                req.path(),
                req.size(),
                req.fingerprint(),
                req.transferId(),
                req.abort());
    }

    @PostMapping("/internal/verify-ticket")
    public Map<String, Object> verify(@RequestBody Map<String, String> body) {
        String ticket = body.get("ticket");
        if (ticket == null || ticket.isBlank()) {
            throw new IllegalArgumentException("ticket required");
        }
        return tickets.verifyForGateway(ticket);
    }
}
