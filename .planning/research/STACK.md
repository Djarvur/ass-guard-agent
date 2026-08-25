# Stack Research

**Domain:** Go AI coding agent — v1.2 Claude Code Parity additions (ACP completeness, chat commands, skills, parity closure, sandbox/checkpoints/compaction, kit extraction)
**Researched:** 2026-08-26
**Confidence:** HIGH on ACP surface and provider SDKs (verified against official schema + module source); MEDIUM on sandboxing prior art; LOW-MEDIUM on compaction thresholds (community reverse-engineering only)

## Context: What This Research Covers

v1.0/v1.1 stack (Go 1.26 single static binary, hand-rolled ACP JSON-RPC over stdio, anthropic-sdk-go + go-openai, MCP hosting, cobra) is validated and **not re-researched**. This file covers only stack deltas for v1.2 features. Grounded against the actual tree: `internal/acp/` (framer.go, types.go, handlers.go, server.go — hand-rolled, currently streams `agent_message_chunk` only), `internal/checkpoint/store.go` (shadow-git Snapshot/Restore already implemented), `internal/coreexec/bash.go` (sandbox flag parsed as deliberate no-op), no image support anywhere in `internal/provider/`.

## Recommended Stack

### Core Technologies

| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| Go (existing) | 1.26 | Language/runtime | Unchanged; every addition below was verified to compile under the project's hard `CGO_ENABLED=0` gate |
| Hand-rolled ACP layer (`internal/acp`) — **extended, not replaced** | in-tree | All new ACP methods | Already proven live against Zed (incl. the string-UUID id quirk). The new methods are plain JSON-RPC request/response pairs — the framer needs zero changes, only new typed params in types.go + handlers. Swapping layers mid-milestone would invalidate the Phase-2 UAT transport evidence |
| `github.com/coder/acp-go-sdk` | v0.13.5 (2026-06-02) | **Schema reference / test oracle only** | Zero-dependency (stdlib-only, go 1.21), generated types cover the entire target surface incl. unstable elicitation. Use its `schema/schema.json` + `types_gen.go` as the authoritative shape reference when hand-writing our structs; optionally import as a test-only dependency to fuzz our framer against their codec. Do NOT adopt as the transport |

**Critical correction to common belief:** there is **no official Zed Go SDK**. `github.com/zed-industries/agent-client-protocol@v1.7.0` publishes Rust + TS schema only (verified: zero `.go` files in the module; the `/go` subpath 404s on the Go proxy). Search-engine claims of `NewAgentSideConnection` in a Zed Go package are false. Coder's community SDK is the only maintained Go option (release v0.13.5 is literally titled "Session config options take over model selection").

### Supporting Libraries

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/anthropics/anthropic-sdk-go` (bump) | v1.63.0 → **v1.66.0** | Image blocks + thinking-block replay | Routine minor bump (latest 2026-08-19). Image support: `anthropic.NewImageBlockBase64(mediaType, encoded)` (jpeg/png/gif/webp) drops into any `NewUserMessage(...)` block list. Thinking replay: assistant turns must carry `thinking` + `signature` verbatim and in original order when tools + extended thinking are active — load-bearing constraint for both compaction and transcript projection |
| `github.com/sashabaranov/go-openai` (no change) | v1.42.0 (= current latest) | OpenAI-shape images | Already in go.mod and already current. `ChatMessage.MultiContent []ChatMessagePart` with `Type: ChatMessagePartTypeImageURL`, `ImageURL.URL: "data:image/png;base64,<…>"` — verified present in the vendored v1.42.0 source. Zero work beyond a mapper in `internal/provider/openai.go` |
| `github.com/creack/pty` (**new**) | **v1.1.24** | Persistent-shell Bash tool | De-facto standard (successor of deprecated kr/pty), MIT, darwin+linux, zero transitive deps. Latest stable verified on the Go proxy. Use `pty.StartWithSize` (not deprecated `pty.Start`, whose nil-pipe return panics naive closers) |
| `github.com/landlock-lsm/go-landlock` (**new**, Linux-only) | **v0.10.0** (2026-08-24) | Filesystem/network confinement for the sandbox flag | Official Landlock LSM binding, actively maintained by the Landlock org. **Directly verified: builds clean with `CGO_ENABLED=0`** (its psx dep has a pure-Go fallback). Kernel ≥ 5.13, unprivileged, no namespaces — immune to the Ubuntu 24.04 AppArmor userns clamp that breaks bubblewrap. Guard with runtime kernel check; degrade to unsandboxed-with-warning elsewhere |
| `git` binary via `os/exec` (existing pattern) | system | Checkpoint plumbing | `internal/checkpoint/store.go` already shells out to git with `GIT_DIR`/work-tree overrides. Keep shelling out; see What NOT to Use for why not go-git |

### Development Tools

| Tool | Purpose | Notes |
|------|---------|-------|
| `mise ci` (existing gate) | vet + golangci-lint v2 + CGO_ENABLED=0 build + `-race` tests | Every new dep above was pre-checked against the CGO leg; keep this as the acceptance bar for any further additions |
| Zed editor | Live ACP conformance harness | The real oracle for request_permission UI, elicitation forms, configOptions UI. Manual UAT beats any unit test for these surfaces |
| `coder/acp-go-sdk` example suite | Protocol-behavior reference | `example/claude-code` shows how a shipping bridge sequences tool_call → request_permission → tool_call_update; useful reading, not vendored code |

## Installation

```bash
# New dependencies (all CGO_ENABLED=0-safe, verified)
go get github.com/creack/pty@v1.1.24
go get github.com/landlock-lsm/go-landlock@v0.10.0

