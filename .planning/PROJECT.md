# ass-guard-agent (working name)

## What This Is

A Go-based AI coding agent that makes SDD (Spec-Driven Development) workflows run hands-off by automatically doing the routine work a developer keeps forgetting to ask for. It speaks to model providers with requests structured like another agent (the first mimicry target is zcode — a claude-code-compat runtime shipping its own `AGENTS.md`, skills, commands, MCP, and tool catalog), so the model behaves identically to how it behaves in the mimicked agent. The agent hosts unmodified SDD toolkits (OpenSpec in v1), drives them to completion without manual "continue" taps, and runs configurable post-stage routines (review / memory / tests / linters / improvement proposals) on its own. Primary interface is ACP (IDE-native, e.g. Zed); Telegram is a planned full peer surface (text + voice, v2). Built for SDD-capable teams who want the toolkit to just run.

## Core Value

Outgoing requests to the model provider must be structurally indistinguishable from the mimicked agent's (zcode first) — if the model can tell the requests apart, everything built on top is compromised, because model behavior diverges. Every other capability (autocontinue, hooks, scheduling, interfaces) is downstream of this.

*Validated v1.0:* the Phase-1 A/B parity test proved the thesis — ass-guard's shaped requests (3 byte-identical system blocks, 103-tool catalog, thinking/tool_choice/stream fields) produce statistically indistinguishable tool-call sequences from live zcode.

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
- Unified engine + Hook-DAG + learning (Phase 4 — ENG-01..05, HOOK-01..05, LRN-01..04, OPEN-01..03, TOOL-04/05): post-turn observer deciding continue/hook/ask/wait; dual-signal handoffs (text-pattern OR tool-call); structural safety (unmatched ⇒ nothing, testing/quick + E2E); cancel-drain at ACP level; hand-rolled ≤300-LOC DAG executor with on-failure halt/continue/ask + provenance loop prevention; learning store (ask-once-remember, propose-hooks, versioned, revertible via `ass-guard learning list/revert`); read-only tools concurrent / mutating serialized; swappable WebSearch/WebFetch backends. *Caveat carried: live OpenSpec kickoff requires slash-command invocation — see Active.*
- Multi-provider config & credentials (Phase 7 — PCFG-01..04, completes PROV-01): provider+model declarations with base URL, protocol shape (anthropic/openai), per-model capability/pricing metadata; credential resolution flag > env > config with `${VAR}` expansion; two-shape ProviderFactory wired at all three construction sites; lazy uncredentialed failure (typed structural error); 0600-perm hygiene warning + uncredentialed-provider warnings on stderr. Proven live: zero-env editor-spawned turn authenticating from config file.
- Ecosystem discovery + MCP hosting (Phase 5 — ECOS-01/02/03/05): MCP servers hosted as robust subprocesses (process-group spawn, group-signal shutdown, reaper, tools/list re-fetch per connection); skills/commands/plugins discovered from `.claude/` (project + user) merged with `.ass-guard/` additions under explicit precedence, strictly read-only on `.claude/`.
- Audit logging (Phase 1/2 — LOG-01, tracer-wired): redacted request logging via the tracer path with redactor. *Caveat carried: `--audit-log` is not written on the `acp serve` path — the tracer is wired through main.go; closing the acp-serve audit path is next-milestone material.*

### Active

**Close the OpenSpec kickoff surface (from v1.0 UAT gap, 2026-08-14)**

- [ ] Wire `internal/ecosys` command/skill discovery into the session/ACP layer: expose loaded slash-commands and expand `/namespace:name` invocations (e.g. `/opsx:explore`) into the command's markdown prompt before the provider turn
- [ ] Reconcile the OpenSpec adapter command set with the real `openspec` v1.5.0 binary surface (`list/view/change/spec/archive/doctor/context`, not `show/validate/apply/implement`); the toolkit's workflow is driven by agent-executed command files (`openspec init --tools claude` installs `.claude/commands/opsx/*.md` + skills), with the binary as supporting tooling
- [ ] Run the operator-gated real-binary test (`ASSGUARD_OPENSPEC_BIN=1`) as a fix gate; then complete the 11 deferred Phase-4 UAT checks

**Mimicry follow-ups**

- [ ] Re-capture a divergence-prone zcode session to unblock the parity stability test (Phase-1 data-source carry-forward)
- [ ] Profile #2 candidate: `deepseek-ai/deepseek-harness` ("dsh" — DeepSeek's official agent harness) for DeepSeek-model turns; ground-truth capture from source + its own logs (operator direction, 2026-08-14)
- [ ] Claude-Code slash-commands/plugins must not just load but *work unchanged* end-to-end in ass-guard (ECOS-04 completion — invocation surface)

**Remaining v1 vision (unaddressed)**

- [ ] Telegram peer interface: full SDD-scenario driving (text + voice), voice transcribed via configurable STT backend (v2)
- [ ] LOG-01 completion: audit log written on the `acp serve` path

### Out of Scope

- Windows platform support — v1 macOS+Linux only; Windows deferred (win-developer teams are not the first target)
- GSD / spec-kit / BMad toolkit adapters in v1 — OpenSpec only; interface accommodates them, implementation deferred
- Perplexity or other paid search backends — configurable backend design replaces the hardcode, but no specific paid integrations are committed
- Byte-for-byte request identity with the mimicked agent — "structurally indistinguishable to the model" is the bar, not binary diff equality (cosmetic field ordering/optional fields allowed)
- A standalone CLI surface — ACP (IDE) and Telegram are the only interfaces; no terminal REPL to maintain
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
*Last updated: 2026-08-14 after v1.0 milestone*
