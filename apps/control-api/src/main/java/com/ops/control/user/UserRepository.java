package com.ops.control.user;

import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.query.Param;

import java.util.List;
import java.util.Optional;
import java.util.UUID;

public interface UserRepository extends JpaRepository<UserEntity, UUID> {
    @Query("select u from UserEntity u where u.username = :username and u.deletedAt is null")
    Optional<UserEntity> findActiveByUsername(@Param("username") String username);

    @Query("select u from UserEntity u where u.deletedAt is null order by lower(u.username)")
    List<UserEntity> findAllActive();

    @Query("select count(u) from UserEntity u where u.role = :role and u.deletedAt is null")
    long countActiveByRole(@Param("role") String role);

    @Query("select count(u) from UserEntity u where u.role = :role and u.enabled = true and u.deletedAt is null")
    long countActiveEnabledByRole(@Param("role") String role);

    default long countByRole(String role) {
        return countActiveByRole(role);
    }

    long countByAuthSource(String authSource);
}
