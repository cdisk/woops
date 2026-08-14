package com.ops.control.common;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.springframework.stereotype.Component;

import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.util.List;
import java.util.Map;

@Component
public class GatewayClient {
    private final OpsProperties props;
    private final ObjectMapper mapper;
    private final HttpClient http = HttpClient.newBuilder()
            .connectTimeout(Duration.ofSeconds(5))
            .build();

    public GatewayClient(OpsProperties props, ObjectMapper mapper) {
        this.props = props;
        this.mapper = mapper;
    }

    public List<Integer> listeningPorts() {
        Map<String, Object> body = get("/internal/portmap/listening-ports");
        Object ports = body.get("ports");
        if (!(ports instanceof List<?> list)) {
            return List.of();
        }
        return list.stream()
                .filter(Number.class::isInstance)
                .map(n -> ((Number) n).intValue())
                .toList();
    }

    public Map<String, Object> openPortMap(Map<String, Object> req) {
        return post("/internal/portmap/open", req);
    }

    public void closePortMap(String mappingId) {
        post("/internal/portmap/close", Map.of("mappingId", mappingId));
    }

    public List<Map<String, Object>> runtimeList() {
        Map<String, Object> body = get("/internal/portmap/list");
        Object items = body.get("items");
        if (!(items instanceof List<?> list)) {
            return List.of();
        }
        return list.stream()
                .filter(Map.class::isInstance)
                .map(m -> {
                    @SuppressWarnings("unchecked")
                    Map<String, Object> row = (Map<String, Object>) m;
                    return row;
                })
                .toList();
    }

    private Map<String, Object> get(String path) {
        try {
            HttpRequest req = HttpRequest.newBuilder()
                    .uri(URI.create(trimSlash(props.gatewayInternalHttpOrPublic()) + path))
                    .timeout(Duration.ofSeconds(10))
                    .GET()
                    .build();
            HttpResponse<String> res = http.send(req, HttpResponse.BodyHandlers.ofString());
            if (res.statusCode() >= 300) {
                throw new IllegalStateException("gateway " + res.statusCode() + ": " + res.body());
            }
            return mapper.readValue(res.body(), new TypeReference<>() {});
        } catch (IllegalStateException e) {
            throw e;
        } catch (Exception e) {
            throw new IllegalStateException("gateway unreachable: " + e.getMessage(), e);
        }
    }

    private Map<String, Object> post(String path, Object body) {
        try {
            byte[] json = mapper.writeValueAsBytes(body);
            HttpRequest req = HttpRequest.newBuilder()
                    .uri(URI.create(trimSlash(props.gatewayInternalHttpOrPublic()) + path))
                    .timeout(Duration.ofSeconds(15))
                    .header("Content-Type", "application/json")
                    .POST(HttpRequest.BodyPublishers.ofByteArray(json))
                    .build();
            HttpResponse<String> res = http.send(req, HttpResponse.BodyHandlers.ofString());
            if (res.statusCode() >= 300) {
                throw new IllegalStateException("gateway " + res.statusCode() + ": " + res.body());
            }
            if (res.body() == null || res.body().isBlank()) {
                return Map.of();
            }
            return mapper.readValue(res.body(), new TypeReference<>() {});
        } catch (IllegalStateException e) {
            throw e;
        } catch (Exception e) {
            throw new IllegalStateException("gateway unreachable: " + e.getMessage(), e);
        }
    }

    private static String trimSlash(String base) {
        if (base == null || base.isBlank()) {
            return "http://127.0.0.1:9201";
        }
        return base.endsWith("/") ? base.substring(0, base.length() - 1) : base;
    }
}
