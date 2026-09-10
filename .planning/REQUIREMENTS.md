# Requirements: ass-guard-agent — Milestone v1.2 Claude Code Parity

**Defined:** 2026-08-26
**Core Value:** A hands-off coding agent that feels native in the editor: full SDD workflows run end-to-end without manual continues, and the agent surfaces through the client's own UX — clickable permission prompts, native file diffs, session management. (Pivoted 2026-08-25 from mimicry bar.)

## v1.2 Requirements

Requirements for this milestone. Each maps to roadmap phases.

### Runtime Carve

- [x] **RUNT-01**: `sessionTurnRunner` carved verbatim from `cmd/ass-guard/acp_serve.go` into `internal/runtime`; cmd composes it; zero behavior change (`mise ci` proves equivalence)

### ACP Completeness

- [x] **ACP-01**: User sees clickable permission asks via `session/request_permission` (allow/reject × once/always options; cancelled handled as a normal response when turn dies mid-ask); new `permissions.mode: ungated|gated` config switchable via editor configOptions — available, not default (safety-model amendment)
- [x] **ACP-02**: Learning-store and engine asks surface as structured forms via `elicitation/create` (form mode) with plain-text AskBroker fallback and -32601 probe-and-degrade on older clients; url mode deferred
- [x] **ACP-03**: Zed renders live turn activity: `tool_call`/`tool_call_update` streaming (kind/status/diff/locations), `plan` updates mirroring TodoWrite, `agent_thought_chunk` — all through one ordered inline TurnEmitter with explicit backpressure policy
- [x] **ACP-04**: Editor autocompletes `/` commands: `available_commands_update` sent on session start and on discovery change
- [x] **ACP-05**: User can list sessions from the editor via `session/list` (header-scan, cursor pagination)
- [x] **ACP-06**: User can resume any past session via `session/load` — full replay through TurnEmitter plus live-state reconciliation (synthetic interrupted-closures for dangling expectations, continued id sequences from transcript maxima, orphaned in-flight tool_calls closed as failed, commands re-advertised); `--resume` anywhere
- [x] **ACP-07**: User can close or delete a session via `session/close` / `session/delete` with tombstoning (never rm — D-20 audit invariant); delete is spec-unstable → best-effort
- [x] **ACP-08**: Editor drives configuration: `configOptions[]` advertised at initialize/new/load/resume responses, Zed settings payload read at initialize, `session/set_config_option` handled (tier/model defaults switchable from editor UI); API keys stay env/file, never editor settings

### Built-in Chat Commands

- [x] **CMDS-01**: Slash-invocation resolves via one chain: builtins → skills → agents → file-discovered commands (current behavior last)
- [x] **CMDS-02**: Class-B control-plane commands execute at the runner seam with NO model turn and write `local_command` transcript lines: /help /status /cost /mcp /memory /permissions /doctor /config /model /clear /resume /compact (/compact requires PAR-01)
- [x] **CMDS-03**: Class-A prompt-expanding commands (/init) ride the existing `expandUserBlocks` seam and are recorded as user turns with provenance
- [x] **CMDS-04**: Live rescan — newly created/installed commands, skills, agents (and removals) are discovered without restart: filesystem watch or invoke-time rescan re-runs discovery and available_commands_update re-fires so the editor autocomplete reflects changes immediately

### Slash-invocable Skills

- [x] **SKLS-01**: Typing `/<skill-name>` resolves skill keys in invocationFor; SKILL.md body expands as the prompt with args appended
- [x] **SKLS-02**: Discovered AGENTS are addressable as slash commands (BMad-style installer layout: `.claude/agents/*.md`)
- [x] **SKLS-03**: Per-agent `model:` frontmatter wired into subagent dispatch — precedence frontmatter > dispatch > session default > tier — with resolvedModel reported back

### CC Parity Closures

