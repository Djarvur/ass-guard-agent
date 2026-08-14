# Roadmap: ass-guard-agent (working name)

**Project mode:** mvp (vertical slices — each phase delivers an end-to-end user capability)
**Phases:** 7 (Phase 0 spike + Phases 1-6 delivery)
**Requirements mapped:** 67/67 v1 ✓

The north star is **mimicry**: outgoing model requests must be structurally indistinguishable from the mimicked agent (zcode first). Phase 1 proves this thesis on the thinnest possible stack. If the Phase 1 behavioral A/B parity test fails, the project stops and re-plans — nothing downstream is built on an unvalidated thesis. This ordering is non-negotiable (PROJECT.md Anti-Pattern 5; PITFALLS N18).

The six deltas (mimicry, multi-tier scheduling, configurable backends, learning mode, unified engine, Telegram peer) are **serialized** across phases, never thin-sliced together — risk-multiplication, not risk-reduction (PITFALLS N18).

## Phase Overview

| # | Phase | Goal | Requirements | Success Criteria |
|---|-------|------|--------------|------------------|
| 0 | Spike + Re-verification ✅ **COMPLETE 2026-08-09** (5/5 plans; VERIFIED-FACTS.md authored + gated + sanitized; STACK.md unchanged) | The team can trust every inherited fact (5 weeks old) before building on it | — (no REQ-IDs; closes 5 Phase-0 research flags) | 3 |
| 1 | Mimicry MVP (north-star proof) | 0/6 | Planned    |  |
| 2 | Session Core + ACP Interface | 7/7 | Complete   | 2026-08-11 |
| 3 | Model Scheduling | An operator can configure heavy/good/light tiers with time-windowed model substitution, per-project overrides, and fallback chains, and ass-guard picks the right model at request time without the developer noticing | SCHED-01, SCHED-02, SCHED-03, SCHED-04, SCHED-05, SCHED-06 | 4 |
| 4 | Unified Engine + Hook-DAG + OpenSpec + Learning | A developer can run an unmodified OpenSpec scenario end-to-end through ass-guard with zero manual "continue" taps, while the forgotten routine (tests, lint, review, memory, improvement proposals) runs automatically after each stage | ENG-01, ENG-02, ENG-03, ENG-04, ENG-05, HOOK-01, HOOK-02, HOOK-03, HOOK-04, HOOK-05, LRN-01, LRN-02, LRN-03, LRN-04, OPEN-01, OPEN-02, OPEN-03, TOOL-04, TOOL-05 | 5 |
| 5 | Ecosystem Compatibility | A Claude Code user can drop their existing `.claude/` setup (MCP servers, skills, slash-commands, plugins) into ass-guard and have it work unchanged, alongside ass-guard's own additions | ECOS-01, ECOS-02, ECOS-03, ECOS-04, ECOS-05 | 4 |
| 6 | Distribution + Polish | 2/2 | Complete   | 2026-08-13 |
| 7 | Multi-Provider Config & Credentials | 2/2 | Complete    | 2026-08-14 |

## Phase Details

### Phase 0: Spike + Re-verification

**Goal:** The team can trust every load-bearing inherited fact before building on it — closing the 5 Phase-0 research flags raised in STACK.md, so that no stale predecessor fact (5 weeks old as of 2026-08-09) silently corrupts the build.
**Mode:** mvp
**Requirements:** — (verification phase; closes the Phase-0 spike checklist from research, not REQ-IDs)
**Plans:** 5/5 plans executed (4 in Wave 1+2 run in parallel — each owns its own `spikes/0N-*/` subdir; Plan 05 in Wave 3 is the single writer of VERIFIED-FACTS.md). **Phase 0 COMPLETE 2026-08-09.**
Plans:
**Wave 1**

- [x] 00-01-PLAN.md — spikes module skeleton + #1 zcode JSONL capture + #4 STT structural closure

**Wave 2** *(blocked on Wave 1 completion)*

- [x] 00-02-PLAN.md — #2 go-openai tool-calling schema spike (MiniMax M3 + Groq)
- [x] 00-03-PLAN.md — #3 ACP v1 handshake spike (initialize → session/new → session/prompt)
- [x] 00-04-PLAN.md — #5 go-telegram/bot + ACP stdout-collision integration spike

**Wave 3** *(blocked on Wave 2 completion)*

- [x] 00-05-PLAN.md — author VERIFIED-FACTS.md from the 4 evidence files + §3 Tier-B decision checkpoint + completeness gate

**Success Criteria:**

