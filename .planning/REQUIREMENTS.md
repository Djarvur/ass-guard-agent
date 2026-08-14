# Requirements: ass-guard-agent (working name)

**Defined:** 2026-08-09
**Core Value:** Outgoing requests to the model provider must be structurally indistinguishable from the mimicked agent's (zcode first) — if the model can tell the requests apart, everything built on top is compromised.

**v1 scope discipline:** OpenSpec only (GSD/spec-kit/BMad deferred). ACP primary interface only (Telegram peer → v2). Full Claude-Code ecosystem compat (MCP + plugins + skills + commands). macOS + Linux (Windows deferred). Fresh build (sdd-acp-agent is reference-only). The six deltas are serialized, not thin-sliced.

## v1 Requirements

Requirements for initial release. Each maps to roadmap phases. Categorized by capability area; REQ-ID format `[CAT]-[NN]`.

### Mimicry (north star — Phase 1, gates everything)

- [ ] **MIMC-01**: A Profile Shaper component sits between the Turn Loop and the provider adapter, applying the active profile's {system prompt, tool catalog declaration, message shape, identity fields} to every outgoing model request
- [ ] **MIMC-02**: The zcode profile is extracted from zcode's on-disk JSONL transcripts (located at the claude-code-compat canonical path `~/.claude/projects/<munged-cwd>/<session-id>.jsonl`), not hand-written or guessed
- [ ] **MIMC-03**: A behavioral mimicry A/B parity test validates that a fixed prompt suite sent through ass-guard-with-zcode-profile produces statistically indistinguishable tool-call sequences from live zcode (the "model can't tell them apart" bar — empirical/behavioral, NOT byte-diff)
- [ ] **MIMC-04**: The byte-identical bar is explicitly out of scope — structural indistinguishability to the model is the acceptance criterion (prevents over-investment in cosmetic field-ordering/optional-field parity)

### Profiles (Phase 1)

- [ ] **PROF-01**: A profile is a configurable bundle of {system prompt composition, tool catalog declaration, message block shape, identity/significant headers}, selected by name
- [ ] **PROF-02**: zcode is the first profile; the architecture supports N profiles from day one (no zcode-specific code paths in the Shaper)
- [ ] **PROF-03**: A profile artifact is versioned and carries a `target_capture_ref` (the log/capture it was extracted from) for drift detection
- [ ] **PROF-04**: A profile-drift detector (`ass-guard profile check <name>`) flags when the target agent's observed requests diverge from the captured profile
- [ ] **PROF-05**: A coverage manifest records which request fields/headers the profile covers, surfacing incomplete capture before it causes silent behavioral divergence

### Tool catalog (Phase 1 substrate + Phase 2)

- [ ] **TOOL-01**: A built-in tool catalog matching Claude Code's set (Read, Write, Edit, Glob, Grep, Bash, TodoWrite, Task/Agent, WebSearch, WebFetch, Skill, AskUserQuestion, ScheduleWakeup, NotebookEdit, LSP) is implemented, each tool's name + parameter schema + result shape faithful to the captured reference
- [ ] **TOOL-02**: The profile-declared tool schema is authoritative at runtime — the catalog implements a schema-adapter layer so the model's tool calls resolve against the profile's declared schemas, not the built-in's (prevents catalog drift)
- [ ] **TOOL-03**: A CI catalog-consistency check rejects profiles whose declared tool schemas the built-in catalog cannot satisfy
- [ ] **TOOL-04**: Read-only tools (Glob, Grep, Read) execute concurrently within a turn; mutating tools (Bash, Write, Edit) serialize relative to each other
- [ ] **TOOL-05**: Complex tools (WebSearch, WebFetch) use a configurable backend, not a hardcoded one; the backend is swappable without code changes

### Providers (Phase 1 substrate + Phase 3)

- [x] **PROV-01**: Both Anthropic-shape and OpenAI-shape protocols are supported, with any compatible provider via configurable base URL
- [ ] **PROV-02**: Both adapter shapes implement a common `Provider` interface with `TranslateToInternal`/`TranslateFromInternal` tool-call translation, verified by round-trip conformance tests
- [ ] **PROV-03**: The Anthropic-shape adapter uses `anthropics/anthropic-sdk-go` with swappable base URL (Z.ai for GLM); the OpenAI-shape adapter uses `sashabaranov/go-openai`

### Multi-provider config & credentials (Phase 7)

