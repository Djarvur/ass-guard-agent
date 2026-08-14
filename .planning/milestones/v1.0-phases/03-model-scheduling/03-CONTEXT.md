# Phase 3: Model Scheduling - Context

**Gathered:** 2026-08-10
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 3 introduces the model scheduling layer — the abstraction that lets a command/skill/subagent select a *tier* (heavy/good/light) while the operator configures which concrete (provider, model) that resolves to, with time-windowed substitution, per-project override, graceful degradation on outages, and anti-cascade safety. The developer never thinks about which concrete model is running; the operator manages cost/reliability via config.

**In scope (6 REQ-IDs):**
- **Tier abstraction (SCHED-01):** heavy/good/light ≈ opus/sonnet/haiku. A command/skill/subagent selects a tier, not a concrete model. The Model Scheduler resolves tier → concrete (provider, model) at request time.
- **Time-windowed substitution (SCHED-02):** tier→model mapping is time-scheduled (e.g. heavy = glm-5.2 normally, minimax-m3 in peak hours). Time-windows are timezone-explicit (IANA zones, `time/tzdata` bundled).
- **Per-project override (SCHED-03):** override the tier→model table per project; falls back to config default when unset.
- **Fallback chains + failure classification (SCHED-04):** on transient failure (429/5xx/network), walk a configured chain (degrade tier OR walk provider list); on structural failure (401/403), report instead of silently retrying. Transient-vs-structural classification (N5 engineered out).
- **Circuit breakers + cost ceilings (SCHED-05):** stop fallback-chain cascading failures and cost surprises.
- **Capability profiles (SCHED-06):** each (provider, tier) pair has a documented capability profile so tier mismatch across providers is explicit.

**Out of scope:**
- The provider adapters themselves (PROV-01..03, Phase 1) — the scheduler sits *above* them. Anthropic-shape and OpenAI-shape adapters already exist; the scheduler resolves tier → concrete (provider, model) before the adapter is called.
- The provider-layer concurrency semaphore (Phase 2 D-12, default 6) — bounds outbound concurrency; the scheduler's fallback logic interacts with it but doesn't own it.
- The unified engine / autocontinue (Phase 4) — the scheduler is a passive resolver; the engine decides when to make requests.
- Ecosystem compatibility (Phase 5), distribution (Phase 6).

**Mode:** mvp (ROADMAP.md Phase 3). The smallest phase (6 REQ-IDs). Built on the Phase 1 + Phase 2 provider/adapter infrastructure.

**STACK grounding:** STACK Focus 2 surveyed the routing landscape (LiteLLM Router, OpenRouter, Portkey, Requesty — all Python or hosted-proxy products, NOT Go-embeddable libraries). The recommendation, carried into this phase, is an internal config-table resolver over a config table. LiteLLM's *design* (fallbacks, cooldowns) is worth studying as a template; it is explicitly NOT a runtime dependency (would violate the single-static-binary constraint).

</domain>

<decisions>
## Implementation Decisions

### Resolver config shape (SCHED-01/02/03)

- **D-01:** The scheduling config is a **declarative YAML file**, viper-loaded, layered (global → per-project `.ass-guard/scheduling.yaml`). Sections for tiers, time-windows, per-project overrides, fallback chains, capability profiles. Human-readable, commentable, matches the Claude-Code config-layout convention. Operators edit it directly. (Rejected: TOML — awkward nested-table syntax for the time-window rules + fallback chains; programmatic Go API — violates config-not-code convention, requires rebuild to change scheduling.)
- **D-02:** The resolver applies this precedence at request time: **time-window → project → global** (the user explicitly reversed the recommended project-first order). Time-window is the **primary signal** — a structural peak-hours reality (capacity/cost) that applies to everyone; the per-project override **narrows within** whatever the time-window picked, rather than replacing it entirely; the global default is the fallback. First match wins. Most specific *applicable* context wins, but time-window is checked first because it's a structural constraint. Rationale: time-window reflects an operational reality (peak hours = capacity/cost constraints) that a single project shouldn't be able to override entirely; the project override refines within that.
- **D-03:** Time is evaluated **lazily per-request**: each model request asks the resolver "what's the tier right now?"; the resolver checks the current wall-clock time (IANA zone, `time/tzdata` bundled per SCHED-02) against the configured windows. Time-windows are live, not precomputed. A turn's tool-loop re-resolves on each iteration (a turn that crosses a window boundary switches models between tool-call iterations, which is acceptable — boundaries cross between iterations, not within a single provider request).

