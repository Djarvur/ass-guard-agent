# Phase 3: Model Scheduling — Pattern Map

**Mapped:** 2026-08-09
**Phase:** 03 — Model Scheduling
**Source:** `03-CONTEXT.md`, `03-RESEARCH.md`, `internal/redact/` (the only executed Go code in-repo), Phase-1 plans + `01-CONTEXT.md` (provider adapter contracts), Phase-2 `02-RESEARCH.md` (event bus §2, semaphore §11, ACP adapter §8.2 — the intended interfaces), STACK §Focus 2 (LiteLLM template), `sony/gobreaker` (breaker state-machine template)
**Files analyzed:** 14 new/modified files across `internal/scheduler`, `internal/provider`, `cmd/ass-guard`, `internal/scheduler/testdata/`, `go.mod`
**Analogs found:** 6 / 14 (the rest are greenfield with external design-template references only)

> **Greenfield caveat (carries from Phase 1/2).** The production module has only `doc.go` skeletons + `internal/redact/redact.go` (the sole executed Go code, from Phase-1 plan 01-01 T2). Phase 3 sits ABOVE the unexecuted Phase 1 (provider adapters) and Phase 2 (event bus, semaphore, Session Core). PATTERNS.md maps each Phase-3 file to (a) the closest in-repo analog (`internal/redact` for Go style + test style + typed-error idiom), (b) the Phase-1/2 **intended interface** it consumes/extends (cited to the 02-RESEARCH section that defines it), and (c) the external design template (LiteLLM, sony/gobreaker, Go stdlib `time/tzdata`) studied but NOT depended on. Every pattern is grounded in dated, inspectable source.

---

## In-repo conventions (carry-forward from Phase 0/1/2)

These conventions are established and Phase 3 MUST honor them:

- **C1 — Transport discipline (PROJECT.md, load-bearing):** stdout is reserved for ACP JSON-RPC frames (and machine-readable `--json` output from operator CLI commands, explicitly requested). ALL diagnostics, logs, and human-readable CLI output go to stderr. The scheduler's logging (breaker trips, cost breaches, fallback walks) MUST go to stderr via `log/slog` — never stdout.
- **C2 — Redaction chokepoint (`internal/redact`):** any error string or log line that could wrap a request body/secret routes through `redact.ScrubError`. The scheduler's `ProviderError.Error()` strings and cost-tracking logs (which may include provider/model names — not secret, but the wrapped cause may be) use `redact.ScrubError` before formatting. Pattern reference: `internal/redact/redact.go` `ScrubError`.
- **C3 — Config-not-code (Claude-Code convention, D-01):** scheduling is a declarative YAML file, not a programmatic Go API. Operators edit `scheduling.yaml`; no rebuild to change scheduling. Matches the profile-as-config pattern (Phase 1) and the `.claude/` settings hierarchy.
- **C4 — Typed sentinel errors (Go idiom, D-04):** `ProviderError` with a `.Kind` field follows the typed-error pattern. The adapter owns classification (it sees HTTP status); the scheduler pattern-matches. Closest in-repo analog: `internal/redact`'s structured error handling; the Phase-2 plans' `RPCError` type.
- **C5 — Investigate-and-fix-ready logging (PROJECT.md must-have, NEW for Phase 3):** every scheduler failure, breaker trip, cost breach, and fallback walk is logged with enough detail to diagnose from the transcript + stderr alone: the `(provider, model)`, the error kind, the HTTP status, the breaker state transition, the cost accumulated. Not "an error occurred" — the context, the inputs, the failure point, the recoverable/non-recoverable classification.

