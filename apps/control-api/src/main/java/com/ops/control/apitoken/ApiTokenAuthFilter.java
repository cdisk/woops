package com.ops.control.apitoken;

import com.ops.control.user.UserEntity;
import jakarta.servlet.FilterChain;
import jakarta.servlet.ServletException;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import org.springframework.http.HttpHeaders;
import org.springframework.http.MediaType;
import org.springframework.security.authentication.UsernamePasswordAuthenticationToken;
import org.springframework.security.core.authority.SimpleGrantedAuthority;
import org.springframework.security.core.context.SecurityContextHolder;
import org.springframework.stereotype.Component;
import org.springframework.web.filter.OncePerRequestFilter;

import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.util.List;

/**
 * Authenticates {@code Authorization: Bearer wpat_...} user API tokens and enforces path→scope.
 */
@Component
public class ApiTokenAuthFilter extends OncePerRequestFilter {
    private final ApiTokenService tokens;

    public ApiTokenAuthFilter(ApiTokenService tokens) {
        this.tokens = tokens;
    }

    @Override
    protected boolean shouldNotFilter(HttpServletRequest request) {
        String path = request.getRequestURI();
        return path.startsWith("/api/opsctl/")
                || path.startsWith("/api/internal/")
                || path.startsWith("/api/sessions/internal/");
    }

    @Override
    protected void doFilterInternal(HttpServletRequest request, HttpServletResponse response, FilterChain filterChain)
            throws ServletException, IOException {
        String header = request.getHeader(HttpHeaders.AUTHORIZATION);
        if (header == null || !header.startsWith("Bearer ")) {
            filterChain.doFilter(request, response);
            return;
        }
        String bearer = header.substring(7).trim();
        if (!bearer.startsWith(ApiTokenService.TOKEN_PREFIX)) {
            filterChain.doFilter(request, response);
            return;
        }

        ApiTokenService.ResolvedToken resolved = tokens.resolve(bearer);
        if (resolved == null) {
            writeUnauthorized(response, "invalid api token");
            return;
        }

        String required = ApiTokenPathScopes.requiredScope(request.getMethod(), request.getRequestURI());
        if (required == null || !resolved.auth().scopes().contains(required)) {
            writeForbidden(response, "insufficient api token scope");
            return;
        }

        UserEntity user = resolved.user();
        var auth = new UsernamePasswordAuthenticationToken(
                user.getId().toString(),
                null,
                List.of(new SimpleGrantedAuthority("ROLE_" + user.getRole()))
        );
        auth.setDetails(resolved.auth());
        SecurityContextHolder.getContext().setAuthentication(auth);

        try {
            tokens.touchLastUsed(resolved.auth().tokenId());
        } catch (Exception ignored) {
            // never fail the request on lastUsed throttle write
        }

        filterChain.doFilter(request, response);
    }

    private static void writeUnauthorized(HttpServletResponse response, String msg) throws IOException {
        response.setStatus(HttpServletResponse.SC_UNAUTHORIZED);
        response.setCharacterEncoding(StandardCharsets.UTF_8.name());
        response.setContentType(MediaType.APPLICATION_JSON_VALUE);
        response.getWriter().write("{\"error\":\"" + msg + "\"}");
    }

    private static void writeForbidden(HttpServletResponse response, String msg) throws IOException {
        response.setStatus(HttpServletResponse.SC_FORBIDDEN);
        response.setCharacterEncoding(StandardCharsets.UTF_8.name());
        response.setContentType(MediaType.APPLICATION_JSON_VALUE);
        response.getWriter().write("{\"error\":\"" + msg + "\"}");
    }
}
