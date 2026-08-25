# Project Research Summary

**Project:** ass-guard-agent — milestone v1.2 "Claude Code Parity"
**Domain:** Go AI coding agent, ACP/Zed-native — additive feature waves onto the shipped v1.1 architecture
**Researched:** 2026-08-26
**Confidence:** HIGH overall (stack/architecture repo-grounded and official-schema-verified; specific LOW areas flagged below)

## Executive Summary

v1.2 converts a proven hands-off Go agent into an editor-native peer of Claude Code inside Zed, without rewriting anything that works. Six feature clusters attach to four existing layers: ACP completeness (request_permission, elicitation/create, tool_call/plan/thought streaming, available_commands_update, the session list/load/resume/close/delete family, editor-driven configOptions), built-in chat commands (/model /config /compact /clear /cost /mcp …), slash-invocable skills + per-agent model dispatch, ten CC-parity closures (compaction, full subagents + background Bash, hooks PreToolUse deny, AGENTS.md injection, streamed thinking, rich prompt content), SEED-004 gap fixes (shadow-git checkpoint hardening, real sandbox, steering queue), and finally SEED-001 kit extraction. Every serious ACP agent ships the first cluster — plain-text gates and a black-box turn feel broken next to Gemini CLI's post-fix native UX, and the RPC surfaces (not formatted text) are what buy the editor's native dialogs, dropdowns, diff cards, and autocomplete.

The recommended approach is strictly additive integration onto five proven in-repo patterns. (1) **Outbound id'd JSON-RPC requests are a brand-new server capability** — today `internal/acp` only writes notifications and responses; request_permission and elicitation/create need an id'd request writer plus a pending-response registry beside the writer mutex, and the framer itself needs zero changes. (2) Client-visible frames move to a **single ordered inline TurnEmitter driven by the turn goroutine** — bus fan-out across per-kind channels gives no cross-kind ordering guarantee and will misrender Zed's panel once tool_call/update/plan/thought coexist with chunks. (3) Permissions and elicitation **reuse the AskBroker suspension pattern** (`internal/session/ask.go`): park the turn, release every lock, resolve on the client's response — never block inline while holding the turn mutex or mutating slot. (4) **Compaction is purely additive**: a new typed transcript line (`AppendCompaction`) interpreted by the Projector as a second reset-point class; the transcript stays append-only (D-20 intact), and the decisive research fact is that the zcode corpus contains **no auto-compact at all** — `cache_control {"type":"ephemeral"}` on system blocks is the actual parity-faithful lever and ships alongside. (5) The sandbox flag becomes real via a **landlock (Linux) / sandbox-exec (macOS) split** — both unprivileged and CGO_ENABLED=0-clean — with loud probe-and-degrade everywhere else. Two structural moves bookend the milestone: a **mechanical step-0 carve of `sessionTurnRunner` out of the 2,116-line `acp_serve.go` into `internal/runtime`** (seven of ten clusters modify exactly that code; carving first means features land in their final home instead of migrating twice), and **kit extraction strictly last**, after all interfaces stabilize.