### Fallback chain mechanics (SCHED-04)

- **D-04:** The provider adapter returns a **typed `ProviderError` with a `.Kind` field**: `Transient` (429/5xx/network/timeout) | `Structural` (401/403) | `Exhausted` (budget/ceiling hit). The scheduler pattern-matches on `.Kind`: `Transient` → walk the fallback chain; `Structural` → report to the turn loop (no retry, the operator must fix the auth); `Exhausted` → hard stop (circuit-breaker territory, D-07/D-08). Failure classification is the **adapter's responsibility** (it sees the HTTP status); the scheduler gets a clean enum to pattern-match. This makes SCHED-04's transient/structural split explicit in the type system.
- **D-05:** "Walk a configured fallback chain" = an **explicit per-tier fallback list**. Each tier entry in the config has an explicit `fallback` array of (provider, model) candidates, ordered. The resolver returns the primary binding PLUS this list. On `Transient`, the scheduler walks the list in order, trying each until one succeeds or the list is exhausted. Per-tier (heavy's fallback may differ from light's). Explicit, auditable, operator-authored — no silent capability mismatch (which SCHED-06 exists to prevent). (Rejected: auto-degrade-tier — risks silent capability mismatch as heavy→good crosses providers with different capabilities; hybrid — auto-degrade tail may mask misconfiguration, research validates whether needed.)
- **D-06:** When the scheduler falls back (primary failed, using a fallback candidate), it emits an **info `session/update` notification** to the ACP adapter: "provider X failed (429), falling back to provider Y." The turn continues on Y. Transparent — the developer knows a degradation happened but isn't blocked. Routed via the Phase 2 event-bus → ACP `session/update` path.

### Circuit breakers & cost ceilings (SCHED-05)

- **D-07:** The circuit breaker uses **both consecutive-failure AND error-rate** trip mechanisms (the most robust option). **Consecutive-failure** trips fast on hard outage (e.g. 5 consecutive Transient failures = clearly down); **error-rate** trips slow on degraded performance (e.g. >50% errors over the last M requests). Per-(provider, model) binding — one bad provider doesn't kill the others. After tripping, the circuit is **open for a cooldown** (e.g. 60s), during which the resolver skips that candidate. After cooldown, a **half-open probe** (one request) tests recovery before fully closing. Research defines the exact thresholds (N consecutive, M window size, error-rate %, cooldown duration) — these are tunable parameters with documented defaults.
- **D-08:** The cost ceiling is expressed in **dollars per time window** (e.g. `$50/day`). The scheduler tracks **estimated cost per request** (from token counts × per-model pricing declared in the config) and accumulates per window. When the window's total hits the ceiling: the scheduler **degrades to the cheapest configured tier/model** AND emits a **warn `session/update`** ("cost ceiling approaching/ hit, degrading to light tier"). If the degraded tier ALSO hits its own (lower) ceiling: **hard-stop** (the turn fails with a clear cost-exhausted error). Operator-friendly unit (dollars), graceful degradation (degrade-then-stop, not hard-stop on first breach). (Rejected: tokens — abstract for operators; requests — ≠ cost, doesn't prevent surprises.)

### Capability profiles (SCHED-06)

