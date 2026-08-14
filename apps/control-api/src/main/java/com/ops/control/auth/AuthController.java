package com.ops.control.auth;

import com.ops.control.access.AccessService;
import com.ops.control.common.OpsProperties;
import jakarta.servlet.http.HttpServletResponse;
import jakarta.validation.constraints.NotBlank;
import org.springframework.security.core.Authentication;
import org.springframework.web.bind.annotation.*;
import org.springframework.web.servlet.view.RedirectView;

import java.io.IOException;
import java.util.Map;

@RestController
@RequestMapping("/api/auth")
public class AuthController {
    private final AuthService authService;
    private final GitLabOAuthService gitlab;
    private final AccessService access;
    private final OpsProperties props;

    public AuthController(AuthService authService, GitLabOAuthService gitlab, AccessService access, OpsProperties props) {
        this.authService = authService;
        this.gitlab = gitlab;
        this.access = access;
        this.props = props;
    }

    public record LoginRequest(@NotBlank String username, @NotBlank String password) {}

    public record TotpRequest(@NotBlank String pendingToken, @NotBlank String code) {}

    @GetMapping("/login-options")
    public Map<String, Object> loginOptions() {
        return authService.loginOptions();
    }

    @PostMapping("/login")
    public Map<String, Object> login(@RequestBody LoginRequest req) {
        return authService.login(req.username(), req.password());
    }

    @PostMapping("/login/totp")
    public Map<String, Object> loginTotp(@RequestBody TotpRequest req) {
        return authService.completeTotpLogin(req.pendingToken(), req.code());
    }

    @PostMapping("/login/totp-setup")
    public Map<String, Object> loginTotpSetup(@RequestBody TotpRequest req) {
        return authService.completeTotpSetup(req.pendingToken(), req.code());
    }

    @GetMapping("/me")
    public Map<String, Object> me(Authentication authentication) {
        return authService.me(access.requireUser(authentication));
    }

    @GetMapping("/gitlab/authorize")
    public RedirectView gitlabAuthorize() {
        return new RedirectView(gitlab.authorizeUrl());
    }

    @GetMapping("/gitlab/callback")
    public void gitlabCallback(
            @RequestParam(required = false) String code,
            @RequestParam(required = false) String state,
            @RequestParam(required = false) String error,
            HttpServletResponse response) throws IOException {
        if (error != null && !error.isBlank()) {
            response.sendRedirect(props.consoleBase() + "/login?error=" + error);
            return;
        }
        Map<String, Object> tokens = gitlab.handleCallback(code, state);
        String accessToken = String.valueOf(tokens.get("accessToken"));
        response.sendRedirect(gitlab.consoleRedirectWithToken(accessToken));
    }
}
