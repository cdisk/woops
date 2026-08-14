package com.ops.control.serverops;

import com.fasterxml.jackson.databind.JsonNode;
import com.ops.control.common.OpsProperties;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Component;
import org.springframework.transaction.annotation.Transactional;

import java.io.IOException;
import java.nio.file.*;
import java.nio.file.attribute.BasicFileAttributes;
import java.time.Instant;
import java.time.LocalDate;
import java.time.ZoneOffset;
import java.time.format.DateTimeFormatter;
import java.time.temporal.ChronoUnit;
import java.util.List;
import java.util.stream.Stream;

/**
 * Daily purge of aged recordings and sealed JSONL date trees.
 * OPERATION rows keep metadata with {@code status=PURGED}.
 */
@Component
public class OpsAuditRetentionJob {
    private static final Logger log = LoggerFactory.getLogger(OpsAuditRetentionJob.class);
    private static final DateTimeFormatter DAY = DateTimeFormatter.ofPattern("yyyy/MM/dd");

    private final ServerOperationRecordRepository records;
    private final OpsProperties props;

    public OpsAuditRetentionJob(ServerOperationRecordRepository records, OpsProperties props) {
        this.records = records;
        this.props = props;
    }

    @Scheduled(cron = "0 15 4 * * *")
    @Transactional
    public void purge() {
        int days = props.auditRetentionDays();
        if (days <= 0) {
            return;
        }
        Instant cutoff = Instant.now().minus(days, ChronoUnit.DAYS);
        int marked = purgeRecordings(cutoff);
        int dirs = purgeOldDayTrees(cutoff);
        if (marked > 0 || dirs > 0) {
            log.info("ops-audit retention: purged recordings for {} operations, removed {} day dirs (cutoff={})",
                    marked, dirs, cutoff);
        }
    }

    private int purgeRecordings(Instant cutoff) {
        List<ServerOperationRecordEntity> rows = records.findByRecordKindAndOccurredAtBeforeAndStatusNot(
                ServerOperationAuditService.KIND_OPERATION,
                cutoff,
                ServerOperationAuditService.STATUS_PURGED);
        Path root = auditRoot();
        int n = 0;
        for (ServerOperationRecordEntity row : rows) {
            JsonNode detail = row.getDetail();
            if (detail != null && detail.hasNonNull("recordingPath") && root != null) {
                deleteRecording(root, detail.get("recordingPath").asText());
            }
            row.setStatus(ServerOperationAuditService.STATUS_PURGED);
            records.save(row);
            n++;
        }
        return n;
    }

    private void deleteRecording(Path root, String relative) {
        if (relative == null || relative.isBlank()) {
            return;
        }
        try {
            Path target = root.resolve(relative.replace('\\', '/')).normalize();
            if (!target.startsWith(root)) {
                log.warn("skip retention path escape: {}", relative);
                return;
            }
            if (Files.isRegularFile(target)) {
                Files.deleteIfExists(target);
                Path parent = target.getParent();
                if (parent != null && parent.startsWith(root) && isEmptyDir(parent)) {
                    Files.deleteIfExists(parent);
                }
            } else if (Files.isDirectory(target)) {
                deleteTree(target);
            }
        } catch (IOException e) {
            log.warn("failed to delete recording {}: {}", relative, e.toString());
        }
    }

    private int purgeOldDayTrees(Instant cutoff) {
        Path root = auditRoot();
        if (root == null || !Files.isDirectory(root)) {
            return 0;
        }
        LocalDate cutoffDay = LocalDate.ofInstant(cutoff, ZoneOffset.UTC);
        int removed = 0;
        try (Stream<Path> years = Files.list(root)) {
            for (Path year : years.toList()) {
                if (!Files.isDirectory(year) || year.getFileName().toString().startsWith(".")) {
                    continue;
                }
                try (Stream<Path> months = Files.list(year)) {
                    for (Path month : months.toList()) {
                        if (!Files.isDirectory(month)) {
                            continue;
                        }
                        try (Stream<Path> days = Files.list(month)) {
                            for (Path day : days.toList()) {
                                if (!Files.isDirectory(day)) {
                                    continue;
                                }
                                String rel = root.relativize(day).toString().replace('\\', '/');
                                LocalDate d;
                                try {
                                    d = LocalDate.parse(rel, DAY);
                                } catch (Exception e) {
                                    continue;
                                }
                                if (d.isBefore(cutoffDay)) {
                                    deleteTree(day);
                                    removed++;
                                }
                            }
                        }
                        if (isEmptyDir(month)) {
                            Files.deleteIfExists(month);
                        }
                    }
                }
                if (isEmptyDir(year)) {
                    Files.deleteIfExists(year);
                }
            }
        } catch (IOException e) {
            log.warn("ops-audit day-tree purge failed: {}", e.toString());
        }
        return removed;
    }

    private Path auditRoot() {
        String dir = props.auditDir();
        if (dir == null || dir.isBlank()) {
            return null;
        }
        return Paths.get(dir).toAbsolutePath().normalize();
    }

    private static boolean isEmptyDir(Path dir) throws IOException {
        if (!Files.isDirectory(dir)) {
            return false;
        }
        try (Stream<Path> s = Files.list(dir)) {
            return s.findAny().isEmpty();
        }
    }

    private static void deleteTree(Path root) throws IOException {
        if (!Files.exists(root)) {
            return;
        }
        Files.walkFileTree(root, new SimpleFileVisitor<>() {
            @Override
            public FileVisitResult visitFile(Path file, BasicFileAttributes attrs) throws IOException {
                Files.deleteIfExists(file);
                return FileVisitResult.CONTINUE;
            }

            @Override
            public FileVisitResult postVisitDirectory(Path dir, IOException exc) throws IOException {
                Files.deleteIfExists(dir);
                return FileVisitResult.CONTINUE;
            }
        });
    }
}
