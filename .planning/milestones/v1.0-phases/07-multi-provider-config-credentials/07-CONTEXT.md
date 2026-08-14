# Phase 7: Multi-Provider Config & Credentials - Context

**Gathered:** 2026-08-13
**Status:** Ready for planning (decisions are best-judgment — see "Discussion mode" note)

<domain>
## Phase Boundary

Phase 7 is the **provider/model/credential configuration layer** that makes v1's multi-provider promise real and connects the Phase-3 scheduler to actual credentialed provider instances. Today only one provider (Z.ai, Anthropic-shape) and one model (glm-5.2) are wired, and the key is read from `$ZAI_API_KEY` only — so an editor-spawned `ass-guard acp serve` (Zed) cannot authenticate. Phase 7 lets an operator declare multiple providers (each with `base_url` + protocol `shape` + credential) and multiple models per provider in config, and ass-guard resolves a scheduler tier to a fully-credentialed provider+model instance at request time, reading credentials from the file so editor-spawned processes work with zero environment.

**In scope (completes PROV-01; new PCFG-01..04):**
- **Config schema extension (PCFG-01):** add credential fields to the existing `scheduling.yaml` `providers` block; declare ≥2 providers + ≥2 models per provider; loader validates the extended schema.
- **Credential resolution + safety (PCFG-02):** credentials resolve flag > env > config-file (env overrides config so CI/operators win; config is the editor-spawned floor); `${ENV_VAR}` expansion in-config; secrets never reach logs (redactor already covers `api_key`/`sk-`/`Bearer`; config file is gitignored + warned if looser than 0600).
- **Scheduler → provider wiring (PCFG-03):** a provider factory/registry, built at startup from loaded config, keyed by provider name; the scheduler's resolved `(provider, model)` produces a `provider.Provider` instance (Anthropic or OpenAI adapter) constructed with the right `base_url` + resolved key + shape.
- **Zero-config backward-compat (PCFG-04):** the embedded default keeps today's `$ZAI_API_KEY`-only flow working unchanged; an operator who changes nothing gets today's behavior.

**Out of scope:**
- Re-deciding tier abstraction / model selection (locked by Phase 3 — selection stays tier→model table via the scheduler).
- External-command credential fetch (Claude-Code `apiKeyHelper` style) — deferred (YAGNI for v1; `${ENV_VAR}` expansion covers the no-secret-on-disk case).
- Provider-level model defaults, model aliases, `default_provider` — deferred (YAGNI; the existing `models:`/`tiers:` schema already supports N models).
- A separate `credentials.yaml` / secret store — deferred (one-config-file per Phase 3 D-01; `.ass-guard/` already self-gitignored + redacted).
- Per-(provider) custom HTTP transports, retries beyond Phase 3's fallback chain — out of scope.

**Mode:** mvp (ROADMAP.md Phase 7). Depends on Phase 3 (scheduler + scheduling.yaml schema) and Phase 6 (`.ass-guard/` dir + go:embed first-run seed + redactor).

</domain>

<decisions>
## Implementation Decisions

> **Discussion mode:** the operator declined per-area selection and delegated the gray areas to best-judgment. Every D below is therefore **derived from prior-phase decisions + codebase conventions + the stated requirement (credentials-in-config for editor-spawned; multi-provider; agent selects per config)** — NOT user-selected. They are reversible-to-costly and explicitly open to override before `/gsd:plan-phase 7`. If the operator disagrees with any, it should be raised now; the planner treats these as locked otherwise.

