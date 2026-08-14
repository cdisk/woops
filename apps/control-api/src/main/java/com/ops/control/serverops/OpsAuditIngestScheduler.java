package com.ops.control.serverops;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.ops.control.common.OpsProperties;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Component;

import java.io.IOException;
import java.nio.ByteBuffer;
import java.nio.channels.FileChannel;
import java.nio.charset.StandardCharsets;
import java.nio.file.*;
import java.time.LocalDate;
import java.time.ZoneOffset;
import java.time.format.DateTimeFormatter;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.List;

/**
 * Tails the Gateway JSONL spool into {@code server_operation_records}.
 *
 * <p>Gateway appends envelopes to {@code {auditDir}/{yyyy}/{mm}/{dd}/events-*.open}
 * and seals finished segments as {@code .jsonl}. Progress per segment is a byte
 * offset in {@code {auditDir}/.cursor/{yyyy-mm-dd}-{filename}.offset}, so a
 * control-api restart resumes exactly where it stopped. Delivery is
 * at-least-once; {@link ServerOperationAuditService} is idempotent per
 * operationId / eventId.
 */
@Component
public class OpsAuditIngestScheduler {
    private static final Logger log = LoggerFactory.getLogger(OpsAuditIngestScheduler.class);

    private static final DateTimeFormatter DAY_KEY = DateTimeFormatter.ofPattern("yyyy-MM-dd");
    private static final String SEGMENT_PREFIX = "events-";
    /** Bytes read per segment per pass; the rest is picked up on the next tick. */
    private static final int MAX_BYTES_PER_PASS = 4 << 20;
    /** Guards against a corrupt segment without newlines eating heap. */
    private static final int MAX_LINE_BYTES = 1 << 20;

    private final OpsProperties props;
    private final ServerOperationAuditService audit;
    private final ObjectMapper json;

    public OpsAuditIngestScheduler(
            OpsProperties props, ServerOperationAuditService audit, ObjectMapper json) {
        this.props = props;
        this.audit = audit;
        this.json = json;
    }

    @Scheduled(fixedDelayString = "${ops.audit-scan-seconds:5}000", initialDelay = 10_000)
    public void scan() {
        String dir = props.auditDir();
        if (dir == null || dir.isBlank()) {
            return;
        }
        Path root = Path.of(dir.trim()).toAbsolutePath();
        if (!Files.isDirectory(root)) {
            return;
        }
        // Yesterday too: a segment can still be appended to just after midnight UTC,
        // and its tail must not be skipped when the date partition rolls over.
        LocalDate today = LocalDate.now(ZoneOffset.UTC);
        for (LocalDate day : List.of(today.minusDays(1), today)) {
            for (Path segment : segmentsFor(root, day)) {
                try {
                    ingestSegment(root, day, segment);
                } catch (IOException e) {
                    log.warn("ops-audit segment {} failed: {}", segment.getFileName(), e.toString());
                }
            }
        }
    }

    private List<Path> segmentsFor(Path root, LocalDate day) {
        Path dayDir = root
                .resolve(String.format("%04d", day.getYear()))
                .resolve(String.format("%02d", day.getMonthValue()))
                .resolve(String.format("%02d", day.getDayOfMonth()));
        if (!Files.isDirectory(dayDir)) {
            return List.of();
        }
        try (DirectoryStream<Path> stream = Files.newDirectoryStream(dayDir)) {
            List<Path> out = new ArrayList<>();
            for (Path p : stream) {
                String name = p.getFileName().toString();
                if (!name.startsWith(SEGMENT_PREFIX)) {
                    continue;
                }
                if (name.endsWith(".jsonl") || name.endsWith(".open")) {
                    out.add(p);
                }
            }
            out.sort(Comparator.comparing(p -> p.getFileName().toString()));
            return out;
        } catch (IOException e) {
            log.warn("ops-audit list {} failed: {}", dayDir, e.toString());
            return List.of();
        }
    }

