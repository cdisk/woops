package com.ops.control.settings;

import com.ops.control.access.AccessService;
import com.ops.control.common.OpsProperties;
import com.ops.control.user.UserEntity;
import org.springframework.security.core.Authentication;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import java.util.LinkedHashMap;
import java.util.Map;

@RestController
@RequestMapping("/api/settings")
public class SettingsController {
    private final OpsProperties props;
    private final AccessService access;

    public SettingsController(OpsProperties props, AccessService access) {
        this.props = props;
        this.access = access;
    }

    @GetMapping("/gitlab/install-info")
    public Map<String, Object> gitlabInstallInfo(Authentication auth) {
        UserEntity user = access.requireUser(auth);
        if (!access.isSuperAdmin(user)) {
            throw new com.ops.control.access.ForbiddenException("requires super admin");
        }
        Map<String, Object> out = new LinkedHashMap<>();
        out.put("enabled", props.gitlabEnabled());
        out.put("baseUrl", props.gitlabBaseUrl());
        out.put("redirectUri", props.gitlabRedirectUri());
        out.put("adminUser", props.gitlab() == null || props.gitlab().adminUser() == null
                ? "" : props.gitlab().adminUser());
        out.put("localLoginEnabled", props.localLoginEnabled());
        out.put("scopes", "read_user");
        out.put("hint", "配置 OPS_GITLAB_BASE_URL / CLIENT_ID / CLIENT_SECRET 启用并关本地登录；OPS_GITLAB_ADMIN_USER=GitLab用户名（可逗号多个）指定超管");
        return out;
    }
}
