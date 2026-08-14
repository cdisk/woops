package com.ops.control.ci;

import org.springframework.http.HttpHeaders;
import org.springframework.http.HttpStatus;
import org.springframework.web.bind.annotation.*;
import org.springframework.web.server.ResponseStatusException;

import java.util.Map;

@RestController
@RequestMapping("/api/opsctl")
public class OpsctlController {
    private final DeployTokenService service;

    public OpsctlController(DeployTokenService service) {
        this.service = service;
    }

    public record TicketRequest(String action, Map<String, Object> meta) {}

    @PostMapping("/tickets")
    public Map<String, Object> createTicket(
            @RequestHeader(value = HttpHeaders.AUTHORIZATION, required = false) String authorization,
            @RequestBody TicketRequest req) {
        String token = extractBearer(authorization);
        try {
            return service.createOpsctlTicket(token, req.action(), req.meta());
        } catch (IllegalArgumentException e) {
            String msg = e.getMessage() == null ? "unauthorized" : e.getMessage();
            // Auth/scope problems → 401; malformed action/meta → 400
            if (msg.contains("token") || msg.contains("allow") || msg.contains("revoked") || msg.contains("expired")) {
                throw new ResponseStatusException(HttpStatus.UNAUTHORIZED, msg);
            }
            throw new ResponseStatusException(HttpStatus.BAD_REQUEST, msg);
        }
    }

    private static String extractBearer(String authorization) {
        if (authorization == null || !authorization.startsWith("Bearer ")) {
            throw new ResponseStatusException(HttpStatus.UNAUTHORIZED, "token required");
        }
        return authorization.substring("Bearer ".length()).trim();
    }
}