---

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `internal/scheduler/config.go` | model | — | `internal/profile/` (Phase-1 profile types — intended; config-bundle pattern, STACK §Focus 1) | role-match (intended) |
| `internal/scheduler/load.go` | service | file-I/O (viper load + merge) | `spf13/viper` standard layered-load pattern (STACK inherited); `.claude/` settings hierarchy (C3) | role-match |
| `internal/scheduler/init.go` | init | — (blank import + defaults) | Go stdlib blank-import idiom (`_ "time/tzdata"`); `database/sql` driver-import pattern | exact (idiom) |
| `internal/scheduler/resolver.go` | service | pure transform `(config, tier, project, now) → Target` | (greenfield; `03-RESEARCH` §4) — table-tested pure function | no analog |
| `internal/scheduler/events.go` | model | pub-sub (event types) | Phase-2 `internal/event/bus.go` (`Event` interface + `Kind()` — 02-RESEARCH §2.2, EXPANDED additively) | role-match (extends) |
| `internal/provider/errors.go` | model | — (typed error) | `internal/redact/redact.go` (typed-error + `ScrubError` idiom, C2/C4); Phase-2 `RPCError` (02-RESEARCH §7.1) | role-match (idiom) |
| `internal/scheduler/dispatch.go` | service | orchestration (resolve → provider → fallback → events) | (greenfield; `03-RESEARCH` §8) — the Scheduler composite | no analog |
| `internal/scheduler/breaker.go` | service | state-machine + concurrency | `github.com/sony/gobreaker` (Closed/Open/HalfOpen + `ReadyToTrip` — study, NOT a dep; 03-RESEARCH §6.1) | role-match (external template) |
| `internal/scheduler/cost.go` | service | accumulation (window accounting) | (greenfield; `03-RESEARCH` §6.2) — fixed-window accumulator | no analog |
| `internal/scheduler/safety.go` | service | wiring (breaker+cost into Dispatch) | (greenfield; the §8.2 breaker/cost branches) | no analog |
| `internal/scheduler/capability.go` | utility | transform (cap-req satisfaction check) | (greenfield; `03-RESEARCH` §7.2) | no analog |
| `cmd/ass-guard/scheduling_validate.go` | command | CLI (cobra subcommand) | Phase-1 `cmd/ass-guard/profile_check.go` (cobra subcommand pattern — intended); Phase-2 `acp_serve.go` (02-01-PLAN T3) | role-match (intended) |
| `cmd/ass-guard/scheduling_resolve.go` | command | CLI (cobra subcommand) | same as above | role-match (intended) |
| `internal/scheduler/testdata/*.yaml` | fixture | — | `scheduling.yaml` schema (03-RESEARCH §1.2) | exact (self-referential) |
| `go.mod` | manifest | — | (modified — adds viper + yaml.v3 + testify require lines) | modified |

---

## Pattern Assignments

### `internal/scheduler/config.go` (model — config types)

**Analog:** Phase-1 `internal/profile/` (intended, not yet executed) — the profile-as-config-bundle pattern (STACK §Focus 1). The scheduling config is the same shape: a declarative YAML struct loaded by viper, not code. Closest EXECUTED analog: none (greenfield). The Go-style + package-doc reference is `internal/redact/redact.go` (the package comment block + struct-with-yaml-tags style).