- [x] **PCFG-01**: The scheduling.yaml `providers` block gains `api_key`/`api_key_env` credential fields (literal or `${VAR}`); the loader parses them and validation warns (not rejects) on a provider missing both, preserving Phase-3 D-10 shape/reference rejection
- [x] **PCFG-02**: Credential precedence at provider construction: `--api-key` flag > provider env var > config literal, with `${VAR}` expansion (minimal surface); secrets never reach logs; an uncredentialed provider fails lazily with a typed structural error (classified like 401/403, never retried)
- [x] **PCFG-03**: A provider factory built once at startup constructs the correct credentialed Anthropic/OpenAI adapter per declared provider (right base_url + right key + right shape), replacing the single hardcoded provider construction sites
- [x] **PCFG-04**: Zero-config backward-compat: the embedded default seeds `api_key_env: ZAI_API_KEY` with no literal, so an operator who changes nothing keeps today's `$ZAI_API_KEY`-only flow

### Model scheduling (Phase 3)

- [ ] **SCHED-01**: Model tiers (heavy/good/light ≈ opus/sonnet/haiku) abstract the concrete model; a command/skill/subagent selects a tier, not a specific model
- [ ] **SCHED-02**: Tier→model mapping is time-scheduled (e.g. heavy = glm-5.2 normally, minimax-m3 in peak hours); time-windows are timezone-explicit (IANA zones, `time/tzdata` bundled)
- [ ] **SCHED-03**: Per-project override of the tier→model table (falls back to config default if unset for the project)
- [ ] **SCHED-04**: Fallback chains on provider error/limit: degrade tier OR walk a configured chain, with transient-vs-structural failure classification (429/5xx/network = transient → retry/fallback; 401/403 = structural → report)
- [ ] **SCHED-05**: Circuit breakers and cost ceilings prevent fallback-chain cascading failures and cost surprises
- [ ] **SCHED-06**: Each (provider, tier) pair has a documented capability profile so tier mismatch across providers is explicit, not silent

### Session & context (Phase 2)

