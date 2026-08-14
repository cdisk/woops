package com.ops.control.metrics;

import org.springframework.data.jpa.repository.JpaRepository;

import java.util.List;
import java.util.UUID;

public interface MetricAlertRuleRepository extends JpaRepository<MetricAlertRuleEntity, UUID> {
    List<MetricAlertRuleEntity> findByEnabledTrue();
    List<MetricAlertRuleEntity> findAllByOrderByNameAsc();
}