1. The exact on-disk path and JSONL line schema of zcode's transcripts is located, documented, and a sample log captured (closes STACK Phase-0 item #1; MIMC-02's ground truth)
2. The latest pinned tag of `sashabaranov/go-openai` is confirmed and its tool-calling schema fidelity verified per OpenAI-shape provider (MiniMax M3, Groq) (closes STACK Phase-0 item #2)
3. The ACP v1 method names and wire shape are confirmed against the canonical spec, and the `go-telegram/bot` + ACP stdout-collision integration test demonstrates the two transports don't fight over stdout (closes STACK Phase-0 items #3 and #5; confirms the LSP-style stdout discipline)
4. Every re-verified fact is recorded as `{fact, source, verified_date, verified_against_version}` so Phase 1 builds on grounded, dated evidence (PITFALLS N18 mitigation)

### Phase 1: Mimicry MVP (north-star proof)

**Goal:** The team can send a fixed prompt suite through ass-guard (with the zcode profile loaded) and the model produces tool-call sequences statistically indistinguishable from live zcode — the mimicry thesis is empirically proven before anything downstream is built. If the A/B parity test fails, the project stops and re-plans.
**Mode:** mvp
**Requirements:** MIMC-01, MIMC-02, MIMC-03, MIMC-04, PROF-01, PROF-02, PROF-03, PROF-04, PROF-05, TOOL-01, TOOL-02, TOOL-03, PROV-01, PROV-02, PROV-03, LOG-01
**Success Criteria:**

1. A user can select the zcode profile by name and send a prompt through ass-guard; the outgoing model request carries zcode's system-prompt composition, tool-catalog declaration, message-block shape, and identity fields (MIMC-01, PROF-01, PROF-02 — single mimicry chokepoint at the Profile Shaper, no zcode-specific code paths)
2. The zcode profile is extracted from zcode's real on-disk JSONL transcripts (Phase 0's verified path), not hand-written, and carries a `target_capture_ref` and coverage manifest so drift and incomplete capture are visible, not silent (MIMC-02, PROF-03, PROF-05)
3. A user can run `ass-guard profile check zcode` and the drift detector reports whether the target agent's observed requests still match the captured profile (PROF-04 — north-star killer N1 engineered out from day one)
4. The behavioral mimicry A/B parity test passes: a fixed prompt suite run through both ass-guard-with-zcode-profile and live zcode produces statistically indistinguishable tool-call sequences, with the byte-identical bar explicitly documented as out of scope (MIMC-03, MIMC-04 — the gate)
5. A developer can inspect the audit log and see the verbatim shaped outgoing request for every turn — the mimicry evidence source is recorded from day one (LOG-01; foundation that Phase 2 completes)

Plans:
**Wave 1**

- [ ] 01-01-PLAN.md — tracer: profile → shaper → Anthropic adapter → Z.ai → tool-call (the seam)

**Wave 2** *(blocked on Wave 1 completion)*

- [ ] 01-02-PLAN.md — profile fidelity: extractor, coverage manifest, tier assignment, target_capture_ref
- [ ] 01-03-PLAN.md — built-in tool catalog + schema-adapter layer + catalog-consistency CI

**Wave 3** *(blocked on Wave 2 completion)*

- [ ] 01-04-PLAN.md — Shaper completeness, PROF-02 enforcement, OpenAI-shape adapter, provider conformance
- [ ] 01-05-PLAN.md — audit log + event bus + drift detector + `ass-guard profile check zcode`

**Wave 4** *(blocked on Wave 3 completion)*

- [ ] 01-06-PLAN.md — the A/B parity test: curated divergence suite + two-layer metric + the gate

### Phase 2: Session Core + ACP Interface

**Goal:** A developer can spawn ass-guard from Zed via ACP, send a prompt, watch the model's output stream token-by-token, restart Zed, and have the session replay correctly — all while the model sees a clean, lean context window at each command boundary. ass-guard becomes usable as a daily coding agent inside the IDE.
**Mode:** mvp
**Requirements:** SESS-01, SESS-02, SESS-03, SESS-04, SESS-05, SESS-06, ACP-01, ACP-02, ACP-03, ACP-04, ACP-05, LOG-02, LOG-03, LOG-04, PARA-01, PARA-02, PARA-03, PARA-04
**Success Criteria:**

