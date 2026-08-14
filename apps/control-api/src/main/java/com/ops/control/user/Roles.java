package com.ops.control.user;

public final class Roles {
    public static final String SUPER_ADMIN = "SUPER_ADMIN";
    public static final String ADMIN = "ADMIN";
    public static final String MEMBER = "MEMBER";

    private Roles() {}

    public static boolean isSuperAdmin(String role) {
        return SUPER_ADMIN.equals(role);
    }

    public static boolean isAdmin(String role) {
        return ADMIN.equals(role);
    }

    public static boolean isMember(String role) {
        return MEMBER.equals(role);
    }

    /** Admin or super — can manage assets/install codes/groups/alert rules. */
    public static boolean canManageInventory(String role) {
        return isSuperAdmin(role) || isAdmin(role);
    }

    public static String requireValid(String role) {
        if (role == null || role.isBlank()) {
            throw new IllegalArgumentException("role required");
        }
        String r = role.trim().toUpperCase();
        if (!SUPER_ADMIN.equals(r) && !ADMIN.equals(r) && !MEMBER.equals(r)) {
            throw new IllegalArgumentException("invalid role: " + role);
        }
        return r;
    }
}