The dominant risks are all integration-shaped against the working v1.1 system. Human-timescale waits must never hold locks (the #1 pitfall — an unanswered dialog would otherwise park every queued prompt overnight); concurrent emitters (subagents, engine firings, background completions) must route through one sequencer with explicit backpressure policy or Zed drops/misorders frames; resume is far more than transcript replay (task registries, parked asks, pending permissions, id sequences all evaporate with the process — every dangling expectation needs synthetic closure); thinking blocks must round-trip **byte-identical including the cryptographic signature** through redactor, projector, and compactor alike, or Anthropic returns 400; and compaction must respect the delicate two-layer boundary/projection machinery rather than manipulate flat message arrays. Mitigation is uniform: broker-based suspension, one sequencer, append-only transcripts, RawMessage passthrough, real-Zed UAT for every protocol surface, and the standing `mise ci` (-race) gate.

## Key Findings

### Recommended Stack

The v1.0/v1.1 stack (Go 1.26, hand-rolled ACP over stdio, anthropic-sdk-go + go-openai, MCP hosting, cobra) is validated and unchanged in kind. Only deltas were researched.

**Core technologies:**
- **Hand-rolled ACP layer (`internal/acp`) — extended, never replaced.** Already UAT-proven live against Zed including the string-UUID id quirk; new methods are plain request/response pairs needing only typed params + handlers. Mid-milestone transport swaps re-open fixed bugs.
- **No official Zed Go SDK exists.** `zed-industries/agent-client-protocol@v1.7.0` publishes Rust+TS schema only (zero `.go` files, verified). Search-engine claims of `NewAgentSideConnection` are false.
- **`coder/acp-go-sdk@v0.13.5`** — schema reference / optional test-only fuzzing oracle, NOT the transport. Its release is literally titled "Session config options take over model selection"; elicitation lives in its unstable half (expect churn).
- **`creack/pty@v1.1.24`** (new) — persistent-shell Bash tool; de-facto standard, zero transitive deps, darwin+linux. Use `pty.StartWithSize`, not deprecated `pty.Start`.
- **`landlock-lsm/go-landlock@v0.10.0`** (new, Linux-only) — Landlock LSM confinement; directly verified to build clean under `CGO_ENABLED=0`; kernel ≥ 5.13, unprivileged, immune to Ubuntu 24.04's AppArmor userns clamp that breaks bubblewrap. Bubblewrap demoted to opt-in "stronger isolation" mode only.
- **macOS sandbox: `sandbox-exec` with generated `.sb` profiles** (go:embed data, Codex CLI/Claude Code precedent despite Apple's deprecation stamp). Targeted denies, never `(deny default)`.
- **`anthropic-sdk-go` bump v1.63.0 → v1.66.0** — image blocks (`NewImageBlockBase64`) + thinking-replay contract. go-openai v1.42.0 already current (`MultiContent` image parts).
- Continue shelling out to the `git` binary for checkpoints — go-git rejected (dep weight, plumbing gaps).

### Expected Features

**Must have (table stakes — the "feels native in the editor" bar):**
- `session/request_permission` clickable asks + session-modes advertisement (Zed renders the dropdown free) — replaces plain-text gating; cancelled arrives as a NORMAL response, mandatory on turn cancel
- `tool_call`/`tool_call_update` streaming (kind/status/diff/locations drive Zed's cards) + `plan` updates (TodoWrite mirror) + `agent_thought_chunk`
- `available_commands_update` — highest-leverage LOW-cost item; powers `/` autocomplete for everything else in the milestone
- Session list/load/resume/close/delete — operator must-have (D-09 reversal); delete is still unstable upstream, ship best-effort/tombstone
- Editor-driven configuration (`configOptions` / `set_config_option`; response returns the authoritative full array). API keys never ride this path (explicit project decision)
- Cheap class-B built-ins: /help /status /cost /mcp /memory /permissions (dump) — control-plane, no model turn
- Slash-invocable skills + AGENTS-addressable-as-commands + per-agent `model:` wired into dispatch (frontmatter parsed-but-ignored today)

**Should have (parity differentiators):**
- Compaction + cache_control emission (see below); /compact [instructions]
- elicitation/create form mode (learning-store asks become clickable forms — unique among ACP agents); url mode deferred
- Full subagents (background dispatch, structured task_notification events by kind-not-text, output-file retrieval, resolvedModel reporting) + background Bash completion callbacks on ONE shared task-notification subsystem
- Hooks PreToolUse deny (sync pre-execution interceptor, deny-only authority from project scope); AGENTS.md/CLAUDE.md auto-injection (Pattern-3 profile-copy merge); rich content (@-mentions/images with ingress capability validation); persistent shell; real sandbox; steering queue; checkpoint restore guard + /undo; /init /doctor /config /resume /model /clear

**Defer (v1.3+):**
- Elicitation url mode; interactive permission-rule editing; remote-launched (cloud) agents; Telegram peer consuming the steering core; TUI-style pickers inside commands (anti-feature — clients own list UX); theme/vim-mode config keys

**Compaction specifics (the most misunderstood item):** the SEED-004 verify-first question is answered — `docs/compaction-decision.md` proves the zcode corpus manages context via the rolling 64-tail (already replicated) plus `cache_control`, with **no auto-compact**. Therefore: ship cache_control on every system block alongside a conservative threshold-triggered summarizer (~80% default, configurable), implemented additively via a typed transcript marker; ignore community-reported ~92% figures as spec.

### Architecture Approach

All ten clusters attach to four layers of the shipped v1.1 architecture; no component is rewritten. The largest concentration of change is `cmd/ass-guard/acp_serve.go`'s `sessionTurnRunner` (2,116 lines) — itself the argument for the step-0 `internal/runtime` carve. Five patterns carry the milestone: (1) broker/suspension (AskBroker twin: PermissionGate keyed by toolCallID, ctx-cancel ⇒ reject_once semantics); (2) ordered inline TurnEmitter replacing bus fan-out for client frames (bus remains the async/audit spine); (3) dynamic merge into the per-session profile copy as trailing System TextBlocks (the injection vehicle for AGENTS.md/memory/skill listings); (4) optional-capability interface type-assertion at the server boundary (`SessionLister`/`Loader`/`Eraser`/`ConfigOptionsProvider`, mirroring `SessionCloser`); (5) executor-only override on the captured catalog. New seams follow the file-per-seam convention inside `internal/session` (`permission.go`, `steering.go`, `compaction.go`, `bgsubagent.go`) with `Set*` accessor wiring; `internal/session` must never import `internal/acp`.

**Major components:**
1. **`internal/acp` Server** — MODIFIED: new handlers (session family, set_config_option, elicitation responses), richer initialize capabilities (`loadSession:true`, `sessionCapabilities`, `configOptions`), generalized TurnEmitter, and the NEW outbound-request writer path
2. **Runner layer (`internal/runtime`, carved)** — invocation resolver chain (builtins → skills → agents → commands), session-family ops, editor-config override plumbing, permission/elicitation brokering
3. **Session core** — MODIFIED: PermissionGate seam beside the plan-mode gate, SteerQueue drain in `runTurn`, Compactor marker handling, Projector gains the compaction reset-point class
4. **Observer/ecosystem/provider** — engine stays post-turn; ecosys parses settings.json hooks + memory files; shaper gains cache_control/images/thinking; provider streams `"thinking"` chunks
5. **Stores** — `permissions.yaml` joins learned.yaml/schedule store; shadow.git checkpoints exist (EARLY-01) and need only safety edges

### Critical Pitfalls

1. **Permission ask blocks while holding locks (Critical, #1)** — an inline await parks the turn mutex for human timescales with no protocol timeout. Avoid: AskBroker-style suspend-and-release; outcome resumes the SAME turn; acquire the mutating slot only at actual execution; cancelled handled as normal response; explicit watchdog default (none, matching ACP/Zed).
2. **Out-of-order frames from concurrent emitters (Critical)** — per-kind bus channels give zero cross-kind ordering; `tool_call_update` can overtake its creator. Avoid: one per-session sequencer owning all client frames from day one of streaming; N-emitter `-race` stress test; explicit interleaving policy (foreground priority).
3. **Lossy resume (Critical)** — replay-only `session/load` strands ghost expectations: unknown task ids, parked asks, dispatched-but-unfinished subagents, restarted id counters colliding with checkpoint refs. Avoid: enumerate live-state inventory; synthetic "interrupted" closure for every dangling record; continue ids from transcript maxima; kill -9 mid-turn E2E test.
4. **Thinking-signature corruption (Critical, schema-level)** — Anthropic rejects edited/reordered/redacted thinking blocks with 400 when tools + extended thinking are active. Avoid: `json.RawMessage` passthrough end-to-end, redactor excluded by construction, never drop the latest tool_use exchange's thinking under compaction, shape-branched shaper (OpenAI-shape providers have no signatures).
5. **Compaction vs the two-layer context (Critical)** — manipulating messages directly breaks boundary resets, pair pairing (orphaned results silently dropped), and the append-only forensic contract. Avoid: append-only marker lines + Projector seed policy; tool_use/result pairs atomic; summary survives mutating-boundary reset (pinned projector test); oversized single result pre-capped or compaction loses the race against "prompt is too long."

Also load-bearing: open tool_call frames poisoning UI + resume (deferred terminal update via wrapper guarantee); PTY master-close/EIO/ANSI traps; background-process orphans on SIGKILL (editor owns lifecycle); sandbox breaking legitimate tools (degrade-loudly, targeted denies, escape hatch); nested repos silently unprotected by shadow-git; steering semantics chosen wrong (boundary-drain + ticket/cutoff cancel protocol); hooks gaining allow-authority from repo-shipped files (deny-only from project scope).

## Implications for Roadmap

Based on combined research, suggested phase structure (roadmapper may renumber; every suggestion names its dependency so mapping survives):

### Phase 1: `internal/runtime` Carve (Step 0)
**Rationale:** Seven of ten clusters modify `sessionTurnRunner`; deferring the carve means every feature lands twice (Anti-Pattern 6). Purely mechanical — verbatim move, zero behavior change, `mise ci` proves equivalence.
**Delivers:** `internal/runtime` package housing sessionTurnRunner + engine/MCP adapters + cron wiring; `cmd/ass-guard` composes it.
**Avoids:** Pitfall-class "archaeology tax" at kit extraction; double migration of every Wave-1+ feature.

### Phase 2: ACP Wire Foundation
**Rationale:** Everything interactive depends on three primitives that don't exist yet: outbound id'd requests, multi-kind ordered emission, and richer transcript line types.
**Delivers:** Pending-response registry + id'd request writer; TurnEmitter (chunk/thought/tool_call/update/plan); backpressure + overflow policy per frame class; initialize capability overhaul; transcript line-type extensions (raw thinking passthrough as `json.RawMessage`, `local_command`, compaction-marker type) — the schema changes Phases 4–5 depend on.
**Uses:** Framer unchanged (RawMessage id echo already correct); Pattern-2 sequencer.
**Avoids:** Pitfalls 2, 3 (ordering, backpressure) and pre-empts Pitfall 8's storage half.

### Phase 3: Permissions + Elicitation
**Rationale:** The safety-model amendment ("tier available but not default": `permissions.mode: ungated|gated`, switchable via configOptions) lands here; request_permission is the single highest-risk item and sets the gate pipeline that Phase 7's hooks join.
**Delivers:** PermissionGate (AskBroker twin) in the pre-batch classification loop; `request_permission` end-to-end with allow_once/allow_always/reject semantics; `permissions.yaml` store; session-modes advertisement; `elicitation/create` form mode with AskBroker text fallback and -32601 probe-and-degrade.
**Decision locked here:** ONE gate pipeline with documented precedence (hook verdict → permission ask → execute) so Phase 7 doesn't bolt on an incompatible second gate. Hooks: deny-only from project scope.
**Avoids:** Pitfalls 1, 15, 16 (lock-holding, version skew, dual pipelines).

### Phase 4: Session Family (list/load/resume/close/delete)
**Rationale:** Depends on Phase 2 emitters for replay frames; operator must-have (D-09 reversal); resume reconciliation is the phase's hard part — budget accordingly.
**Delivers:** Header-scan session/list with cursor pagination; load with full replay via TurnEmitter + live-state reconciliation (synthetic interrupted-closures, continued id sequences, commands re-advertised, orphaned in-flight tool_calls closed as failed); alias-map identity; close/delete with tombstoning (never rm — D-20).
**Avoids:** Pitfalls 4, 5 (open frames, ghost state).

### Phase 5: Compaction + cache_control
**Rationale:** Pulled ahead of its nominal parity slot because /compact (Phase 6) requires it and it touches the Projector that Phase 4 already disturbed. Design is settled by the corpus evidence — no invention needed.
**Delivers:** Threshold-triggered light-tier summarizer appending a typed `compaction` line; Projector treats it as a hard reset point (summary = durable seed, immune to boundary resets); cache_control on every system block via Shaper (the parity-faithful lever); per-result budget ceiling tied to smallest routed window; single retry-once recovery on overflow error.
**Avoids:** Pitfalls 6, 7 (late/defeated compaction, two-layer fights).

### Phase 6: Built-in Commands + Skills + Per-Agent Model
**Rationale:** Resolver chain and command table are one coherent seam; per-agent model MUST precede full subagents (Phase 8) or background agents launch mis-routed.
**Delivers:** `invocationFor` chain (builtins → skills → agents → commands); `available_commands_update` on session start + discovery change; class-B control-plane handlers (no model turn, `local_command` transcript lines) — /help /status /cost /mcp /memory /permissions /doctor /config /model /clear /resume /compact; class-A expanders (/init /memory, skills, AGENTS) riding `expandUserBlocks` unchanged; per-agent model resolution (frontmatter > dispatch > session default > tier) with resolvedModel reporting; routing precedence editor-override > window > project > global.
**Avoids:** Anti-Pattern 2 (commands as model turns).

### Phase 7: Context & Policy Parity Closures
**Rationale:** Independent content-path items sharing the profile-copy injection vehicle and the Phase-3 gate pipeline.
**Delivers:** settings.json hooks parsing (project + user scopes) + PreToolUse deny interceptor at the executor chokepoint (bounded sync, hard timeout, fail-open, structured verdicts, MCP-boundary wrap); AGENTS.md/CLAUDE.md injection with mtime caching; `agent_thought_chunk` streaming (provider `"thinking"` chunk → transcript → emitter fan-out); rich prompt content (image/resource ContentBlocks with ingress capability validation, @-mention expansion gated by the Read-tool rules with provenance).
**Avoids:** Pitfalls 8, 14, 16; Security mistakes (allow-from-project-scope, traversal reads, base64 in audit mirrors).

### Phase 8: Background Execution + Sandbox Reality
**Rationale:** Shared lifecycle infrastructure — pitfalls 9 and 10 explicitly belong in ONE phase; subagents and background Bash converge on the task-notification subsystem designed here.
**Delivers:** BackgroundDispatch registry + `BackgroundTaskDone` events riding existing tool_call frames (no new UI concept); signal-handler ReapAll coverage + Linux Pdeathsig + TERM-before-KILL escalation + startup stale-log sweep; persistent shell via creack/pty (Setsid/group-kill escalation, EIO-as-EOF, ANSI stripping, best-effort cd/export tracking); sandbox made real — landlock (Linux) / sandbox-exec (macOS) split, embedded profiles, probe-and-degrade-loudly, default OFF, `--sandbox=off` escape hatch.
**Uses:** go-landlock v0.10.0, creack/pty v1.1.24.
**Avoids:** Pitfalls 9, 10, 11.

### Phase 9: SEED Gaps Close-Out (Steering + Checkpoints)
**Rationale:** Steering is the Telegram prerequisite — do not descope to queue-behind silently (if forced, flag it as a fallback); checkpoint remainder is small safety-edge work on an existing store.
**Delivers:** SteerQueue drained at model-request boundaries (never mid-in-flight-request, never splitting pairs) with ticket/cutoff cancel protocol and parked-ask disambiguation; transport-neutral queue API (kit-friendly); checkpoint restore guard (refuse with active turn/chains), pre-restore snapshot, nested-repo refusal, object-expiry GC, `.ass-guard/` in `.git/info/exclude`, optional `/undo` class-B command.
**Avoids:** Pitfalls 12, 13.

### Phase 10: SEED-001 Kit Extraction
**Rationale:** Strictly last — any earlier ordering invalidates extracted interfaces. With Phases 1–9 settled, this is mostly re-homing.
**Delivers:** Library boundary over profile/shaper/provider/modelrouting/toolcat/toolexec/engine/hookdag/event/session/redact/checkpoint/audit/mcp behind a composition-root API in `pkg/`; app retains ecosys/openspec/coreexec/learning/firstrun/evalsuite/parity + acp frontend; cmd/ass-guard becomes the reference app.
**The one genuinely new design act:** expressing the Frontend seam (emitter + requester interfaces defined session-side, implemented acp-side) as kit interfaces rather than concrete ACP types.

### Phase Ordering Rationale

- **Dependency spine:** carve (1) → wire primitives (2) → interactive asks (3) → sessions (4, needs 2's emitters) → compaction (5, before /compact) → commands/skills (6) → content closures (7) → process lifecycle (8, shared infra; needs 6's per-agent model for subagents) → SEED tails (9) → extraction (10, strictly last).
- **Groupings follow architecture:** everything modifying the turn loop's emission path clusters in 2–5; everything adding processes/lifecycle clusters in 8; everything injecting context clusters in 7.
- **Risk burn-down:** the two highest-severity pitfalls (lock-holding asks, frame ordering) are structurally resolved in Phases 2–3, before breadth accumulates; the transcript schema decisions Phase 8's replay and Phase 5's marker need are locked in Phase 2.
- **Standing gates every phase:** `mise ci` with `-race`; real-Zed UAT for every protocol surface (mocks supplement, never replace — the v1.0 stub lesson); behavioral-eval extension for every change touching request shape.

### Research Flags

Phases likely needing `/gsd-plan-phase --research-phase <N>`:
- **Phase 2:** several ACP details are web-derived LOW confidence — sequenceNumber continuity/reset-on-load legality, available_commands_update full-replacement semantics, cancel contract — verify against `zed-industries/agent-client-protocol` source before coding.
- **Phase 4:** resume reconciliation is the densest correctness surface; enumerate the live-state inventory and design the kill -9 test matrix during planning.
- **Phase 5:** compaction × projector interaction is MEDIUM-confidence territory; pin the boundary-survival and pair-atomicity tests up front.
- **Phase 8:** platform facts drift (Ubuntu userns policy moving; Seatbelt per-version fragility; creack/pty issue-derived failure modes) — re-verify at planning time.
- **Phase 9:** pi/strands steering reference implementations were NOT verified this round (LOW confidence) — consult during discuss-phase as PROJECT.md suggests.

Phases with standard patterns (skip research-phase):
- **Phase 1:** verbatim code move proven by the CI gate.
- **Phase 6:** command tables, expansion seam, and scheduler overrides are all established in-repo patterns with shipped precedents.
- **Phase 7 (AGENTS.md portion):** Pattern-3 profile-copy merge has four existing sites to copy; hooks parsing extends an existing loader.
- **Phase 10:** re-homing stabilized code; the only novel act (Frontend seam inversion) was scoped by this research.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | ACP surface verified against official schema + coder SDK generated types; every dep pre-checked against the CGO_ENABLED=0 gate (go-landlock build-probed in-environment). MEDIUM only on sandboxing prior art |
| Features | HIGH | ACP shapes + CC documented behavior from official sources. MEDIUM for compaction internals, Zed panel specifics, steering-semantics comparisons (version-drifted/community sources) |
| Architecture | HIGH | Every named package/type/function verified against this repo's source; wire shapes primary-source fetched but NOT yet live-Zed-handshake validated |
| Pitfalls | HIGH | Grounded in direct inspection of actual v1.0/v1.1 code; externally-derived facts individually graded (Context7 Anthropic = MEDIUM; web ACP/Seatbelt/bwrap/git/pty = LOW–MEDIUM, flagged) |

**Overall confidence:** HIGH — proceed to roadmap. Weakest links are concentrated in Phase 8 platform behavior and Phase 9 steering references, both flagged for planning-time re-verification.

### Gaps to Address

- **sequenceNumber semantics across session/load:** whether reset-on-load is legal is unresolved — verify in ACP source during Phase 2 planning before shipping resume.
- **available_commands_update replace-vs-diff:** LOW-confidence web detail (believed full-replacement) — confirm against SDK.
- **Elicitation skew on older Zed:** -32601-vs-ignore behavior on pre-v0.202.0 clients untestable without an old binary — cover empirically in UAT; the text-ask fallback makes any answer non-fatal.
- **pi/strands steering contracts:** named in PROJECT.md but not verified this round — consult during Phase 9 discuss-phase.
- **Auto-compact thresholds:** moot for design (corpus proves zcode has none); pick ~80% configurable default and expose in /doctor rather than chasing community percentages.
- **Distro sandbox posture drift:** Ubuntu AppArmor userns defaults and bwrap availability are moving targets — probe at startup regardless, re-verify distro claims at Phase 8.
- **Live Zed handshake validation:** all new methods validated against docs only — every Phase 2–4 protocol surface needs a real-Zed UAT round (standing gate, inherited stub lesson).
- **PROJECT.md amendment:** Out-of-Scope "no confirmation/permission tier" must be formally amended at requirements time (tier available, default ungated) — don't leave the docs contradicting the ship.

## Sources

### Primary (HIGH confidence)
- agentclientprotocol.com — /protocol/schema, /protocol/v1/elicitation, /protocol/v1/tool-calls, /protocol/v1/slash-commands (live fetches; method/field-level verification)
- `github.com/zed-industries/agent-client-protocol@v1.7.0` — direct inspection proving schema-only, no Go SDK
- `github.com/coder/acp-go-sdk@v0.13.5` — downloaded; Agent/Client interfaces + unstable/stable split inspected
- Context7 `/anthropics/anthropic-sdk-go` — NewImageBlockBase64, thinking-block signature contract; `/llmstxt/platform_claude_llms_txt` — 400-on-edit behavior
- go-openai v1.42.0 module source — MultiContent/image parts (direct inspection)
- Go proxy @latest — anthropic-sdk-go v1.66.0, creack/pty v1.1.24, go-landlock v0.10.0
- Direct `CGO_ENABLED=0` build probe of go-landlock in this environment
- Codebase (HIGH — direct inspection): acp_serve.go, cron_wiring.go, internal/{acp,session,event,ecosys,shaper,provider,coreexec,checkpoint,toolcat}, docs/compaction-decision.md, SEED-001/SEED-004
- Official Claude Code docs via Context7 (`/websites/code_claude`) — commands, settings-reference, hooks, agent-sdk, skills, context-window

### Secondary (MEDIUM confidence)
- Codex CLI / Claude Code sandboxing write-ups (Seatbelt .sb generation; Landlock+seccomp native paths)
- Cline/Roo/Gemini CLI/Claude Code checkpoint documentation
- Canonical Ubuntu 24.04 AppArmor userns restriction reports
- Anthropic/GitHub-issue compaction behavior reports (#2497/#4251, r/ClaudeAI) — directional only; overridden by our corpus scan
- pi (badlogic/pi-mono) steering docs + Strands interrupt machinery (#855/#913) — convergence insight used, exact contracts unverified
- creack/pty GitHub issues (#96/#65/#115/#118) — consistent failure-mode picture

### Tertiary (LOW confidence — validate during planning)
- Community Claude Code auto-compact percentage reports (~80–95% scatter) — treated as noise, not spec
- ACP sequenceNumber buffering / full-list-replacement details (web-derived)
- Zed elicitation landing version (v0.202.0) — release-notes derived
- eclecticlight/newosxbook SBPL guidance; git index.lock mechanics threads

---
*Research completed: 2026-08-26*
*Ready for roadmap: yes*