- [ ] **SESS-01**: A two-layer context model: durable replayable transcript (ACP-visible) + lean projected window (model-visible), reset at command boundaries
- [ ] **SESS-02**: Mutating toolkit commands are ALWAYS boundaries (cannot be removed or overridden by config); config may only ADD boundaries
- [ ] **SESS-03**: Effective mutability = `mutating if (adapter-class(command) == mutating OR tool.Mutability == mutating) else read-only` — the more-mutating wins
- [ ] **SESS-04**: A boundary resets the window to a lean seed (system prompt + new command's intent + zero carry-forward)
- [ ] **SESS-05**: Boundary markers are durable in the transcript; on `session/load` rehydration, the projector treats them as authoritative (resets at the most recent pre-rehydration boundary)
- [ ] **SESS-06**: Session Manager is the sole transcript owner; all reads/writes go through its append/read API

### ACP interface (Phase 2 — v1 primary interface)

- [ ] **ACP-01**: ass-guard is an ACP v1 server over stdio JSON-RPC, spawned by the editor (Zed) as a subprocess; stdout carries protocol frames, stderr carries logs
- [ ] **ACP-02**: ACP v1 lifecycle methods are implemented (initialize, session/prompt, session/load) sufficient for Zed to spawn, send a prompt, and receive streamed `session/update` notifications
- [ ] **ACP-03**: On `session/load`, the agent replays the durable transcript as replay-tagged `session/update` notifications
- [ ] **ACP-04**: Streaming is end-to-end: provider SSE → event bus → ACP `session/update`; no full-turn buffering before display
- [ ] **ACP-05**: The JSON-RPC framing choice (hand-rolled vs library) is resolved and documented

### Unified engine (Phase 4 — autocontinue is the upper mechanism)

- [ ] **ENG-01**: After turn-complete, a single engine decides: continue the SDD scenario (dual-signal — text-pattern match OR known handoff tool-call), trigger a hook-DAG, ask the user, or wait
- [ ] **ENG-02**: Hooks are a special case of engine action, not a separate mechanism (one decision point, one provenance-tagged turn stream)
- [ ] **ENG-03**: Unmatched output triggers nothing — the structural safety property; the only off-switch is manual cancellation, which cancels the in-flight turn AND drains queued injections
- [ ] **ENG-04**: If the engine fails (panic/misconfig), the turn still completes normally and the agent degrades to manual-continue (graceful degradation — the engine is an observer, never in a turn's critical path)
- [ ] **ENG-05**: An unmodified OpenSpec workflow runs end-to-end with zero manual "continue" taps, except where OpenSpec explicitly requests user input (the success bar)

### Hook-DAG ("the forgotten routine" — Phase 4)

- [ ] **HOOK-01**: A configurable DAG executor runs arbitrary steps (run-command / send-prompt / fresh-context / wait) in any order — fully customizable per stage
- [ ] **HOOK-02**: A seeded hook set ships out of the box: post-implement (test + lint + review + memory) and post-phase (improvement proposals), for zero-config first run
- [ ] **HOOK-03**: Every hook declares `on-failure: halt | continue | ask` — hook failures never silently block the workflow
- [ ] **HOOK-04**: Provenance tagging (structural from the start) prevents hook-DAG infinite loops (a hook can't trigger the stage that triggered it without explicit opt-in)
- [ ] **HOOK-05**: Each "send-prompt" step in a DAG IS a turn; "fresh-context" steps own their own context windows via the boundary semantics

### Learning mode (Phase 4)

- [ ] **LRN-01**: When the engine doesn't know how to launch what should be launched (no matching pattern/handoff/hook), it asks the user (fresh context? wait for confirmation? how long?) and remembers the answer in a learned-config store
- [ ] **LRN-02**: Learning mode proposes new hooks based on the work log and asks the user whether to add them
- [ ] **LRN-03**: Learned settings carry a confidence threshold (≥3 confirmations to persist), an expiry/review date, and are conflict-checked at accept time
- [ ] **LRN-04**: Learned settings and proposed hooks are versioned and revertible

### Ecosystem compatibility (Phase 5 — full claude-code-compat)

- [ ] **ECOS-01**: Claude Code-installed MCP servers work in ass-guard: hosted as subprocesses via `modelcontextprotocol/go-sdk` `StdioMCPClient`, their tools bridged into the tool catalog
- [ ] **ECOS-02**: MCP server subprocess management is robust: process-group spawn, group-signal shutdown, reaper goroutine (no zombies)
- [ ] **ECOS-03**: MCP `tools/list` is re-fetched on every connection (never cached) to prevent schema drift
- [ ] **ECOS-04**: Claude Code skills, slash-commands, and plugins work unchanged in ass-guard (loaded from `.claude/` layout)
- [ ] **ECOS-05**: ass-guard's own additions namespace under `.claude/` cleanly, never clobbering Claude Code's files (user → project precedence respected)

### Audit log (Phase 1 + Phase 2)

- [ ] **LOG-01**: An audit log records user input, model requests (the verbatim shaped outgoing request — the mimicry evidence source), tool calls, and engine decisions — everything needed to reconstruct the exact sequence of actions
- [ ] **LOG-02**: The audit log is an async consumer of the event bus (never in the critical path)
- [ ] **LOG-03**: Sensitive data (API keys, secrets) is redacted from the log
- [ ] **LOG-04**: Log rotation prevents volume explosion; a reconstruction-sufficiency test verifies the log actually lets you replay what happened

### OpenSpec hosting (Phase 4 — v1 toolkit)

- [ ] **OPEN-01**: A shell toolkit adapter runs OpenSpec commands as subprocesses, captures stdout/stderr, and surfaces stdout into the model's context
- [ ] **OPEN-02**: A `.claude/sdd/openspec.toml` config declares OpenSpec's `[[patterns]]` (text→action) and `[[handoff_tools]]` (tool-call→action), seeded from real OpenSpec handoff examples
- [ ] **OPEN-03**: Each registered OpenSpec command declares its shape (`mutating` | `read-only`) — this classification is the single source of truth for context boundaries

### Parallelism (Phase 2 + Phase 4)

- [ ] **PARA-01**: The `Task`/`Agent` tool dispatches subagents as isolated goroutine turn-loops with scoped context and a restricted tool subset
- [ ] **PARA-02**: Subagent results return via the event bus tagged with parent-turn-id; only results, not accumulated context
- [ ] **PARA-03**: A panicking subagent is recovered → tool-error result to the parent, never a process crash
- [ ] **PARA-04**: A semaphore in the provider layer bounds outbound concurrency across parent + subagents

### Distribution (Phase 6)

- [x] **DIST-01**: A single static Go binary is produced via goreleaser for macOS + Linux (amd64 + arm64), no runtime deps
- [x] **DIST-02**: An ACP registry `agent.json` manifest declares `cmd`/`args` (per the canonical `agent.schema.json`) so Zed can spawn the agent via `zed: acp registry` — the original `command`/`command_args`/`cwd` wording was superseded (Tier-A correction, Phase 6)
- [x] **DIST-03**: First run via `zed: acp registry` works with sensible defaults (a default model + a pre-seeded zcode profile + a pre-seeded openspec.toml) — zero-config for the team

## v2 Requirements

Deferred to a future release. Tracked but not in the current roadmap.

### Telegram peer interface

- **TG-01**: Telegram is a secondary but full peer interface (can drive an entire SDD scenario via text); an Interface Adapter abstraction above Session Manager enables it alongside ACP
- **TG-02**: Voice messages are transcribed to text as ordinary user input via a configurable STT backend (OpenAI Whisper API default); STT sits at the Telegram adapter boundary, the core sees text only
- **TG-03**: Concurrent inputs from ACP and Telegram serialize via a single session mailbox; status queries and cancel are available from either interface
- **TG-04**: STT errors are mitigated via echo-back-before-acting and a domain-term dictionary

### Additional toolkits

- **KIT-01**: GSD toolkit adapter (shell + prompt/skill adapter)
- **KIT-02**: spec-kit toolkit adapter
- **KIT-03**: BMad toolkit adapter (skill/prompt model)

### Additional profiles

- **PROF-06**: claude-code profile (in addition to zcode) — direct Claude Code mimicry
- **PROF-07**: Additional target-agent profiles as the ecosystem grows

### Platform expansion

- **PLAT-01**: Windows support (amd64 + arm64)

## Out of Scope

Explicitly excluded. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Telegram peer interface in v1 | v1 = ACP-only to keep the multi-interface risk out of the first release; Telegram builds on the proven core in v2 |
| Byte-for-byte request identity with the mimicked agent | "Structurally indistinguishable to the model" is the bar; binary diff equality is over-investment (empirical/behavioral parity is the acceptance criterion) |
| GSD / spec-kit / BMad toolkit adapters in v1 | OpenSpec only; the adapter interface accommodates them, implementation deferred to v2 |
| Windows platform support | v1 macOS+Linux only; Windows deferred (win-developer teams are not the first target) |
| Perplexity or paid search backends | Configurable backend design replaces the hardcode, but no specific paid integrations are committed |
| A standalone CLI surface | ACP (IDE) is the only v1 interface; no terminal REPL to maintain |
| A tool-execution confirmation/permission tier | Tools run ungated; the pattern/hook table + manual cancellation is the safety mechanism (inherited) |
| Porting code from `sdd-acp-agent` | Fresh build; predecessor is reference-only |
| Thin-slicing all six deltas in Phase 1 | Risk-multiplication, not risk-reduction — when a thin-slice breaks, you can't tell which delta is at fault (serialize instead) |

## Traceability

Which phases cover which requirements. Updated during roadmap creation (2026-08-09). See ROADMAP.md for phase goals and success criteria.

| Requirement | Phase | Status |
|-------------|-------|--------|
| MIMC-01 | 1 — Mimicry MVP | Pending |
| MIMC-02 | 1 — Mimicry MVP | Pending |
| MIMC-03 | 1 — Mimicry MVP | Pending |
| MIMC-04 | 1 — Mimicry MVP | Pending |
| PROF-01 | 1 — Mimicry MVP | Pending |
| PROF-02 | 1 — Mimicry MVP | Pending |
| PROF-03 | 1 — Mimicry MVP | Pending |
| PROF-04 | 1 — Mimicry MVP | Pending |
| PROF-05 | 1 — Mimicry MVP | Pending |
| TOOL-01 | 1 — Mimicry MVP | Pending |
| TOOL-02 | 1 — Mimicry MVP | Pending |
| TOOL-03 | 1 — Mimicry MVP | Pending |
| PROV-01 | 1 — Mimicry MVP | Complete |
| PROV-02 | 1 — Mimicry MVP | Pending |
| PROV-03 | 1 — Mimicry MVP | Pending |
| PCFG-01 | 7 — Multi-Provider Config & Credentials | Complete |
| PCFG-02 | 7 — Multi-Provider Config & Credentials | Complete |
| PCFG-03 | 7 — Multi-Provider Config & Credentials | Complete |
| PCFG-04 | 7 — Multi-Provider Config & Credentials | Complete |
| LOG-01 | 1 — Mimicry MVP | Pending |
| SESS-01 | 2 — Session Core + ACP | Pending |
| SESS-02 | 2 — Session Core + ACP | Pending |
| SESS-03 | 2 — Session Core + ACP | Pending |
| SESS-04 | 2 — Session Core + ACP | Pending |
| SESS-05 | 2 — Session Core + ACP | Pending |
| SESS-06 | 2 — Session Core + ACP | Pending |
| ACP-01 | 2 — Session Core + ACP | Pending |
| ACP-02 | 2 — Session Core + ACP | Pending |
| ACP-03 | 2 — Session Core + ACP | Pending |
| ACP-04 | 2 — Session Core + ACP | Pending |
| ACP-05 | 2 — Session Core + ACP | Pending |
| LOG-02 | 2 — Session Core + ACP | Pending |
| LOG-03 | 2 — Session Core + ACP | Pending |
| LOG-04 | 2 — Session Core + ACP | Pending |
| PARA-01 | 2 — Session Core + ACP | Pending |
| PARA-02 | 2 — Session Core + ACP | Pending |
| PARA-03 | 2 — Session Core + ACP | Pending |
| PARA-04 | 2 — Session Core + ACP | Pending |
| SCHED-01 | 3 — Model Scheduling | Pending |
| SCHED-02 | 3 — Model Scheduling | Pending |
| SCHED-03 | 3 — Model Scheduling | Pending |
| SCHED-04 | 3 — Model Scheduling | Pending |
| SCHED-05 | 3 — Model Scheduling | Pending |
| SCHED-06 | 3 — Model Scheduling | Pending |
| ENG-01 | 4 — Unified Engine + Hooks + OpenSpec + Learning | Pending |
| ENG-02 | 4 — Unified Engine + Hooks + OpenSpec + Learning | Pending |
| ENG-03 | 4 — Unified Engine + Hooks + OpenSpec + Learning | Pending |
| ENG-04 | 4 — Unified Engine + Hooks + OpenSpec + Learning | Pending |
| ENG-05 | 4 — Unified Engine + Hooks + OpenSpec + Learning | Pending |
| HOOK-01 | 4 — Unified Engine + Hooks + OpenSpec + Learning | Pending |
| HOOK-02 | 4 — Unified Engine + Hooks + OpenSpec + Learning | Pending |
| HOOK-03 | 4 — Unified Engine + Hooks + OpenSpec + Learning | Pending |
| HOOK-04 | 4 — Unified Engine + Hooks + OpenSpec + Learning | Pending |
| HOOK-05 | 4 — Unified Engine + Hooks + OpenSpec + Learning | Pending |
| LRN-01 | 4 — Unified Engine + Hooks + OpenSpec + Learning | Pending |
| LRN-02 | 4 — Unified Engine + Hooks + OpenSpec + Learning | Pending |
| LRN-03 | 4 — Unified Engine + Hooks + OpenSpec + Learning | Pending |
| LRN-04 | 4 — Unified Engine + Hooks + OpenSpec + Learning | Pending |
| OPEN-01 | 4 — Unified Engine + Hooks + OpenSpec + Learning | Pending |
| OPEN-02 | 4 — Unified Engine + Hooks + OpenSpec + Learning | Pending |
| OPEN-03 | 4 — Unified Engine + Hooks + OpenSpec + Learning | Pending |
| TOOL-04 | 4 — Unified Engine + Hooks + OpenSpec + Learning | Pending |
| TOOL-05 | 4 — Unified Engine + Hooks + OpenSpec + Learning | Pending |
| ECOS-01 | 5 — Ecosystem Compatibility | Pending |
| ECOS-02 | 5 — Ecosystem Compatibility | Pending |
| ECOS-03 | 5 — Ecosystem Compatibility | Pending |
| ECOS-04 | 5 — Ecosystem Compatibility | Pending |
| ECOS-05 | 5 — Ecosystem Compatibility | Pending |
| DIST-01 | 6 — Distribution + Polish | Complete |
| DIST-02 | 6 — Distribution + Polish | Complete |
| DIST-03 | 6 — Distribution + Polish | Complete |

**Coverage:**
- v1 requirements: 67 total
- Mapped to phases: 67 (100%)
- Unmapped: 0 ✓
- Phase distribution: Phase 0 = 0 (spike); Phase 1 = 16; Phase 2 = 18; Phase 3 = 6; Phase 4 = 19; Phase 5 = 5; Phase 6 = 3

---
*Requirements defined: 2026-08-09*
*Last updated: 2026-08-09 after initial definition*
