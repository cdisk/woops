package com.ops.control.session;

import com.ops.control.access.AccessService;
import com.ops.control.user.UserEntity;
import org.springframework.security.core.Authentication;
import org.springframework.web.bind.annotation.*;

import java.util.Map;
import java.util.UUID;

@RestController
@RequestMapping("/api/sessions")
public class SessionController {
    private final SessionTicketService tickets;
    private final AccessService access;

    public SessionController(SessionTicketService tickets, AccessService access) {
        this.tickets = tickets;
        this.access = access;
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
        UserEntity user = access.requireUser(auth);
        return tickets.createTicket(
                user.getId(),
                user.getUsername(),
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
