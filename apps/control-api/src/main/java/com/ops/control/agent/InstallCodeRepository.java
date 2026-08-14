package com.ops.control.agent;

import org.springframework.data.jpa.repository.JpaRepository;
import java.util.Optional;
import java.util.UUID;

public interface InstallCodeRepository extends JpaRepository<InstallCodeEntity, UUID> {
    Optional<InstallCodeEntity> findByCode(String code);
}
