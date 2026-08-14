package com.ops.control.assetevent;

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
public class AssetEventController {
    private final AssetEventService events;
    private final AccessService access;

    public AssetEventController(AssetEventService events, AccessService access) {
        this.events = events;
        this.access = access;
    }

    @GetMapping("/api/asset-events")
    public Map<String, Object> list(
            @RequestParam(required = false) UUID assetId,
            @RequestParam(required = false) String category,
            @RequestParam(required = false) String eventType,
            @RequestParam(required = false) @DateTimeFormat(iso = DateTimeFormat.ISO.DATE_TIME) Instant from,
            @RequestParam(required = false) @DateTimeFormat(iso = DateTimeFormat.ISO.DATE_TIME) Instant to,
            @RequestParam(defaultValue = "1") int page,
            @RequestParam(defaultValue = "50") int pageSize,
            Authentication auth) {
        UserEntity user = access.requireUser(auth);
        return events.page(user, assetId, category, eventType, from, to, page, pageSize);
    }
}
