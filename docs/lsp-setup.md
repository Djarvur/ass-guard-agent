# Setting up LSP tools for ass-guard sessions

ass-guard has no built-in LSP client and does not become one: **agent-side LSP
support is explicitly out of scope** (operator decision, 2026-08-25 — this
guide is documentation only, and no agent-side LSP implementation is included
or promised by it). What an ass-guard session *can* do today is host MCP
servers, and any MCP server that wraps a language server (exposing symbol,
definition, references, and diagnostics tools over LSP) therefore gives your
sessions LSP-grade code intelligence. This guide shows how to configure that —
with [Zed](https://zed.dev) as the worked-example IDE and the generic
"any LSP-capable MCP server" pattern as the secondary framing. The verified
Zed surface is documented honestly below: one of the three configuration
surfaces actually reaches ass-guard sessions today, and it is not the Zed-side
one.

## 1. The three surfaces

There are three places an LSP capability could be configured in a
Zed + ass-guard setup. Only the third reaches ass-guard sessions today:

| Surface | What it is | Reaches ass-guard sessions? | What to do |
|---------|------------|------------------------------|------------|
| Zed's native built-in `diagnostics` tool | LSP-backed errors/warnings for a file or the whole project — Zed's own editing LSP data surfaced to Zed's Agent Panel ([zed.dev/docs/ai/tools](https://zed.dev/docs/ai/tools)) | **No** — built-in tools belong to Zed's own agent; they are not documented as exposed to external ACP agents | Nothing on the ass-guard side; this is the boundary that agent-side LSP work (out of scope, see above) would one day address |
| Zed `context_servers` forwarding | MCP servers configured in Zed settings are *forwarded over ACP* to external agents such as ass-guard ([zed.dev/docs/ai/mcp](https://zed.dev/docs/ai/mcp), [zed.dev/docs/ai/external-agents](https://zed.dev/docs/ai/external-agents)) | **Not yet** — Zed forwards them, but ass-guard currently *parses and ignores* the forwarded `mcpServers` field (`handleSessionNew` in `internal/acp/handlers.go`); consuming it is agent-side work, explicitly out of scope | Skip for now: configure the server in the project `.mcp.json` (below) instead — ass-guard reads that config directly |
| ass-guard project `.mcp.json` | The Claude-Code-compatible project MCP config (camelCase `mcpServers` key) loaded by ass-guard's MCP host (`internal/mcp/host.go`) | **Yes** — this is the leg that works today | Configure the LSP-capable server here; its tools appear in the session catalog as `mcp__<server>__<tool>` |

Honest caveats on the forwarding leg, so this table cannot dead-end you:

- **Extension-installed servers are not forwarded at all, even to agents that
  consume forwarding**: extension-provided MCP servers are not made available
  to local ACP agents — upstream gap
  [zed#52449](https://github.com/zed-industries/zed/issues/52449), still
  **open** as of 2026-09-10 (re-checked at writing time; link carries the
  current status).
- **ass-guard discards the forwarded set today**: the `session/new` handler
  decodes the `mcpServers` field and drops it — see `handleSessionNew` in
  `internal/acp/handlers.go`. Any change to that is agent-side work and
  out of scope for this guide. The dry-run in section 4 therefore routes
  through the `.mcp.json` leg.

## 2. Zed-side setup

This section is for completeness of the worked example: it configures Zed's
own `context_servers` so that Zed-side agents (and, once forwarding is
consumed, ass-guard) can use the same server. It is **not required** for the
dry-run in section 4 — if you only want LSP tools in ass-guard sessions, skip
to section 3.

MCP servers configured in Zed settings live under the `context_servers` key
([zed.dev/docs/ai/mcp](https://zed.dev/docs/ai/mcp)). A local (stdio) server:

```jsonc
// Zed settings — open with `zed: open settings file`
{
  "context_servers": {
    "gopls": {
      "command": "mcp-language-server",
      "args": ["--workspace", "/absolute/path/to/your/project", "--lsp", "gopls"]
    }
  }
}
```

A remote (HTTP) server uses `url` and `headers`; when a remote server has no
configured `Authorization` header, Zed prompts the standard MCP OAuth flow
([zed.dev/docs/ai/mcp](https://zed.dev/docs/ai/mcp)):

```jsonc
{
  "context_servers": {
    "remote-mcp-server": {
      "url": "https://example.com/mcp",
      "headers": { "Authorization": "Bearer <token>" }
    }
  }
}
```

You can also add servers through the GUI: **Settings → AI → MCP Servers →
Add Server** ([zed.dev/docs/ai/mcp](https://zed.dev/docs/ai/mcp)).

Two Zed-side notes:

- **Per-tool permission rules** use the format `mcp:<server>:<tool_name>` —
  for example `mcp:github:create_issue` — in Zed's tool-permission rules
  ([zed.dev/docs/ai/mcp](https://zed.dev/docs/ai/mcp),
  [zed.dev/docs/ai/tool-permissions](https://zed.dev/docs/ai/tool-permissions)).
- **Credentials stay in Zed settings, never in shared project files.**
  Zed settings are machine-local; a bearer header pasted into a committed
  `.mcp.json` or project file leaks to everyone with repo access. Remote
  auth belongs in Zed's settings/OAuth support (or the provider's own env),
  full stop.

## 3. ass-guard-side setup

ass-guard loads the project-scope `.mcp.json` from the working directory
(Claude-Code-compatible layout; the file uses the camelCase `mcpServers`
key). The loader (`internal/mcp/host.go`) supports `command`, `args`, `env`,
and `cwd` per server. This is the surface that actually reaches ass-guard
sessions, so the LSP-capable server is configured here.

The worked example uses
[mcp-language-server](https://github.com/isaacphi/mcp-language-server) — an
MCP server that wraps a language server (gopls for Go) and exposes the LSP
tools `definition`, `references`, `diagnostics`, `hover`, `rename_symbol`,
and `edit_file`. Any MCP server speaking over stdio works the same way; swap
the command/args for your language server of choice (rust-analyzer, pyright,
typescript-language-server, clangd — see the
[mcp-language-server README](https://github.com/isaacphi/mcp-language-server)
for per-language shapes).

```jsonc
// <project>/.mcp.json
{
  "mcpServers": {
    "gopls": {
      "command": "mcp-language-server",
      "args": ["--workspace", "/absolute/path/to/your/project", "--lsp", "gopls"]
    }
  }
}
```

What ass-guard does with this at session start (`internal/mcp/host.go` and
`internal/runtime/runtime.go`):

1. The project `.mcp.json` is loaded and merged *below* nothing — project
   entries win; servers discovered from the user's `~/.claude.json` and
   plugin-bundled `.mcp.json` files join only on non-colliding names.
2. Each server is spawned as a subprocess in its own process group, given a
   bounded 30-second start window, and asked for its tool list. A server
   that fails to start is skipped with a stderr note — one bad server never
   breaks the session.
3. The discovered tools land in the session catalog named
   `mcp__<server>__<tool>` — with the config above:
   `mcp__gopls__definition`, `mcp__gopls__references`,
   `mcp__gopls__diagnostics`, `mcp__gopls__hover`,
   `mcp__gopls__rename_symbol`, `mcp__gopls__edit_file`. The model can call
   them like any built-in tool.

One environment note that matters for editor-launched agents: the MCP server
subprocess **inherits the environment of the ass-guard process, with the
optional `env` entries from `.mcp.json` overlaid on top** (`internal/mcp/lifecycle.go`).
If you launch ass-guard from an editor whose PATH lacks `~/go/bin` (or
wherever `go`/`gopls`/your language server is installed), the server cannot
spawn — either add the directory to the `env` block explicitly:

```jsonc
{
  "mcpServers": {
    "gopls": {
      "command": "mcp-language-server",
      "args": ["--workspace", "/absolute/path/to/your/project", "--lsp", "gopls"],
      "env": { "PATH": "/usr/local/bin:/usr/bin:/bin:/home/you/go/bin" }
    }
  }
}
```

…or make sure the editor itself inherits a PATH that contains the Go bin
directory (`export PATH="$PATH:$(go env GOPATH)/bin"` before launching Zed).

## 4. Dry run

Follow these steps top-to-bottom to reach working LSP-symbol tools inside a
real ass-guard session. Steps 1–5 are the manual setup; step 6 verifies the
tools reach the session catalog hermetically (a scripted ACP client plus a
stub provider — no editor, no model API spend); step 7 is the live editor
path. Commands are verbatim for Linux/macOS with Go installed.

### 1. Install the worked-example MCP server

```sh
go install github.com/isaacphi/mcp-language-server@latest
export PATH="$PATH:$(go env GOPATH)/bin"   # make go-installed binaries visible
mcp-language-server --help >/dev/null 2>&1 && echo mls-ok
```

### 2. Install gopls (the wrapped language server)

```sh
go install golang.org/x/tools/gopls@latest
gopls version | head -1
```

### 3. Create a scratch Go project with symbols to find

```sh
mkdir -p /tmp/ass-guard-lsp-dryrun && cd /tmp/ass-guard-lsp-dryrun
go mod init example.com/dryrun
cat > main.go <<'EOF'
package main

import "fmt"

func greet(who string) string { return "hello " + who }

func main() { fmt.Println(greet("ass-guard")) }
EOF
go build ./... && echo scratch-ok
```

### 4. Write the project `.mcp.json`

```sh
cat > /tmp/ass-guard-lsp-dryrun/.mcp.json <<'EOF'
{
  "mcpServers": {
    "gopls": {
      "command": "mcp-language-server",
      "args": ["--workspace", "/tmp/ass-guard-lsp-dryrun", "--lsp", "gopls"]
    }
  }
}
EOF
cat /tmp/ass-guard-lsp-dryrun/.mcp.json
```

### 5. Build ass-guard

```sh
cd /path/to/ass-guard-agent   # the ass-guard source checkout
go build -o /tmp/ass-guard-dryrun ./cmd/ass-guard/
/tmp/ass-guard-dryrun --version 2>&1 | head -1   # sanity check (prints to stderr)
```

### 6. Verify the tools reach the session catalog (scripted check)

Save the following helper as `/tmp/lsp-dryrun-check.py`. It starts a stub
Anthropic-shape SSE provider on localhost, points the scratch project's
`.ass-guard/config.yaml` at it, spawns the **real** ass-guard binary, drives
the ACP handshake over stdio (newline-delimited JSON-RPC — the same
discipline as ass-guard's own Zed-simulator E2E,
`internal/acpserve/simulator_e2e_test.go`), and prints the `mcp__`-prefixed
tool names found in the outgoing provider request. No editor and no model
API key are consumed:

```python
#!/usr/bin/env python3
"""Hermetic catalog check for docs/lsp-setup.md: prove the .mcp.json server's
LSP tools reach a real ass-guard session's tool catalog."""
import json, os, subprocess, sys, threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

SCRATCH, BIN = sys.argv[1], sys.argv[2]
captured = []  # every outgoing provider request body

SSE = (b'data: {"type":"message_start","message":{"usage":{"input_tokens":5,"output_tokens":1}}}\n\n'
       b'data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}\n\n'
       b'data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"turn complete."}}\n\n'
       b'data: {"type":"content_block_stop","index":0}\n\n'
       b'data: {"type":"message_delta","delta":{"stop_reason":"end_turn"}}\n\n'
       b'data: [DONE]\n\n')

class Stub(BaseHTTPRequestHandler):
    def do_POST(self):
        captured.append(self.rfile.read(int(self.headers["Content-Length"])))
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.end_headers()
        self.wfile.write(SSE)
    def log_message(self, *_): pass

srv = ThreadingHTTPServer(("127.0.0.1", 0), Stub)
threading.Thread(target=srv.serve_forever, daemon=True).start()
url = f"http://127.0.0.1:{srv.server_address[1]}"

os.makedirs(f"{SCRATCH}/.ass-guard", exist_ok=True)
with open(f"{SCRATCH}/.ass-guard/config.yaml", "w") as f:  # point the provider at the stub
    f.write(f'providers:\n  anthropic:\n    base_url: "{url}"\n    api_key: "sk-dryrun"\n')

env = {**os.environ, "ZAI_API_KEY": ""}
proc = subprocess.Popen([BIN, "acp", "serve", "--work-dir", SCRATCH],
                        stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                        stderr=open(f"{SCRATCH}/dryrun-stderr.log", "wb"), env=env, text=True)

def send(obj):
    proc.stdin.write(json.dumps(obj) + "\n"); proc.stdin.flush()

def read_until(predicate, timeout=90.0):
    import time
    deadline = time.time() + timeout
    while time.time() < deadline:
        line = proc.stdout.readline()
        if not line:
            break
        msg = json.loads(line)
        if predicate(msg):
            return msg
    raise TimeoutError("expected frame not observed within guard timeout")

send({"jsonrpc": "2.0", "id": "dry-1", "method": "initialize", "params": {
    "protocolVersion": 1,
    "clientInfo": {"name": "lsp-dryrun", "version": "1"},
    "clientCapabilities": {}}})
read_until(lambda m: m.get("id") == "dry-1")

send({"jsonrpc": "2.0", "id": "dry-2", "method": "session/new",
      "params": {"cwd": SCRATCH, "mcpServers": []}})
new = read_until(lambda m: m.get("id") == "dry-2")
session_id = new["result"]["sessionId"]

send({"jsonrpc": "2.0", "id": "dry-3", "method": "session/prompt",
      "params": {"sessionId": session_id,
                 "prompt": [{"type": "text", "text": "list the symbols in main.go"}]}})
read_until(lambda m: m.get("id") == "dry-3")  # turn finished
proc.terminate()

names = sorted({t["name"] for body in captured for t in json.loads(body).get("tools", [])}
               - {""})
mcp = [n for n in names if n.startswith("mcp__")]
print("provider requests captured:", len(captured))
print("mcp tools in session catalog:")
for n in mcp:
    print(" ", n)
sys.exit(0 if any("__definition" in n or "__references" in n for n in mcp) else 1)
```

Run it:

```sh
python3 /tmp/lsp-dryrun-check.py /tmp/ass-guard-lsp-dryrun /tmp/ass-guard-dryrun
```

Expected output — the LSP-symbol tools from the `.mcp.json` server, under the
`mcp__<server>__<tool>` naming:

```text
provider requests captured: 1
mcp tools in session catalog:
  mcp__gopls__definition
  mcp__gopls__diagnostics
  mcp__gopls__edit_file
  mcp__gopls__hover
  mcp__gopls__references
  mcp__gopls__rename_symbol
```

(Exit status 0 means an LSP-symbol tool was present; the scratch project's
`dryrun-stderr.log` carries ass-guard's diagnostics if anything goes wrong.)

### 7. Try it live in the editor

Remove or ignore the scratch `.ass-guard/config.yaml` from step 6 (it points
the provider at a stub that no longer exists) — in a real project you would
not have created it. Open the project in Zed, start an ass-guard thread, and
ask: *"use the definition tool to find where greet is defined, then list the
references to it."* The model discovers `mcp__gopls__definition` and
`mcp__gopls__references` in its tool catalog and calls them.

## 5. Known limitations

- **Forwarding is not consumed.** Zed forwards `context_servers`-configured
  MCP servers to external agents over ACP
  ([zed.dev/docs/ai/mcp](https://zed.dev/docs/ai/mcp)), but ass-guard parses
  and ignores the forwarded `mcpServers` field today
  (`internal/acp/handlers.go`). Configure servers in the project `.mcp.json`
  instead. Making ass-guard consume forwarding is agent-side work and
  explicitly out of scope for this guide (operator decision 2026-08-25).
- **Extension-installed servers are not forwarded upstream.** Even a
  forwarding-consuming agent would not see MCP servers installed as Zed
  extensions — [zed#52449](https://github.com/zed-industries/zed/issues/52449),
  open as of 2026-09-10.
- **Zed's native `diagnostics` tool is Zed-Panel-only.** Zed's own editing
  LSP data (errors/warnings) reaches Zed's built-in agent via the native
  `diagnostics` tool ([zed.dev/docs/ai/tools](https://zed.dev/docs/ai/tools));
  it is not documented as exposed to external ACP agents. The worked example
  gets equivalent (and richer symbol-level) capability into ass-guard
  sessions by hosting the MCP-wrapped language server directly.
- **No agent-side LSP implementation is included or planned by this guide.**
  The 2026-08-25 operator decision keeps ass-guard LSP-free on the agent
  side; everything here is IDE-side/project-side configuration.

## See also

- [`docs/install.md`](./install.md) — installing ass-guard and the Zed ACP
  registry setup.
- [`docs/recapture-runbook.md`](./recapture-runbook.md) — re-capturing the
  mimicry profile.
- [Zed MCP (context servers)](https://zed.dev/docs/ai/mcp) — the canonical
  Zed-side configuration reference.
- [Zed built-in agent tools](https://zed.dev/docs/ai/tools) — the native
  `diagnostics` tool.
- [Zed external agents](https://zed.dev/docs/ai/external-agents) — the
  ACP forwarding boundary.
- [mcp-language-server](https://github.com/isaacphi/mcp-language-server) —
  the worked-example LSP-to-MCP server.