# Routine bump
go get github.com/anthropics/anthropic-sdk-go@v1.66.0

# Optional, test-only conformance oracle
go get github.com/coder/acp-go-sdk@v0.13.5
```

No changes to: `go-openai` (current), `modelcontextprotocol/go-sdk`, `cobra`, `toml`, `yaml`.

## Alternatives Considered

| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|-------------------------|
| Extend hand-rolled ACP types | Adopt `coder/acp-go-sdk` as the transport | If a future milestone adds MCP-over-ACP or NES (its unstable surface) and maintaining parallel types exceeds ~1k LOC; also fine to adopt wholesale during SEED-001 kit extraction so the *library* owns the codec while the app keeps its framer |
| Landlock-first Linux sandbox | bubblewrap subprocess wrapper | Only as an opt-in "stronger isolation" mode where available (bubblewrap 0.11+/Ubuntu 24.10+, or after an AppArmor allow-profile install). Never the default — unprivileged bwrap breaks on stock Ubuntu 24.04 (`kernel.apparmor_restrict_unprivileged_userns=1`) |
| Landlock-only confinement | `elastic/go-seccomp-bpf` v1.6.0 syscall filtering | Defer until a concrete need for syscall-level denies beyond Landlock's FS/net scope; it drags `go-ucfg` + `yaml.v2` into the tree for marginal gain. Landlock ABI v6 (`RestrictNet`) covers the network-deny case first |
| `sandbox-exec` generated `.sb` profile (macOS) | App Sandbox / Virtualization.framework | Never for v1 — entitlement-based App Sandbox can't wrap ad-hoc subprocesses; VMs are out of scope for a single-binary CLI tool. Precedent: Codex CLI and Claude Code both ship sandbox-exec wrappers despite Apple's deprecation stamp |
| Shell-out to `git` for checkpoints | `go-git` library | Only if checkpointing ever needs to run where no git binary exists. go-git is slow on large trees, partial on plumbing (commit-tree semantics), and would add a large dep tree for zero user-visible benefit |
| Client-owned terminals via ACP `terminal/*` | Agent-side PTY (creack/pty) | Use ACP `terminal/create`+`terminal/output` when the operator wants commands visible in the editor's terminal panel AND the client advertises the capability; use embedded creack/pty for the headless/background Bash path (background-completion notifications feature) and as fallback for clients without terminal capability. Ship both behind one tool flag — they share the output-streaming seam |
| Monolithic auto-compact (Claude Code style ~92%) | Microcompaction (selective eviction of large tool outputs) | Both, staged: threshold-triggered whole-context summary first (simple, matches CC behavior the parity audit targets), then per-item eviction of oversized tool_results as a fast-follow — it preserves more continuity and is where community consensus says CC itself is heading |

## What NOT to Use

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| `github.com/zed-industries/agent-client-protocol/go` | Does not exist (404 on Go proxy; module contains no Go source) | Hand-rolled layer + coder/acp-go-sdk as reference |
| Wholesale transport swap mid-v1.2 | Re-opens Phase-2 transport bugs (string ids, notification/response interleaving on one stdout) that are already fixed and UAT-proven | Additive handler registration in `internal/acp/handlers.go` |
| bubblewrap as default Linux sandbox | Ubuntu 24.04+ restricts unprivileged user namespaces by default; bwrap fails with "Failed to fully mount capsicum" without host-level AppArmor/sysctl changes — unacceptable for a drop-in binary | go-landlock (in-process, unprivileged, kernel-gated) |
| `kr/pty` | Deprecated upstream | `github.com/creack/pty` |
| Rewriting signed thinking blocks during compaction/projection | Anthropic API rejects modified thinking blocks with 400 `invalid_request_error` when tools + extended thinking are active; signatures must round-trip byte-identical and in order | Either preserve thinking blocks verbatim inside an unresolved tool-use chain, or drop the entire assistant turn (thinking + tool_use together) at compaction boundaries |
| go-git for the checkpoint store | Dep weight + plumbing gaps; existing store works | Continue `os/exec` git plumbing (`add -A → write-tree → commit-tree → update-ref`) |
| Trusting community-reported auto-compact percentages as spec | Measured values scatter 80–95% across Claude Code versions/models; Anthropic publishes no contract | Pick a configurable internal default (~85–90% of window, reserve headroom for the summarization call itself), expose as config; verify-first per SEED-004 note (check whether the recaptured zcode profile already encodes zcode's own auto-compact + cache_control placement before designing ours) |

## Stack Patterns by Variant

**If the client advertises `clientCapabilities.elicitation` (form/url):**
- Gate `elicitation/create` on the exact advertised mode — spec is strict: `{}` advertises zero modes; requesting an unadvertised mode returns JSON-RPC `-32602`
- Because: agents MUST NOT request unadvertised modes; sensitive flows must fail rather than silently downgrade URL→form

**If the client lacks terminal capability:**
- Route the persistent-shell Bash option through embedded creack/pty; otherwise prefer `terminal/create` for editor visibility
- Because: same tool semantics, two transports, chosen per-client at initialize

**If kernel < 5.13 (Linux) or sandbox-exec missing (macOS):**
- Run unsandboxed with a stderr warning (matches the project's investigate-and-fix logging constraint)
- Because: silent degradation is worse than explicit degradation; the safety-model note stays honest

**If a turn hits the compaction threshold:**
- Summarize via the light tier; keep the most recent N messages verbatim; never split a thinking/tool_use chain
- Because: cheapest correct design consistent with CC behavior and the signature constraint

## Version Compatibility

| Package A | Compatible With | Notes |
|-----------|-----------------|-------|
| `landlock-lsm/go-landlock@v0.10.0` | Go ≥ 1.24, kernel ≥ 5.13 (ABI v1+; net rules need ABI v4+/v6 features) | Requires `golang.org/x/sys v0.40.0` — project pins v0.47.0, satisfied; psx v1.2.77 compiles pure-Go under CGO_ENABLED=0 (verified) |
| `creack/pty@v1.1.24` | Go ≥ 1.22-ish; darwin + linux; no transitive deps | Windows ConPTY exists but out of scope (project defers Windows) |
| `anthropic-sdk-go@v1.66.0` | Drop-in over v1.63.0 | Minor-line bump; no breaking param renames encountered in vision/thinking surface |
| `coder/acp-go-sdk@v0.13.5` | go 1.21+, stdlib-only | Tracks ACP spec with stable + unstable schema split; elicitation lives in the unstable half — expect shape churn, another reason to keep it at arm's length |
| go.mod `golang.org/x/sys v0.47.0` | all new deps | No downgrade needed; landlock wants ≤ v0.40.0 semantics, MVS resolves upward fine |

## ACP Surface Verification (feature-by-feature, checked against agentclientprotocol.com schema + coder SDK generated types)

| v1.2 Feature | Wire surface (exact names verified) | Go-side work |
|--------------|--------------------------------------|--------------|
| Clickable permission asks | `session/request_permission` (client method): params `sessionId`, `toolCall` (ToolCallUpdate), `options[] {optionId, name, kind: allow_once\|allow_always\|reject_once\|reject_always}`; outcome `{outcome:"selected", optionId}` \| `{outcome:"cancelled"}`; cancelled is mandatory if the turn cancels mid-request | New outbound request in `internal/acp`; hook into the tool-gate seam; persist allow_always decisions |
| Tool-call streaming | `session/update` variants `tool_call` (`toolCallId`, `title`, `kind`: read/edit/delete/move/search/execute/think/fetch/other; `status`: pending/in_progress/completed/failed; `content`, `locations[]{path,line}`, `rawInput`, `rawOutput`) and `tool_call_update` (all fields optional except `toolCallId`; partial merge) | Emit from `internal/toolexec` lifecycle points through the existing ChunkEmitter |
| Plan streaming | `session/update` variant `plan` (`entries[]{priority: high/medium/low, status: pending/in_progress/completed, content}`) | Map engine/todo state → plan entries |
| Slash-command autocomplete | `session/update` variant `available_commands_update` (`availableCommands[]{name*, description*, input.hint}`); resends allowed any time; invocation is just a prompt turn starting with the command text | Push built-ins + discovered skills/AGENTS after `session/new` and after discovery changes; parse command prefix in prompt intake (rides the command-expansion seam) |
| Structured asks | `elicitation/create` (client method): `sessionId`(+`toolCallId`) or `requestId`, `mode` form\|url (explicit, no MCP-style default), `message`, form: `requestedSchema` flat primitives/enums, url: `elicitationId`+`url`; reply `action: accept\|decline\|cancel` (+`content` on form accept) | Outbound request + clientCapabilities gate |
| Editor-driven configuration | `ClientSessionCapabilities.configOptions` (boolean `{}` and/or value-id support); `configOptions[]` returned on new/load/resume; `session/set_config_option` request `{sessionId, configId, type: "boolean"\|"value_id", value}` → response carries full `configOptions[]`; also `session/update` `configOptionUpdate`; legacy-adjacent `session/set_mode` still exists | Advertise tier/model options; apply to scheduler routing at session scope; API keys never ride this path |
| Session management family | `session/list`, `session/resume`, `session/close` (stable; gated by `sessionCapabilities.{list,resume,close}`), `session/delete` (**still unstable** in coder SDK v0.13.5), full-replay `session/load` via separate `AgentLoader`-style capability | Existing durable transcript makes resume cheap; advertise conservatively (skip delete or send it best-effort) |
| Thinking streaming | `session/update` variant `agent_thought_chunk` | Pipe reasoning deltas from provider stream through ChunkEmitter |

## Sources

- agentclientprotocol.com `/protocol/schema`, `/protocol/v1/elicitation`, `/protocol/v1/tool-calls`, `/protocol/v1/slash-commands` — official method/field verification (HIGH)
- `github.com/zed-industries/agent-client-protocol@v1.7.0` module contents — proves schema-only, no Go SDK (HIGH, direct inspection)
- `github.com/coder/acp-go-sdk@v0.13.5` — downloaded, `types_gen.go` Agent/Client interfaces read; Go-proxy version metadata (HIGH, direct inspection)
- Context7 `/anthropics/anthropic-sdk-go` — `NewImageBlockBase64`, thinking-block signature contract (HIGH)
- go-openai v1.42.0 module source — `MultiContent`/`ChatMessageImageURL` (HIGH, direct inspection)
- Go proxy `@latest` for anthropic-sdk-go, go-openai, creack/pty, go-landlock, go-seccomp-bpf (HIGH)
- Direct `CGO_ENABLED=0` build probe of go-landlock v0.10.0 in this environment (HIGH)
- Codex CLI sandboxing write-ups (Seatbelt `.sb` generation; Landlock+seccomp native path) and Cline/Roo/Gemini CLI/Claude Code checkpoint documentation (MEDIUM — multiple independent corroborating sources)
- Ubuntu 24.04 AppArmor userns restriction affecting bwrap (MEDIUM — Canonical-default behavior, widely reported)
- Claude Code auto-compact threshold reports ~92% (LOW-MEDIUM — community reverse-engineering, version-dependent scatter)

---
*Stack research for: ass-guard-agent v1.2 Claude Code Parity*
*Researched: 2026-08-26*
