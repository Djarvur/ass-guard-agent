# Roadmap: ass-guard-agent (working name)

**Project mode:** mvp (vertical slices — each phase delivers an end-to-end user capability)

## Milestones

- ✅ **v1.0 MVP** — Phases 0–7 (shipped 2026-08-14; full detail: `.planning/milestones/v1.0-ROADMAP.md`, artifacts in `.planning/milestones/v1.0-phases/`)
- ✅ **v1.1 ACP Early Adoption** — Phases 8, 9, 12–14 (shipped 2026-08-25; full detail: `.planning/milestones/v1.1-ROADMAP.md`, artifacts in `.planning/milestones/v1.1-phases/`)
- 🚧 **v1.2 Claude Code Parity** — Phases 15–25 (in progress; this roadmap)

## Phases

<details>
<summary>✅ v1.0 MVP (Phases 0–7) — SHIPPED 2026-08-14</summary>

- [x] Phase 0: Spike + Re-verification (5/5 plans) — completed 2026-08-09
- [x] Phase 1: Mimicry MVP (north-star proof) (6/6 plans) — completed 2026-08-13
- [x] Phase 2: Session Core + ACP Interface (7/7 plans) — completed 2026-08-11
- [x] Phase 3: Model Scheduling (4/4 plans) — completed 2026-08-11
- [x] Phase 4: Unified Engine + Hook-DAG + OpenSpec + Learning (7/7 plans) — completed 2026-08-12
- [x] Phase 5: Ecosystem Compatibility (3/3 plans) — completed 2026-08-13
- [x] Phase 6: Distribution + Polish (2/2 plans) — completed 2026-08-13
- [x] Phase 7: Multi-Provider Config & Credentials (2/2 plans) — completed 2026-08-14

</details>

<details>
<summary>✅ v1.1 ACP Early Adoption (Phases 8, 9, 12–14) — SHIPPED 2026-08-25</summary>

- [x] Phase 8: Slash-Command Kickoff (9/9 plans) — completed 2026-08-16; zero-continue product proof
- [x] Phase 9: Serve-Path Audit + zcode Parity Re-capture (6/6 plans) — completed 2026-08-18
- [x] Phase 12: Product Functional Completeness (11/11 plans incl. gap-closure 12-09..12-11) — completed 2026-08-20; re-verified 2026-08-25 after the live UAT round found and fixed three real gaps
- [x] Phase 13: OpenSpec Workflow Completion (5/5 plans) — completed 2026-08-21
- [x] Phase 14: Adoption Readiness (Analysis Dispositions) (6/6 plans) — completed 2026-08-19

Post-adoption UAT reopen/close (2026-08-22…25): live stdio driving found dead plan-mode wiring, silent tool-result loss, ReadSessionContext id mismatch, TaskOutput catalog omission — all fixed via 12-09/12-10/12-11, UAT 11/11 pass.

Operator decisions at close (2026-08-23…25): D-09 REVERSED (session resume = must-have); "scheduling" renamed to model-routing (commit 732a8a3); **mimicry abandoned as the product bar in favor of client-native ACP surfaces** (Zed-provided fs/permission/session UX); LSP routed to IDE-side MCP configuration (documented requirement, not own implementation).

</details>

### 🚧 v1.2 Claude Code Parity (Phases 15–25, In Progress)

**Milestone Goal:** Make ass-guard feel native in the editor and behave like Claude Code end-to-end — full ACP surfaces, built-in + skill slash-commands, deliberate parity-closure of the ten known divergences — then extract the agent-creation kit as a library with ass-guard as its reference app.

**Explicitly deferred from v1.2:** Telegram peer → v1.3 pool, LOWEST priority per operator 2026-08-26 (steering queue SEEDG-01 is built transport-neutral as its prerequisite). dsh profile #2 dropped entirely (operator 2026-08-26; mimicry bar abandoned).

- [x] **Phase 15: internal/runtime Carve (Step 0)** - Mechanical verbatim move of sessionTurnRunner into internal/runtime so seven later feature clusters land in their final home (completed 2026-08-27)
- [x] **Phase 16: ACP Wire Foundation** - Outbound id'd requests, one ordered TurnEmitter, transcript line-type extensions, initialize capability overhaul (completed 2026-09-01)
- [x] **Phase 17: Permissions + Elicitation** - Clickable permission asks and structured form asks through the editor; ONE gate pipeline locked (completed 2026-09-03)
- [x] **Phase 18: Session Family** - list / load-resume / close-delete with full replay and live-state reconciliation (completed 2026-09-06)
- [x] **Phase 19: Compaction + cache_control** - Threshold-triggered compaction marker plus parity-faithful cache_control emission (completed 2026-09-07)
- [x] **Phase 20: Built-in Commands + Skills + Per-Agent Model** - Resolver chain, class-B/class-A command families, slash-invocable skills, per-agent model dispatch (completed 2026-09-07)
- [x] **Phase 21: Context & Policy Parity Closures** - Hooks PreToolUse deny joining the gate pipeline, AGENTS.md injection, thinking streaming, rich prompt content (completed 2026-09-06)
- [ ] **Phase 22: Background Execution + Sandbox Reality** - Full subagents, background Bash, persistent shell on shared process lifecycle; real sandbox
- [ ] **Phase 23: SEED Gaps Close-out** - Steering queue, checkpoint restore guard, /undo
- [ ] **Phase 24: Documentation & Ops Tails** - LSP doc requirement, outcome store, nightly CI, ecosystem end-to-end
- [ ] **Phase 25: SEED-001 Kit Extraction (strictly last)** - Agent machinery extracted behind a composition-root API; ass-guard becomes the reference app

