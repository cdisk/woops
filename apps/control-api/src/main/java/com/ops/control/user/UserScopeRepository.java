package com.ops.control.user;

import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Modifying;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.query.Param;

import java.util.List;
import java.util.UUID;

public interface UserScopeRepository extends JpaRepository<UserScopeEntity, UUID> {
    List<UserScopeEntity> findByUserId(UUID userId);

    @Query("select s from UserScopeEntity s where s.userId = :userId and s.scopeType = :scopeType")
    List<UserScopeEntity> findByUserIdAndScopeType(
            @Param("userId") UUID userId, @Param("scopeType") String scopeType);

    long countByScopeType(String scopeType);

    @Modifying(clearAutomatically = true, flushAutomatically = true)
    @Query("delete from UserScopeEntity s where s.userId = :userId")
    int deleteByUserId(@Param("userId") UUID userId);

    @Modifying(clearAutomatically = true, flushAutomatically = true)
    @Query("delete from UserScopeEntity s where s.scopeType = :scopeType and s.scopeId = :scopeId")
    int deleteByScopeTypeAndScopeId(@Param("scopeType") String scopeType, @Param("scopeId") UUID scopeId);
}
