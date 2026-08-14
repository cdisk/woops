package com.ops.control.asset;

import java.util.List;

/** Derive remote-access protocol and allowed actions from agent-reported OS. */
public final class AssetAccess {
    private AssetAccess() {}

    public static boolean isWindows(String os) {
        if (os == null || os.isBlank()) return false;
        String s = os.toLowerCase();
        // Matches "windows", "Windows Server 2016"; avoids unrelated "win*" substrings.
        return s.equals("windows") || s.contains("windows");
    }

    /** Display badge: windows → rdp; linux/mac → vnc */
    public static String protocolOf(AssetEntity asset) {
        return isWindows(asset.getOs()) ? "rdp" : "vnc";
    }

    /** Allowed session protocols for this asset OS. */
    public static boolean allows(AssetEntity asset, String protocol) {
        if (protocol == null || protocol.isBlank()) return false;
        return switch (protocol.toLowerCase()) {
            case "shell_powershell", "rdp" -> isWindows(asset.getOs());
            case "shell_bash", "vnc" -> !isWindows(asset.getOs());
            case "filemanager", "exec", "filetransfer" -> true;
            default -> false;
        };
    }

    /** UI action list (ordered). */
    public static List<String> actions(AssetEntity asset) {
        if (isWindows(asset.getOs())) {
            return List.of("shell_powershell", "filemanager", "rdp");
        }
        return List.of("shell_bash", "filemanager", "vnc");
    }

    /** First-register desktop port defaults (Windows RDP / Linux VNC). */
    public static int defaultDesktopPort(String os) {
        return isWindows(os) ? 3389 : 5900;
    }

    public static void applyRegisterDefaults(AssetEntity asset, boolean isNew) {
        if (!isNew) {
            // Reinstall preserves existing desktop credentials and port.
            return;
        }
        if (isWindows(asset.getOs())) {
            asset.setDesktopPort(3389);
            asset.setDesktopUsername("Administrator");
        } else {
            asset.setDesktopPort(5900);
            asset.setDesktopUsername("");
        }
    }
}