1. A developer can spawn ass-guard from Zed via the ACP registry, send a prompt, and receive streamed `session/update` notifications end-to-end (provider SSE → event bus → ACP), with no full-turn buffering before display (ACP-01, ACP-02, ACP-04, ACP-05)
2. After restarting Zed, the developer sees the prior session replayed correctly — the durable transcript rehydrates and the projector resets the model-visible window at the most recent pre-rehydration boundary (ACP-03, SESS-01, SESS-05, SESS-06)
3. A mutating toolkit command (Bash, Write, Edit) always resets the model's context window to a lean seed, regardless of config — the developer never has to manage context hygiene manually, and the structural mutability rule (more-mutating wins) is the single source of truth (SESS-02, SESS-03, SESS-04)
4. A developer can dispatch a `Task`/`Agent` subagent that runs as an isolated goroutine turn-loop with a restricted tool subset; if it panics, the parent gets a tool-error result (not a crash), and the audit log records the full sequence with secrets redacted and rotation preventing volume explosion (PARA-01, PARA-02, PARA-03, PARA-04, LOG-02, LOG-03, LOG-04)

> **Note on criterion #2 + ACP-03 (D-09 v1 cut-line):** per Phase-2 CONTEXT.md D-09, session/load REPLAY is DROPPED from v1. The transcript's primary purpose is human investigation (D-03/D-20), not editor replay. `session/load` is a no-op/error (`loadSession: false` advertised). Criterion #2's replay portion is therefore out of v1 scope; the transcript persistence for human investigation + boundary-marker durability for live projection (SESS-05) remain in scope.

**Plans:** 7/7 plans complete
Plans:

- [x] 02-01-PLAN.md — Tracer: ACP v1 stdio server (initialize → session/new → session/prompt → streamed session/update) with hand-rolled framing (ACP-01/02/04/05)
- [x] 02-02-PLAN.md — Session Core: transcript (one artifact) + projector (lean window) + turn loop (SESS-01/04/05/06, LOG-02/03)
- [x] 02-03-PLAN.md — Mutability declaration interface (per-tool field + SESS-03 formula) + runtime tool restriction (SESS-02/03, PARA-01)
- [x] 02-04-PLAN.md — Streaming infrastructure: expanded event bus (typed channels) + Provider.Stream + semaphore (ACP-04, PARA-04, LOG-02)
- [x] 02-05-PLAN.md — Integration: real Session Core in ACP + streaming wired + session/load no-op (D-09) + cancel end-to-end (D-16) + boundaries (ACP-03, ACP-04, PARA-04)
- [x] 02-06-PLAN.md — Subagent dispatch: isolated goroutine turn-loops, streamed progress, panic recovery (PARA-01/02/03)
- [x] 02-07-PLAN.md — Reconstruction-sufficiency test (LOG-04) + transcript-writer audit fold + end-to-end session gate (LOG-04, LOG-02)

### Phase 3: Model Scheduling

**Goal:** An operator can configure ass-guard to use cheap models off-peak and heavy models at peak hours, override the tier→model table per project, and trust that a provider outage degrades gracefully instead of cascading — while the developer never thinks about which concrete model is running.
**Mode:** mvp
**Requirements:** SCHED-01, SCHED-02, SCHED-03, SCHED-04, SCHED-05, SCHED-06
**Success Criteria:**

1. A command, skill, or subagent selects a tier (heavy/good/light), not a concrete model; the Model Scheduler resolves the tier to a concrete provider+model at request time (SCHED-01)
2. An operator can configure time-windowed tier→model substitution (e.g. heavy = glm-5.2 normally, minimax-m3 in peak) with timezone-explicit IANA zones, and a per-project override that falls back to the config default when unset (SCHED-02, SCHED-03)
3. On a transient provider failure (429/5xx/network), ass-guard walks a configured fallback chain (degrade tier OR walk the chain); on a structural failure (401/403), it reports instead of silently retrying (SCHED-04 — N5 engineered out)
4. Circuit breakers and cost ceilings stop fallback-chain cascading failures and cost surprises, and each (provider, tier) pair has a documented capability profile so tier mismatch across providers is explicit (SCHED-05, SCHED-06)

### Phase 4: Unified Engine + Hook-DAG + OpenSpec + Learning

**Goal:** A developer can run an unmodified OpenSpec workflow (the v1 toolkit) end-to-end through ass-guard with zero manual "continue" taps, while the forgotten routine runs automatically after each stage — and when the agent encounters an unfamiliar launch situation, it asks the user once, remembers the answer, and never asks the same question again. This is the project's reason to exist for the author's SDD practice.
**Mode:** mvp
**Requirements:** ENG-01, ENG-02, ENG-03, ENG-04, ENG-05, HOOK-01, HOOK-02, HOOK-03, HOOK-04, HOOK-05, LRN-01, LRN-02, LRN-03, LRN-04, OPEN-01, OPEN-02, OPEN-03, TOOL-04, TOOL-05
**Success Criteria:**

