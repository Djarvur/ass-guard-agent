# ass-guard-agent (working name)

## What This Is

A Go-based AI coding agent that makes SDD (Spec-Driven Development) workflows run hands-off by automatically doing the routine work a developer keeps forgetting to ask for. It speaks to model providers with requests structured like another agent (the first mimicry target is zcode — a claude-code-compat runtime shipping its own `AGENTS.md`, skills, commands, MCP, and tool catalog), so the model behaves identically to how it behaves in the mimicked agent. The agent hosts unmodified SDD toolkits (OpenSpec in v1), drives them to completion without manual "continue" taps, and runs configurable post-stage routines (review / memory / tests / linters / improvement proposals) on its own. Primary interface is ACP (IDE-native, e.g. Zed); Telegram is a planned full peer surface (text + voice, v2). Built for SDD-capable teams who want the toolkit to just run.

## Core Value

**(PIVOTED 2026-08-25)** A hands-off coding agent that feels native in the editor: full SDD workflows run end-to-end without manual continues, and the agent surfaces through the client's own UX — clickable permission prompts, native file diffs, session management. *(Former bar — request-shape indistinguishability from zcode ("mimicry") — validated v1.0 by the Phase-1 A/B parity test and maintained through v1.1; abandoned by operator decision 2026-08-25 in favor of client-native ACP surfaces. Mimicry assets retained as reference; the tool catalog, engine, and all execution machinery carry forward unchanged.)*

## Current Milestone: v1.2 Claude Code Parity

**Goal:** Make ass-guard feel native in the editor and behave like Claude Code end-to-end — full ACP surfaces, built-in + skill slash-commands, deliberate parity-closure of known divergences — then extract the agent-creation kit as a library.

<details>
<summary>✅ v1.1 ACP Early Adoption — SHIPPED 2026-08-25</summary>

Delivered the hands-off OpenSpec promise end-to-end (zero-continue product proof), every catalog tool executing for real with a behavioral-eval regression net, redacted decision-explaining audit + parity re-capture, adoption-readiness backstops (checkpoints/rollback, model-routing rename, uniform tool contract), and a live UAT round that found and fixed three real product gaps. Operator decisions at close: D-09 reversed (session resume must-have); mimicry abandoned for client-native surfaces; LSP = IDE-side MCP documentation requirement.

</details>

**Target features (priority order — ACP/commands/skills first, parity audit, SEED gaps, kit extraction last; dsh dropped entirely 2026-08-26; Telegram lowest priority — slips to v1.3 if milestone overflows):**
- ACP completeness: session/request_permission (clickable asks), elicitation/create, tool_call+plan streaming, available_commands_update, session list/resume/close/delete (operator must-have), editor-driven configuration (configOptions / session/set_config_option)
- Built-in chat commands: /model /config /compact /clear /cost /resume /memory /mcp /permissions /doctor /status /help /init — surfaced via available_commands_update (compaction is a prerequisite)
- Slash-invocable skills: invocationFor resolves skill keys (+ discovered AGENTS as commands), SKILL.md body expanded as the prompt with args appended, per-agent `model:` frontmatter wired into subagent dispatch
- CC parity audit: close the 10 known divergences deliberately — compaction on context overflow, session resume, permissions UX, full subagents, slash-command autocomplete in editor, hooks PreToolUse deny path, AGENTS.md/CLAUDE.md auto-injection, streamed thinking blocks, rich prompt content (@-file mentions, images), background Bash + persistent shell
- SEED-004 gap fixes: checkpoints/undo (shadow-git), compaction verify-first, sandbox flag made real, steering/input queue during a running turn
- SEED-001: agent-creation kit extracted as a library in this repo; ass-guard becomes its reference app (rides internal/runtime extraction)
- LSP support: documented IDE-side MCP configuration requirement only (no agent-side implementation)
- Scheduler outcome store + feedback loop; nightly-parity CI automation tail
- Telegram peer (lowest priority — text + voice STT, shared turn core, context-first shutdown)