    private void ingestSegment(Path root, LocalDate day, Path segment) throws IOException {
        Path cursor = cursorPath(root, day, segment);
        long offset = readOffset(cursor);
        long size = Files.size(segment);
        if (size < offset) {
            // Segment was replaced by a different file with the same name.
            log.warn("ops-audit segment {} shrank ({} < {}); restarting from 0",
                    segment.getFileName(), size, offset);
            offset = 0;
        }
        if (size == offset) {
            return;
        }

        int want = (int) Math.min(size - offset, MAX_BYTES_PER_PASS);
        byte[] buf = new byte[want];
        try (FileChannel ch = FileChannel.open(segment, StandardOpenOption.READ)) {
            ByteBuffer bb = ByteBuffer.wrap(buf);
            ch.position(offset);
            while (bb.hasRemaining() && ch.read(bb) > 0) {
                // read until the requested window is filled
            }
            want = bb.position();
        }

        long consumed = 0;
        int lineStart = 0;
        while (true) {
            int nl = indexOfNewline(buf, lineStart, want);
            if (nl < 0) {
                // A partial trailing line means Gateway is mid-append: leave it
                // for the next pass. Absurdly long means the segment is corrupt.
                if (want - lineStart > MAX_LINE_BYTES) {
                    quarantine(root, segment, "no newline within " + MAX_LINE_BYTES + " bytes", "");
                    consumed = want;
                }
                break;
            }
            String line = new String(buf, lineStart, nl - lineStart, StandardCharsets.UTF_8).trim();
            lineStart = nl + 1;
            if (line.isEmpty()) {
                consumed = lineStart;
                continue;
            }
            LineResult result = handleLine(root, segment, line);
            if (result == LineResult.RETRY) {
                // Keep the offset so the line is retried once the DB recovers.
                break;
            }
            consumed = lineStart;
        }
        if (consumed > 0) {
            writeOffset(cursor, offset + consumed);
        }
    }

    private static int indexOfNewline(byte[] buf, int from, int limit) {
        for (int i = from; i < limit; i++) {
            if (buf[i] == '\n') {
                return i;
            }
        }
        return -1;
    }

    private enum LineResult { OK, BAD_LINE, RETRY }

    private LineResult handleLine(Path root, Path segment, String line) {
        JsonNode envelope;
        try {
            envelope = json.readTree(line);
        } catch (Exception e) {
            quarantine(root, segment, "invalid json: " + e.getMessage(), line);
            return LineResult.BAD_LINE;
        }
        try {
            audit.ingestEnvelope(envelope);
            return LineResult.OK;
        } catch (IllegalArgumentException e) {
            quarantine(root, segment, "rejected: " + e.getMessage(), line);
            return LineResult.BAD_LINE;
        } catch (RuntimeException e) {
            log.warn("ops-audit ingest deferred for {}: {}", segment.getFileName(), e.toString());
            return LineResult.RETRY;
        }
    }

    private Path cursorPath(Path root, LocalDate day, Path segment) {
        String name = day.format(DAY_KEY) + "-" + segment.getFileName() + ".offset";
        return root.resolve(".cursor").resolve(name);
    }

    private long readOffset(Path cursor) {
        try {
            if (!Files.isRegularFile(cursor)) {
                return 0;
            }
            String s = Files.readString(cursor, StandardCharsets.UTF_8).trim();
            long v = s.isEmpty() ? 0 : Long.parseLong(s);
            return Math.max(v, 0);
        } catch (IOException | NumberFormatException e) {
            log.warn("ops-audit cursor {} unreadable, restarting from 0: {}", cursor, e.toString());
            return 0;
        }
    }

    /** Temp file + rename so a crash never leaves a half-written offset. */
    private void writeOffset(Path cursor, long offset) throws IOException {
        Files.createDirectories(cursor.getParent());
        Path tmp = cursor.resolveSibling(cursor.getFileName() + ".tmp");
        Files.writeString(tmp, Long.toString(offset), StandardCharsets.UTF_8,
                StandardOpenOption.CREATE, StandardOpenOption.TRUNCATE_EXISTING,
                StandardOpenOption.WRITE);
        try {
            Files.move(tmp, cursor, StandardCopyOption.REPLACE_EXISTING, StandardCopyOption.ATOMIC_MOVE);
        } catch (AtomicMoveNotSupportedException e) {
            Files.move(tmp, cursor, StandardCopyOption.REPLACE_EXISTING);
        }
    }

    /** Bad lines are parked for inspection; ingest still moves past them. */
    private void quarantine(Path root, Path segment, String reason, String line) {
        log.warn("ops-audit quarantine {} ({}): {}", segment.getFileName(), reason,
                line.length() > 500 ? line.substring(0, 500) + "…" : line);
        Path file = root.resolve(".cursor").resolve("quarantine.jsonl");
        String record = "{\"segment\":\"" + segment.getFileName()
                + "\",\"reason\":" + quote(reason)
                + ",\"line\":" + quote(line) + "}\n";
        try {
            Files.createDirectories(file.getParent());
            Files.writeString(file, record, StandardCharsets.UTF_8,
                    StandardOpenOption.CREATE, StandardOpenOption.APPEND);
        } catch (IOException e) {
            log.warn("ops-audit quarantine write failed: {}", e.toString());
        }
    }

    private String quote(String s) {
        try {
            return json.writeValueAsString(s == null ? "" : s);
        } catch (Exception e) {
            return "\"\"";
        }
    }
}