- [x] **PAR-01**: Compaction on context overflow risk: threshold-triggered (~80% default, configurable) light-tier summarizer appending an additive typed `compaction` transcript line; Projector treats it as a hard reset-point class (summary = durable seed, immune to mutating-boundary resets); tool_use/result pairs atomic; retry-once recovery on provider overflow error; thinking blocks never rewritten mid-chain
- [x] **PAR-02**: `cache_control {"type":"ephemeral"}` emitted on every system block via the Shaper (parity-faithful lever; zcode corpus has no auto-compact — verified in docs/compaction-decision.md)
- [x] **PAR-03**: Hooks: settings.json parsing (project + user scopes), PreToolUse deny interceptor at the executor chokepoint — bounded sync execution, hard timeout, fail-open, structured verdicts, deny-only authority from project scope (no allow from repo-shipped files); joins the ONE gate pipeline locked in Phase 17 (hook verdict → permission ask → execute)
- [x] **PAR-04**: AGENTS.md/CLAUDE.md auto-injected into system context every session via dynamic merge into the per-session profile copy (trailing System TextBlocks), mtime-cached
- [x] **PAR-05**: Thinking blocks streamed to client: provider `"thinking"` chunks → raw `json.RawMessage` passthrough end-to-end (transcript, redactor excluded by construction, projector) — Anthropic signatures round-trip byte-identical
- [x] **PAR-06**: Rich prompt content: image blocks (base64) and @-file mentions expand with Read-tool rule gating and provenance; ingress capability validation per provider shape
- [ ] **PAR-07**: Full subagents: background dispatch via discriminated results (completed/async_launched), structured task-notifications detected by kind (not text-match), output-file retrieval for running tasks, cancellation
- [ ] **PAR-08**: Background Bash: completion notifications ride the same task-notification subsystem as PAR-07; process-group lifecycle (TERM-before-KILL escalation, Pdeathsig on Linux, startup stale-log sweep)
- [x] **PAR-09**: Persistent-shell Bash option via creack/pty v1.1.24 (Setsid + group-kill escalation, EIO-as-EOF, ANSI stripping, best-effort cd/export tracking)

### Sandbox Reality

- [x] **SAND-01**: Sandbox flag made real: landlock-lsm/go-landlock v0.10.0 (Linux, kernel ≥5.13) / sandbox-exec generated `.sb` profiles embedded (macOS, targeted denies never deny-default); probe-and-degrade loudly; default OFF; `--sandbox=off` escape hatch; bubblewrap demoted to opt-in stronger-isolation mode

### SEED Gap Close-out

- [x] **SEEDG-01**: Steering queue during a running turn: drained at model-request boundaries only (never mid-in-flight-request, never splitting tool_use/result pairs), ticket/cutoff cancel protocol, parked-ask disambiguation, transport-neutral API (Telegram prerequisite — do not descope to queue-behind silently)
- [ ] **SEEDG-02**: Checkpoint restore guard: refuse restore with active turn/engine chains; pre-restore snapshot before overwriting; nested-repo refusal (gitlink contents silently unprotected otherwise); checkpoint object-expiry GC; `.ass-guard/` in `.git/info/exclude`
- [ ] **SEEDG-03**: `/undo` command (class-B) restoring last checkpoint

### Kit Extraction (strictly last)

- [ ] **KIT-01**: Core agent machinery extracted behind a composition-root API in `pkg/`: profile/shaper/provider/modelrouting/toolcat/toolexec/engine/hookdag/event/session/redact/checkpoint/audit/mcp
- [ ] **KIT-02**: Frontend seam expressed as kit interfaces — emitter + requester defined session-side, implemented acp-side (the one genuinely new design act)
- [ ] **KIT-03**: ass-guard becomes the kit's reference app — retains ecosys/openspec/coreexec/learning/firstrun/evalsuite/parity + ACP frontend; SEED-002 fantasy + SEED-003 landscape as design prior art (reading material, zero runtime deps)

### Documentation & Tails

- [ ] **DOC-01**: LSP support documented as an IDE-side MCP configuration requirement (operator decision 2026-08-25 — no agent-side implementation)
- [ ] **TAIL-01**: Scheduler outcome store + feedback loop (deterministic, zero LLM calls)
- [ ] **TAIL-02**: Nightly upstream-parity gate CI automation (12-08 tail)
- [ ] **TAIL-03**: ECOS-04 — plugins/skills not just loaded but working unchanged in every interaction mode

