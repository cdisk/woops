package com.ops.control.portmap;

import org.springframework.web.bind.annotation.*;

import java.util.List;
import java.util.Map;
import java.util.UUID;

@RestController
@RequestMapping("/api/internal/port-mappings")
public class PortMappingInternalController {
    private final PortMappingService service;

    public PortMappingInternalController(PortMappingService service) {
        this.service = service;
    }

    @GetMapping
    public List<Map<String, Object>> listForAsset(@RequestParam UUID assetId) {
        return service.listForAsset(assetId);
    }

    public record OpenRecordRequest(UUID mappingId, String clientAddr) {}

    @PostMapping("/connections/open-record")
    public Map<String, Object> openRecord(@RequestBody OpenRecordRequest req) {
        if (req.mappingId() == null) {
            throw new IllegalArgumentException("mappingId required");
        }
        return service.openConnectionRecord(req.mappingId(), req.clientAddr());
    }

    public record LastErrorRequest(UUID mappingId, String error) {}

    @PostMapping("/last-error")
    public Map<String, String> lastError(@RequestBody LastErrorRequest req) {
        if (req.mappingId() == null) {
            throw new IllegalArgumentException("mappingId required");
        }
        service.setLastError(req.mappingId(), req.error());
        return Map.of("status", "ok");
    }
}
