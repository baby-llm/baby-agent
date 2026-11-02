# Optional local code lookup with Sem

Chapter 4 can use [Sem](https://github.com/Ataraxy-Labs/sem) through its existing
stdio MCP transport. This example adds repository lookup without changing the
default servers or the agent loop.

1. Install Sem on the machine running Baby Agent (tested with Sem 0.29.0).
2. Replace the executable and checkout paths in `ch04/mcp-sem.example.json`.
3. Merge its `sem` entry into the root `mcp-server.json`, preserving other entries.
4. Configure the chapter's LLM as usual, then run from the repository root:

```bash
go run ./ch04/main
```

Ask: "Use sem to find LoadMcpServerConfig and show its source location."
The model-facing tool name is `babyagent_mcp__sem__sem_find`; its arguments can
be `{"query":"LoadMcpServerConfig"}` when the selected checkout is Baby Agent.
Check the returned locations against the checkout.

`SEM_LOCAL=1` disables the MCP cloud client; `SEM_MCP_NO_SHARED=1` prevents
reusing a daemon with different settings. These flags do not sandbox the server
or commands it exposes. Sem can write local indexes, and discovered tools include
command-running capabilities, not only lookup. Use a trusted checkout and normal
host/container permissions. Source returned to the agent may reach its LLM.

Remove the `sem` entry and restart the chapter to disconnect it. A failed server
initialization is skipped by the existing client, so inspect startup logs if
tools do not appear. This example does not add retries or change other chapters.