### Credential storage (PCFG-02)
- **D-01:** Credentials live as an `api_key` field **inline in the existing `scheduling.yaml`**, under each `providers.<name>` entry (alongside the existing `base_url` + `shape`). The value may be a **literal** OR a **`${ENV_VAR}` expansion** (e.g. `api_key: "${ZAI_API_KEY}"`). Literal is the default so editor-spawned processes authenticate with zero env; `${VAR}` is for operators who refuse to put a secret on disk. — **Reversibility:** reversible — adding/changing a config field is local. (Rejected: a separate `credentials.yaml` — re-litigates Phase 3 D-01's "one declarative YAML" and adds a second loader for no security gain once the single file is already gitignored + redacted. Rejected: external `api_key_cmd` / apiKeyHelper — YAGNI for v1; `${VAR}` covers the no-secret-on-disk case without a subprocess+secret-in-argv surface.)
- **D-02:** The loader **expands `${VAR}` at load time** and **errors clearly if a referenced env var is unset** (names the provider + the missing var). A literal is taken as-is. Expansion is limited to `${NAME}` (no shell, no defaults syntax) — a deliberate minimal surface.

### Config structure (PCFG-01) — extends, does not replace
- **D-03:** **Extend the existing `scheduling.yaml`** schema; do NOT introduce a separate providers/credentials file. Add `api_key` (+ optional `api_key_env` as a self-documenting alias for the `${...}` form) to each `providers.<name>` entry. The existing `models:` block (keyed by slug, `provider:` ref, capabilities per Phase 3 D-09) already supports N models per provider — Phase 7 just lets the operator declare more of them. — **Reversibility:** costly — the schema is parsed by `internal/scheduler/load.go` and seeded via `internal/defaults` go:embed; undo touches both + the embedded default. Honors Phase 3 D-01 (one declarative YAML, layered global→per-project `.ass-guard/scheduling.yaml`).
- **D-04:** **Load-time validation extends Phase 3 D-10.** In addition to capability-mismatch rejection, the loader rejects: a provider referenced by a model/tier that isn't declared; an unknown `shape` (only `anthropic`/`openai` valid); and warns (does not reject) on providers with no resolvable credential (see D-06).

### Credential precedence + sources (PCFG-02)
- **D-05:** Precedence at provider construction time: **`--api-key` flag (if added) > provider-specific env var > config-file `api_key`**. Env wins over config so CI/operator deployments override the file; the config file is the **floor** that makes an editor-spawned process (no env) authenticate. The provider-specific env var name defaults to `<PROVIDER>_API_KEY` upper-cased (the Z.ai/anthropic provider keeps `$ZAI_API_KEY` for backward-compat). — **Reversibility:** reversible — precedence is a resolver rule. This is the standard 12-factor-leaning order with config as the additional lowest tier.

### Backward-compat + zero-config (PCFG-04)
- **D-06:** The **embedded default** `scheduling.yaml` ships the Z.ai anthropic provider with **`api_key_env: ZAI_API_KEY` and NO literal key**. Therefore today's `$ZAI_API_KEY`-only flow keeps working byte-for-byte; an operator who changes nothing gets today's behavior. To enable editor-spawned-without-env, the operator adds a literal `api_key` (or exports the var into the editor's launch environment). — **Reversibility:** one-way — the embedded default is baked into the binary via Phase-6 go:embed; changing the default post-release is a re-release. Rationale: the default is the documented zero-config contract.

### Missing-credential behavior (PCFG-02)
- **D-07:** **Warn at startup, fail lazy at first use.** At startup, ass-guard logs a warning naming any provider whose credential is unresolvable (neither flag, env, nor config) — but does NOT refuse to start as long as ≥1 provider IS credentialed (a config may declare fallback providers the operator hasn't keyed yet). On the first request routed to an uncredentialed provider, ass-guard returns a clear, typed error ("provider X has no credential — set api_key in config or $X_API_KEY") instead of an opaque 401. — **Reversibility:** reversible. Matches the project's graceful-degradation philosophy (Phase 3 D-04/D-06, Phase 4 D-04); fail-fast-on-startup would break the "declare fallbacks incrementally" workflow.

### Scheduler → provider wiring (PCFG-03) — architecture, Claude's domain
- **D-08:** A **provider factory/registry**, constructed **once at startup** from the loaded config, keyed by provider name → `(shape, base_url, resolved_key)`. When the scheduler resolves a tier to `(provider, model)`, the turn loop / session asks the factory for the provider instance; the factory returns a `provider.Provider` — `provider.NewAnthropicProvider(shaper, WithAnthropicBaseURL(base_url), WithAnthropicAPIKey(key))` for `shape: anthropic`, the OpenAI-shape constructor for `shape: openai`. The existing `WithAnthropicAPIKey`/`WithAnthropicBaseURL` options ARE the construction seam (today used only by tests). The factory replaces the current hardcoded `makeProvider: func() provider.Provider { return provider.NewAnthropicProvider(shaper.New()) }` in `cmd/ass-guard/acp_serve.go:257` and the single-provider path in `cmd/ass-guard/main.go:127` + `parity.go:87`. — **Reversibility:** costly — the factory touches every provider construction site (tracer, ACP serve, parity) and the scheduler↔session boundary. This is architecture, not a user preference; the planner may refine the exact seam.

### REQ-IDs
- **D-09:** Assign new REQ-IDs: **PCFG-01** (provider/model/credential config schema + loader + validation), **PCFG-02** (credential precedence + `${VAR}` expansion + no-log/redaction + missing-credential behavior), **PCFG-03** (scheduler→provider credentialed-instance factory wiring), **PCFG-04** (zero-config backward-compat: default seeds env-only). Phase 7 also **completes PROV-01** ("any compatible provider via configurable base URL"). Add these to REQUIREMENTS.md §Providers + the traceability matrix during planning.

### Claude's Discretion
All gray areas (credential storage, config structure, precedence, backward-compat, missing-credential behavior, declaration scope) were delegated by the operator to best-judgment. The decisions above are derived, not user-confirmed. Specific discretionary choices made: (a) `${VAR}` expansion over external command; (b) one file over a split credentials file; (c) warn+lazy over fail-fast; (d) minimal declaration scope (no aliases/`default_provider`/provider-level model defaults). The wiring (D-08) is entirely Claude's-discretion architecture for the planner to refine.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Prior-phase decisions (load-bearing — these are the foundation)
- `.planning/phases/03-model-scheduling/03-CONTEXT.md` — **D-01** (scheduling config = declarative YAML, layered global→per-project `.ass-guard/scheduling.yaml`; one file), **D-04** (`ProviderError.Kind` typed errors — Transient/Structural/Exhausted), **D-09** (per-(provider,model) structured capability declarations — already in `models:`), **D-10** (capability mismatch rejected at load). Phase 7 extends this schema with credentials; it does not replace it.
- `.planning/phases/06-distribution-polish/06-CONTEXT.md` — **D-01/D-04** (embedded defaults via go:embed written to `.ass-guard/` on first run, non-clobbering; `.ass-guard/` self-gitignored). Phase 7's embedded default must follow this exact seed mechanism.
- `.planning/phases/01-mimicry-mvp-north-star-proof/01-CONTEXT.md` — the provider adapter contracts (PROV-01..03), the Anthropic-shape vs OpenAI-shape split, the `Provider` interface.

### Requirements
- `.planning/REQUIREMENTS.md` §"Providers (Phase 1 substrate + Phase 3)" — **PROV-01** (both shapes, configurable base URL — Phase 7 completes this), PROV-02 (common `Provider` interface), PROV-03 (Anthropic adapter via anthropic-sdk-go swappable base URL; OpenAI via go-openai).
- `.planning/REQUIREMENTS.md` §"Model scheduling (Phase 3)" — SCHED-01..06 (the scheduler Phase 7 wires to).

### Phase goal + success criteria
- `.planning/ROADMAP.md` §"Phase 7: Multi-Provider Config & Credentials" — the 4 success criteria (≥2 providers w/ credentials + editor-spawned turn; ≥2 models/provider + correct instance per resolved pair; credential precedence + no-log + gitignore/0600; zero-config backward-compat).

### Project-level constraints (load-bearing)
- `.planning/PROJECT.md` — **Constraints** (single static binary — the factory is in-process, no external proxy; transport discipline — stdout = ACP only, all logs incl. credential warnings → stderr; Claude-Code config layout drop-in — `.ass-guard/` naming, namespaced additions).
- `.planning/research/STACK.md` §Focus 2 + §"What NOT to Use" — internal resolver over config table (not LiteLLM/OpenRouter/Portkey as runtime deps); provider adapters = anthropic-sdk-go + go-openai.

### Codebase (the integration surface)
- `internal/scheduler/defaults/scheduling.yaml` — the current embedded schema (`providers`/`models`/`tiers`/`circuit_breaker`/`cost_ceiling`); Phase 7 extends `providers` with `api_key`/`api_key_env`.
- `internal/scheduler/load.go` — the manual deep-merge loader (NOT viper — deliberate project deviation from Phase 3 D-01's "viper" wording); Phase 7's schema extension + `${VAR}` expansion + validation live here.
- `internal/provider/streaming.go:46-56` + `internal/provider/anthropic.go:33-40` — the key/base-URL resolution + `WithAnthropicAPIKey`/`WithAnthropicBaseURL` options (the construction seam D-08 uses).
- `cmd/ass-guard/acp_serve.go:257-259` (ACP `makeProvider`), `cmd/ass-guard/main.go:127` (tracer), `cmd/ass-guard/parity.go:87` (parity gate) — the 3 provider construction sites D-08 replaces with the factory.
- `internal/redact/redact.go` — already scrubs `api_key`/`sk-`/`Bearer`; Phase 7 relies on this for any credential that reaches a log/audit path.
- `internal/firstrun` + `internal/defaults` — the non-clobbering first-run seed the extended default must flow through.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- **`internal/scheduler`** (Phase 3): the tier→(provider,model) resolver, circuit breakers, cost tracker, and the `scheduling.yaml` schema/loader. Phase 7 extends the schema (`api_key`/`api_key_env` on providers) and adds the provider-instance factory the resolver's output feeds. The `providerModelKey{provider, model}` concept already exists in `internal/scheduler/breaker.go` — the factory is keyed by the same `provider` name.
- **`internal/provider`** (Phase 1): `AnthropicProvider` + OpenAI-shape provider, both with `With*APIKey`/`With*BaseURL` options and a common `Provider` interface. These ARE the instances the factory builds — no new adapter code needed for v1.
- **`internal/redact`** (Phase 1): secrets scrubbing (`api_key`, `sk-`, `Bearer`). Credential warnings/audit paths route through this.
- **`internal/defaults` + `internal/firstrun`** (Phase 6): go:embed of the seed tree + non-clobbering first-run write to `.ass-guard/`. The extended default `scheduling.yaml` (with `api_key_env` on the Z.ai provider) flows through this unchanged mechanism.

### Established Patterns
- **Config-not-code, manual deep-merge (NOT viper):** `internal/scheduler/load.go` deliberately hand-rolls layered YAML loading (documented deviation from Phase 3 D-01's "viper" wording). Phase 7's `${VAR}` expansion + credential validation extend this loader, not a new viper setup.
- **Transport discipline:** all credential warnings → stderr (stdout is ACP-only). The startup "provider X has no credential" warning is a `log.Printf` → stderr, same as `acp_serve.go`'s existing degradation warnings.
- **Graceful degradation:** warn-and-continue on partial misconfiguration (engine setup failure → continue without engine, `acp_serve.go:266`; learning-store open failure → continue without learning). D-07's warn+lazy follows this exact pattern.
- **Typed errors at the boundary:** `ProviderError.Kind` (Phase 3 D-04). The "no credential" error should be a typed structural error the turn loop reports (not retried) — same classification as 401/403.

### Integration Points
- **Startup:** `acp serve` RunE → `runACPServe` → load config → build provider factory (D-08) → pass factory (not a single `makeProvider`) to `sessionTurnRunner`. The factory is consulted per-turn when the scheduler resolves the tier.
- **Per-turn:** scheduler resolves tier → `(provider, model)` → session asks factory for the provider instance → factory returns `provider.Provider` with the right base_url+key+shape → existing `loop.Run`/`sess.Prompt` path uses it unchanged.
- **First-run seed:** `internal/firstrun.Ensure` writes the extended `scheduling.yaml` (Z.ai provider with `api_key_env: ZAI_API_KEY`, no literal) to `.ass-guard/`.

</code_context>

<specifics>
## Specific Ideas

- The operator's concrete trigger (this session): testing from inside a zcode session where env vars can't be exported into the spawned `ass-guard` process. The `${VAR}` expansion (D-01/D-02) and the literal-in-config path are both justified by this real workflow — but the **literal-in-config** is what unblocks the editor-spawned (Zed) case, which is the durable rationale (Zed spawns `ass-guard acp serve` without the operator's shell env).
- The live proof gathered this session (with a real `$ZAI_API_KEY` via a `$(cat .ass-guard/api_key)` wrapper): mimicry shape is correct end-to-end (3 system blocks, 103 tools, GLM-5.2, thinking, tool_choice; a `Read` tool-call parsed correctly). So Phase 7 changes the **credential/provider-construction** path only — the mimicry shaper + scheduler + turn loop are proven and untouched.
- `.ass-guard/api_key` (the ad-hoc file created for this session's testing) is now gitignored (`.ass-guard/` wholesale). Phase 7 supersedes that workaround with first-class config support; the ad-hoc file is NOT part of the design.

</specifics>

<deferred>
## Deferred Ideas

- **External credential command (apiKeyHelper-style):** `api_key_cmd: "op read ..."` / `pass ...` — fetches the secret from a password manager / vault at startup, no secret on disk. Deferred: `${VAR}` expansion covers the no-literal-on-disk case for v1; the subprocess+secret-in-argv surface + OS keychain integration is a v2 nicety.
- **Separate `credentials.yaml` / OS keychain backend:** a dedicated secret store decoupled from `scheduling.yaml`. Deferred: one-file (Phase 3 D-01) + gitignore + redact is sufficient for v1.
- **Provider-level model defaults, model aliases, `default_provider`, declare-once-use-many:** richer declaration ergonomics. Deferred (YAGNI) — the existing `models:`/`tiers:` schema supports N models/providers already.
- **`--api-key` / `--provider` CLI flags:** convenient for ad-hoc runs. Deferred to planning discretion — a `--api-key` flag is named in D-05's precedence but is optional; the planner decides whether v1 ships it.

</deferred>

---

*Phase: 7-Multi-Provider Config & Credentials*
*Context gathered: 2026-08-13*
