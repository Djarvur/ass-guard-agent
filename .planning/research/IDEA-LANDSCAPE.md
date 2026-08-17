# IDEA-LANDSCAPE — The "ideal agent" idea landscape vs the ass-guard plan

> Captured 2026-08-17. Companion to SEED-002 (fantasy deep-dive) and SEED-003 (Go landscape).
> Method: repo-verified survey of 34 sources (operator's list of 12 + fantasy + 21 new finds
> + exclusions), cross-referenced against a codebase inventory of ass-guard's implemented
> packages, roadmap phases, and rejected-items list. Star counts/activity are point-in-time.

## The source list (expanded from the operator's 12)

**Operator's original list:** ass-guard-agent, opencode agent (sst→anomalyco, TS, active,
~198k★), pi agent (earendil-works/pi, TS, ~91k★), justxor/Claudecourse (RU-language Claude-Code
internals guide, 164★), cloudwego/eino, google/adk-go, GoAI SDK (zendev-sh/goai),
mozilla-ai/any-llm-go, tmc/langchaingo, opencode-ai/opencode (Go, ARCHIVED → Crush),
charmbracelet/crush (FSL-1.1-MIT — read-only!), plandex-ai/plandex. Plus charmbracelet/fantasy
(SEED-002).

**New finds (verified 2026-08-17):**

| # | Project | Lang | ★ | License | One-line idea value |
|---|---|---|---|---|---|
| 1 | openai/codex | Rust | 106k | Apache-2.0 | OS-sandbox × approval-policy matrix; one engine, many frontends |
| 2 | cline/cline | TS | 66k | Apache-2.0 | Plan/Act modes; shadow-git checkpoints; broadest surface fan-out |
| 3 | aaif-goose/goose | Rust | 53k | Apache-2.0 | Everything-is-MCP; custom distros; recipes |
| 4 | OpenHands/OpenHands | Py/TS | 84k | MIT | Event-stream core (replay/fork); pluggable condensers; Agent/Automation Server pivot |
| 5 | google-gemini/gemini-cli | TS | 107k | Apache-2.0 | Extension packaging; behavioral evals in-repo; headless stream-json |
| 6 | QwenLM/qwen-code | TS | 27k | Apache-2.0 | Auto-Memory + Auto-Skills; Agent Arena; `serve` daemon (ACP over HTTP) |
| 7 | github/gh-aw | TS | 5k | MIT | Agents-as-CI; buffer-then-apply safe outputs; workflow lockfile |
| 8 | Aider-AI/aider | Py | 48k | Apache-2.0 | Tree-sitter+PageRank repo map; edit-format negotiation; architect mode |
| 9 | SWE-agent/SWE-agent | Py | 20k | MIT | ACI research: interface design IS the capability variable |
| 10 | openai/openai-agents-python | Py | 29k | MIT | Minimal primitives (agent/handoff/guardrail/session); sandbox agents |
| 11 | anthropics/claude-agent-sdk-python | Py | 8k | MIT | Harness-as-library; in-process MCP servers; hooks as app-level interception |
| 12 | strands-agents/harness-sdk | Py/TS | 7k | Apache-2.0 | Steering handlers; pre-execution guardrails; loop-traces-everything |
| 13 | google/adk-python | Py | 21k | Apache-2.0 | Workflow Runtime; evalset files; versioned session schemas (2.0 reads 1.28) |
| 14 | microsoft/agent-framework | Py/.NET | 13k | MIT | Checkpointing + time-travel; middleware; durability as extension |
| 15 | langchain-ai/langgraph | Py/JS | 40k | MIT | Durable graph state; checkpointer; HITL interrupts |
| 16 | pydantic/pydantic-ai | Py | 19k | MIT | Typed agents; deferred capability loading ("model pulls tools like a skill") |
| 17 | huggingface/smolagents | Py | 29k | Apache-2.0 | Code-as-action; ~1k-line minimal loop; agents as Hub artifacts |
| 18 | letta-ai/letta-code | TS | 3k | Apache-2.0 | Memory blocks as editable state; MemFS (git-backed memory); sleeptime |
| 19 | mastra-ai/mastra | TS | 27k | Apache-2.0/+EE | Typed workflow DSL; suspend/resume; MCP-server authoring |
| 20 | agno-agi/agno | Py | 42k | Apache-2.0 | AgentOS: multi-tenant agent runtime; in-process cron; 50+ endpoint API |
| 21 | crewAIInc/crewAI | Py | 57k | MIT | Role-based crews; Flows (deterministic pipeline vs crew execution) |

