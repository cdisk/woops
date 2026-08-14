package com.ops.control.agent;

import org.junit.jupiter.api.Test;

import java.util.Base64;
import java.util.HexFormat;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

class InstallCommandPinTest {

    @Test
    void linuxCurlWithoutPin() {
        assertEquals(
                "curl -fsSL https://gw/i/c/install.sh | bash",
                AgentService.buildLinuxInstallCurl("https://gw/i/c/install.sh", ""));
    }

    @Test
    void linuxCurlWithPin() {
        String hex = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff";
        String b64 = Base64.getEncoder().encodeToString(HexFormat.of().parseHex(hex));
        String cmd = AgentService.buildLinuxInstallCurl("https://gw/i/c/install.sh", hex);
        assertTrue(cmd.contains("-k --pinnedpubkey sha256//" + b64));
        assertTrue(cmd.endsWith("https://gw/i/c/install.sh | bash"));
    }

    @Test
    void windowsCurlWithoutPin() {
        String cmd = AgentService.buildWindowsInstallCommand("https://gw/i/c/install.ps1", "");
        assertTrue(cmd.startsWith("$f=Join-Path $env:TEMP woops-install.ps1; "));
        assertTrue(cmd.contains("curl.exe -fsSL -o $f https://gw/i/c/install.ps1"));
        assertTrue(cmd.endsWith("powershell -NoProfile -ExecutionPolicy Bypass -File $f"));
        assertTrue(!cmd.contains("$env:TEMP\\"));
    }

    @Test
    void linuxUpdateExportsProxyFromAgentYaml() {
        String base = AgentService.buildLinuxInstallCurl("https://gw/i/c/install.sh", "");
        String cmd = AgentService.withAgentProxyLinux(base);
        assertTrue(cmd.contains("/etc/woops-agent"));
        assertTrue(!cmd.contains("/etc/ops-agent"));
        assertTrue(cmd.contains("gatewayProxy"));
        assertTrue(cmd.contains("export https_proxy=\"$OPS_PROXY\""));
        assertTrue(cmd.endsWith(base));
        // The proxy may carry credentials, so it must never be echoed back to the console.
        assertTrue(!cmd.contains("echo \"==> using gatewayProxy"));
    }

    @Test
    void windowsUpdateExportsProxyFromAgentYaml() {
        String base = AgentService.buildWindowsInstallCommand("https://gw/i/c/install.ps1", "");
        String cmd = AgentService.withAgentProxyWindows(base);
        assertTrue(cmd.contains("woops-agent"));
        assertTrue(!cmd.contains("'ops-agent'"));
        assertTrue(cmd.contains("$env:HTTPS_PROXY=$p"));
        assertTrue(cmd.endsWith(base));
    }

    @Test
    void windowsUsesCurlWhenPinned() {
        String hex = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff";
        String b64 = Base64.getEncoder().encodeToString(HexFormat.of().parseHex(hex));
        String cmd = AgentService.buildWindowsInstallCommand("https://gw/i/c/install.ps1", hex);
        assertTrue(cmd.contains("curl.exe -fsSL -k --pinnedpubkey sha256//" + b64 + " -o $f "));
        assertTrue(cmd.contains("-File $f"));
        assertTrue(!cmd.contains("| iex"));
        assertTrue(!cmd.contains("Out-String"));
    }
}