## Business Context

- **Customer**: SDD-practicing engineering teams (and the author's own SDD workflow as the first instance)
- **Revenue model**: Undecided (open-source team tooling; possible hosted/managed later)
- **Success metric**: A team can install ass-guard via ACP registry, run an unmodified OpenSpec workflow, and never tap "continue" or remember to ask for review/tests/lint/memory — the agent does the forgotten routine automatically
- **Strategy notes**: Successor to `sdd-acp-agent` (closed in favor of this project). The predecessor's research, architecture spine, and specs survive as reference; its Go code does not carry over — ass-guard is built fresh against its own scope. Model access in daily practice runs through an opencode subscription serving MiniMax + DeepSeek models (recorded 2026-08-14; informs provider-config defaults and future profile targets).

## Requirements

### Validated

- Model scheduling layer (Phase 3 — Model Scheduling): tier abstraction (heavy/good/light → concrete provider+model), time-windowed substitution with IANA-zone support + bundled tzdata, per-project override (D-02 precedence: time-window → project → global), typed ProviderError classification (Transient/Structural) driving an explicit fallback chain, circuit breakers (consecutive + error-rate, D-07) and a dollars-per-window cost ceiling with degrade-then-stop (D-08), and structured capability profiles with load-time + request-time mismatch enforcement (D-09/D-10). Validated by `internal/scheduler` (config/resolver/dispatch/breaker/cost/capability) + `internal/provider/errors.go` + the `ass-guard scheduling validate|resolve` CLI.
- Distribution (Phase 6 — DIST-01/02/03): single static Go binary via goreleaser (macOS + Linux, amd64 + arm64, CGO_ENABLED=0), canonical ACP registry `agent.json` (cmd/args schema), `--version` with ldflags injection, and zero-config first run (go:embed seed: sanitized zcode profile + openspec.toml + scheduling.yaml written to `.ass-guard/` non-clobbering).
- Mimicry core (Phase 1 — MIMC-01..04, PROF-01..05, TOOL-01..03, PROV-01..03): Profile Shaper between Turn Loop and provider adapter; zcode profile extracted from real rollout logs (3 system blocks, 103 tools, identity, thinking, tool_choice); A/B parity thesis proven; drift detector (`ass-guard profile check`); coverage manifest. *Caveat carried: the pinned capture session (`eea3dc48`) is absent on disk, so the within-session stability test fails until a divergence-prone session is re-captured (operator action).*
- Session core + ACP v1 (Phase 2 — SESS-01..06, ACP-01..05, LOG-02..04, PARA-01..04): Zed-spawnable stdio JSON-RPC server, token streaming, session replay on restart, two-layer context (durable transcript + lean projected window reset at command boundaries), parallel subagents as goroutine turn-loops. Post-ship fix: JSON-RPC string ids accepted (Zed sends UUIDs).
- Tool/plan streaming wire (Phase 16 — ACP-03 leg): one ordered inline TurnEmitter owns every session/update frame from the first frame (lanes fg 128 / bg 256, bounded block-never-drop, stall detector logs loudly), proven under adversarial soak (4M+ frames) and live Zed; id'd outbound requests with pending-response registry ride the same discipline. Validated by `internal/acp` (emitter/requester/metrics) + UAT test 1 (chip==wire across tier switch, live Zed).
- Editor-driven configuration (Phase 16 — configOptions / session/set_config_option): initialize `_meta` blob fills are in-memory only (never persist; layer files stay operator-owned), writes whitelisted to declared menu ids with typed rejection, atomic 0600 layer writes, and the model chip tracks the tier resolution (chip==wire pinned from both sides). Validated live in Zed — operator switched tier glm-5.2/GLM-5.3 from the panel; config mtime + wire model moved together (UAT tests 1–2).
- Unified engine + Hook-DAG + learning (Phase 4 — ENG-01..05, HOOK-01..05, LRN-01..04, OPEN-01..03, TOOL-04/05): post-turn observer deciding continue/hook/ask/wait; dual-signal handoffs (text-pattern OR tool-call); structural safety (unmatched ⇒ nothing, testing/quick + E2E); cancel-drain at ACP level; hand-rolled ≤300-LOC DAG executor with on-failure halt/continue/ask + provenance loop prevention; learning store (ask-once-remember, propose-hooks, versioned, revertible via `ass-guard learning list/revert`); read-only tools concurrent / mutating serialized; swappable WebSearch/WebFetch backends. *Caveat carried: live OpenSpec kickoff requires slash-command invocation — see Active.*
- Multi-provider config & credentials (Phase 7 — PCFG-01..04, completes PROV-01): provider+model declarations with base URL, protocol shape (anthropic/openai), per-model capability/pricing metadata; credential resolution flag > env > config with `${VAR}` expansion; two-shape ProviderFactory wired at all three construction sites; lazy uncredentialed failure (typed structural error); 0600-perm hygiene warning + uncredentialed-provider warnings on stderr. Proven live: zero-env editor-spawned turn authenticating from config file.
- Ecosystem discovery + MCP hosting (Phase 5 — ECOS-01/02/03/05): MCP servers hosted as robust subprocesses (process-group spawn, group-signal shutdown, reaper, tools/list re-fetch per connection); skills/commands/plugins discovered from `.claude/` (project + user) merged with `.ass-guard/` additions under explicit precedence, strictly read-only on `.claude/`.
- Audit logging (Phase 1/2 — LOG-01, tracer-wired): redacted request logging via the tracer path with redactor. *Caveat carried: `--audit-log` is not written on the `acp serve` path — the tracer is wired through main.go; closing the acp-serve audit path is next-milestone material.*
- Editor asks (Phase 17 — ACP-04 legs): permission asks ride `session/request_permission` with allow/reject × once/always semantics persisted as bare tool×project rules; engine asks ride `elicitation/create` forms — both on the AskBroker suspension pattern so human-timescale waits never hold locks. Live-verified in Zed both directions (allow + deny persistence, form answer-landing) after G-17-1 gap closure.
- Session family (Phase 18 — ACP-05/06/07): sessions are first-class editor objects — `session/list` (header-scan + composite-cursor pagination + tombstone filter), `session/load` full replay through the ordered TurnEmitter plus live-state reconciliation (kill -9 leaves no ghost state: dangling tool_calls closed failed, parked asks synthetically resolved, ids continued from transcript maxima), `session/close`/`session/delete` with tombstoning that never rms (D-20 grep-proven), and `--resume`/`--continue` as CLI flags with a pipe-safe numbered picker. Operator-verified live in Zed + real TTY 2026-09-06 after G-18-1 gap closure (real sessions now write the session_start opener; legacy transcripts enumerate via tolerant opener). *Deviation accepted: `--resume <id|name>` resolution is cwd-scoped (per-directory store), not a cross-project registry.*
- Context & policy parity closures (Phase 21 — PAR-01..05): hooks' PreToolUse verdict joins the ONE gate pipeline (deny/ask/allow totally mapped at gateCall's head; project scope deny-only, allow never widens trust), AGENTS.md/CLAUDE.md auto-inject via the profile-copy merge (mtime-cached, whole-file fits-or-skips budgets), thinking streams end-to-end field-value-identical (raw payloads verbatim on disk, values extracted only at the projector), and rich prompt content enters with ingress validation (@-mentions Read-rule-gated with per-attempt provenance; images two-tier — provider limits downscale D-09, 100 Mpx decode ceiling refuses). Operator-verified live 2026-09-06 (UAT 4/4, incl. the corrected-dialect hook-deny retest).