**Checked and excluded:** autogen + swarm (stagnant, superseded), Semantic Kernel (merged into
ms/agent-framework), Roo/Kilo Code (Cline duplicates), mentat (dead), vibe-kanban (orchestrator
not agent, stalled), strands sdk-go (404 — Go SDK retired), zed (not an agent; ACP reference),
mem0/llama_index/rig (adjacent layers), Amp/Droid/Cursor (closed).

---

## The four-column analysis

Legend: ✅ = we already implemented an equivalent · 📋 = in our plan (phase noted) ·
🧲 = gap → borrow (ranked in next section) · 🚫 = we deliberately don't need (rejection reason).

### Category 1 — Our direct peers (coding agents, Go + the two big TS ones)

| Source | ✅ Implemented by us | 📋 Planned by us | 🧲 Should borrow | 🚫 Don't need |
|---|---|---|---|---|
| **charmbracelet/fantasy** | Tool-schema authority (toolcat); UA/identity fields (shaper); parallel-tool discipline (toolexec concurrent/mutating split); provider error classification (Transient/Structural) | Retry `retry-after` honoring (SEED-002 §4, scheduler); VCR cassettes (parity direction) | SEED-002 list (jsonrepair, schema gen, raw-JSON beta tools, pairing fallback, incomplete-stream check) | Normalized one-API core (mimicry conflict) |
| **charmbracelet/crush** (FSL! read-only) | MCP hosting (internal/mcp); skills discovery (internal/ecosys); session persistence (internal/session) | LSP cluster (v1.2 pool); per-turn profile switching ≈ mid-session model switch (backlog) | Its skills/permissions/hooks surface as a zcode-parity checklist item | TUI-first design; desktop app; its license terms forbid code reuse |
| **opencode-ai/opencode** (Go, archived) | Headless non-interactive mode ≈ `acp serve`; MCP stdio; session storage; provider layer (internal/provider) | — | Auto-compact@95% behavior (compaction gap, see 🧲-2); SQLite-vs-JSONL tradeoff note | TUI; its frozen catalog (zcode profile is ours) |
| **sst/anomalyco opencode** (TS) | ACP support (internal/acp); plugin discovery (Phase 12 PLUG); config surface (defaults+firstrun) | /undo /redo (🧲-1 checkpoints); permissions/policies docs rigor | `/share` session export (🧲-12) | Zen curated models (scheduler tiers cover); web/desktop surfaces; daemon-ish serve |
| **plandex-ai/plandex** | Post-stage verify loops ≈ hookdag; OpenSpec SDD ≈ its plan-first core (Phase 13 completes) | Zero-continue chaining (Phase 13) | Branch-per-attempt sandbox + buffer-then-apply (🧲-5/6) | 2M-token context mgmt (projected window is our answer); tree-sitter maps (🧲-note on parity risk); cloud service |
| **earendil-works/pi** | Minimal-core philosophy (validated by our 24 small packages); no-permission position (validates our no-confirmation invariant!) | Extensions ≈ PLUG cluster (v1.2) | Telemetry contracts + conformance tests (🧲-8); compaction-as-extension (🧲-2); steering queue (🧲-4); supply-chain min-release-age (🧲-10); HF session corpus (🧲-7) | TUI; its own permission-free sandbox docs (ours: 🧲-3 does sandboxing) |
| **justxor/Claudecourse** | Core-loop-+-harness thesis == our engine/loop split; JSONL persistence (session); retries (provider); streaming (provider) | Sub-agents (session.subagent ✓); plan mode (Phase 12); sessions; evals (Phase 12) | Use its 34-section harness list as a **profile-coverage QA checklist** (🧲-11); compaction + worktree-isolation items (🧲-2/5); verify prompt-cache breakpoints are captured (🧲-2a) | Permission modes/gates (rejected invariant) |

### Category 2 — Go frameworks (SEED-003 set)

