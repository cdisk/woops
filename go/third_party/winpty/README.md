# WinPTY runtime (Server 2016 / pre-ConPTY)

Upstream: [rprichard/winpty](https://github.com/rprichard/winpty) **0.4.3** (MIT).  
Binaries from `winpty-0.4.3-msvc2015.zip` → `x64/bin/`.

| File | Role |
|------|------|
| `x64/winpty.dll` | Loaded by Agent |
| `x64/winpty-agent.exe` | Hidden console bridge |
| `LICENSE` | MIT notice (required when redistributing) |

Gateway serves copies under `bin/winpty/amd64/` (see install.ps1; only downloaded when OS build &lt; 17763).