### Active

**v1.2 — ACP completeness (priority 1)**

- [ ] available_commands_update: editor-side slash-command autocomplete for discovered commands and skills (line types locked in Phase 16; the method itself lands in Phase 20)

**v1.2 — Built-in chat commands (priority 2)**

- [ ] Built-in slash-commands riding the command-expansion seam: /model (session-scope routing override), /config (rides editor-config), /compact (requires compaction), /clear (new session same thread), /cost (usage aggregation from transcript records), /resume, /memory, /mcp status, /permissions, /doctor, /status, /help, /init — advertised via available_commands_update

**v1.2 — Slash-invocable skills (priority 3)**

- [ ] invocationFor resolves skill keys (not just reg.Commands): SKILL.md body expands as the prompt with args appended
- [ ] Discovered AGENTS addressable as slash commands (BMad-style installer layout)
- [ ] Per-agent `model:` frontmatter wired into subagent dispatch (parsed today but ignored; routing comes from light tier only)

**v1.2 — CC parity audit (priority 4)**

- [ ] Close the remaining known divergences deliberately: compaction on context overflow (Phase 19); full subagents (Task tool w/ background agents + completion notifications, Phase 22); slash-command autocomplete in editor (rides ACP-completeness, Phase 20); persistent-shell Bash option + background-completion notifications (Phase 22). *(Closed: session resume — Phase 18; permissions UX — Phase 17; hooks full lifecycle incl. PreToolUse deny — Phase 21; AGENTS.md/CLAUDE.md auto-injection — Phase 21; structured thinking blocks — Phase 21; rich prompt content — Phase 21.)*