| Source | ✅ | 📋 | 🧲 | 🚫 |
|---|---|---|---|---|
| **cloudwego/eino** | Callbacks ≈ event bus; stream handling ≈ provider streaming | Interrupt/resume ≈ engine ask/wait + Phase 12 AskUserQuestion | Core/ext repo split for SEED-001 kit API surface | Graph orchestration engine (hookdag is deliberately 300 LOC) |
| **google/adk-go** | Session core (internal/session ≫ its session pkg) | Memory ≈ MEM cluster (v1.2 pool) | `httprr` record/replay → parity cassettes (second validation); session-schema versioning (🧲-8) | Its model-agnostic abstraction layer |
| **zendev-sh/goai** | Zero-dep ethos (we already live it); MCP-tools-as-native (toolcat mcpexec) | — | MCP client breadth check for internal/mcp (HTTP/SSE transports) | Its generics API shape (toolcat is profile-driven) |
| **mozilla-ai/any-llm-go** | Provider seam (internal/provider) | — | **Sentinel error classes** (ErrRateLimit/ErrAuthentication/ErrContextLength) as scheduler breaker triggers (SEED-003) | Its normalization layer |
| **tmc/langchaingo** | — | — | Nothing specific | Chains/RAG abstraction stack — wrong shape for us |

### Category 3 — Coding agents, other languages (the idea-dense set)

