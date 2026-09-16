package com.ops.control.apitoken;

import java.util.Set;
import java.util.UUID;

/** Authentication details for user API tokens (never contains secret). */
public record ApiTokenAuth(UUID tokenId, String name, Set<String> scopes) {}