**v1.2 — SEED-004 gap fixes (priority 5)**

- [ ] Checkpoints/undo via shadow-git: workspace snapshot at turn boundaries; rollback surface (`ass-guard checkpoint` CLI + optional ACP command)
- [ ] Compaction verify-first: check whether the zcode profile already captures zcode auto-compact + cache_control placement before designing our own
- [ ] Sandbox flag made real: parity-driven sandboxing implementing what zcode's tool semantics imply (macOS Seatbelt / Linux bwrap+seccomp reference)
- [ ] Steering/input queue during a running turn (Telegram prerequisite; pi/strands reference semantics)

**v1.2 — SEED-001 kit extraction (priority 6, last)**

- [ ] Extract agent-building machinery as a library in this repo: profile mechanism, provider clients, session/turn loop, projector, unified engine + hook-DAG, tool catalog/execution, scheduling, redaction
- [ ] ass-guard-agent becomes the kit's reference app (zcode profile, .claude/ compat, OpenSpec hosting, ACP frontend = one composition)
- [ ] Rides internal/runtime extraction; SEED-002 fantasy + SEED-003 landscape as design prior art

**v1.2 — Small tails (priority 7)**

- [ ] LSP support: documented IDE-side MCP configuration requirement only
- [ ] Scheduler outcome store + feedback loop (deterministic, zero LLM calls)
- [ ] Nightly upstream-parity gate CI automation (12-08 tail)
- [ ] ECOS-04 end-to-end beyond commands: plugins/skills working unchanged in every interaction mode

**v1.3 pool (post-v1.2)**

- [ ] Telegram peer: full peer interface, text + voice STT, shared turn core (`internal/runtime` extraction), context-first shutdown beside ACP stdio — LOWEST priority per operator 2026-08-26; slips here if v1.2 overflows

### Out of Scope

- Windows platform support — v1 macOS+Linux only; Windows deferred (win-developer teams are not the first target)
- GSD / spec-kit / BMad toolkit adapters in v1 — OpenSpec only; interface accommodates them, implementation deferred
- Perplexity or other paid search backends — configurable backend design replaces the hardcode, but no specific paid integrations are committed
- Byte-for-byte request identity with the mimicked agent — "structurally indistinguishable to the model" is the bar, not binary diff equality (cosmetic field ordering/optional fields allowed)
- A standalone terminal REPL — ACP (IDE) and Telegram are the only interfaces; `acp serve` and `telegram` are launch modes for those two interfaces, not CLI surfaces (amended at Phase-10 discussion 2026-08-14, resolving the flagged tension)
- A confirmation/permission tier for tool execution — tools run ungated; the pattern/hook table + manual cancellation is the safety mechanism (inherited from predecessor)
- Porting code from `sdd-acp-agent` — fresh build; predecessor is reference-only
- opencode as a mimicry target — explicitly excluded (operator, 2026-08-14); opencode appears in research as landscape context only