## Future (v1.3+)

### Telegram Peer (lowest priority — operator 2026-08-26)

- **TG-01**: Telegram is a full peer interface driving entire SDD scenarios; voice messages transcribed to text via configurable STT backend
- **TG-02**: Telegram frontend goroutine shares the core engine with ACP stdio; context-first shutdown drains long-poll loop and in-flight turns; consumes the steering core (SEEDG-01)
- Deferred from v1.1 requirements: TG-01..06 original set

### Other deferred

- **DSH-01..05**: deepseek-harness profile #2 — dropped entirely by operator decision 2026-08-26 (mimicry bar abandoned 2026-08-25)
- Elicitation url mode; interactive permission-rule editing; remote-launched cloud agents; theme/vim-mode config keys

## Out of Scope

| Feature | Reason |
|---------|--------|
| dsh profile #2 | Dropped entirely (operator, 2026-08-26) |
| Telegram peer in v1.2 | Lowest priority (operator, 2026-08-26); slips to v1.3 pool |
| Agent-side TUI pickers inside slash-commands | Anti-feature — clients own list UX (Zed session list, pickers); rebuild nothing agent-side |
| Bubblewrap as Linux sandbox default | Ubuntu 24.04+ AppArmor userns clamp breaks unprivileged bwrap; landlock chosen |
| API keys via editor configOptions | Explicit project decision — credentials env/file only |
| Byte-for-byte request identity | "Structurally indistinguishable" was the bar pre-pivot; mimicry assets retained as reference only |
| Windows platform support | v1 macOS+Linux only (unchanged) |
| Porting code from sdd-acp-agent | Fresh build; predecessor reference-only (unchanged) |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| RUNT-01 | Phase 15 | Complete |
| ACP-03 | Phase 16 | Complete |
| ACP-08 | Phase 16 | Complete |
| ACP-01 | Phase 17 | Complete |
| ACP-02 | Phase 17 | Complete |
| ACP-05 | Phase 18 | Complete |
| ACP-06 | Phase 18 | Complete |
| ACP-07 | Phase 18 | Complete |
| PAR-01 | Phase 19 | Complete |
| PAR-02 | Phase 19 | Complete |
| ACP-04 | Phase 20 | Complete |
| CMDS-01 | Phase 20 | Complete |
| CMDS-02 | Phase 20 | Complete |
| CMDS-03 | Phase 20 | Complete |
| CMDS-04 | Phase 20 | Complete |
| SKLS-01 | Phase 20 | Complete |
| SKLS-02 | Phase 20 | Complete |
| SKLS-03 | Phase 20 | Complete |
| PAR-03 | Phase 21 | Complete |
| PAR-04 | Phase 21 | Complete |
| PAR-05 | Phase 21 | Complete |
| PAR-06 | Phase 21 | Complete |
| PAR-07 | Phase 22 | Pending |
| PAR-08 | Phase 22 | Pending |
| PAR-09 | Phase 22 | Complete |
| SAND-01 | Phase 22 | Complete |
| SEEDG-01 | Phase 23 | Complete |
| SEEDG-02 | Phase 23 | Pending |
| SEEDG-03 | Phase 23 | Pending |
| DOC-01 | Phase 24 | Pending |
| TAIL-01 | Phase 24 | Pending |
| TAIL-02 | Phase 24 | Pending |
| TAIL-03 | Phase 24 | Pending |
| KIT-01 | Phase 25 | Pending |
| KIT-02 | Phase 25 | Pending |
| KIT-03 | Phase 25 | Pending |

**Coverage:**

- v1.2 requirements: 36 total
- Mapped to phases: 36
- Unmapped: 0 ✓ (Phases 15–25; numbering continues from v1.1's Phase 14)

---
*Requirements defined: 2026-08-26*
*Last updated: 2026-08-26 — traceability mapped to roadmap Phases 15–25 (35/35)*
