package com.ops.control.metrics;

import jakarta.validation.constraints.NotBlank;
import org.springframework.web.bind.annotation.*;

import java.util.List;
import java.util.Map;

@RestController
@RequestMapping("/api/internal/metrics")
public class MetricsInternalController {
    private final MetricsService metrics;

    public MetricsInternalController(MetricsService metrics) {
        this.metrics = metrics;
    }

    public record PointBody(String itemId, String instance, double value) {}

    public record IngestBody(
            @NotBlank String assetId,
            @NotBlank String agentToken,
            String collectedAt,
            List<PointBody> points
    ) {}

    @PostMapping("/ingest")
    public Map<String, String> ingest(@RequestBody IngestBody body) {
        List<MetricsService.PointIn> points = body.points() == null ? List.of() : body.points().stream()
                .filter(p -> p != null && p.itemId() != null && !p.itemId().isBlank())
                .map(p -> new MetricsService.PointIn(p.itemId(), p.instance(), p.value()))
                .toList();
        metrics.ingest(body.assetId(), body.agentToken(), body.collectedAt(), points);
        return Map.of("status", "ok");
    }
}
