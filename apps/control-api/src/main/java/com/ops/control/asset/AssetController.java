package com.ops.control.asset;

import com.ops.control.access.AccessService;
import com.ops.control.agent.AgentService;
import com.ops.control.user.UserEntity;
import jakarta.servlet.http.HttpServletRequest;
import org.springframework.security.core.Authentication;
import org.springframework.web.bind.annotation.*;

import java.util.List;
import java.util.Map;
import java.util.UUID;

@RestController
@RequestMapping("/api/assets")
public class AssetController {
    private final AssetService assetService;
    private final AssetRepository assets;
    private final AccessService access;
    private final AgentService agents;

    public AssetController(
            AssetService assetService,
            AssetRepository assets,
            AccessService access,
            AgentService agents) {
        this.assetService = assetService;
        this.assets = assets;
        this.access = access;
        this.agents = agents;
    }

    @GetMapping
    public List<Map<String, Object>> list(
            @RequestParam(required = false) UUID groupId,
            @RequestParam(required = false, defaultValue = "false") boolean rootOnly,
            @RequestParam(required = false, defaultValue = "false") boolean includeSubtree,
            Authentication auth) {
        UserEntity user = access.requireUser(auth);
        return assetService.list(user, groupId, rootOnly, includeSubtree);
    }

    @GetMapping("/{id}")
    public Map<String, Object> get(@PathVariable UUID id, Authentication auth) {
        UserEntity user = access.requireUser(auth);
        return assetService.get(user, id);
    }

    @PatchMapping("/{id}")
    public Map<String, Object> update(
            @PathVariable UUID id,
            @RequestBody AssetService.UpdateRequest req,
            Authentication auth) {
        UserEntity user = access.requireUser(auth);
        return assetService.update(user, id, req);
    }

    @DeleteMapping("/{id}")
    public Map<String, String> delete(@PathVariable UUID id, Authentication auth) {
        UserEntity user = access.requireUser(auth);
        return assetService.delete(user, id);
    }

    /**
     * One-click Agent update: mint group install code, control-audit UPDATE_AGENT,
     * return curl/powershell for the console to run via /ws/exec.
     */
    @PostMapping("/{id}/agent-update")
    public Map<String, Object> agentUpdate(
            @PathVariable UUID id,
            Authentication auth,
            HttpServletRequest request) {
        UserEntity user = access.requireUser(auth);
        AssetEntity asset = assets.findById(id).orElseThrow(() -> new IllegalArgumentException("asset not found"));
        access.assertCanAccessAsset(user, asset);
        return agents.prepareAgentUpdate(user.getId(), asset, request);
    }

    @GetMapping("/{id}/shell-commands")
    public Map<String, Object> getShellCommands(@PathVariable UUID id, Authentication auth) {
        UserEntity user = access.requireUser(auth);
        return assetService.getShellCommands(user, id);
    }

    @PutMapping("/{id}/shell-commands")
    public Map<String, Object> putShellCommands(
            @PathVariable UUID id,
            @RequestBody AssetService.ShellCommandsRequest req,
            Authentication auth) {
        UserEntity user = access.requireUser(auth);
        return assetService.putShellCommands(user, id, req);
    }
}