## Phase Details

### Phase 15: internal/runtime Carve (Step 0)

**Goal**: The turn runner lives in its final home before any feature touches it — seven of ten feature clusters modify `sessionTurnRunner`, and carving first means features land once instead of migrating twice.
**Depends on**: Nothing (first phase of the milestone)
**Requirements**: RUNT-01
**Success Criteria** (what must be TRUE):

  1. `internal/runtime` houses `sessionTurnRunner` verbatim with engine/MCP adapters and cron wiring; `cmd/ass-guard` composes it — zero behavior change, proven by the standing `mise ci` gate (vet + lint + CGO_ENABLED=0 build + `go test -race`) green.
  2. Every existing serve-path behavior is observably identical after the move: a live Zed-spawned session streams tokens, executes tools, replays on restart exactly as at v1.1 close (the operator's daily-use surface unchanged).
  3. No feature code moved or rewritten during the carve — the diff is a pure relocation (verbatim bodies, import fixes only), reviewable as such.

**Plans:** 7/7 plans complete

Plans:
**Wave 1**

- [x] 15-01-PLAN.md — Wave-0 instruments: pre-move test ledger (130) + CLI-contract golden test

**Wave 2** *(blocked on Wave 1 completion)*

- [x] 15-02-PLAN.md — Extract internal/providerfactory (shared-infra; consumed by acpserve/tracer/parity)

**Wave 3** *(blocked on Wave 2 completion)*

- [x] 15-03-PLAN.md — Extract checkpointcmd/learningcmd/modelroutingcmd CLI logic packages
- [x] 15-04-PLAN.md — Extract profilecheckcmd + paritycli; retire providerfactory wrapper bridge

**Wave 4** *(blocked on Wave 3 completion)*

- [x] 15-05-PLAN.md — Extract internal/acpserve (Options + serve pipeline, two sanctioned de-cobra edits)

**Wave 5** *(blocked on Wave 4 completion)*

- [x] 15-06-PLAN.md — THE CARVE: runtime + enginebridge + cron family as one compile closure; acpserve.Run unified

**Wave 6** *(blocked on Wave 5 completion)*

- [x] 15-07-PLAN.md — Equivalence proof battery + color-moved review record + live-Zed operator checkpoint

### Phase 16: ACP Wire Foundation

**Goal**: The three primitives every interactive phase depends on exist and are proven under concurrency: outbound id'd JSON-RPC requests with a pending-response registry, one ordered inline TurnEmitter owning all client frames with explicit backpressure, and the extended transcript line types (raw-thinking passthrough as `json.RawMessage`, `local_command`, compaction marker) that later phases' schemas lock here.
**Depends on**: Phase 15 (the emitter and request paths attach to the carved runner)
**Requirements**: ACP-03, ACP-08
**Success Criteria** (what must be TRUE):

  1. During a live Zed turn the user sees activity stream in real time in the editor's native cards: tool_call/tool_call_update (kind/status/diff/locations), plan updates mirroring TodoWrite entries, and agent_thought_chunk — never a black-box spinner for the whole turn.
  2. Frames from concurrent emitters (foreground turn, subagents, engine firings) arrive at the client in one consistent order — a multi-emitter `-race` stress test proves no cross-kind reordering and no dropped frames under backpressure, with the interleaving policy documented (foreground priority).
  3. An id'd request written to the client (e.g. a probe elicitation) receives its response matched by id even while notifications stream — the pending-response registry resolves responses concurrently without blocking the writer mutex, and an unanswered request degrades loudly rather than wedging the connection.
  4. Initialize/new/load/resume responses carry the richer capability set (loadSession, sessionCapabilities, configOptions advertisement shape), verified against the ACP schema in a real Zed handshake.
  5. Transcript lines for raw thinking (`json.RawMessage` passthrough), local_command, and the compaction marker type exist append-only with redaction excluded by construction for thinking bytes.

**Plans**: 9/9 plans executed + 3 gap-closure plans (16-07..16-09, from 16-VERIFICATION.md)

Plans:
**Wave 1**

- [x] 16-01-PLAN.md — Tracer: ordered TurnEmitter + tool_call/plan streaming through the carved seam (ACP-03 spine, D-01..D-04)
- [x] 16-02-PLAN.md — Transcript additive kinds: raw_thinking (unredacted), local_command, compaction marker (D-20..D-23)
- [x] 16-04-PLAN.md — Config layer writer: atomic persist-then-apply, session_tier key (D-07 writer half, D-08 groundwork)

**Wave 2** *(blocked on Wave 1 completion)*

- [x] 16-03-PLAN.md — Outbound request registry, response interception, capability probe, telemetry (D-13..D-19)

**Wave 3** *(blocked on Wave 2 completion)*

- [x] 16-05-PLAN.md — ACP-08 wire surface: configOptions advertisement, set_config_option, _meta blob, live apply (D-05..D-12)

**Wave 4** *(blocked on Wave 3 completion)*

- [x] 16-06-PLAN.md — Zed-client simulator E2E + adversarial soak + live-Zed operator checkpoint

**Gap closure** *(from 16-VERIFICATION.md: gaps_found — CR-01 barrier lost-wakeup, WR-05 scope semantics, chip truthfulness)*

**Wave 1**

- [x] 16-07-PLAN.md — Gap closure: Barrier broadcast wake — CR-01 two-concurrent-waiter lost wakeup (gaps 1-2, ACP-03)
- [x] 16-08-PLAN.md — Gap closure: scope-aware config surface — global-layer idempotence + layer-true _global twins (gaps 3, 4a, 5, ACP-08)
- [x] 16-09-PLAN.md — Gap closure: chip truthfulness — runner default follows the tier resolution (gap 4b, ACP-08)

### Phase 17: Permissions + Elicitation

**Goal**: The agent's asks become clickable editor surfaces instead of plain text — permission asks via session/request_permission with allow/reject × once/always semantics, learning/engine asks via elicitation/create forms — riding the AskBroker suspension pattern so human-timescale waits never hold locks; the ONE gate pipeline (hook verdict → permission ask → execute) is locked and documented here so Phase 21's hooks join rather than bolt on.
**Depends on**: Phase 16 (id'd outbound requests + pending-response registry are the transport these asks ride)
**Requirements**: ACP-01, ACP-02
**Success Criteria** (what must be TRUE):

  1. With `permissions.mode: gated`, a mutating tool call pops Zed's native permission dialog with allow/reject × once/always options; the chosen option persists correctly across subsequent calls (always = no further ask).
  2. While a permission dialog is open, the session stays alive and responsive — other prompts queue, cancellation works, a turn death mid-ask delivers cancelled as a NORMAL response (no hang, no orphaned dialog), and nothing blocks holding the turn mutex.
  3. Learning-store and engine asks render as structured forms via elicitation/create on current clients; on older clients the -32601 probe degrades to today's plain-text AskBroker path automatically — either way the user can answer and the answer lands as the tool result.
  4. Default remains ungated (safety-model amendment "available, not default"): with default config, zero new dialogs appear versus v1.1 behavior; mode flips via editor configOptions take effect on the running session.
  5. The gate pipeline precedence (hook verdict → permission ask → execute) is implemented at ONE chokepoint and documented, with permissions.yaml persisted choices surviving restarts.

**Plans**: 6/6 plans executed (5 executed; 1 gap closure)

Plans:
**Wave 1**

- [x] 17-01-PLAN.md — internal/perm: CC-parity rule grammar + permissions.yaml store (D-01..D-03, both directions, 0600 atomic)

**Wave 2** *(blocked on Wave 1 completion)*

- [x] 17-02-PLAN.md — TRACER: the ONE gate chokepoint + permission ask end-to-end + live permissions.mode flip (criterion 1+4, D-04..D-07)

**Wave 3** *(blocked on Wave 2 completion)*

- [x] 17-03-PLAN.md — Ask queue semantics + turn-death drain on all teardown paths (criterion 2, D-11..D-13)
- [x] 17-04-PLAN.md — Elicitation surfaces: D-08 mapping, whole-family conversion, D-10 re-validation, plain-text fallback (ACP-02, D-08..D-10)

**Wave 4** *(blocked on Wave 3 completion)*

- [x] 17-05-PLAN.md — Chokepoint documentation + Zed-simulator E2E + live-Zed operator checkpoint (criterion 5, criteria 1+3 manual legs)

**Wave 5** *(gap closure — UAT G-17-1 blocker)*

- [x] 17-06-PLAN.md — GAP G-17-1: decode the canonical ACP v1 NESTED request_permission outcome (types.go + ask_surface.go + both test clients canonical; RED-proven pins; criterion 1 un-blocks, UAT Test 2 re-testable)

### Phase 18: Session Family

**Goal**: Sessions become first-class objects the editor can enumerate and restore: list with pagination, load/resume with full replay plus live-state reconciliation (the hard part — dangling expectations get synthetic closure), and close/delete with tombstoning preserving the D-20 audit invariant.
**Depends on**: Phase 16 (replay rides the ordered TurnEmitter); Phase 17 (parked asks/pending permissions are part of the reconciled live state)
**Requirements**: ACP-05, ACP-06, ACP-07
**Success Criteria** (what must be TRUE):

  1. The user opens Zed's session picker and sees past sessions listed with headers/cursor pagination, and can open any of them — the full conversation replays visibly through the same ordered frames as live turns.
  2. Resuming mid-history works like Claude Code: ids continue from transcript maxima (no collisions), commands re-advertise, and a kill -9 mid-turn followed by resume leaves NO ghost state — dangling tool_calls close as failed, parked asks and pending permissions resolve synthetically (kill -9 E2E test green).
  3. `ass-guard --resume` works anywhere (CLI flag), not only from the editor.
  4. Closing a session stops its work cleanly; deleting tombstones the record (never rm) so audit history survives — deleted sessions disappear from the list but remain investigable on disk.

**Plans**: 7/7 plans executed (6 executed; 1 gap closure)

Plans:
**Wave 1**

- [x] 18-01-PLAN.md — TRACER: session/load clean-path — load engine, replay mapping, capability flip (ACP-06)
- [x] 18-02-PLAN.md — Reconciliation engine: ten-row inventory classifier + append-only closures (ACP-06)
- [x] 18-03-PLAN.md — List engine: header scan, composite cursor, tombstone filter (ACP-05)

**Wave 2** *(blocked on Wave 1 completion)*

- [x] 18-04-PLAN.md — list/close/delete RPC + tombstone lifecycle + startup sweep (ACP-05, ACP-07)
- [x] 18-05-PLAN.md — Reconciliation wired into load + kill -9 matrix E2E (criterion 2, ACP-06)

**Wave 3** *(blocked on Wave 2 completion)*

- [x] 18-06-PLAN.md — CLI trio (--resume/--continue) + numbered picker + serve injection (criterion 3, ACP-06)

**Wave 4** *(gap closure — UAT G-18-1 blocker)*

- [x] 18-07-PLAN.md — GAP G-18-1: real sessions write the session_start opener (sessionFor size-gated wiring) + tolerant legacy listing (readHeaderOpener known-type fallback) — RED-proven pins; UAT Test 2 re-testable

### Phase 19: Compaction + cache_control

**Goal**: Long sessions stop silently losing earlier turns: threshold-triggered light-tier compaction appends an additive typed marker the Projector treats as a durable reset-point class, and cache_control ephemeral breakpoints ship on every system block — the corpus-proven parity-faithful lever (zcode has no auto-compact; docs/compaction-decision.md settles design).
**Depends on**: Phase 16 (transcript marker line type locked there); independent of the session family otherwise — pulled ahead of its nominal slot because /compact (Phase 20) requires it and it touches the Projector Phase 18 disturbed
**Requirements**: PAR-01, PAR-02
**Success Criteria** (what must be TRUE):

  1. A long session crossing the ~80% context threshold (configurable) compacts automatically: a summary persists in context as a durable seed immune to mutating-boundary resets, and the user observes the conversation continuing coherently where v1.1 would have lost earlier turns.
  2. Tool_use/result pairs survive compaction atomically (no orphaned results), thinking blocks are never rewritten mid-chain, and pinned Projector tests prove boundary-survival and pair-atomicity — the two-layer context machinery is respected, not bypassed.
  3. On a provider overflow error ("prompt too long"), recovery retries once post-compaction instead of failing the turn.
  4. Every outgoing request carries cache_control {"type":"ephemeral"} on each system block — visible in the request log — and the existing cache-discipline probe in `ass-guard parity` flips green on placement.

**Plans**: 7/7 plans executed (6 executed; 1 gap closure planned)

Plans:
**Wave 4** *(gap closure — UAT G-19-1, operator ruling (b) 2026-09-07)*

- [x] 19-06-PLAN.md — GAP G-19-1: same-turn marker carve-out + engine-armed retry override + WR-03 rider (bounded summarize span, once-per-turn re-fire guard) + content-sensitive regression

**Wave 5** *(gap closure — VERIFICATION 2026-09-07 CR-01 residual)*

- [x] 19-07-PLAN.md — GAP CR-01: armed carve-out precedence over the pre-user marker scan (retry-once recovery on every overflow, not just a session's first) + prior-marker regression leg (PAR-01)

**Wave 1**

- [x] 19-01-PLAN.md — Tracer: cache_control chain end-to-end — profile flag → shaper emission → probe flip, WINDOWS #5 closed (PAR-02, D-12)
- [x] 19-02-PLAN.md — Provider overflow truth: non-2xx error-chunk surfacing + IsOverflow predicate (PAR-01 foundation, Pitfall 1)
- [x] 19-03-PLAN.md — Marker summary payload + Projector durable reset-point class + budget-fill pair-safe tail cut (PAR-01 semantics)

**Wave 2** *(blocked on Wave 1 completion)*

- [x] 19-04-PLAN.md — Compaction engine: threshold check, blocking same-pipeline summarizer, loop-head wiring, retry-once, CompactNow (PAR-01 assembly)

**Wave 3** *(blocked on Wave 2 completion)*

- [x] 19-05-PLAN.md — Config keys + configOptions menu live-apply: compaction.threshold_pct / compaction.enabled (D-03)

### Phase 20: Built-in Commands + Skills + Per-Agent Model

**Goal**: Slash-invocation becomes one coherent resolver chain — builtins → skills → agents → file-discovered commands — with CC's useful internal commands executing control-plane-fast (no model turn) or prompt-expanding as appropriate, skills addressable as slash commands, AGENTS addressable likewise, and per-agent `model:` frontmatter actually routing subagent dispatch. Everything autocompletes in the editor via available_commands_update.
**Depends on**: Phase 16 (available_commands_update + local_command transcript lines); Phase 19 (/compact requires compaction)
**Requirements**: ACP-04, CMDS-01, CMDS-02, CMDS-03, CMDS-04, SKLS-01, SKLS-02, SKLS-03
**Success Criteria** (what must be TRUE):

  1. Typing `/` in Zed autocompletes the full command set (builtins, discovered commands, skills, AGENTS), and available_commands_update re-fires when discovery changes mid-session.
  2. Newly created/installed (or removed) commands, skills, and agents are picked up WITHOUT restarting the session — live rescan (filesystem watch or invoke-time re-discovery) updates the resolver chain and the editor autocomplete reflects the change immediately.
  3. `/help /status /cost /mcp /memory /permissions /doctor /config /model /clear /resume /compact` respond instantly with NO model turn, recorded as local_command transcript lines; /compact performs real compaction (Phase 19's machinery); /model switches session-scope routing and takes effect on the next request.
  4. `/init` expands as a prompt-expanding command recorded as a user turn with provenance, riding the existing expansion seam unchanged in behavior.
  5. Typing `/<skill-name>` runs the skill — SKILL.md body expands as the prompt with args appended; discovered AGENTS are invocable the same way (BMad-style `.claude/agents/*.md` layout works).
  6. A subagent dispatched to an agent whose frontmatter declares `model:` routes to that model (precedence frontmatter > dispatch > session default > tier), with resolvedModel reported back — mis-routed background launches become impossible before Phase 22 builds on this.

**Plans**: 6 plans

Plans:
**Wave 1**

- [x] 20-01-PLAN.md — Tracer: command chain + /status class-B end-to-end + available_commands_update advertisement (ACP-04, CMDS-01, CMDS-02 spine; D-01..D-05)

**Wave 2** *(blocked on Wave 1 completion)*

- [x] 20-02-PLAN.md — Class-B family: twelve commands, /clear boundary, /cost endpoint-first, /model live-apply, /resume + /compact delegation seams (CMDS-02, D-06..D-09)
- [x] 20-03-PLAN.md — Per-agent model routing: precedence, inherit, cross-provider factory routing, resolvedModel both ways (SKLS-03, D-13..D-16)

**Wave 3** *(blocked on Wave 2 completion)*

- [x] 20-04-PLAN.md — Skills + agents as slash commands + /init class-A (SKLS-01, SKLS-02, CMDS-03, D-03)

**Wave 4** *(blocked on Wave 3 completion)*

- [x] 20-05-PLAN.md — Live rescan: fsnotify watch + debounce + atomic swap + freshness backstop + re-fire (CMDS-04, D-10..D-12)

**Wave 5** *(blocked on Wave 4 completion)*

- [x] 20-06-PLAN.md — Simulator E2E + criteria-to-evidence matrix + live-Zed operator checkpoint (all criteria)

### Phase 21: Context & Policy Parity Closures

**Goal**: The content-path parity closures land as one coherent wave sharing two vehicles already built: hooks PreToolUse deny joins Phase 17's single gate pipeline (deny-only authority from project scope), AGENTS.md/CLAUDE.md auto-inject via the profile-copy merge, thinking streams end-to-end byte-identical, and rich prompt content (images, @-mentions) enters with ingress validation.
**Depends on**: Phase 17 (the gate pipeline hooks join); Phase 16 (thinking passthrough line types); Phase 15 (runner seam for injection points)
**Requirements**: PAR-03, PAR-04, PAR-05, PAR-06
**Success Criteria** (what must be TRUE):

  1. A settings.json hook (project or user scope) returning deny on PreToolUse blocks the tool call at the executor chokepoint — bounded sync execution with hard timeout, fail-open on hook failure, structured verdicts, and repo-shipped files can NEVER grant allow (deny-only from project scope).
  2. Hook verdicts flow through the SAME pipeline as permission asks (documented precedence hook verdict → permission ask → execute) — no second gate exists.
  3. A fresh session in a repo with AGENTS.md/CLAUDE.md shows those files' content in system context automatically (mtime-cached; edits picked up on change) without any manual step.
  4. Extended thinking renders live in Zed as thought chunks, and thinking blocks round-trip byte-identical including the Anthropic cryptographic signature through transcript/redactor/projector — no 400s from edited or reordered signatures.
  5. Pasting an image reference or @-mention into the prompt produces the corresponding content block in the outgoing request (@ expansion gated by Read-tool rules with provenance; ingress validated per provider shape — unsupported shapes degrade loudly, not silently).

**Plans**: 6/6 plans executed

Plans:
**Wave 1**

- [x] 21-01-PLAN.md — Hook substrate: settings-scope loading + JSON verdicts + deny-wins ResolveVerdict in internal/ecosys (PAR-03 substrate, D-01..D-04)
- [x] 21-02-PLAN.md — Memory injection: AGENTS.md/CLAUDE.md walker + mtime cache + fourth profile-copy merge (PAR-04, D-05..D-08)

**Wave 2** *(blocked on Wave 1 completion)*

- [x] 21-03-PLAN.md — Thinking pipeline: SSE → StreamChunk → transcript → agent_thought_chunk → projector → shaper + D-14 goldens (PAR-05, D-12..D-14)

**Wave 3** *(blocked on Wave 2 completion)*

- [x] 21-04-PLAN.md — @-mention expansion: parse-only extraction in ecosys + Read-rule-gated runtime expansion + per-mention provenance (PAR-06, D-10)

**Wave 4** *(blocked on Wave 3 completion)*

- [x] 21-05-PLAN.md — Image ingress: DecodeConfig-first validation + pure-Go auto-downscale + Ref-based transcript lines + provider capability D-11 (PAR-06, D-09, D-11)

**Wave 5** *(blocked on Wave 4 completion; preconditions on Phase 17 executed)*

- [x] 21-06-PLAN.md — TRACER gate-join: hook verdicts join gateCall's head + executor-leg disposal + Read-rule wiring (PAR-03 join, criterion 1+2, D-04)

### Phase 22: Background Execution + Sandbox Reality

**Goal**: All long-lived process work converges on ONE lifecycle infrastructure: full subagents (background dispatch, structured task-notifications by kind, output retrieval, cancellation), background Bash completion callbacks on the same task-notification subsystem, persistent-shell Bash via PTY, and the sandbox flag made real with landlock/sandbox-exec split — probe-and-degrade loudly, default OFF.
**Depends on**: Phase 20 (per-agent model routing must precede subagent dispatch or background agents launch mis-routed); Phase 16 (task-notifications ride existing tool_call frames)
**Requirements**: PAR-07, PAR-08, PAR-09, SAND-01
**Success Criteria** (what must be TRUE):

  1. The model can launch a subagent in the background and keep working: the dispatch returns immediately with a discriminated result, a structured task-notification arrives on completion (detected by kind, not text-matching), output is retrievable for running tasks, and cancellation kills cleanly.
  2. Background Bash behaves identically lifecycle-wise: completion notifications ride the same task-notification subsystem, processes die with their process group (TERM-before-KILL escalation, Linux Pdeathsig), stale logs sweep at startup, and no orphans survive agent shutdown.
  3. The persistent-shell Bash option holds state across calls in one PTY session (cd/export persist, ANSI stripped, EIO-as-EOF handled) without leaking the master fd.
  4. With sandbox enabled, tools run confined (landlock on Linux kernel ≥5.13, sandbox-exec generated profiles on macOS with targeted denies); unsupported kernels/environments degrade LOUDLY at startup, default stays OFF, and `--sandbox=off` always escapes.

**Plans:** 6 plans

Plans:
**Wave 1**

- [ ] 22-01-PLAN.md — Tracer: the ONE task-notification subsystem (tracker kinds, D-02 payloads, D-03 coalescing, D-01 wake-turn drain) with background Bash as first producer; D-10 subagent cap/queue; OQ5 close-cancel
- [ ] 22-05-PLAN.md — Sandbox policy core (SAND-01): one policy struct → landlock + seatbelt backends behind ONE portable WrapCmd entry, strict probe taxonomy, D-05 honesty doc — live-probed on darwin, compile-gated on linux

**Wave 2** *(blocked on Wave 1 completion)*

- [ ] 22-02-PLAN.md — Background Bash lifecycle hardening: TERM-before-KILL ladder + Linux Pdeathsig, D-11 queue-on-cap + D-12 configOptions caps, OQ4 tombstone stale-log sweep

**Wave 3** *(blocked on Wave 2 completion)*

- [ ] 22-03-PLAN.md — Background subagents (PAR-07): discriminated async_launched dispatch, output-file retrieval, cancellation, Phase-20 routing seam (precondition-marked), OQ3 ask-decline parity

**Wave 4** *(blocked on Wave 3 completion)*

- [ ] 22-04-PLAN.md — Persistent-shell PTY (PAR-09): one lazy session PTY, nonce-sentinel cycle, ANSI strip, EIO-as-EOF, D-08 lazy restart + close drain, OQ1 additive `persistent` schema property per D-09

**Wave 5** *(blocked on Wave 4 completion)*

- [ ] 22-06-PLAN.md — Sandbox enforcement (SAND-01): --sandbox flag + startup probe-and-degrade + sentinel main hook, wrap at ALL THREE exec sites (foreground Bash, background TaskRegistry.Start, PTY persistent shell — D-09 orthogonality), OQ2 honored dangerouslyDisableSandbox, default OFF

### Phase 23: SEED Gaps Close-out

**Goal**: The remaining SEED-004 gaps close: steering/input queue during a running turn (transport-neutral — the Telegram prerequisite, not descoped to queue-behind), checkpoint restore hardened against active turns and nested repos, and /undo exposed as a class-B command.
**Depends on**: Phase 20 (steering drains at model-request boundaries in the carved runner; /undo lands in the class-B command table)
**Requirements**: SEEDG-01, SEEDG-02, SEEDG-03
**Success Criteria** (what must be TRUE):

  1. Typing while a turn runs queues input and applies it at the next model-request boundary — never mid-in-flight-request, never splitting tool_use/result pairs — with the ticket/cutoff cancel protocol resolving which queued inputs the running turn acknowledges.
  2. A queued-but-unresolved ask disambiguates cleanly (parked-ask handling): the user can tell what's waiting and answer or cancel it without killing the turn.
  3. Checkpoint restore refuses safely when a turn or engine chain is active, snapshots pre-restore state first, refuses nested-repo restores (gitlink contents would be silently unprotected), GCs expired checkpoint objects, and `.ass-guard/` is excluded from the user's git via `.git/info/exclude`.
  4. `/undo` restores the last checkpoint instantly with no model turn (class-B), and the restored workspace is byte-identical to the pre-turn snapshot.
  5. The steering queue API is transport-neutral (consumable by a non-ACP frontend) — Telegram could adopt it without rework.

**Plans**: 5 plans

Plans:
**Wave 1**

- [x] 23-01-PLAN.md — SEEDG-01 steering core: transport-neutral SteerQueue, boundary drain at the runTurn iteration top, steering_delivery fold (anchor-safe, replay-parity)
- [x] 23-03-PLAN.md — SEEDG-02 store guards: pre-restore snapshot id family (three-site grammar), nested-repo refusal, age+count GC with object expiry, user-repo .git/info/exclude append

**Wave 2** *(blocked on Wave 1 completion)*

- [ ] 23-02-PLAN.md — SEEDG-01 ingress: pre-mutex input classifier, steered-prompt response semantics (return-after-enqueue), parked-ask visibility + cancel grammar, combined-scenario + engine-chain E2E

**Wave 3** *(blocked on Wave 2 completion)*

- [ ] 23-04-PLAN.md — SEEDG-02 composition: Runner-level store handle, restore guard over turnActive/chainCount, session-start GC sweep, checkpoint.expiry_days/max_per_session configOptions + persistence + read-back

**Wave 4** *(blocked on Wave 3 completion + Phase 20 class-B contract)*

- [ ] 23-05-PLAN.md — SEEDG-03 /undo: 14th RESERVED class-B name, D-11 stack walk, D-12 auto-cancel-then-restore, /undo battery + live-Zed UAT checkpoint

### Phase 24: Documentation & Ops Tails

**Goal**: Small independent tails close out: LSP documented as an IDE-side MCP configuration requirement (no agent-side implementation), the scheduler gains its deterministic outcome store + feedback loop, nightly upstream-parity CI automation runs unattended, and plugins/skills prove working unchanged in every interaction mode (ECOS-04 end-to-end).
**Depends on**: Phases 15–23 (tails verify settled surfaces; ECOS-04 exercises modes built across the milestone)
**Requirements**: DOC-01, TAIL-01, TAIL-02, TAIL-03
**Success Criteria** (what must be TRUE):

  1. Following the documented guide, a user configures IDE-side MCP/LSP for ass-guard sessions (operator decision 2026-08-25 honored: documentation only, no agent-side implementation).
  2. Scheduler outcomes are stored deterministically (zero LLM calls) and feed back into routing decisions observable in subsequent scheduling behavior.
  3. Nightly CI runs the upstream-parity gate unattended on a schedule and reports drift (zcode version or structure changes) without human triggering.
  4. Plugins/skills installed for Claude Code work unchanged in EVERY interaction mode — not just loaded at discovery but functional end-to-end wherever they apply.

**Plans**: 5 plans

Plans:
**Wave 1**

- [ ] 24-01-PLAN.md — TAIL-01 core: outcome store + seam-replay aggregation (TDD, D-07 contract)
- [ ] 24-03-PLAN.md — DOC-01: docs/lsp-setup.md three-leg Zed guide + D-03 dry-run (checkpoint: A1 server choice)
- [ ] 24-04-PLAN.md — TAIL-02: nightly drift core + first GitHub Actions workflow + dispatch smoke

**Wave 2** *(blocked on Wave 1 completion)*

- [ ] 24-02-PLAN.md — TAIL-01 live wiring: record at real dispatch sites + stats CLI + live demotion

**Wave 3** *(blocked on Wave 2 completion)*

- [ ] 24-05-PLAN.md — TAIL-03: ECOS-04 4x3 mode/surface harness with loud precondition marks (checkpoint: real-plugin spot-check)

### Phase 25: SEED-001 Kit Extraction (strictly last)

**Goal**: The agent-building machinery extracts as a library in this repo behind a composition-root API, with the frontend seam (emitter + requester defined session-side, implemented acp-side) as the kit interfaces — the milestone's one genuinely new design act. Any earlier ordering invalidates extracted interfaces; with Phases 15–24 settled this is mostly re-homing.
**Depends on**: All previous phases (extraction strictly last)
**Requirements**: KIT-01, KIT-02, KIT-03
**Success Criteria** (what must be TRUE):

  1. Core machinery (profile/shaper/provider/modelrouting/toolcat/toolexec/engine/hookdag/event/session/redact/checkpoint/audit/mcp) lives behind a composition-root API in `pkg/`, importable as a library without touching cmd/ or app-internal packages.
  2. The frontend seam is expressed as kit interfaces — emitter + requester defined session-side, implemented acp-side — proving a non-ACP frontend could host the kit (design-level proof; the Telegram peer consumes this in v1.3).
  3. ass-guard builds and behaves identically as the kit's reference app (retains ecosys/openspec/coreexec/learning/firstrun/evalsuite/parity + ACP frontend) — `mise ci` green, live Zed session unchanged, all behavioral eval suites still pass against the extracted layout.
  4. SEED-002 fantasy + SEED-003 landscape materials are present as reading material/design prior art with zero runtime dependencies.

**Plans**: 9 plans (waves 1-9, strictly sequential — each plan touches kit/ packages the previous one settled)

Plans:
**Wave 1**

- [ ] 25-01-PLAN.md — One-way boundary checkpoint + Wave-0 instruments (ledger baseline, eval class extension) + TRACER: rank-0 verbatim moves (KIT-01, D-01/D-02/D-03/D-07/D-20)

**Wave 2** *(blocked on Wave 1)*

- [ ] 25-02-PLAN.md — Pass-1 ranks 1-3: audit/hookdag/shaper/toolcat + provider/mcp + toolexec/modelrouting verbatim, embeds intact (KIT-01)

**Wave 3** *(blocked on Wave 2)*

- [ ] 25-03-PLAN.md — Pass-1 completion: session/engine + enginebridge→kit/internal + runtime; ledger sum proof, D-06 disposition (KIT-01, KIT-03)

**Wave 4** *(blocked on Wave 3)*

- [ ] 25-04-PLAN.md — KIT-02: Emitter + Requester seams kit-side, kitTurnAdapter acp-side, runtime→acp edge severed (D-13/D-14/D-15, OQ2/OQ3)

**Wave 5** *(blocked on Wave 4)*

- [ ] 25-05-PLAN.md — Scheduler port (D-16) + LearnedStore port + SetupEngine parameterization (OQ4); sched/learning/openspec edges severed

**Wave 6** *(blocked on Wave 5)*

- [ ] 25-06-PLAN.md — kit/session neutral types + CommandCatalog injection (D-17); ecosys edges severed

**Wave 7** *(blocked on Wave 6)*

- [ ] 25-07-PLAN.md — SessionToolkit injection (OQ1 coarse Attach); runtime→coreexec severed — kit/ import-clean

**Wave 8** *(blocked on Wave 7)*

- [ ] 25-08-PLAN.md — White-box test disposition: subject-split to acpserve + kit fakes; gate-scope decision made real

**Wave 9** *(blocked on Wave 8)*

- [ ] 25-09-PLAN.md — D-19 gates (mise + depguard) + hostproof test + kit/README (D-08) + full D-20 battery + live-Zed operator checkpoint (KIT-01/02/03 close)

## Research Flags

Phases likely needing `/gsd-plan-phase --research-phase <N>` (from research/SUMMARY.md):

- **Phase 16:** several ACP details web-derived LOW confidence — sequenceNumber continuity/reset-on-load legality, available_commands_update full-replacement semantics, cancel contract; verify against `zed-industries/agent-client-protocol` source before coding.
- **Phase 18:** resume reconciliation is the densest correctness surface; enumerate the live-state inventory and design the kill -9 test matrix during planning.
- **Phase 19:** compaction × projector interaction MEDIUM confidence; pin boundary-survival and pair-atomicity tests up front.
- **Phase 22:** platform facts drift (Ubuntu userns policy, Seatbelt fragility, creack/pty failure modes) — re-verify at planning time.
- **Phase 23:** pi/strands steering reference implementations NOT verified (LOW confidence) — consult during discuss-phase.

Standard patterns (skip research-phase): Phase 15 (verbatim move), Phase 20 (established patterns), Phase 21's AGENTS.md portion (four merge sites to copy), Phase 25 (re-homing stabilized code).

## Progress

**Execution Order:**
Phases execute in numeric order: 15 → 16 → … → 25

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 15. internal/runtime Carve | v1.2 | 7/7 | Complete    | 2026-08-27 |
| 16. ACP Wire Foundation | v1.2 | 9/9 | Complete    | 2026-09-01 |
| 17. Permissions + Elicitation | v1.2 | 6/6 | Complete    | 2026-09-03 |
| 18. Session Family | v1.2 | 7/7 | Complete    | 2026-09-06 |
| 19. Compaction + cache_control | v1.2 | 7/7 | Complete    | 2026-09-07 |
| 20. Built-in Commands + Skills + Per-Agent Model | v1.2 | 6/6 | Complete    | 2026-09-07 |
| 21. Context & Policy Parity Closures | v1.2 | 6/6 | Complete    | 2026-09-06 |
| 22. Background Execution + Sandbox Reality | v1.2 | 0/? | Not started | - |
| 23. SEED Gaps Close-out | v1.2 | 2/5 | In Progress|  |
| 24. Documentation & Ops Tails | v1.2 | 0/? | Not started | - |
| 25. SEED-001 Kit Extraction | v1.2 | 0/? | Not started | - |

## Backlog

### Phase 999.1: Study ai-agent-book, extract useful patterns (BACKLOG)

**Goal:** [Captured for future planning] Study https://github.com/bojieli/ai-agent-book/ (Li Bojie, 《深入理解 AI Agent：设计原理与工程实践》, ~45k stars, Apache-2.0, 10 chapters + 109 companion experiments, English translation in book-en/) and extract patterns useful for ass-guard-agent. See NOTES.md in the phase directory for chapter-to-surface mapping.
**Requirements:** TBD
**Plans:** 0 plans

Plans:
- [ ] TBD (promote with /gsd:review-backlog when ready)
