package com.ops.control.serverops;

import org.springframework.data.domain.Page;
import org.springframework.data.domain.Pageable;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.query.Param;

import java.time.Instant;
import java.util.Collection;
import java.util.List;
import java.util.UUID;

public interface ServerOperationRecordRepository extends JpaRepository<ServerOperationRecordEntity, UUID> {

    /**
     * Pass concrete Instant bounds (never null); PostgreSQL cannot infer types
     * for null Instant binds in {@code :x is null} comparisons.
     * {@code mode}: ALL | SESSION (exclude portmap tunnels) | PORTMAP (only tunnels).
     */
    @Query("""
            select r from ServerOperationRecordEntity r
            where r.recordKind = 'OPERATION'
              and (:assetId is null or r.assetId = :assetId)
              and (:operationType is null or r.operationType = :operationType)
              and (
                    :mode = 'ALL'
                 or (:mode = 'SESSION' and r.operationType not in ('PORTMAP_TCP', 'PORTMAP_UDP'))
                 or (:mode = 'PORTMAP' and r.operationType in ('PORTMAP_TCP', 'PORTMAP_UDP'))
              )
              and r.occurredAt >= :from
              and r.occurredAt <= :to
            order by r.occurredAt desc
            """)
    Page<ServerOperationRecordEntity> searchOperations(
            @Param("assetId") UUID assetId,
            @Param("operationType") String operationType,
            @Param("mode") String mode,
            @Param("from") Instant from,
            @Param("to") Instant to,
            Pageable pageable);

    @Query("""
            select r from ServerOperationRecordEntity r
            where r.recordKind = 'OPERATION'
              and r.assetId in :assetIds
              and (:operationType is null or r.operationType = :operationType)
              and (
                    :mode = 'ALL'
                 or (:mode = 'SESSION' and r.operationType not in ('PORTMAP_TCP', 'PORTMAP_UDP'))
                 or (:mode = 'PORTMAP' and r.operationType in ('PORTMAP_TCP', 'PORTMAP_UDP'))
              )
              and r.occurredAt >= :from
              and r.occurredAt <= :to
            order by r.occurredAt desc
            """)
    Page<ServerOperationRecordEntity> searchInAssets(
            @Param("assetIds") Collection<UUID> assetIds,
            @Param("operationType") String operationType,
            @Param("mode") String mode,
            @Param("from") Instant from,
            @Param("to") Instant to,
            Pageable pageable);

    List<ServerOperationRecordEntity> findByOperationIdOrderByOccurredAtAsc(UUID operationId);

    List<ServerOperationRecordEntity> findByRecordKindAndOperationTypeInAndAssetIdOrderByOccurredAtDesc(
            String recordKind, Collection<String> operationTypes, UUID assetId);

    List<ServerOperationRecordEntity> findByRecordKindAndOperationTypeInOrderByOccurredAtDesc(
            String recordKind, Collection<String> operationTypes);

    List<ServerOperationRecordEntity> findByRecordKindAndOccurredAtBeforeAndStatusNot(
            String recordKind, Instant before, String statusNot);

    List<ServerOperationRecordEntity> findByRecordKindAndStatusAndOccurredAtBefore(
            String recordKind, String status, Instant before);

    List<ServerOperationRecordEntity> findByRecordKindAndStatus(String recordKind, String status);
}
