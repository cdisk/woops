package com.ops.control.auth;

import com.ops.control.access.AccessService;
import com.ops.control.common.OpsProperties;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import jakarta.validation.constraints.NotBlank;
import org.springframework.security.core.Authentication;
import org.springframework.web.bind.annotation.*;
import org.springframework.web.servlet.view.RedirectView;

import java.io.IOException;
import java.util.Map;
import java.util.function.Supplier;

@RestController
@RequestMapping("/api/auth")
public class AuthController {
    private final AuthService authService;
    private final GitLabOAuthService gitlab;
    private final AccessService access;
    private final OpsProperties props;
    private final LoginRateLimiter rateLimiter;

    public AuthController(
            AuthService authService,
            GitLabOAuthService gitlab,
            AccessService access,
            OpsProperties props,
            LoginRateLimiter rateLimiter) {
        this.authService = authService;
        this.gitlab = gitlab;
        this.access = access;
        this.props = props;
        this.rateLimiter = rateLimiter;
    }

    public record LoginRequest(@NotBlank String username, @NotBlank String password) {}

    public record TotpRequest(@NotBlank String pendingToken, @NotBlank String code) {}

    public record GitlabExchangeRequest(@NotBlank String code) {}

    @GetMapping("/login-options")
    public Map<String, Object> loginOptions() {
        return authService.loginOptions();
    }

    @PostMapping("/login")
    public Map<String, Object> login(@RequestBody LoginRequest req, HttpServletRequest request) {
        return guarded(request, req.username(), () -> authService.login(req.username(), req.password()));
    }

    @PostMapping("/login/totp")
    public Map<String, Object> loginTotp(@RequestBody TotpRequest req, HttpServletRequest request) {
        return guarded(request, pendingIdentity(req.pendingToken()), () ->
                authService.completeTotpLogin(req.pendingToken(), req.code()));
    }

    @PostMapping("/login/totp-setup")
    public Map<String, Object> loginTotpSetup(@RequestBody TotpRequest req, HttpServletRequest request) {
        return guarded(request, pendingIdentity(req.pendingToken()), () ->
                authService.completeTotpSetup(req.pendingToken(), req.code()));
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
        String loginCode = gitlab.handleCallback(code, state);
        response.sendRedirect(gitlab.consoleRedirectWithCode(loginCode));
    }

    @PostMapping("/gitlab/exchange")
    public Map<String, Object> gitlabExchange(
            @RequestBody GitlabExchangeRequest req,
            HttpServletRequest request) {
        return guarded(request, "gitlab-exchange", () -> gitlab.exchangeLoginCode(req.code()));
    }

    private Map<String, Object> guarded(HttpServletRequest request, String identity, Supplier<Map<String, Object>> action) {
        String key = LoginRateLimiter.key(clientIp(request), identity);
        rateLimiter.assertNotLocked(key);
        try {
            Map<String, Object> out = action.get();
            rateLimiter.recordSuccess(key);
            return out;
        } catch (IllegalArgumentException e) {
            rateLimiter.recordFailure(key);
            throw e;
        }
    }

    private String pendingIdentity(String pendingToken) {
        try {
            String username = authService.parse(pendingToken).get("username", String.class);
            return username == null || username.isBlank() ? "pending" : username;
        } catch (Exception e) {
            return "pending";
        }
    }

    private static String clientIp(HttpServletRequest request) {
        String real = request.getHeader("X-Real-IP");
        if (real != null && !real.isBlank()) {
            return real.trim();
        }
        String forwarded = request.getHeader("X-Forwarded-For");
        if (forwarded != null && !forwarded.isBlank()) {
            return forwarded.split(",")[0].trim();
        }
        return request.getRemoteAddr();
    }
}