| Source | ✅ | 📋 | 🧲 | 🚫 |
|---|---|---|---|---|
| **openai/codex** | Config profiles ≈ scheduling.yaml; rollout JSONL ≈ transcript; plan tool ≈ Phase 12 plan mode; apply_patch ≈ coreexec Edit (zcode semantics) | Bash `dangerouslyDisableSandbox` passthrough (Phase 12) — **make the flag real behind the mimicry surface** (🧲-3) | Sandbox-exec/bwrap execution profiles; `--log-denials` audit style; execpolicy WASM idea (long-term, for hookdag run-command) | Approval-policy matrix (rejected invariant); desktop/cloud frontends |
| **cline/cline** | Rules files ≈ ecosys `.claude/` discovery; MCP marketplace ≈ mcp host; SDK tools ≈ coreexec register | Plan/Act (Phase 12 plan mode); scheduled agents (Phase 12 cron); worktree-per-task (🧲-5) | **Shadow-git checkpoints** (🧲-1); per-mode model settings (scheduler tier recipe, 🧲-9) | VSCode/JetBrains UI; kanban product; team coordinator |
| **aaif-goose/goose** | MCP subprocess hosting; `.goosehints` ≈ AGENTS.md/CLAUDE.md hierarchy (ecosys); session resume (transcript) | — | Custom-distro packaging idea for the kit era (SEED-001); self-test yaml ≈ behavioral evals (Phase 12) | Everything-is-MCP bus (normalizes tool shapes — mimicry conflict); recipes YAML (hookdag covers) |
| **OpenHands/OpenHands** | Event-stream ≈ transcript+event bus; mock-LLM e2e ≈ parity replay; delegation ≈ subagents | Behavioral evals (Phase 12 EVAL-01..03) | **Pluggable condensers** design for compaction (🧲-2); Stryker mutation testing idea for eval net rigor | Agent/Automation Server (REST daemon — no-port invariant); Docker-as-default runtime; multi-tenant |
| **google-gemini/gemini-cli** | Custom commands ≈ ecosys slash-commands; headless stream-json ≈ acp serve | Behavioral evals (Phase 12) — use their behavioral-evals doc as format reference (🧲-7); checkpointing (🧲-1) | Extension packaging (commands+MCP+prompts as one unit) for PLUG cluster | `@server` mention syntax; hierarchical GEMINI.md (zcode's own hierarchy is captured); OTel server |
| **QwenLM/qwen-code** | — | Auto-memory ≈ MEM cluster (v1.2); IM channel ≈ Telegram peer (v1.2) | Auto-Skills (agent-learned skills feeding our learning package); fork-governance README practice (document zcode-fork divergences in profile) | `serve` HTTP daemon; Agent Teams; arena product feature |
| **github/gh-aw** | — | — | **Buffer-then-apply** safe outputs (🧲-6); compiled-workflow lockfile idea (`.lock.yml` determinism for hookdag DAGs?) | CI-as-the-runtime model (our hookdag runs in-process by design) |
| **Aider-AI/aider** | Auto lint/test hooks ≈ hookdag post-stage; auto-commit ≈ run-command step | — | **Architect mode** as scheduler recipe: plan-on-heavy / apply-on-good (🧲-9); flexible patch matching ethos (jsonrepair, SEED-002) | Repo-map injection into prompts (**would break request parity** — context assembly belongs to the profile); edit-format negotiation (zcode's Edit semantics are the target); watch/voice modes |
| **SWE-agent** | **Our entire thesis, productized**: "agent-computer interface design drives capability" == profile/schema authority. Cite in docs | Bundles ≈ profiles as config artifacts | Trajectory-logging rigor for parity corpora (we have it — keep) | Their research harness; guarded editors (zcode tool semantics rule) |

### Category 4 — Vendor SDKs

| Source | ✅ | 📋 | 🧲 | 🚫 |
|---|---|---|---|---|
| **openai/openai-agents** | Sessions (session core); tracing (audit); tools-from-functions (coreexec) | Handoffs ≈ Phase 12 SendMessage/ReadSessionContext (agent-to-agent!) | Sandbox-agents preset as a hookdag fresh-context step option (v1.2+, 🧲-5) | Guardrails (rejected); RealtimeAgent voice (STT model differs); hosted tools |
| **anthropics/claude-agent-sdk** | Harness-as-library ≈ our future kit (SEED-001); hooks ≈ hookdag; allowed_tools≠toolset-filtering ≈ toolcat restricted-tools distinction | Session forking (minor, subagent variants) | In-process MCP servers (kit-era only — mimicked path must stay subprocess, go-sdk) | permission_mode presets (rejected); wrapping-a-CLI model (we ARE the engine) |
| **strands-agents/harness-sdk** | Loop-traces-everything ≈ audit/transcript discipline | — | "Steering handlers" vocabulary → input-queue during running turns (🧲-4) | Pre-execution guardrail gates (rejected invariant) |
| **google/adk-python** | — | Eval suites (Phase 12 + Phase 13 per-command) | **Evalset file format** (`.evalset.json`) as EVAL-01 format reference (🧲-7); versioned session schema w/ cross-version read (🧲-8) | Workflow Runtime graph engine; YAML no-code agents; Cloud Run deployment |
| **microsoft/agent-framework** | Middleware seams ≈ engine/session wiring points | — | Time-travel framing for checkpoints (🧲-1 naming/semantics reference) | Durable-execution extension (daemon-shaped); .NET/Py dual API |

### Category 5 — Independent frameworks

| Source | ✅ | 📋 | 🧲 | 🚫 |
|---|---|---|---|---|
| **langchain-ai/langgraph** | HITL interrupts ≈ engine ask/wait + Phase 12 AskUserQuestion | — | Checkpointer backend separation (memory/SQLite/Postgres) as the shape for checkpoint store (🧲-1) | Pregel graph state machine (hookdag is the right size) |
| **pydantic/pydantic-ai** | Typed tool schemas (toolcat adapter) | Evals (Phase 12) | Deferred capability loading — kit-era only ("model pulls tools like a skill" breaks fixed-catalog mimicry) | Durability via Temporal/DBOS; output-type retries (mimicry defines retry behavior) |
| **huggingface/smolagents** | Minimal loop (our session core is comparably small) | — | Agents-as-versioned-artifacts idea ≈ profile pinning (already ours — validated) | Code-as-action paradigm (tool_use blocks are the mimicry target); managed sandboxes |
| **letta-ai/letta-code** | Learning store (our ask-once-remember is a safer subset) | MEM cluster (v1.2 pool) — **study memory-blocks, MemFS, sleeptime when MEM is planned** | Secrets-obfuscated-from-model-context idea (tension: redaction must stay OUT of mimicked requests — log-side only, by design) | State-in-cloud routing; sleeptime compute; agent-calls-itself recursion |
| **mastra-ai/mastra** | Suspend/resume ≈ hookdag wait step | — | MCP-server authoring (expose ass-guard as MCP) — kit-era candidate | Typed workflow DSL; Studio; deployer packages; EE license split |
| **agno-agi/agno** | In-process cron ≈ Phase 12 CronCreate in-lifecycle firing (same insight, validated) | — | — | AgentOS multi-tenant runtime, 50+ endpoint API, RBAC, channels beyond Telegram |
| **crewAIInc/crewAI** | Flows ≈ hookdag determinism | — | — | Role/backstory crews; knowledge sources; Studio |

---

## 🧲 The borrow list, ranked (the gaps this study found)

1. **Checkpoints / undo (shadow-git).** Cline's workspace checkpoints, opencode's `/undo`,
   gemini-cli checkpointing, LangGraph checkpointer, ms time-travel. For a **no-confirmation-tier**
   agent this is the missing safety backstop: snapshot workspace at turn boundaries, offer
   rollback. Fits every invariant (external git, no daemon, no port). Surface: `ass-guard
   checkpoint` CLI + optional ACP/Telegram command. *(This is the single highest-value gap.)*
2. **Compaction policy.** Everyone has one (opencode auto-compact@95%, OpenHands pluggable
   condensers, pi compaction-as-extension, Claudecourse teaches it). We have two-layer context
   but no summarization story. **First step is verification, not design**: does the zcode
   profile capture zcode's own auto-compact behavior? If yes, mimicry gives us compaction for
   free; pi's extension-point shape is how profile-specific compaction should plug in.
   2a. **Prompt-cache breakpoints** (Claudecourse): verify the profile captures zcode's
   `cache_control` placement — it's both a parity item and a cost lever.
3. **Make the sandbox flag real.** Phase 12 passes `dangerouslyDisableSandbox` through to match
   zcode's Bash surface — but our executor shouldn't fake the default case. codex proves
   sandbox≠approval (macOS Seatbelt profile, Linux bwrap+seccomp). Parity-driven sandboxing:
   implement the sandbox zcode's tool semantics imply.
4. **Steering / input queue during a running turn.** pi (Enter vs Alt+Enter), strands steering
   handlers, claude-sdk steering. Our engine has cancel but no queue. Needed for both ACP
   (prompt-while-running) and especially the v1.2 Telegram peer (messages arriving mid-turn).
5. **Worktree isolation for parallel work.** Claudecourse's git-worktree subagent isolation,
   cline kanban per-task worktrees, OpenAI sandbox-agents. Natural fit: hookdag `fresh-context`
   steps and parallel improvement proposals — isolation without confirmation.
6. **Buffer-then-apply for mutating steps.** gh-aw's safe-outputs pattern (agent writes buffered,
   validated, then applied with scoped permissions) + plandex branch-per-attempt. A hookdag
   step-mode option, v1.2+.
7. **Behavioral-eval corpus + format.** Phase 12 EVAL-01..03 exists — feed it: pi's HF
   real-world session corpus as task source; ADK `.evalset.json` as file format reference;
   gemini-cli's in-repo behavioral-evals doc as suite-structure reference.
8. **Schema versioning + conformance.** pi-telemetry's typed contracts + conformance tests for
   our event/audit schemas; ADK 2.0's cross-version-readable session schema for the transcript
   format. Cheap insurance before formats ossify.
9. **Architect-mode scheduling recipe.** aider's plan/apply model split, expressed in our
   scheduler: hookdag `send-prompt` steps pin tiers (plan-on-heavy, apply-on-good). A config
   recipe, not code.
10. **Supply-chain age gates.** pi's `min-release-age=2` + exact pins + release smoke tests —
    adopt as dependency policy + a goreleaser smoke step.
11. **Claudecourse as a free mimicry-surface checklist.** Its 34 sections enumerate the Claude
    Code harness (hooks, permissions, compaction, memory, plan mode, sessions, caching, evals,
    injection, retries, streaming). Run the profile coverage manifest against it — any section
    with no coverage answer is either a captured behavior or a gap.
12. **Session export/share.** opencode `/share`, pi-share-hf. Cheap: render transcript JSONL to
    shareable Markdown; doubles as parity-corpus material. (Backlog-adjacent, tiny.)

## 🚫 Confirmations (the field validates our rejections)

- **No confirmation tier** — pi ships with no permission system at all; strands/guardrail SDKs
  exist but our pattern/hook gate + manual cancel is a defensible pole. With 🧲-1 (undo) added,
  the safety story is complete.
- **Small DAG, not a graph engine** — LangGraph/ADK/ms workflow runtimes are powerful but
  daemon- or server-shaped; hookdag's 300-LOC in-process executor is the right call (validated
  again by fantasy's StopConditions and mastra's DSL being overkill for 4 step types).
