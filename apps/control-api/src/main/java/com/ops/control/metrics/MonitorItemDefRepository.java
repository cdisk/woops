package com.ops.control.metrics;

import org.springframework.data.jpa.repository.JpaRepository;

import java.util.List;

public interface MonitorItemDefRepository extends JpaRepository<MonitorItemDefEntity, String> {
    List<MonitorItemDefEntity> findByChartDefaultTrueOrderByItemIdAsc();
}