## Context

**Shipped v1.0 (2026-08-14).** 8 phases, 36 plans, 210 commits over 6 days (2026-08-09 → 2026-08-14); ~33.5k LOC Go across 24 packages; 65.8k insertions over 410 files. Every phase closed through the `mise ci` gate (vet + golangci-lint v2 all-linters + CGO_ENABLED=0 build + `go test -race`), zero issues.

**Known gaps at ship (see MILESTONES.md v1.0 entry):** (1) Phase-4 UAT kickoff gap — `/opsx:*` command invocation unsupported (ecosys unwired; adapter model mismatched the real openspec surface); 11 UAT checks deferred. (2) Phase-1 parity stability test blocked on absent pinned capture session (operator re-capture needed). (3) LOG-01 audit-log not written on the `acp serve` path. (4) tools.json catalog carries 103 tools vs the plans' stale 77 (documented drift, seed mirrors source).

**Predecessor — `sdd-acp-agent`.** A Go-based SDD-toolkit host with ACP UI, Claude-Code-compatible tooling, and pattern-matching autocontinue. Closed in favor of ass-guard. Its planning artifacts (technical research, architecture spine with 11 architectural decisions, epic breakdown with 5 epics/27 stories, log analysis of tool catalog and system prompts) are first-class reference material and live at `/Users/nil/DiskD/W/Djarvur/sdd-acp-agent`. The predecessor validated several load-bearing facts that carry forward as given:

- ACP is JSON-RPC 2.0 over stdio; the editor spawns the agent as a subprocess; stdout is reserved for protocol frames, all logging goes to stderr
- The model-provider layer collapses to two API shapes (Anthropic-shape covering GLM via Z.ai's compatible endpoint; OpenAI-shape covering MiniMax M3 and others) — per-command routing is a config concern, not an architecture
- Context-drop is resolved by a two-layer model (durable replayable transcript vs. lean projected window) — satisfies ACP's replay-on-load contract while letting each command start clean
- Parallel subagents map cleanly to goroutine turn-loops with isolated context
- The autocontinue engine is an observer on an internal event bus, not in a turn's critical path — graceful degradation is structural

**What ass-guard adds beyond the predecessor.** Six deltas, each load-bearing for the new scope:

1. **Mimicry as the north star** — the predecessor had Claude Code *compatibility*; ass-guard makes structural indistinguishability from a configurable target agent (zcode first) the primary success criterion
2. **Multi-tier model scheduling** — heavy/good/light abstraction with time-windowed substitution and per-project overrides, broader than the predecessor's flat per-command routing
3. **Configurable tool backends** — generalizing the predecessor's hardcoded DDG search into configurable backends
4. **Learning mode** — the autocontinue engine asks and remembers how to handle unfamiliar launch situations (fresh context? wait? how long?), and proposes new hooks from the work log
5. **Unified engine** — autocontinue and hooks collapse into one post-turn-complete decision engine (hooks are a special case), simplifying the predecessor's two-mechanism design
6. **Telegram peer** — a second full interface (text and voice) the predecessor explicitly did not have

**Mimicry reference — zcode.** zcode is a claude-code-compat runtime that ships its own `AGENTS.md`, skills, slash-commands, MCP integrations, and a built-in tool catalog. Its on-disk logs (`~/.zcode/cli/rollout/model-io-sess_<id>.jsonl` — corrected during Phase 0 from the research-predicted `~/.claude/projects/` path) are the source of truth for the zcode profile: system prompts, tool catalog (names + schemas), message shape, and identity fields. The profile is then expressed as ass-guard config.

**Author's SDD practice.** The author runs OpenSpec, GSD, BMad, and spec-kit workflows and routinely forgets to invoke the routine work between stages (review, memory updates, tests, linters, improvement proposals). The hook-DAG + seeded set directly addresses this — it is the project's reason to exist for the author personally.

## Constraints

- **Tech stack**: Go (single static binary; first-class concurrency for parallel subagents; mature streaming HTTP clients) — load-bearing, not stylistic
- **Transport discipline**: stdout reserved exclusively for ACP JSON-RPC frames; all logging/diagnostics to stderr (non-negotiable LSP-style discipline)
- **Compatibility**: Claude Code config layout drop-in (existing `.claude/` setups work unchanged)
- **Distribution**: Static binary via goreleaser; ACP registry manifest; no daemon, no network port (the editor owns process lifecycle)
- **Platform**: macOS + Linux, amd64 + arm64 — Windows deferred
- **Safety model**: No tool-execution confirmation tier; the pattern/hook table (no match → nothing runs) plus manual cancellation is the only safety mechanism
- **Investigate-and-fix-ready logging (must-have)**: ass-guard must log every problem it encounters — errors, panics, failed tool calls, provider failures, engine misfires, unexpected states — in a form detailed enough that a developer reading the transcript can diagnose the root cause and fix it. Not just "an error occurred" but the context, the inputs, the failure point, and the recoverable/non-recoverable classification. The transcript is the primary diagnostic surface (corollary of Phase 2 D-03/D-20: transcript = human-investigation artifact); every problem must be investigate-able from the transcript alone.
- **Phase completion gate (non-negotiable)**: every phase MUST end with `mise ci` (go vet + golangci-lint v2 all-linters + CGO_ENABLED=0 go build + go test -race) passing clean — zero lint issues, zero vet issues, zero test failures, zero build failures. No phase is marked complete until this gate passes. No exceptions, no "fix it later," no disabling linters for noisiness. The `.mise.toml` and `.golangci.yml` configs define the gate; `mise ci` is the command.

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Fresh build; `sdd-acp-agent` is reference-only (no code port) | Predecessor's scope was narrower; its code carries decisions that don't fit the mimicry + scheduling + hooks + Telegram scope. Building fresh against the new scope is cleaner than retrofitting. | ✓ Good — v1.0 shipped clean in 6 days |
| Mimicry is the north star, not a feature | Model behavior diverges if requests aren't structurally identical; everything downstream is meaningless if the model doesn't behave as in the target agent. Validated first, on the thinnest possible stack. | ✓ Good — A/B parity thesis proven (Phase 1) |
| Autocontinue is the upper mechanism; hooks are a special case | Collapses two predecessor ideas into one decision point after turn-complete, avoiding a two-mechanism design and unifying learning under one engine. | ✓ Good — unified engine shipped (Phase 4), structural safety proven |
| Profiles are config (zcode first, architecture for N) | "Mimic under any agent" is an explicit goal; baking zcode as the only profile would force a rewrite later. | ✓ Good — profile #2 candidate (deepseek-harness) already scoped without architecture change |
| Profile content is log-extracted, not hand-written | Grounded mimicry: the only honest source of "what does zcode actually send" is zcode's own request logs. Hand-written profiles are guesses. | ✓ Good — ⚠️ revisit: pinned capture session absent; re-capture needed for the stability test |
| ACP primary, Telegram secondary-but-peer | ACP is the IDE-native working surface for code; Telegram extends reach (mobile, voice, async) without duplicating the IDE experience. Both are full-capability fronts to one core. | — Pending — ACP shipped (Phase 2); Telegram deferred to v2 |
| v1 toolkits = OpenSpec only | OpenSpec has the deepest reference; GSD/spec-kit/BMad accommodated by the adapter interface, implemented later. | ✓ Good — ⚠️ revisit: hosting model corrected at UAT (workflow is command-file-driven, binary is tooling — see Active) |
| Credentials resolve lazily at factory construction, never at load (Phase 7 D-02) | A load-fatal on unset `$ZAI_API_KEY` would break zero-config first run; lazy resolution + typed structural error preserves both. | ✓ Good — proven live (zero-env editor-spawned turn, Phase 7 UAT) |
| opencode excluded as mimicry target (2026-08-14) | Operator runs MiniMax + DeepSeek via an opencode subscription; the profile should match the harness the target model behaves in — deepseek-harness is the candidate for DeepSeek turns. | — Pending — profile #2 scoping at next milestone |
| Chip truthfulness: runner default follows the tier resolution; profile slug leaves the precedence chain (Phase 16-09, operator-confirmed) | The model chip must show what the wire will use; a mimicry-leftover profile slug silently overriding config broke that. Explicit editor stamps keep absolute precedence (D-12). Phases 18/20 build on this. | ✓ Good — UAT 2026-09-01, pinned by TestDefaultTurnModel_FollowsTierResolution |
| `_meta` blob fills are in-memory only; layer files stay operator-owned (Phase 16 D-10, operator-confirmed) | A redundant Zed re-push must never become persisted operator config, and a blob fill must never mutate files the operator owns — ownership boundary drawn at the layer files. | ✓ Good — UAT 2026-09-01 (blob-tier override accepted as product intent) |
| Wire shapes pin to the canonical schema, and the test client models the REAL client (Phase 17 G-17-1 lesson) | A flat-string decode of the request_permission outcome passed the whole E2E battery because the simulator answered the same wrong shape — tests were green against a nonconformant client while live Zed failed every dialog answer. Canonical truth comes from the spec (agentclientprotocol.com schema), and simulator answers must mirror the real client's bytes, never our own codec. | ✓ Good — fixed by 17-06 gap closure (nested outcome, simulator corrected, flat shape now hard-rejected); found by live UAT 2026-09-03 |
| Permission-dialog persistence is bare tool×project (Phase 17 D-01, operator-confirmed) | One Always-click persists the whole tool for the project; richer patterns (paths, prefixes) are hand-edit-only — the dialog never writes them. Keeps the trust store predictable and the UX honest about scope. | ✓ Good — live-verified both directions (allow + deny) 2026-09-03 |
| Fixtures must model the real wire shape, never a hand-written fiction (Phase 18 G-18-1, the G-17-1 class) | The entire session-list battery was green while real transcripts never matched the fixture shape (real sessions didn't write the session_start opener — AppendSessionStart had zero production callers). RED pins must encode the shape production actually produces, or tests certify a fiction. | ✓ Good — fixed by 18-07 (opener wired + tolerant listing + real-shape pins); found by live picker UAT 2026-09-06 |
| `--resume` resolution is cwd-scoped (Phase 18, operator-accepted deviation) | The per-directory `.ass-guard/` store is the unit of trust; a cross-project registry (Claude Code-style) would be a new store design beyond Phase 18's scope. Not-found errors name the directory searched. | ✓ Accepted — operator sign-off 2026-09-06 |
| Schema-invalid hook output is fail-open by design (Phase 21 hookverdict D-02) | Hooks are a policy channel, not a confirmation tier: only the two documented dialects (hookSpecificOutput permissionDecision JSON, legacy exit-2) carry authority; anything else yields VerdictNone and the call proceeds. A garbage hook script must not brick the agent. | ✓ Good — live-verified 2026-09-06 (invalid dialect fail-open, corrected dialect denies with reason) |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd-transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd:complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-09-06 after Phase 21 (Context & Policy Parity Closures) closed — hooks deny joins the one gate pipeline, memory auto-inject, thinking byte-identity, @-mentions/images with ingress validation; UAT 4/4 (hook-deny retest on the corrected dialect; fail-open on invalid hook JSON confirmed by design). Same day: Phase 18 (Session Family) closed 7/7 with G-18-1 gap closure*
