package com.ops.control.controlaudit;

import com.ops.control.access.AccessService;
import com.ops.control.user.UserEntity;
import org.springframework.format.annotation.DateTimeFormat;
import org.springframework.security.core.Authentication;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

import java.time.Instant;
import java.util.Map;
import java.util.UUID;

@RestController
public class ControlAuditController {
    private final ControlAuditService audit;
    private final AccessService access;

    public ControlAuditController(ControlAuditService audit, AccessService access) {
        this.audit = audit;
        this.access = access;
    }

    @GetMapping("/api/control-audit")
    public Map<String, Object> list(
            @RequestParam(required = false) String category,
            @RequestParam(required = false) UUID assetId,
            @RequestParam(required = false) @DateTimeFormat(iso = DateTimeFormat.ISO.DATE_TIME) Instant from,
            @RequestParam(required = false) @DateTimeFormat(iso = DateTimeFormat.ISO.DATE_TIME) Instant to,
            @RequestParam(defaultValue = "1") int page,
            @RequestParam(defaultValue = "50") int pageSize,
            Authentication auth) {
        UserEntity user = access.requireUser(auth);
        return audit.page(user, category, assetId, from, to, page, pageSize);
    }
}
