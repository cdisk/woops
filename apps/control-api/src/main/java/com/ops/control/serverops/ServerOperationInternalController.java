package com.ops.control.serverops;

import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import java.util.LinkedHashMap;
import java.util.Map;

/**
 * Internal hooks for Gateway. Single-Gateway deployments call
 * {@code interrupt-running} on process start so SIGKILL orphans do not stay RUNNING.
 */
@RestController
@RequestMapping("/api/internal/server-operations")
public class ServerOperationInternalController {
    private final ServerOperationAuditService operations;

    public ServerOperationInternalController(ServerOperationAuditService operations) {
        this.operations = operations;
    }

    public record InterruptRequest(String reason) {}

    @PostMapping("/interrupt-running")
    public Map<String, Object> interruptRunning(@RequestBody(required = false) InterruptRequest body) {
        String reason = body != null && body.reason() != null && !body.reason().isBlank()
                ? body.reason().trim()
                : "gateway_restart";
        int n = operations.interruptAllRunning(reason);
        Map<String, Object> out = new LinkedHashMap<>();
        out.put("interrupted", n);
        out.put("reason", reason);
        return out;
    }
}
