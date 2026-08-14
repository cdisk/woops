package com.ops.control.serverops;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.ops.control.common.OpsProperties;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;
import org.mockito.ArgumentCaptor;

import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.StandardOpenOption;
import java.time.LocalDate;
import java.time.ZoneOffset;
import java.util.List;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.*;

class OpsAuditIngestSchedulerTest {
    private static final String OP = "11111111-1111-4111-8111-111111111111";
    private static final String ASSET = "22222222-2222-4222-8222-222222222222";

    @TempDir
    Path root;

    private ServerOperationAuditService audit;
    private OpsAuditIngestScheduler scheduler;
    private Path segment;

    @BeforeEach
    void setUp() throws IOException {
        audit = mock(ServerOperationAuditService.class);
        scheduler = new OpsAuditIngestScheduler(props(root.toString()), audit, new ObjectMapper());
        LocalDate today = LocalDate.now(ZoneOffset.UTC);
        Path dayDir = root
                .resolve(String.format("%04d", today.getYear()))
                .resolve(String.format("%02d", today.getMonthValue()))
                .resolve(String.format("%02d", today.getDayOfMonth()));
        Files.createDirectories(dayDir);
        segment = dayDir.resolve("events-test-0.open");
        Files.createFile(segment);
    }

    @Test
    void ingestsCompleteLinesAndKeepsPartialTail() throws IOException {
        append(envelope("START") + "\n" + envelope("END") + "\n" + "{\"phase\":\"ACT");
        scheduler.scan();

        ArgumentCaptor<JsonNode> captor = ArgumentCaptor.forClass(JsonNode.class);
        verify(audit, times(2)).ingestEnvelope(captor.capture());
        assertEquals(List.of("START", "END"),
                captor.getAllValues().stream().map(n -> n.get("phase").asText()).toList());

        // A rescan must not replay the two lines already committed.
        scheduler.scan();
        verify(audit, times(2)).ingestEnvelope(any());

        // Completing the partial line makes it visible on the next pass.
        append("ION\",\"operationId\":\"" + OP + "\",\"assetId\":\"" + ASSET + "\"}\n");
        scheduler.scan();
        verify(audit, times(3)).ingestEnvelope(any());
    }

    @Test
    void quarantinesBadLinesAndKeepsGoing() throws IOException {
        append("not json\n" + envelope("START") + "\n");
        scheduler.scan();

        verify(audit, times(1)).ingestEnvelope(any());
        Path quarantine = root.resolve(".cursor").resolve("quarantine.jsonl");
        assertTrue(Files.exists(quarantine), "bad line should be parked in quarantine.jsonl");
        assertTrue(Files.readString(quarantine).contains("not json"));

        scheduler.scan();
        verify(audit, times(1)).ingestEnvelope(any());
    }

    @Test
    void rejectedEnvelopesDoNotBlockTheSegment() throws IOException {
        doThrow(new IllegalArgumentException("operationId required"))
                .when(audit).ingestEnvelope(any());
        append(envelope("START") + "\n" + envelope("END") + "\n");
        scheduler.scan();
        verify(audit, times(2)).ingestEnvelope(any());

        reset(audit);
        scheduler.scan();
        verify(audit, never()).ingestEnvelope(any());
    }

    @Test
    void transientFailureLeavesLineForRetry() throws IOException {
        doThrow(new IllegalStateException("db down")).when(audit).ingestEnvelope(any());
        append(envelope("START") + "\n");
        scheduler.scan();
        verify(audit, times(1)).ingestEnvelope(any());

        reset(audit);
        scheduler.scan();
        verify(audit, times(1)).ingestEnvelope(any());
    }

    private void append(String data) throws IOException {
        Files.writeString(segment, data, StandardCharsets.UTF_8,
                StandardOpenOption.CREATE, StandardOpenOption.APPEND);
    }

    private static String envelope(String phase) {
        return "{\"phase\":\"" + phase + "\",\"operationId\":\"" + OP
                + "\",\"operationType\":\"SHELL\",\"assetId\":\"" + ASSET + "\"}";
    }

    private static OpsProperties props(String auditDir) {
        return new OpsProperties(
                "http://127.0.0.1:9100",
                "https://127.0.0.1:9200",
                "wss://127.0.0.1:9200",
                "http://127.0.0.1:9201",
                "https://127.0.0.1:5173",
                "jwt", "ticket", "admin", "admin123", 15, "",
                auditDir, 5, 90, 24,
                new OpsProperties.Gitlab(null, null, null, null, null));
    }
}
