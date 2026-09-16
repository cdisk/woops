package com.ops.control.auth;

import com.ops.control.access.AccessService;
import com.ops.control.user.UserEntity;
import org.springframework.security.core.Authentication;
import org.springframework.web.bind.annotation.*;

import java.util.Map;

@RestController
@RequestMapping("/api/profile")
public class ProfileController {
    private final AuthService auth;
    private final AccessService access;

    public ProfileController(AuthService auth, AccessService access) {
        this.auth = auth;
        this.access = access;
    }

    public record ChangePasswordBody(String currentPassword, String newPassword) {}

    public record ResetTotpBody(String currentPassword, String totpCode) {}

    @PostMapping("/password")
    public Map<String, String> changePassword(@RequestBody ChangePasswordBody body, Authentication authentication) {
        UserEntity user = access.requireUser(authentication);
        String current = body == null ? null : body.currentPassword();
        String next = body == null ? null : body.newPassword();
        return auth.changeOwnPassword(user, current, next);
    }

    @PostMapping("/totp/reset")
    public Map<String, Object> resetTotp(@RequestBody ResetTotpBody body, Authentication authentication) {
        UserEntity user = access.requireUser(authentication);
        String current = body == null ? null : body.currentPassword();
        String code = body == null ? null : body.totpCode();
        return auth.resetOwnTotp(user, current, code);
    }
}
