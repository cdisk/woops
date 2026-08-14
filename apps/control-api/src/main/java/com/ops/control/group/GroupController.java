package com.ops.control.group;

import com.ops.control.access.AccessService;
import com.ops.control.controlaudit.ControlAuditService;
import com.ops.control.user.UserEntity;
import org.springframework.security.core.Authentication;
import org.springframework.web.bind.annotation.*;

import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.UUID;

@RestController
@RequestMapping("/api/groups")
public class GroupController {
    private final GroupService groups;
    private final AccessService access;
    private final ControlAuditService audit;

    public GroupController(GroupService groups, AccessService access, ControlAuditService audit) {
        this.groups = groups;
        this.access = access;
        this.audit = audit;
    }

    @GetMapping
    public List<Map<String, Object>> tree(Authentication auth) {
        UserEntity user = access.requireUser(auth);
        return access.pruneGroupTree(user, groups.tree());
    }

    public record CreateRequest(String name, UUID parentId) {}

    @PostMapping
    public Map<String, Object> create(@RequestBody CreateRequest req, Authentication auth) {
        UserEntity user = access.requireUser(auth);
        access.assertCanManageInventory(user);
        if (req.parentId() != null) {
            access.assertCanManageGroup(user, req.parentId());
        } else if (!access.isSuperAdmin(user)) {
            // Admins may only create under an existing manageable parent, not new roots outside scope.
            throw new com.ops.control.access.ForbiddenException("admin cannot create root groups outside scope; pick a parent");
        }
        ServerGroupEntity created = groups.create(req.name(), req.parentId());
        audit.record(
                ControlAuditService.CAT_GROUP,
                ControlAuditService.ACT_CREATE,
                user.getId(),
                user.getUsername(),
                null,
                created.getId(),
                null,
                ControlAuditService.jsonDetail(Map.of(
                        "name", created.getName(),
                        "parentId", created.getParentId() == null ? "" : created.getParentId().toString()
                )));
        return toView(created);
    }

    /** Set updateParent=true to change parent (parentId null = move to root). */
    public record UpdateRequest(String name, UUID parentId, Boolean updateParent) {}

    @PatchMapping("/{id}")
    public Map<String, Object> update(@PathVariable UUID id, @RequestBody UpdateRequest req, Authentication auth) {
        UserEntity user = access.requireUser(auth);
        access.assertCanManageInventory(user);
        access.assertCanManageGroup(user, id);
        boolean updateParent = Boolean.TRUE.equals(req.updateParent());
        if (updateParent) {
            if (req.parentId() == null) {
                if (!access.isSuperAdmin(user)) {
                    throw new com.ops.control.access.ForbiddenException("only super admin can move group to root");
                }
            } else {
                access.assertCanManageGroup(user, req.parentId());
            }
        }
        ServerGroupEntity updated = groups.update(id, req.name(), req.parentId(), updateParent);
        Map<String, Object> detail = new LinkedHashMap<>();
        detail.put("name", updated.getName());
        if (updateParent) {
            detail.put("parentId", updated.getParentId() == null ? "" : updated.getParentId().toString());
        }
        audit.record(
                ControlAuditService.CAT_GROUP,
                ControlAuditService.ACT_UPDATE,
                user.getId(),
                user.getUsername(),
                null,
                updated.getId(),
                null,
                ControlAuditService.jsonDetail(detail));
        return toView(updated);
    }

    @DeleteMapping("/{id}")
    public Map<String, String> delete(@PathVariable UUID id, Authentication auth) {
        UserEntity user = access.requireUser(auth);
        access.assertCanManageInventory(user);
        access.assertCanManageGroup(user, id);
        ServerGroupEntity g = groups.find(id).orElseThrow(() -> new IllegalArgumentException("group not found"));
        audit.record(
                ControlAuditService.CAT_GROUP,
                ControlAuditService.ACT_REMOVE,
                user.getId(),
                user.getUsername(),
                null,
                g.getId(),
                null,
                ControlAuditService.jsonDetail(Map.of("name", g.getName())));
        groups.delete(id);
        return Map.of("status", "deleted", "id", id.toString());
    }

    private Map<String, Object> toView(ServerGroupEntity g) {
        Map<String, Object> m = new LinkedHashMap<>();
        m.put("id", g.getId().toString());
        m.put("name", g.getName());
        m.put("parentId", g.getParentId() == null ? null : g.getParentId().toString());
        m.put("sortOrder", g.getSortOrder());
        m.put("createdAt", g.getCreatedAt().toString());
        m.put("updatedAt", g.getUpdatedAt().toString());
        return m;
    }
}
