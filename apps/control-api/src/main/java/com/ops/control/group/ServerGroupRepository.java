package com.ops.control.group;

import org.springframework.data.jpa.repository.JpaRepository;

import java.util.List;
import java.util.Optional;
import java.util.UUID;

public interface ServerGroupRepository extends JpaRepository<ServerGroupEntity, UUID> {
    List<ServerGroupEntity> findByParentIdOrderBySortOrderAscNameAsc(UUID parentId);

    List<ServerGroupEntity> findByParentIdIsNullOrderBySortOrderAscNameAsc();

    boolean existsByParentId(UUID parentId);

    Optional<ServerGroupEntity> findByNameAndParentIdIsNull(String name);
}
