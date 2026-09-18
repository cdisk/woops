package com.ops.control.agent;

import org.junit.jupiter.api.Test;

import java.util.Base64;
import java.util.HexFormat;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

class InstallCommandPinTest {

    private static final String GW = "https://gw.example:9200";
    private static final String CODE = "abc123";

    @Test
    void linuxCurlWithoutPin() {
        String cmd = AgentService.buildLinuxInstallCurl(GW, CODE, "");
        assertTrue(cmd.contains("curl -fsSL --compressed -o /tmp/woops-agent"));
        assertTrue(cmd.contains("/agent/linux/$(uname -m | sed -e s/x86_64/amd64/ -e s/aarch64/arm64/)"));
        assertTrue(cmd.contains("chmod +x /tmp/woops-agent"));
        assertTrue(cmd.contains("/tmp/woops-agent install -gateway " + GW + " -code " + CODE));
        assertTrue(!cmd.contains("install.sh"));
        assertTrue(!cmd.contains("| bash"));
    }

    @Test
    void linuxCurlWithPin() {
        String hex = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff";
        String b64 = Base64.getEncoder().encodeToString(HexFormat.of().parseHex(hex));
        String cmd = AgentService.buildLinuxInstallCurl(GW, CODE, hex);
        assertTrue(cmd.contains("--pinnedpubkey sha256//" + b64));
        assertTrue(cmd.contains(" -pin " + hex));
        assertTrue(cmd.contains("--compressed"));
        // CentOS 6 curl lacks --pinnedpubkey; command must probe and fall back to -k.
        assertTrue(cmd.contains("curl --help 2>&1 | grep -q -- '--pinnedpubkey'"));
        assertTrue(cmd.contains("curl has no --pinnedpubkey"));
    }

    @Test
    void windowsCurlWithoutPin() {
        String cmd = AgentService.buildWindowsInstallCommand(GW, CODE, "", false);
        assertTrue(cmd.startsWith("$f=Join-Path $env:TEMP woops-agent.exe; "));
        assertTrue(cmd.contains("curl.exe -fsSL --compressed -o $f " + GW + "/i/" + CODE + "/agent/windows/amd64"));
        assertTrue(cmd.contains("& $f install -gateway " + GW + " -code " + CODE));
        assertTrue(!cmd.contains("install.ps1"));
        assertTrue(!cmd.contains("-File"));
    }

    @Test
    void windowsUpdateUsesProgramData() {
        String cmd = AgentService.buildWindowsInstallCommand(GW, CODE, "", true);
        assertTrue(cmd.contains("$env:ProgramData"));
        assertTrue(cmd.contains("woops-agent-setup.exe"));
        assertTrue(cmd.contains("& $f install -gateway "));
        assertTrue(!cmd.contains("%TEMP%"));
    }

    @Test
    void linuxUpdateExportsProxyFromAgentYaml() {
        String base = AgentService.buildLinuxInstallCurl(GW, CODE, "");
        String cmd = AgentService.withAgentProxyLinux(base);
        assertTrue(cmd.contains("/etc/woops-agent"));
        assertTrue(!cmd.contains("/etc/ops-agent"));
        assertTrue(cmd.contains("gatewayProxy"));
        assertTrue(cmd.contains("export https_proxy=\"$OPS_PROXY\""));
        assertTrue(cmd.endsWith(base));
        assertTrue(!cmd.contains("echo \"==> using gatewayProxy"));
    }

    @Test
    void windowsUpdateExportsProxyFromAgentYaml() {
        String base = AgentService.buildWindowsInstallCommand(GW, CODE, "", true);
        String cmd = AgentService.withAgentProxyWindows(base);
        assertTrue(cmd.contains("woops-agent"));
        assertTrue(cmd.contains("$env:HTTPS_PROXY=$p"));
        assertTrue(cmd.contains("[System.IO.File]::ReadAllText($c)"));
        assertTrue(!cmd.contains("Get-Content -Raw"));
        assertTrue(cmd.endsWith(base));
    }

    @Test
    void windowsLegacyCmdWithPin() {
        String hex = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff";
        String b64 = Base64.getEncoder().encodeToString(HexFormat.of().parseHex(hex));
        String cmd = AgentService.buildWindowsLegacyInstallCommand(GW, CODE, hex);
        assertTrue(cmd.startsWith("curl.exe -fsSL"));
        assertTrue(cmd.contains("curl.exe -fsSL -k --pinnedpubkey sha256//" + b64));
        assertTrue(cmd.contains("\"%TEMP%\\woops-agent.exe\""));
        assertTrue(cmd.contains(" && \"%TEMP%\\woops-agent.exe\" install -gateway "));
        assertTrue(cmd.contains(" -pin " + hex));
        assertTrue(!cmd.contains("install.bat"));
    }

    @Test
    void windowsLegacyCmdWithoutPin() {
        String cmd = AgentService.buildWindowsLegacyInstallCommand(GW, CODE, "");
        assertEquals(
                "curl.exe -fsSL --compressed -o \"%TEMP%\\woops-agent.exe\" " + GW + "/i/" + CODE
                        + "/agent/windows/amd64 && \"%TEMP%\\woops-agent.exe\" install -gateway "
                        + GW + " -code " + CODE,
                cmd);
    }

    @Test
    void windowsUsesCurlWhenPinned() {
        String hex = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff";
        String b64 = Base64.getEncoder().encodeToString(HexFormat.of().parseHex(hex));
        String cmd = AgentService.buildWindowsInstallCommand(GW, CODE, hex, false);
        assertTrue(cmd.contains("curl.exe -fsSL -k --pinnedpubkey sha256//" + b64 + " --compressed -o $f "));
        assertTrue(cmd.contains("& $f install "));
        assertTrue(!cmd.contains("| iex"));
        assertTrue(!cmd.contains("Out-String"));
    }
}
