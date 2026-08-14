package com.ops.control.group;

import com.ops.control.asset.AssetRepository;
import com.ops.control.user.UserScopeRepository;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.Instant;
import java.util.*;

@Service
public class GroupService {
    private final ServerGroupRepository groups;
    private final AssetRepository assets;
    private final UserScopeRepository scopes;

    public GroupService(ServerGroupRepository groups, AssetRepository assets, UserScopeRepository scopes) {
        this.groups = groups;
        this.assets = assets;
        this.scopes = scopes;
    }

    @Transactional(readOnly = true)
    public List<Map<String, Object>> tree() {
        List<ServerGroupEntity> all = groups.findAll();
        Map<UUID, List<ServerGroupEntity>> byParent = new HashMap<>();
        for (ServerGroupEntity g : all) {
            byParent.computeIfAbsent(g.getParentId(), k -> new ArrayList<>()).add(g);
        }
        for (List<ServerGroupEntity> kids : byParent.values()) {
            kids.sort(Comparator
                    .comparingInt(ServerGroupEntity::getSortOrder)
                    .thenComparing(ServerGroupEntity::getName, String.CASE_INSENSITIVE_ORDER));
        }
        return buildChildren(null, byParent);
    }

    private List<Map<String, Object>> buildChildren(UUID parentId, Map<UUID, List<ServerGroupEntity>> byParent) {
        List<ServerGroupEntity> kids = byParent.getOrDefault(parentId, List.of());
        List<Map<String, Object>> out = new ArrayList<>();
        for (ServerGroupEntity g : kids) {
            Map<String, Object> node = new LinkedHashMap<>();
            node.put("id", g.getId().toString());
            node.put("name", g.getName());
            node.put("parentId", g.getParentId() == null ? null : g.getParentId().toString());
            node.put("sortOrder", g.getSortOrder());
            node.put("children", buildChildren(g.getId(), byParent));
            out.add(node);
        }
        return out;
    }

    @Transactional
    public ServerGroupEntity create(String name, UUID parentId) {
        String n = requireName(name);
        if (parentId != null) {
            groups.findById(parentId).orElseThrow(() -> new IllegalArgumentException("parent group not found"));
        }
        ServerGroupEntity g = new ServerGroupEntity();
        g.setName(n);
        g.setParentId(parentId);
        g.setSortOrder(nextSortOrder(parentId));
        g.setCreatedAt(Instant.now());
        g.setUpdatedAt(Instant.now());
        return groups.save(g);
    }

    @Transactional
    public ServerGroupEntity update(UUID id, String name, UUID parentId, boolean updateParent) {
        ServerGroupEntity g = groups.findById(id).orElseThrow(() -> new IllegalArgumentException("group not found"));
        if (name != null && !name.isBlank()) {
            g.setName(requireName(name));
        }
        if (updateParent) {
            if (parentId != null) {
                if (parentId.equals(id)) {
                    throw new IllegalArgumentException("group cannot be its own parent");
                }
                groups.findById(parentId).orElseThrow(() -> new IllegalArgumentException("parent group not found"));
                if (isDescendant(parentId, id)) {
                    throw new IllegalArgumentException("cannot move group under its descendant");
                }
            }
            g.setParentId(parentId);
        }
        g.setUpdatedAt(Instant.now());
        return groups.save(g);
    }

    @Transactional
    public void delete(UUID id) {
        ServerGroupEntity g = groups.findById(id).orElseThrow(() -> new IllegalArgumentException("group not found"));
        if (groups.existsByParentId(id)) {
            throw new IllegalArgumentException("group has child groups; delete or move them first");
        }
        assets.clearGroupId(id);
        scopes.deleteByScopeTypeAndScopeId(com.ops.control.user.ScopeTypes.GROUP, id);
        groups.delete(g);
    }

    @Transactional(readOnly = true)
    public Optional<ServerGroupEntity> find(UUID id) {
        return groups.findById(id);
    }

    @Transactional(readOnly = true)
    public Map<UUID, String> nameById() {
        Map<UUID, String> out = new LinkedHashMap<>();
        for (ServerGroupEntity g : groups.findAll()) {
            out.put(g.getId(), g.getName());
        }
        return out;
    }

    /** rootId itself plus every descendant group id. */
    @Transactional(readOnly = true)
    public Set<UUID> selfAndDescendantIds(UUID rootId) {
        if (rootId == null) {
            return Set.of();
        }
        Map<UUID, List<UUID>> children = new HashMap<>();
        for (ServerGroupEntity g : groups.findAll()) {
            if (g.getParentId() != null) {
                children.computeIfAbsent(g.getParentId(), k -> new ArrayList<>()).add(g.getId());
            }
        }
        Set<UUID> out = new LinkedHashSet<>();
        Deque<UUID> q = new ArrayDeque<>();
        q.add(rootId);
        while (!q.isEmpty()) {
            UUID id = q.removeFirst();
            if (!out.add(id)) {
                continue;
            }
            for (UUID c : children.getOrDefault(id, List.of())) {
                q.add(c);
            }
        }
        return out;
    }

    /** True if candidateId is id or a descendant of id. */
    private boolean isDescendant(UUID candidateId, UUID ancestorId) {
        UUID cur = candidateId;
        Set<UUID> seen = new HashSet<>();
        while (cur != null) {
            if (cur.equals(ancestorId)) return true;
            if (!seen.add(cur)) break;
            cur = groups.findById(cur).map(ServerGroupEntity::getParentId).orElse(null);
        }
        return false;
    }

    private int nextSortOrder(UUID parentId) {
        List<ServerGroupEntity> siblings = parentId == null
                ? groups.findByParentIdIsNullOrderBySortOrderAscNameAsc()
                : groups.findByParentIdOrderBySortOrderAscNameAsc(parentId);
        return siblings.stream().mapToInt(ServerGroupEntity::getSortOrder).max().orElse(-1) + 1;
    }

    private static String requireName(String name) {
        if (name == null || name.isBlank()) {
            throw new IllegalArgumentException("group name is required");
        }
        String n = name.trim();
        if (n.length() > 128) {
            throw new IllegalArgumentException("group name too long");
        }
        return n;
    }
}
