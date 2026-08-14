package com.ops.control.serverops;

import com.ops.control.access.AccessService;
import com.ops.control.user.UserEntity;
import org.springframework.core.io.FileSystemResource;
import org.springframework.core.io.Resource;
import org.springframework.format.annotation.DateTimeFormat;
import org.springframework.http.HttpHeaders;
import org.springframework.http.MediaType;
import org.springframework.http.ResponseEntity;
import org.springframework.security.core.Authentication;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

import java.time.Instant;
import java.util.Map;
import java.util.UUID;

@RestController
@RequestMapping("/api/server-operations")
public class ServerOperationController {
    private final ServerOperationAuditService operations;
    private final AccessService access;

    public ServerOperationController(ServerOperationAuditService operations, AccessService access) {
        this.operations = operations;
        this.access = access;
    }

    @GetMapping
    public Map<String, Object> list(
            @RequestParam(required = false) UUID assetId,
            @RequestParam(required = false) String operationType,
            @RequestParam(required = false) String scope,
            @RequestParam(required = false) @DateTimeFormat(iso = DateTimeFormat.ISO.DATE_TIME) Instant from,
            @RequestParam(required = false) @DateTimeFormat(iso = DateTimeFormat.ISO.DATE_TIME) Instant to,
            @RequestParam(defaultValue = "1") int page,
            @RequestParam(defaultValue = "50") int pageSize,
            Authentication auth) {
        UserEntity user = access.requireUser(auth);
        return operations.pageOperations(user, assetId, operationType, scope, from, to, page, pageSize);
    }

    @GetMapping("/{operationId}")
    public Map<String, Object> get(@PathVariable UUID operationId, Authentication auth) {
        UserEntity user = access.requireUser(auth);
        return operations.getOperation(user, operationId);
    }

    /** Streams the session recording (asciinema cast / Guacamole) for replay. */
    @GetMapping("/{operationId}/recording")
    public ResponseEntity<Resource> recording(@PathVariable UUID operationId, Authentication auth) {
        UserEntity user = access.requireUser(auth);
        ServerOperationAuditService.Recording rec = operations.openRecording(user, operationId);
        return ResponseEntity.ok()
                .contentType(MediaType.APPLICATION_OCTET_STREAM)
                .contentLength(rec.size())
                .header(HttpHeaders.CONTENT_DISPOSITION,
                        "inline; filename=\"" + rec.file().getFileName() + "\"")
                .header("X-Recording-Format", rec.format())
                .body(new FileSystemResource(rec.file()));
    }
}