1. A developer runs an unmodified OpenSpec scenario through ass-guard; it advances stage-to-stage with zero "continue" taps (dual-signal: text-pattern OR known handoff tool-call), and unmatched output triggers nothing — the structural safety property holds, with manual cancellation draining queued injections (ENG-01, ENG-02, ENG-03, ENG-05, OPEN-01, OPEN-02, OPEN-03)
2. After the implement stage, the seeded hook set runs automatically — tests, lint, review, and memory — and after a phase, improvement proposals are generated; if the engine fails, the turn still completes and the agent degrades to manual-continue (HOOK-01, HOOK-02, HOOK-05, ENG-04)
3. A hook failure never silently blocks the workflow — every hook declares `on-failure: halt|continue|ask` — and provenance tagging structurally prevents hook-DAG infinite loops (a hook can't re-trigger its own stage) (HOOK-03, HOOK-04)
4. When the engine doesn't know how to launch what should be launched, it asks the user (fresh context? wait? how long?), remembers the answer after ≥3 confirmations, and proposes new hooks from the work log that the user can accept or reject — all learned settings are versioned and revertible (LRN-01, LRN-02, LRN-03, LRN-04)
5. Read-only tools (Glob, Grep, Read) execute concurrently within a turn while mutating tools serialize relative to each other, and complex tools (WebSearch, WebFetch) use a swappable backend configured without code changes (TOOL-04, TOOL-05)

### Phase 5: Ecosystem Compatibility

**Goal:** A Claude Code user can take their existing `.claude/` setup — installed MCP servers, skills, slash-commands, and plugins — and have it work unchanged inside ass-guard, with ass-guard's own additions namespaced cleanly alongside. The full Claude-Code ecosystem is drop-in.
**Mode:** mvp
**Requirements:** ECOS-01, ECOS-02, ECOS-03, ECOS-04, ECOS-05
**Success Criteria:**

1. A Claude-Code-installed MCP server runs in ass-guard: spawned as a subprocess via `StdioMCPClient`, its tools bridged into the catalog and callable by the model (ECOS-01)
2. MCP server subprocesses are lifecycle-robust — process-group spawn, group-signal shutdown, reaper goroutine prevent zombies; and `tools/list` is re-fetched on every connection so schema drift can't accumulate (ECOS-02, ECOS-03 — N13 and N14 engineered out)
3. A Claude Code user's skills, slash-commands, and plugins load from the `.claude/` layout and work unchanged in ass-guard (ECOS-04)
4. ass-guard's own additions namespace under `.claude/` cleanly with user→project precedence respected, never clobbering a Claude Code user's files (ECOS-05)

### Phase 6: Distribution + Polish

**Goal:** A team can install ass-guard via the ACP registry with a single command and, on first launch, have a working zero-config setup — default model, pre-seeded zcode profile, pre-seeded openspec.toml — ready to run an OpenSpec workflow with no further configuration.
**Mode:** mvp
**Requirements:** DIST-01, DIST-02, DIST-03
**Success Criteria:**

1. A team downloads a single static Go binary (no runtime deps) for macOS or Linux, amd64 or arm64, produced by goreleaser (DIST-01)
2. A team adds ass-guard to Zed via `zed: acp registry`; the `agent.json` manifest declares `command`/`command_args`/`cwd` so Zed spawns the agent correctly (DIST-02)
3. On first launch after registry install, ass-guard runs with sensible defaults — a default model, a pre-seeded zcode profile, and a pre-seeded openspec.toml — and a developer can immediately run an OpenSpec workflow with zero configuration (DIST-03)

## Dependency Chains

- **Phase 1 gates everything.** Mimicry is a thesis; validating it on the thinnest stack is the deliberate de-risking. No downstream phase proceeds until the A/B parity test passes.
- **Phase 2 needs Phase 1.** Session Manager needs the Turn Loop; ACP Adapter needs the Session Manager; the Audit Log's verbatim shaped-request recording (Phase 1's LOG-01) is completed here as a first-class async consumer.
- **Phase 3 needs Phases 1-2.** Scheduling needs the Profile Shaper (fallback re-shaping when a chain crosses providers) and the wired ACP surface. Per-task routing is the substrate; tiers/windows/chains layer on top.
- **Phase 4 needs Phases 1-3.** The Unified Engine needs context hygiene (FreshContext step needs boundary semantics from Phase 2); the Hook-DAG needs the engine + a stable scheduling layer so seeded hooks run reliably.
- **Phase 5 needs Phase 2 (stable tool registry).** MCP hosting bridges MCP tools into the catalog — it does not depend on the engine. Limited parallelism is possible within Phase 5 (MCP hosting vs the rest), but Phase 5 follows Phase 4 in the serialized-deltas discipline.
- **Phase 6 needs all functional deltas validated.** Ship readiness comes after the v1 cut-line is met.