- **D-09:** Each (provider, model) has a **structured capability declaration** in the config: context window size, max output tokens, tool-calling support (yes/no), streaming support (yes/no), extended-thinking support (yes/no), and any known limitations. Lives in the config alongside the tier mappings. **Machine-readable** — the scheduler consults it when resolving fallbacks (won't route a tool-using turn to a model that lacks tool-calling support). This is what makes the tier abstraction honest: "heavy" on provider A (GLM-5.2, 200K context, tools, streaming) ≠ "heavy" on provider B (different limits). (Rejected: freeform doc — not machine-actionable; no profiles — SCHED-06 explicitly requires them.)
- **D-10:** Capability mismatch is surfaced by **rejecting the config at load time**. When viper loads `scheduling.yaml`, a validation step cross-references the tier mappings + fallback chains against the capability profiles. If a tier's primary model has a capability that its fallback lacks (e.g. primary supports tool-calling but a fallback doesn't), the config is **REJECTED with a clear error naming the conflict** ("tier 'heavy' primary glm-5.2 supports tool-calling but fallback X does not — incompatible chain"). Fail-fast — the operator cannot ship an inconsistent config. This is the load-time guarantee; request-time capability checks (D-09's "won't route to incompatible model") are the runtime guarantee. Together they make tier mismatch explicit both at authoring time and at request time.

### Claude's Discretion
None — every Phase-3 decision was explicitly user-answered. The user reversed the recommended precedence in D-02 (time-window first, not project first) with a clear rationale. Zero Claude's-discretion.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase 1/2 outputs (the infrastructure the scheduler sits above)
- `.planning/phases/01-mimicry-mvp-north-star-proof/01-CONTEXT.md` — D-09 (Shaper hybrid SDK driving), the provider adapter contracts (PROV-01..03), the Anthropic-shape + OpenAI-shape adapter split. The scheduler calls these adapters after resolving tier → (provider, model).
- `.planning/phases/02-session-core-acp-interface/02-CONTEXT.md` — D-04 (event bus = typed Go channels — the scheduler emits fallback/cost warnings through this bus), D-12 (provider-layer semaphore, default 6 — bounds outbound concurrency across parent + subagents; the scheduler's fallback logic coexists with this), D-15/D-16 (ACP server dispatch + session/cancel — the scheduler's session/update emissions route through the ACP adapter).
- `.planning/phases/02-session-core-acp-interface/02-RESEARCH.md` — the provider adapter interface contract the scheduler must call; the event-bus event types the scheduler emits to.

### The 6 Phase-3 requirements
- `.planning/REQUIREMENTS.md` §"Model scheduling (Phase 3)" (SCHED-01..06).

### Phase goal + success criteria
- `.planning/ROADMAP.md` §"Phase 3: Model Scheduling" — the 4 success criteria (tier abstraction; time-windowed + per-project; transient vs structural fallback; circuit breakers + cost ceilings + capability profiles). Mode: `mvp`.

### Project-level constraints (load-bearing)
- `.planning/PROJECT.md` — **Constraints** (single static binary — the scheduler MUST be in-process, no external proxy/daemon; this is why LiteLLM et al. are templates not deps), **v1 Cut-Line** (the six deltas are serialized — Phase 3 builds on Phase 1+2, doesn't thin-slice).

### Research/stack references
- `.planning/research/STACK.md` §Focus 2 (Model Scheduling Layer) — the LiteLLM/OpenRouter/Portkey/Requesty survey (all Python or hosted; NOT Go-embeddable); the internal config-table resolver recommendation; the tier/time-window/per-project/fallback design template distilled from LiteLLM's router docs. §"What NOT to Use" (LiteLLM as runtime dep — violates single-binary constraint; study its design, don't depend on it).

### Phase 0 output (provider grounding)
- `.planning/research/VERIFIED-FACTS.md` — Item #1 (zcode talks to Z.ai GLM via Anthropic protocol; `providerId: "builtin:zai-coding-plan"`) — the default provider the heavy tier resolves to. Item #2 (go-openai tool-calling schema) — the OpenAI-shape providers the scheduler may route to (MiniMax M3, Groq, OpenRouter).

### Codebase (greenfield + Phase 1/2 plans)
- The Phase 1 plans (`.planning/phases/01-*/01-{01..06}-PLAN.md`) define the provider adapter interface (`Provider` with `TranslateToInternal`/`TranslateFromInternal`) the scheduler calls.
- The Phase 2 plans (`.planning/phases/02-*/02-{01..07}-PLAN.md`) define the event-bus event types and the ACP adapter the scheduler emits to.
- Phase 3 adds `internal/scheduler` (the resolver + circuit breakers + cost tracker) and the scheduling config loading (viper).

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- **Provider adapters** (Phase 1, planned): the `Provider` interface with `TranslateToInternal`/`TranslateFromInternal` tool-call translation. The scheduler resolves tier → (provider, model) and hands off to the adapter. The adapters already handle both Anthropic-shape (anthropic-sdk-go) and OpenAI-shape (go-openai) protocols via swappable base URL.
- **Event bus** (Phase 2, planned): typed Go channels. The scheduler emits `ProviderFallback` (info) and `CostCeilingWarn` (warn) events the ACP adapter forwards as `session/update` notifications.
- **Provider-layer semaphore** (Phase 2 D-12, planned, default 6): bounds outbound concurrency. The scheduler's fallback walks coexist with this — a fallback attempt goes through the same semaphore.
- **`internal/redact`** (Phase 1, executed): secrets redaction. The scheduler's cost-tracking logs (which may include provider/model names and pricing) route through this where sensitive.

### Established Patterns
- **Config-not-code** (Claude-Code convention): the scheduling config is a declarative YAML file, not programmatic. Operators edit files; no rebuild to change scheduling.
- **Single static binary** (PROJECT.md): the scheduler is in-process Go. No external proxy, no daemon, no network port. This is why LiteLLM et al. are design templates, not dependencies.
- **Typed errors** (Go idiom): `ProviderError.Kind` follows the pattern of typed sentinel errors. The scheduler pattern-matches; the adapter owns the classification.

### Integration Points
- **Turn loop → Scheduler → Provider adapter:** the turn loop (Phase 2 Session Core) requests a tier; the scheduler resolves it to (provider, model) + fallback chain + capability profile; the turn loop calls the adapter with the resolved binding. On Transient failure, the scheduler walks the chain and re-dispatches.
- **Scheduler → Event bus → ACP:** fallback + cost-ceiling events flow through the Phase 2 event bus to the ACP adapter as info/warn `session/update` notifications.
- **Config-load validation:** at startup (and on config reload), the validation step (D-10) cross-references tiers + fallbacks + capability profiles, rejecting inconsistent configs before any request is served.

</code_context>

<specifics>
## Specific Ideas

- The user's **time-window → project → global precedence reversal** (D-02) is the distinctive choice in this phase. The recommended order was project-first (most specific context wins); the user chose time-window-first because peak-hours reality is structural — a single project shouldn't override the capacity/cost constraints that apply to the whole deployment. The project override narrows *within* the time-window's pick. This is a deliberate operator-cost-protection priority: the operator's time-window rules are the floor that project-level tweaks build on, not the other way around.
- The user chose **both consecutive + error-rate** for the circuit breaker (D-07) — the most robust option, accepting the complexity of two mechanisms and two thresholds. This reflects a "make it actually safe" priority over "make it simple."
- The user chose **dollars/window** for cost ceilings (D-08) over tokens — prioritizing operator-intuitive units. The degrade-then-stop response (not hard-stop on first breach) reflects the "graceful degradation" language in the ROADMAP goal.
- The user chose **reject at config-load** for capability mismatch (D-10) — the fail-fast option. Operators author consistent configs up front; the system refuses to run an inconsistent one. This is the "make it explicit" intent of SCHED-06 enforced at the earliest possible point.

</specifics>

<deferred>
## Deferred Ideas

None raised as new capabilities during discussion. The following are noted as implementation details for research/planning (not user decisions):
- The exact circuit-breaker thresholds (N consecutive, M window, error-rate %, cooldown) — D-07 locks the mechanism; research defines defaults.
- How per-project override is discovered (cwd-based? explicit config path?) — implementation detail; viper's layered loading handles it.
- How the operator updates pricing in config (manual edit? a `pricing.yaml` section? auto-fetch from provider APIs?) — implementation detail; the pricing table is part of the config, edited by the operator.
- How the scheduler tracks cost across a distributed/multi-process deployment — out of scope; ass-guard is a single process per editor (no shared state needed; the cost tracker is in-process).

</deferred>

---

*Phase: 3-Model Scheduling*
*Context gathered: 2026-08-10*