- **No HTTP/REST serve mode** — qwen `serve`, OpenHands Agent Server, agno AgentOS all assume
  an always-on port owner. Our no-port invariant keeps ACP stdio + Telegram as the only
  surfaces; qwen's ACP-over-HTTP is a v2-ACP thing to watch, not adopt.
- **No repo-map/context injection** — aider/plandex tree-sitter maps are the canonical
  big-repo answer for *autonomous* agents, but injecting them into prompts would break request
  parity; the profile owns context assembly. Revisit only for non-mimicked kit modes.
- **No multi-tenant/RBAC/channels-beyond-Telegram/desktop/TUI** — all out of scope, correctly.

## Verdict: how good is the plan?

**Differentiated and ahead on its core; standard-to-strong on surfaces; four real gaps, all
borrowable without violating a single invariant.**

- **Ahead (unique in the field):** structural mimicry + parity harness + drift detection has no
  peer in any of the 34 sources — the closest things (OpenHands mock-LLM e2e, qwen Arena,
  fantasy's providertests) test behavior, not wire fidelity. Log-extracted profiles,
  capture-pinned context tails, and the no-target-specific-code-paths discipline are likewise
  singular. SWE-agent's research thesis (the interface IS the capability) is the academic
  validation of our entire approach.
- **On par with the field's best:** session design (vs adk-go), provider engineering (vs
  fantasy/any-llm-go), post-stage routines (vs plandex/aider hooks — and ours generalizes them
  into a configurable engine), scheduling (nobody else has time-windowed tiers + breakers +
  cost ceilings in-process), minimal-core discipline (pi-class).
