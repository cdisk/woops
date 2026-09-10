package com.ops.control.metrics;

import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Modifying;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.query.Param;

import java.time.Instant;

public interface MonitorTrendsRepository extends JpaRepository<MonitorTrendsEntity, MonitorTrendsId> {

    @Modifying(clearAutomatically = true, flushAutomatically = true)
    @Query("delete from MonitorTrendsEntity t where t.hour < :before")
    int deleteOlderThan(@Param("before") Instant before);
}