**Pattern:** struct types with `yaml:"..."` tags matching the schema in `03-RESEARCH` §1.2 verbatim. The package comment follows `internal/redact`'s style (a doc block explaining the package's role + the load-bearing decisions it implements). No methods on the config structs (they're pure data); behavior lives in `load.go`, `resolver.go`, etc.

**Imports pattern:** `time` (for `time.Duration` fields in breaker/cost config). YAML tags via `gopkg.in/yaml.v3` (viper unmarshals into the struct; the yaml tags are the source of truth for field names).

---

### `internal/scheduler/load.go` (service — viper load + layering + Validate)

**Analog:** `spf13/viper` standard layered-load pattern (STACK inherited core tech). The `.claude/` settings hierarchy (user → project) is the layering model (C3). Closest EXECUTED analog: none in-repo (Phase 1's profile loader is intended but not executed).

**Pattern:** `viper.New()` → `SetConfigType("yaml")` → read embedded default (`go:embed defaults/scheduling.yaml`) via `ReadConfig(bytes.NewReader(...))` → `AddConfigPath` for global (`~/.ass-guard/`) → `MergeInConfig` → `AddConfigPath` for project (`<cwd>/.ass-guard/`) → `MergeInConfig` (project wins). Then `Unmarshal(&cfg)`. Then apply defaults (zero-field → default). Then `Validate(&cfg)` — collect ALL violations, return `*ConfigError` listing every one (03-RESEARCH §7.3, pitfall 10).

**Layering discipline (pitfall 1):** viper file-merge (project → global → embedded default) is the FILE layer. The D-02 RESOLUTION order (time-window → project → global) is applied LATER by the resolver over the MERGED config. Do NOT mix these — load fully first, then resolve. The loader returns one fully-merged `*Config`; the resolver takes it as input.

**Imports pattern:** `bytes`, `embed`, `fmt`, `github.com/spf13/viper`, `sort` (for deterministic violation ordering), `gopkg.in/yaml.v3` (for the embedded default parse, if needed).

---

### `internal/scheduler/init.go` (init — tzdata blank import + defaults)

**Analog:** Go stdlib blank-import idiom (`_ "time/tzdata"`) — the same pattern as `_ "github.com/lib/pq"` for `database/sql` drivers. Documented in `pkg.go.dev/time/tzdata`.

**Pattern:** a single blank import + the `defaults` var/list. This file exists so there is ONE place the tzdata import lives (belt-and-suspenders against someone removing it from a file that calls `time.LoadLocation`). The `defaults` application (`zeroField → default`) can live here or in `load.go`; keep it here so `load.go` is pure I/O.

**The single-binary guarantee (pitfall 2):** the bundled tzdata (~450KB, pure Go) makes `time.LoadLocation("America/New_York")` work on macOS + Linux without host zoneinfo. A test (`TestTzdataBundled`) loads a non-host non-UTC zone and asserts no error — this is the test that catches a removed blank import before deploy.

---

### `internal/scheduler/resolver.go` (service — pure transform, the D-02 algorithm)

**Analog:** none in-repo (greenfield). Closest EXECUTED test-style analog: `internal/redact/redact_test.go` (table-driven tests). The resolver is a PURE FUNCTION of `(config, tier, project, now, capReq)` — the cleanest testable shape (03-RESEARCH §4, §13.1).

**Pattern:** `func (r *Resolver) Resolve(tier, project string, now time.Time, capReq CapabilityReq) (Target, []Target, error)`. The 4-line algorithm (03-RESEARCH §4.1):
1. `w := activeWindow(cfg.TimeWindows, now)` (D-03, evaluated first per D-02)
2. `if w != nil && w.Tiers[tier] set → return w.Tiers[tier]` (STRUCTURAL window-wins-outright — the D-02 reversal, pitfall 8)
3. `else if cfg.Projects[project].Tiers[tier] set → return it` (PROJECT narrows the gap)
4. `else → return cfg.Tiers[tier]` (GLOBAL fallback)

Then enrich: attach `cfg.Models[binding.Model]` (capabilities + pricing) and resolve `binding.Fallback` slugs. Then capability-filter against `capReq` (03-RESEARCH §7.2).

**D-02 reversal test (load-bearing, pitfall 8):** the test table MUST include the cell where BOTH window AND project map the tier, and assert the WINDOW's pick wins (not the project's). This is the decision the user explicitly reversed; a project-first implementation "looks right" and is wrong.

**Window matching (pitfall 3):** the overnight-wrap (`from > to`, e.g. 22:00→06:00 matches 23:59 AND 01:00), the `to`-exclusive boundary (09:00–17:00 does NOT contain exactly 17:00), the weekday filter. Table-test every boundary.

**Imports pattern:** `time`, `fmt`. NO `sync` (the resolver is pure; concurrency lives in Dispatch + the breaker). NO logging (the resolver returns errors/values; Dispatch logs outcomes).

---

### `internal/scheduler/events.go` (model — ProviderFallback + CostCeilingWarn)

**Analog:** Phase-2 `internal/event/bus.go` (intended, 02-RESEARCH §2.2) — the `Event` interface + `Kind() string` + the channel-per-type catalog. Phase 3 EXTENDS the catalog additively (the same grow-don't-replace pattern Phase 2 used on Phase 1's seed).

**Pattern:** define `ProviderFallback` and `CostCeilingWarn` structs implementing `event.Event` (`Kind() string` returning `"ProviderFallback"` / `"CostCeilingWarn"`). Field shapes per 03-RESEARCH §5.2 and §6.2. The Phase-2 ACP adapter (02-RESEARCH §8.2) subscribes to these kinds and maps them to info/warn `session/update` notifications — Phase 3 defines the events; Phase 2's adapter (when executed) consumes them.

**Bus extension contract (03-RESEARCH §0):** Phase 3 does NOT rewrite the bus — it adds two kinds. The `event.Bus.Subscribe("ProviderFallback", buffer)` / `Publish(ProviderFallback{...})` calls use the Phase-2 bus API unchanged. If Phase 2's bus isn't executed yet, Phase 3's tests use a minimal local bus stub (same Subscribe/Publish shape) that Phase 2's real bus drops in over.

---

### `internal/provider/errors.go` (model — ProviderError typed error, D-04)

**Analog:** `internal/redact/redact.go` (the EXECUTED in-repo typed-error + `ScrubError` idiom, C2/C4) + Phase-2 `RPCError` (intended, 02-RESEARCH §7.1). The typed-error-with-Kind pattern is the Go idiom for errors the caller pattern-matches on.

**Pattern (03-RESEARCH §3.1):**
```go
type ErrorKind string
const ( KindTransient ErrorKind = "Transient"; KindStructural = "Structural"; KindExhausted = "Exhausted" )
type ProviderError struct {
    Kind ErrorKind; Provider, Model string; StatusCode int; Reason string; Cause error
}
func (e *ProviderError) Error() string  // investigate-and-fix-ready (C5): names provider, model, kind, status, reason
func (e *ProviderError) Unwrap() error { return e.Cause }
func ClassifyHTTP(provider, model string, status int, err error) *ProviderError
```

**Classification table (03-RESEARCH §3.2):** Transient = {408,425,429,5xx,net errors, ctx deadline}; Structural = {400,401,403,404,405,411,413,422}; Exhausted = NEVER from `ClassifyHTTP` (only the cost tracker constructs it — pitfall 7, unit-test this invariant).

**C2 integration:** `Error()` should route through `redact.ScrubError` on the `Cause` before formatting, in case the wrapped cause is an SDK error containing a request body. The provider/model slugs are NOT secret (operator-visible) but the cause may be.

**SDK conformance (intended-interface, 03-RESEARCH §3.3):** when Phase-1 adapters ship, `AnthropicProvider.Send` wraps `*anthropic.RequestError` via `ClassifyHTTP("anthropic", model, reqErr.StatusCode, reqErr)`; `OpenAIProvider.Send` wraps `*openai.APIError` via `ClassifyHTTP("openai", model, apiErr.HTTPStatusCode, apiErr)`. Phase 3 defines the classifier; Phase 1 conforms. Phase 3's tests use a fake provider that returns canned `*ProviderError`.

---

### `internal/scheduler/breaker.go` (service — state machine + concurrency, D-07)

**Analog:** `github.com/sony/gobreaker` (the canonical Go circuit-breaker — Closed/Open/HalfOpen + `ReadyToTrip` callback). **Study, NOT a dependency** (single-binary + custom dual-mechanism trip). 03-RESEARCH §6.1.

**Pattern (sony/gobreaker distilled):** a `Breaker` struct per `(provider, model)` key, holding `state`, `consecutive` counter, a fixed-capacity ring buffer for error-rate, `openedAt` for cooldown. Methods: `Allow(now) bool` (Closed→pass; Open→check cooldown→HalfOpen; HalfOpen→allow-probe), `RecordSuccess()`, `RecordTransient(now)` (trip if consecutive>=N OR error-rate>threshold). The state machine is the 8-row transition table in 03-RESEARCH §6.1 — test every row (03-VALIDATION §13.2).

**Concurrency discipline (pitfall 6):** the mutex is held ONLY across state read/write (`Allow`, `RecordSuccess`, `RecordTransient`) — NEVER across the provider call. Parent + subagents (Phase-2 PARA) hit the same `(provider, model)` breaker concurrently; a mutex held across the provider call would serialize all dispatches and defeat the Phase-2 semaphore. Test: 100 goroutines `RecordTransient`/`RecordSuccess` under `-race`, assert no race + consistent final state.

**Structural errors don't feed the breaker (pitfall 4):** only `RecordTransient` exists; there is no `RecordStructural`. A 401 (bad auth key) is a config bug, not an outage — recording it would trip the breaker on every request and mask the real cause. Test this invariant.

**Logging (C5):** every state transition logged at Warn via `log/slog` to stderr: `(provider, model, from→to, reason, consecutive, error_rate, cooldown_remaining)`. A breaker trip without a log line is a bug.

---

### `internal/scheduler/cost.go` (service — fixed-window accumulator, D-08)

**Analog:** none in-repo (greenfield). 03-RESEARCH §6.2. Closest pattern: a simple `map[time.Time]float64` keyed by window-boundary (`now.Truncate(cfg.Window)`), with a `degraded bool` flag.

**Pattern:** `Account(model, inTokens, outTokens) float64` (cost = `(in×input_per_mtoken + out×output_per_mtoken)/1e6`, from the resolved model's `Pricing`), `Check(now) CostAction` (Allow/Degrade/HardStop). Fixed calendar-aligned window (MVP — simpler than sliding; deterministic for tests via injected `now`). Window rollover: when `now` crosses a boundary, freeze+log the prior window, reset spent+degraded.

**Degrade-then-stop (D-08 verbatim):** first breach → emit `CostCeilingWarn` + set degraded + resolve to `degrade_to` tier. Second breach (degraded tier also hits ceiling) → HardStop → `&ProviderError{Kind: KindExhausted}`. NOT hard-stop on first breach (pitfall: the "graceful degradation" the ROADMAP goal names).

**Token-count dependency (pitfall 5):** if the Phase-1 adapter's `Response` doesn't expose token counts yet, `Account` logs at Warn "cost tracking disabled: no token counts" and accounts zero (the ceiling can't trip without token counts — documented limitation, not a crash). Define a `TokenCounts() (in, out int)` accessor on the response shape Phase 3 expects (intended-interface).

---

### `internal/scheduler/dispatch.go` (service — the Scheduler composite, §8)

**Analog:** none in-repo (greenfield). 03-RESEARCH §8. This is the turn-loop seam: `Scheduler.Dispatch(ctx, tier, project, capReq, messages) (Response, error)`.

**Pattern:** orchestrates resolve → capability gate → breaker check → cost check → `sem.Acquire` → `provider.Send` → `sem.Release` → on Transient: emit `ProviderFallback`, walk fallback → `cost.Account` → return. The full algorithm is 03-RESEARCH §8.2. The developer-facing surface is `tier` (heavy/good/light); everything else is operator config (the phase-goal promise: "developer never thinks about which concrete model is running").

**Fake-provider test pattern:** a `fakeProvider` in `_test.go` implementing the `provider.Provider` interface, returning canned `Response`/`*ProviderError` per `(provider, model)` key from a test-defined map. This exercises the full Dispatch orchestration deterministically with no network (03-VALIDATION §13.5).

**Semaphore reuse (03-RESEARCH §0):** `sem.Acquire(ctx)`/`Release()` wraps EVERY provider call — primary AND each fallback attempt. The fallback walker does NOT bypass the Phase-2 semaphore (a fallback is still an outbound request).

---

### `cmd/ass-guard/scheduling_*.go` (command — operator CLI)

**Analog:** Phase-1 `cmd/ass-guard/profile_check.go` (intended — the cobra-subcommand pattern, `profile check` from Phase-1 D-08) + Phase-2 `cmd/ass-guard/acp_serve.go` (02-01-PLAN T3). The existing `cmd/ass-guard/main.go` is a stub (`func main() {}`) — Phase 3 adds the cobra root IF Phase 1 hasn't; otherwise extends it.

**Pattern:** two cobra subcommands under a `scheduling` parent:
- `ass-guard scheduling validate [--config <path>]` — loads + validates the config; prints the validation report to STDERR on failure (transport discipline, C1/pitfall 9); exits non-zero. On success: a one-line "scheduling config valid" to stderr + exit 0.
- `ass-guard scheduling resolve --tier <heavy|good|light> [--project <name>] [--at <RFC3339>] [--json]` — resolves a tier at a time (default `time.Now()`, or `--at` for a specific moment) and prints the resolved `(provider, model)` + fallback chain + capability profile. Default human-readable form to STDERR; `--json` machine-readable form to STDOUT (the ONLY stdout output, explicitly requested — transport discipline).

**C1 enforcement (pitfall 9):** the default (no `--json`) NEVER writes to stdout. A test asserts stdout is empty for the human-readable form and non-empty only with `--json`.

---

## Reused / unchanged

| Asset | Phase 3 use |
|---|---|
| `internal/redact/` (Phase-1, executed) | imported — `ProviderError.Error()` + cost/fallback logs route through `redact.ScrubError` (C2) |
| `internal/event/` (Phase-2 intended) | extended additively — two new event kinds (ProviderFallback, CostCeilingWarn); the bus API is unchanged |
| `internal/provider/` doc.go (Phase-1 placeholder) | EXTENDED — Phase 3 adds `errors.go` (ProviderError); the adapter impls land in Phase 1 and call `ClassifyHTTP` |
| `cmd/ass-guard/main.go` (stub) | EXTENDED — cobra root + `scheduling` subcommand parent |

---

*Phase: 3-Model Scheduling — Pattern Map*
*Mapped: 2026-08-09*
*Greenfield caveat carries from Phase 1/2: in-repo analogs are `internal/redact` (executed) + Phase-1/2 intended interfaces (02-RESEARCH §2/§8.2/§11). External templates (LiteLLM router, sony/gobreaker, Go `time/tzdata`) studied, NOT depended on — single-static-binary constraint (PROJECT.md).*
