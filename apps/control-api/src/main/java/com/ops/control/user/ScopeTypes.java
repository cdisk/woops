package com.ops.control.user;

public final class ScopeTypes {
    public static final String GROUP = "GROUP";
    public static final String ASSET = "ASSET";

    private ScopeTypes() {}

    public static String requireValid(String type) {
        if (type == null) {
            throw new IllegalArgumentException("scope type required");
        }
        String t = type.trim().toUpperCase();
        if (!GROUP.equals(t) && !ASSET.equals(t)) {
            throw new IllegalArgumentException("invalid scope type: " + type);
        }
        return t;
    }
}