## v1 Cut-Line (fixed)

- **Toolkits:** OpenSpec only (GSD / spec-kit / BMad deferred to v2; interface accommodates them)
- **Interfaces:** ACP primary only (Telegram peer → v2)
- **Ecosystem:** Full Claude-Code compatibility (MCP + plugins + skills + commands)
- **Platforms:** macOS + Linux, amd64 + arm64 (Windows deferred)
- **Profiles:** zcode only (architecture supports N from day one; additional profiles → v2)
- **Build:** Fresh (sdd-acp-agent is reference-only — no code port)

## Research Flags by Phase

Phases likely needing deeper research during `/gsd:plan-phase`:

- **Phase 0:** Mechanical verification of the 5 STACK Phase-0 items. Low research depth.
- **Phase 1 (highest research depth):** The mimicry A/B parity test methodology is itself a research question (what prompt suite, what tolerance band, what statistical comparison). The profile artifact format, coverage manifest design, and zcode JSONL line schema all need explicit design before implementation.
- **Phase 3:** Fallback-chain semantics under correlated failure (N5) and time-window timezone handling (N6) need explicit design. LiteLLM's router is a design template worth studying (NOT a dependency — it's a Python proxy).
- **Phase 5:** Per-interface threat model (N15) is a PROJECT.md decision: ACP ungated vs Telegram-gated allowlist for mutating MCP tools. (Telegram itself is v2, but the MCP threat-model decision lands here since MCP is v1.)
- **Phases 2, 4, 6:** Standard patterns; skip dedicated research phase.

### Phase 7: Multi-Provider Config & Credentials

**Goal:** An operator can declare multiple model providers in a single config — each with its base URL, protocol shape (anthropic/openai), and credential — and multiple models per provider with capability/pricing metadata. ass-guard resolves a scheduler tier to a fully-credentialed provider+model instance at request time. Credentials read from config so editor-spawned processes (Zed → `ass-guard acp serve`) work with zero env vars, while an explicit env var (or CLI flag) still overrides for CI/operators. This completes the multi-provider promise of PROV-01 and connects SCHED tier-resolution to real provider instances (today only one provider/model is wired and the key is env-only).
**Mode:** mvp
**Requirements:** completes PROV-01 (configurable base URL per provider); new PCFG-01.. (provider/model/credential config schema + loader, credential precedence + redaction, scheduler→provider credentialed-instance wiring) — REQ-IDs finalized in discuss
**Depends on:** Phase 3 (scheduler tier→model resolution + scheduling.yaml schema), Phase 6 (`.ass-guard/` config dir + go:embed first-run seed + redactor)
**Success Criteria:**

1. An operator declares ≥2 providers (e.g. Z.ai anthropic-shape + an OpenAI-shape provider) each with `base_url` + `shape` + `api_key` in `.ass-guard/` config; ass-guard reads credentials from the file (no env var required), so a Zed-spawned `ass-guard acp serve` makes a real model turn with zero environment. (PCFG — config + credentials)
2. An operator declares ≥2 models per provider; a scheduler tier→model table entry resolves to a (provider, model) pair and ass-guard constructs the correct provider instance — right base_url + right key + right shape — for the resolved pair. (PROV-01 + scheduler→provider wiring)
3. Credential precedence is explicit and tested: config-file credential is the fallback when the env var is unset; an explicit env var (or `--api-key` flag) overrides the config value; secrets never reach logs (redactor already covers `api_key`/`sk-`/`Bearer`; config file is gitignored and ass-guard warns if looser than 0600). (security model)
4. Existing single-provider behavior keeps working unchanged: the embedded default seeds the current Z.ai GLM-5.2 provider, and an operator who changes nothing still gets today's behavior (`$ZAI_API_KEY` still works). (zero-config / backward compat)

Plans:

**Wave 1**

- [x] 07-01-PLAN.md — credential schema + precedence resolver + provider factory (PCFG-01..04 core; tracer slice: config→loader→factory→correct credentialed instance)

**Wave 2** *(blocked on Wave 1 completion)*

- [x] 07-02-PLAN.md — wire factory into acp serve/tracer/parity + D-07 startup warnings + 0600 perm + zero-config/zero-env proofs + traceability (PCFG-02..04, PROV-01)

---

*Roadmap created: 2026-08-09*
*Derived from: PROJECT.md, REQUIREMENTS.md (67 v1 REQ-IDs), research/SUMMARY.md (Phase 0-6 proposal)*