- **The gaps:** checkpoints/undo (🧲-1) is the one that matters most given the no-confirmation
  model; compaction (🧲-2) is a verification question before it's a design question; real
  sandboxing (🧲-3) makes an already-planned Phase 12 passthrough honest; steering (🧲-4) is a
  v1.2 Telegram prerequisite we haven't written down. Everything else is refinement.

## Breadcrumbs

- Implemented-capability evidence: `internal/` package map in SEED-003 Breadcrumbs + the
  2026-08-17 codebase inventory (session/engine/shaper/provider/scheduler/toolexec/toolcat/
  mcp/parity/drift/audit/redact/learning/hookdag/ecosys/openspec/coreexec/acp et al.)
- Plan evidence: `.planning/ROADMAP.md` (Phases 9/12/13, v1.2 pool), `.planning/STATE.md`,
  REQUIREMENTS.md backlog, FEATURES.md post-v1.1 list
- Companion seeds: SEED-001 (kit), SEED-002 (fantasy), SEED-003 (Go landscape),
  **SEED-004 (this doc's borrow list + surfacing triggers — the doc is truth, the seed is the reminder)**

## Extension protocol (for future updates to this doc)

This is a **living reference** — the operator intends to extend it. Rules to keep it coherent:

1. **Adding a source:** repo-verify before adding (language, ★, activity date, license, real
   mechanisms — no marketing). Add to the new-finds table OR a followup table with the
   verification date; give it a row in the appropriate ✅/📋/🧲/🚫 table. Mark unverified claims.
2. **License discipline:** keep the vendorable (Apache-2.0/MIT) vs read-only (FSL etc.)
   distinction explicit in every new row — it gates code reuse.
3. **Re-ranking the borrow list:** when a gap is adopted into a roadmap phase, strike it from
   §borrow-list and note the phase; when a new gap emerges, insert with rank + rationale.
   Never silently delete — note supersessions.
4. **Refreshing stats:** on any significant edit, bump a "last verified" note; star counts and
   activity go stale fast. Full re-verification pass: at each new-milestone scan (SEED-004
   trigger d).
5. **Verdict section:** update only with evidence — it is the doc's most load-bearing claim.
6. **Cross-links:** any new seed or phase plan that draws on this doc must breadcrumb back here,
   and this doc's Breadcrumbs section must reference them.
