# Phase 3: Model Scheduling — Research

**Researched:** 2026-08-09
**Status:** Ready for planning

> This research answers "what do I need to know to PLAN Phase 3 well?" It does NOT relitigate CONTEXT.md locked decisions (D-01..D-10 — every one user-answered, zero Claude's-discretion). It resolves the items the decisions explicitly deferred (circuit-breaker thresholds D-07; cost-tracking mechanics D-08; the D-02 precedence algorithm; the capability-profile schema D-09; the `ProviderError` classification table D-04; how `time/tzdata` is bundled D-03), gives the planner concrete identifiers, type names, file layout, and the design template distilled from LiteLLM's router. Every load-bearing claim is grounded in a dated, inspectable source.

## 0. Dependency framing (load-bearing — read first)

Phase 3 sits **above** Phase 1 (provider adapters) and Phase 2 (event bus, semaphore, Session Core). Both are **planned but not executed**: Phase 1 is blocked on data-source drift (STATE.md blocker), Phase 2 has plans `02-01..02-07` but no execution. This research writes against the Phase 1+2 **intended interfaces** — the same approach the Phase 2 planner took writing against Phase 1's intended post-execution state:

- **Provider interface** — `internal/provider` (Phase 1 PROV-01..03, Phase 2 §11.2): `Provider.Send(ctx, profile, messages) (Response, error)` + `Provider.Stream(...)`. `Response{ToolCalls []ToolCall, FinishReason string, Raw json.RawMessage}`. **Phase 3 adds `ProviderError`** to this package (`internal/provider/errors.go`) — the typed error the adapters return. When Phase 1's adapters ship, they populate `ProviderError.Kind` via the classifier defined here (D-04). Phase 3 defines the type + classifier; Phase 1 conforms.
- **Event bus** — `internal/event` (Phase 2 D-04, §2): `Bus.Subscribe(kind, buffer) <-chan Event`, `Bus.Publish(Event)`. **Phase 3 adds two event kinds** (additive — the same grow-don't-replace pattern Phase 2 used on Phase 1's seed): `ProviderFallback` (info) and `CostCeilingWarn` (warn). They route through the bus → Phase 2 ACP adapter → `session/update`.
- **Provider semaphore** — `internal/provider` (Phase 2 D-12, §11.1, default 6): `Semaphore.Acquire/Release`. The scheduler's fallback-walker reuses it — every fallback attempt goes through the same outbound-concurrency gate as the primary.
- **Turn loop** — `internal/loop` (Phase 1 D-12, replaced by `internal/session` in Phase 2 D-17): the caller that requests a tier. Phase 3's `Scheduler.Dispatch(ctx, tier, project, capReq, messages)` is the seam the turn loop calls instead of calling `Provider.Send` directly.

**Implication for plans:** Phase 3's tracer (Plan 03-01) needs only the scheduling-specific deps (`spf13/viper`, `gopkg/yaml.v3`, `stretchr/testify`) — no provider SDK, because the resolver is a pure function of `(config, tier, project, now)` and the provider is a stub at tracer fidelity. The runtime plans (03-02..03-04) consume the Provider interface via a fake in tests; the real adapters land in Phase 1. The current `go.mod` has zero `require` lines and `go.sum` is empty, so Plan 03-01 T1 adds the scheduling deps.

## 1. Config schema (D-01 — declarative YAML, viper-loaded, layered)

### 1.1 File location + layering

D-01 mandates a **declarative YAML file, viper-loaded, layered (global → per-project `.ass-guard/scheduling.yaml`)**. Concrete shape:

- **Global:** `~/.ass-guard/scheduling.yaml` (user-wide default) OR a path passed via `--scheduling-config` / env `ASSGUARD_SCHEDULING_CONFIG`. This is the operator-authored floor.
- **Per-project:** `<cwd>/.ass-guard/scheduling.yaml`. Viper merges it ON TOP of the global (project values win for keys present in both). This honors D-03's per-project override and the `.ass-guard/` convention from Phase 2 D-06 (the same self-gitignoring dir).
- **Bundled default:** a `defaults/scheduling.yaml` embedded via `go:embed` — the zero-config fallback so a fresh install works (DIST-03). Highest-priority-default, lowest-precedence: global overrides it, project overrides global.

Viper precedence (highest wins): project `.ass-guard/scheduling.yaml` → global → embedded default. This is viper's standard `SetConfigName` + `AddConfigPath` + `MergeInConfig` chain. **Note:** this viper *file* layering is distinct from the D-02 *resolution* precedence (time-window → project → global) — §4 covers resolution; §1 covers file load. The two are different axes (file-merge vs runtime-resolution) and must not be conflated.

### 1.2 The YAML schema (the operator's surface)

Distilled from STACK Focus 2's LiteLLM-template + the D-01..D-10 decisions. This is the contract the operator edits:

```yaml
# scheduling.yaml — operator-authored model scheduling config
timezone: "America/New_York"      # IANA zone for ALL window evaluation (D-03)

# --- capability profiles (D-09): one per (provider, model) ---
providers:
  anthropic:
    base_url: "https://api.z.ai/api/anthropic"   # Z.ai for GLM (VERIFIED-FACTS #1)
    shape: anthropic                              # selects the adapter
  openai:
    base_url: "https://api.openai.com/v1"
    shape: openai
  groq:
    base_url: "https://api.groq.com/openai/v1"
    shape: openai

models:
  glm-5.2:                         # key is the model slug; provider scopes it
    provider: anthropic
    pricing: { input_per_mtoken: 0.60, output_per_mtoken: 2.20 }  # USD / 1M tokens (D-08)
    capabilities:                  # D-09 structured capability declaration
      context_window: 200000
      max_output_tokens: 32000
      tool_calling: true
      streaming: true
      extended_thinking: true
      limitations: []
  glm-4.6:
    provider: anthropic
    pricing: { input_per_mtoken: 0.15, output_per_mtoken: 0.60 }
    capabilities: { context_window: 128000, max_output_tokens: 16000, tool_calling: true, streaming: true, extended_thinking: false }
  minimax-m3:
    provider: openai
    pricing: { input_per_mtoken: 0.10, output_per_mtoken: 0.30 }
    capabilities: { context_window: 200000, max_output_tokens: 32000, tool_calling: true, streaming: true, extended_thinking: false }
  haiku-cheap:
    provider: groq
    pricing: { input_per_mtoken: 0.05, output_per_mtoken: 0.10 }
    capabilities: { context_window: 64000, max_output_tokens: 8000, tool_calling: false, streaming: true, extended_thinking: false }

# --- tier table (SCHED-01): heavy/good/light abstract the concrete model ---
tiers:
  heavy:
    model: glm-5.2                 # primary binding: a models{} key
    fallback: [minimax-m3, glm-4.6]   # D-05 explicit ordered per-tier fallback list
  good:
    model: glm-4.6
    fallback: [minimax-m3]
  light:
    model: minimax-m3
    fallback: [haiku-cheap]

# --- time windows (SCHED-02, D-03): structural peak/off-peak substitution ---
time_windows:
  - name: peak
    zone: "America/New_York"       # overrides top-level timezone if set
    schedule:
      from: "09:00"
      to: "17:00"
      days: [Mon, Tue, Wed, Thu, Fri]   # omit for every-day
    tiers:                         # partial override: only the tiers this window substitutes
      heavy: { model: minimax-m3, fallback: [glm-5.2] }
  - name: overnight
    schedule: { from: "22:00", to: "06:00" }   # wraps past midnight (from > to)
    tiers:
      heavy: { model: glm-5.2, fallback: [] }

# --- per-project override (SCHED-03): narrows within the active table (D-02) ---
projects:
  myproj:
    tiers:
      good: { model: minimax-m3, fallback: [] }   # this project prefers minimax for "good"

# --- safety nets (SCHED-05) ---
circuit_breaker:                   # D-07 — defaults documented; tunable
  consecutive_failures: 5          # trip after N consecutive Transient (hard outage)
  error_rate_window: 20            # M requests in the sliding window
  error_rate_threshold: 0.50       # trip when >50% errors over the last M
  cooldown: 60s                    # open state duration before half-open probe
  half_open_probes: 1              # probe requests in half-open (recovery test)

cost_ceiling:                      # D-08 — dollars per time window, degrade-then-stop
  amount_usd: 50.0                 # the ceiling
  window: 24h                      # the window length (fixed window, calendar-aligned)
  degrade_to: light                # tier to degrade to on breach
```

**Why this shape:** every operator knob (tiers, windows, projects, pricing, capabilities, breaker, cost) is one commented section. The `models:` block is the single source of truth for capability profiles (D-09) + pricing (D-08) — referenced by slug from `tiers`, `time_windows.tiers`, `projects.tiers`, and `fallback` arrays. This makes D-10 validation a graph check: every referenced slug must exist, and every fallback's capabilities must be compatible with its primary's (§7).

### 1.3 Go types (`internal/scheduler/config.go`)

```go
type Config struct {
    Timezone       string                       `yaml:"timezone"`
    Providers      map[string]ProviderConfig    `yaml:"providers"`
    Models         map[string]ModelConfig        `yaml:"models"`      // key = model slug
    Tiers          map[string]TierBinding        `yaml:"tiers"`       // heavy/good/light
    TimeWindows    []TimeWindow                 `yaml:"time_windows"`
    Projects       map[string]ProjectOverride   `yaml:"projects"`
    CircuitBreaker CircuitBreakerConfig         `yaml:"circuit_breaker"`
    CostCeiling    CostCeilingConfig            `yaml:"cost_ceiling"`
}

type ProviderConfig struct {
    BaseURL string `yaml:"base_url"`
    Shape   string `yaml:"shape"` // "anthropic" | "openai"
}

type ModelConfig struct {
    Provider     string             `yaml:"provider"`      // a providers{} key
    Pricing      Pricing            `yaml:"pricing"`
    Capabilities CapabilityProfile `yaml:"capabilities"`
}

type Pricing struct {
    InputPerMToken  float64 `yaml:"input_per_mtoken"`   // USD per 1M input tokens
    OutputPerMToken float64 `yaml:"output_per_mtoken"`  // USD per 1M output tokens
}

type CapabilityProfile struct {       // D-09
    ContextWindow     int      `yaml:"context_window"`
    MaxOutputTokens   int      `yaml:"max_output_tokens"`
    ToolCalling       bool     `yaml:"tool_calling"`
    Streaming         bool     `yaml:"streaming"`
    ExtendedThinking  bool     `yaml:"extended_thinking"`
    Limitations       []string `yaml:"limitations"`
}

type TierBinding struct {
    Model    string   `yaml:"model"`     // a models{} slug
    Fallback []string `yaml:"fallback"`  // ordered list of models{} slugs (D-05)
}

type TimeWindow struct {
    Name     string                `yaml:"name"`
    Zone     string                `yaml:"zone"`     // overrides Config.Timezone
    Schedule Schedule              `yaml:"schedule"`
    Tiers    map[string]TierBinding `yaml:"tiers"`  // partial override
}

type Schedule struct {
    From string   `yaml:"from"`    // "HH:MM"
    To   string   `yaml:"to"`      // "HH:MM" — if To < From, wraps past midnight
    Days []string `yaml:"days"`    // Mon/Tue/...; omit = every day
}

type ProjectOverride struct {
    Tiers map[string]TierBinding `yaml:"tiers"` // narrows within the active table (D-02)
}

type CircuitBreakerConfig struct { // D-07
    ConsecutiveFailures  int           `yaml:"consecutive_failures"`
    ErrorRateWindow      int           `yaml:"error_rate_window"`
    ErrorRateThreshold   float64       `yaml:"error_rate_threshold"`
    Cooldown             time.Duration `yaml:"cooldown"`
    HalfOpenProbes       int           `yaml:"half_open_probes"`
}

type CostCeilingConfig struct {    // D-08
    AmountUSD  float64       `yaml:"amount_usd"`
    Window     time.Duration `yaml:"window"`
    DegradeTo  string        `yaml:"degrade_to"` // a tiers{} key
}
```

**Defaults (applied when a field is zero after merge):** `ConsecutiveFailures=5`, `ErrorRateWindow=20`, `ErrorRateThreshold=0.50`, `Cooldown=60s`, `HalfOpenProbes=1`. `CostCeiling.Window=24h`. These are the documented D-07/D-08 defaults — operator-tunable via config.

## 2. Time evaluation (D-03 — lazy per-request, IANA, `time/tzdata`)

### 2.1 `time/tzdata` bundling (the single-binary guarantee)

`import _ "time/tzdata"` bundles the IANA tz database INTO the binary (~450KB, pure Go). Without it, `time.LoadLocation("America/New_York")` reads `/usr/share/zoneinfo` — which may be absent or stale on minimal Linux images and breaks the single-static-binary / no-runtime-deps constraint (PROJECT.md). The bundled db makes `time.LoadLocation` work identically on macOS + Linux, amd64 + arm64, regardless of host. This is the standard Go idiom for timezone-aware binaries; verified against the `time` package docs (the `tzdata` import is the documented bundle mechanism). **Every file in `internal/scheduler` that calls `time.LoadLocation` MUST be covered by one blank import in `internal/scheduler/init.go` (or the package's single `_ "time/tzdata"` import) — belt-and-suspenders via a test that loads a non-UTC non-host zone.**

### 2.2 Lazy per-request evaluation (D-03)

The resolver is called per request: `Resolve(tier, project, now time.Time, capReq CapabilityReq)`. The turn loop passes `now := time.Now()` (live wall-clock). A turn's tool-loop re-resolves on each iteration — a turn that crosses a window boundary switches models between tool-call iterations. This is acceptable (D-03): boundaries cross between iterations, not within a single provider request. The resolver is a **pure function of `(config, tier, project, now)`** — no cached "current model," no stale precomputation. This is the cleanest testable shape: table tests inject `now` values.

### 2.3 Window matching (the overnight-wrap + days-of-week logic)

```go
// activeWindow returns the TimeWindow whose Schedule contains t (in the window's
// zone), or nil if none. Schedule.From/To are "HH:MM" in the window's zone.
func activeWindow(windows []TimeWindow, t time.Time) *TimeWindow {
    for i := range windows {
        w := &windows[i]
        if w.contains(t) { return w }
    }
    return nil
}

func (w TimeWindow) contains(t time.Time) bool {
    loc := zone(w.Zone)                      // time.LoadLocation, fallback UTC
    lt := t.In(loc)
    if !dayMatches(w.Schedule.Days, lt.Weekday()) { return false }
    from := parseHHMM(w.Schedule.From, lt)   // today's from-time
    to   := parseHHMM(w.Schedule.To, lt)
    if from.Before(to) || from.Equal(to) {   // same-day window, e.g. 09:00-17:00
        return !lt.Before(from) && lt.Before(to)
    }
    // overnight wrap, e.g. 22:00-06:00: matches if lt >= from OR lt < to
    return !lt.Before(from) || lt.Before(to)
}
```

Edge cases the planner must test: overnight wrap (22:00→06:00), the `to`-exclusive boundary (a window 09:00–17:00 does NOT contain exactly 17:00), day-of-week filter (weekdays-only), empty `days` = every day, window zone overriding the global `timezone`, and multiple overlapping windows (the FIRST matching window in config order wins — documented; operator should not define overlapping windows, but if they do, first-match is deterministic).

### 2.4 Overlapping-window policy

If two windows match the same `now`, the **first in config order** wins. This is deterministic but the operator should avoid overlaps. D-10 validation (§7) **warns** (does not reject) on overlapping windows — overlap is a soft misconfiguration, not a hard inconsistency. Rejection is reserved for capability conflicts (D-10's hard guarantee).

## 3. `ProviderError` + classification (D-04)

### 3.1 The type (`internal/provider/errors.go` — Phase 3 introduces this)

```go
package provider

type ErrorKind string

const (
    KindTransient ErrorKind = "Transient"  // 429/5xx/network/timeout → walk fallback chain
    KindStructural ErrorKind = "Structural" // 401/403/400/404 → report, NO retry
    KindExhausted  ErrorKind = "Exhausted"  // budget/ceiling hit → hard stop (D-08)
)

// ProviderError is the typed error every Provider implementation returns from
// Send/Stream. The adapter classifies (it sees the HTTP status); the scheduler
// pattern-matches on Kind (D-04).
type ProviderError struct {
    Kind       ErrorKind
    Provider   string   // the provider slug
    Model      string   // the model slug
    StatusCode int      // HTTP status (0 for non-HTTP errors)
    Reason     string   // human-readable, investigate-and-fix-ready (PROJECT.md)
    Cause      error    // wrapped underlying error (net.OpError, context.DeadlineExceeded, ...)
}
func (e *ProviderError) Error() string  // "provider X model Y: <Kind> (HTTP %d): %s: %v"
func (e *ProviderError) Unwrap() error { return e.Cause }

// ClassifyHTTP maps an HTTP status + wrapped error to a ProviderError Kind.
// Used by the adapter implementations (Phase 1) and unit-tested in Phase 3.
func ClassifyHTTP(provider, model string, status int, err error) *ProviderError
```

### 3.2 The classification table (D-04 concrete mapping)

| HTTP status / condition | Kind | Rationale |
|---|---|---|
| 408, 425, 429 | Transient | Timeout / too-many-requests / too-early — retryable, provider is overwhelmed |
| 500, 502, 503, 504 | Transient | Server/gateway errors — provider down or degraded, retryable |
| net.OpError, context.DeadlineExceeded, context.Canceled, io.EOF mid-stream, connection reset | Transient | Network failures — transient by definition |
| 400, 401, 403, 404, 405, 411, 413, 422 | Structural | Bad request / unauthenticated / forbidden / not-found / unsupported — the operator must fix; retrying won't help |
| 200 (success) | (not an error) | — |
| (none — ceiling hit) | Exhausted | **NOT set by the adapter** — set by the scheduler's cost tracker (D-08) when the cost ceiling is breached. The adapter cannot see the budget. |

**The Exhausted kind is scheduler-internal.** The adapter produces only Transient/Structural. The scheduler constructs `&ProviderError{Kind: KindExhausted, ...}` when the cost tracker trips (§6). This keeps D-04's "adapter classifies, scheduler pattern-matches" clean: the adapter owns HTTP→{Transient,Structural}; the scheduler owns the budget→Exhausted escalation. The scheduler pattern-matches all three on the same `.Kind` field.

### 3.3 SDK error-shape grounding (for the Phase-1 adapter conformance, cited here)

- `anthropics/anthropic-sdk-go` returns `*anthropic.RequestError` with a `StatusCode int` field (and `*anthropic.APIError` for some paths). The Phase-1 Anthropic adapter wraps these: `ClassifyHTTP("anthropic", model, reqErr.StatusCode, reqErr)`.
- `sashabaranov/go-openai` returns `*openai.APIError` with `HTTPStatusCode int`. The Phase-1 OpenAI adapter wraps: `ClassifyHTTP("openai", model, apiErr.HTTPStatusCode, apiErr)`.
- Both SDKs surface network errors as plain `net.OpError` / `*url.Error` — the classifier's net-error branch catches these via `errors.Is(err, context.DeadlineExceeded)` / `errors.As(err, &netErr)`.

Phase 3 defines `ClassifyHTTP` + the type. Phase 1's adapters call it. **This is the intended-interface contract Phase 3 writes against** (the dep note, §0).

## 4. Resolution algorithm (D-02 — time-window → project → global)

### 4.1 The interpretation (resolving D-02's wording)

D-02 states the precedence is **time-window → project → global** ("first match wins; time-window is checked first because it's a structural constraint") AND that "the per-project override narrows within whatever the time-window picked, rather than replacing it entirely; the operator's time-window rules are the floor that project-level tweaks build on."

These two phrasings tension: "first match wins, window first" (window outright overrides project) vs "project narrows within the window's pick" (project refines the window). The operationally-defensible reading, grounded in the SPECIFICS section ("operator-cost-protection priority: time-windows are the floor projects build on"), is:

> **The time-window is the structural floor. The project override fills the gaps the window leaves — it cannot displace a window's structural pick. The global tier table is the final fallback.**

This honors "first match wins" (window checked first) AND "projects build on the floor" (projects refine only what the windows don't structurally pin). Concretely:

```
Resolve(tier, project, now):
  1. w := activeWindow(config.TimeWindows, now)         # time-window evaluated first (D-02, D-03)
  2. IF w != nil AND w.Tiers[tier] is set:
        RETURN w.Tiers[tier]                              # STRUCTURAL: window wins outright (the floor)
  3. ELSE IF config.Projects[project].Tiers[tier] is set:
        RETURN config.Projects[project].Tiers[tier]       # PROJECT: narrows (fills the gap the window left)
  4. ELSE:
        RETURN config.Tiers[tier]                          # GLOBAL: the default table
```

**What this means for the operator:** if the peak window maps `heavy → minimax-m3`, a project cannot get `glm-5.2` for heavy during peak (the window is structural — peak capacity is a deployment-wide reality). Off-peak (no window active, or window doesn't map heavy), the project override for heavy applies. This is the deliberate operator-cost-protection priority the user reversed D-02 for.

**Capability enrichment:** after step 2/3/4 produces a `TierBinding{Model, Fallback}`, the resolver looks up `config.Models[binding.Model]` to attach the capability profile + pricing, and resolves `binding.Fallback` slugs to their model configs. The return is `(Target, []Target, error)` where `Target{Provider, Model, BaseURL, Shape, Capabilities, Pricing}`.

**Capability requirement filtering (D-09 runtime — §7.2):** `Resolve` takes a `capReq CapabilityReq{NeedsTools, NeedsStreaming, NeedsThinking}`. If the primary `Target`'s capabilities don't satisfy `capReq`, the resolver walks the fallback list and returns the first candidate that does (or an error if none). This is the request-time capability gate that pairs with D-10's load-time validation.

### 4.2 Why not two-level tables

A more elaborate design would have windows *select a named table* and projects override *within that table*. Rejected for MVP: it doubles the config surface (per-window project overrides) for a feature the decision text doesn't clearly require, and the simple 4-line algorithm above is fully deterministic, testable, and matches "first match wins." The planner should implement the 4-line algorithm; the two-level-table design is a documented v2 option if operators need per-window project refinement.

## 5. Fallback chain mechanics (D-05, D-06)

### 5.1 The walker

On a `KindTransient` failure of the primary, the scheduler walks the `TierBinding.Fallback` list **in order**. Each attempt:

1. Look up candidate `Target` (model slug → model config).
2. **Capability gate (D-09 runtime):** if `capReq` is not satisfied by the candidate, skip it (log at Info: "skipping fallback candidate X: no tool_calling"). This prevents routing a tool-using turn to a tool-less model.
3. **Circuit breaker (D-07):** if the candidate's `(provider, model)` breaker is open, skip it (log at Info: "skipping fallback candidate X: circuit open"). §6.
4. **Cost check (D-08):** if the cost ceiling is breached and this candidate is above the degrade-tier, skip it. §6.
5. Acquire the provider semaphore (Phase 2 D-12), call `Provider.Send/Stream`, release.
6. On success → return. On `KindTransient` → continue to next candidate (emit `ProviderFallback` event). On `KindStructural` → STOP the walk, surface the structural error (D-04: report, don't retry). On `KindExhausted` → STOP, hard-stop the turn.

If the list is exhausted without success → return the last Transient error (the turn loop surfaces it; the developer sees a clear "all N candidates failed transiently" message).

### 5.2 The `ProviderFallback` event (D-06)

Phase 3 adds this event kind to `internal/event` (additive — §0):

```go
// ProviderFallback is emitted when a transient failure causes the scheduler to
// walk from one candidate to the next. Info-level → ACP session/update (D-06).
type ProviderFallback struct {
    TurnID      string
    FromProvider string
    FromModel    string
    ToProvider   string
    ToModel      string
    Reason       string         // "Transient: HTTP 429", "circuit open", etc.
    ErrorKind    provider.ErrorKind
    Attempt      int            // 1-based index into the fallback walk
}
func (ProviderFallback) Kind() string { return "ProviderFallback" }
```

The Phase 2 ACP adapter (02-RESEARCH §8.2) maps it to an info `session/update` notification: "provider X failed (429), falling back to provider Y." The developer sees the degradation but isn't blocked (D-06).

## 6. Circuit breakers + cost ceiling (D-07, D-08 — SCHED-05)

### 6.1 Circuit breaker (D-07 — both consecutive + error-rate)

**Per-(provider, model) binding.** A `Breaker` struct holds the state for one `(provider, model)` pair:

```go
type breakerState int
const (
    breakerClosed breakerState = iota   // normal: requests pass, failures counted
    breakerOpen                          // tripped: requests skip this candidate for cooldown
    breakerHalfOpen                      // after cooldown: probe requests test recovery
)

type Breaker struct {
    provider, model string
    mu              sync.Mutex
    state           breakerState
    consecutive     int                 // consecutive Transient failures (D-07 mechanism 1)
    window          *ringBuffer         // last M results for error-rate (D-07 mechanism 2)
    openedAt        time.Time           // when Open was entered (for cooldown)
    cfg             CircuitBreakerConfig
}
```

**State machine (the transitions the planner must test exhaustively):**

| From | Event | To | Action |
|---|---|---|---|
| Closed | success | Closed | reset `consecutive=0`; push success into window |
| Closed | Transient failure | Closed (if below thresholds) OR Open | `consecutive++`; push failure; trip if `consecutive >= N` OR error-rate `> threshold` over last M |
| Closed | Structural/Exhausted | Closed | Structural errors are NOT breaker events (they don't indicate the provider is down — they indicate a bad request). Reset `consecutive=0`, push nothing. Documented. |
| Open | candidate considered | Open → skip (return "circuit open") | the scheduler skips this candidate (§5.1 step 3) |
| Open | `now - openedAt >= cooldown` | HalfOpen | on the NEXT candidate consideration, transition to HalfOpen and allow `half_open_probes` request(s) |
| HalfOpen | probe success | Closed | fully recovered; reset counters |
| HalfOpen | probe failure | Open | `openedAt = now` (restart cooldown) |

**Both mechanisms (D-07 verbatim):** consecutive-failure trips fast on hard outage (5 in a row = clearly down); error-rate trips slow on degraded performance (>50% over last 20). The breaker trips if **either** fires (OR, not AND). The ring buffer is a fixed-capacity `[]bool` (success/failure) — when full, the oldest is evicted; error-rate = failures/len.

**SONY `gobreaker` is the design template** (`github.com/sony/gobreaker` — study, NOT a dep): its Closed/Open/HalfOpen state machine + `ReadyToTrip` callback is the canonical Go circuit-breaker pattern. ass-guard re-implements it in-process (~120 LOC) because (a) the single-static-binary constraint, (b) the dual-mechanism trip (consecutive OR error-rate) is a custom `ReadyToTrip`, (c) per-(provider,model) keying is ass-guard-specific. **No external dep** — this matches STACK's "internal resolver, not a library" discipline.

**Investigate-and-fix-ready logging (PROJECT.md must-have):** every state transition (Closed→Open, Open→HalfOpen, HalfOpen→Closed/Open) is logged at Warn to stderr with `(provider, model, reason, consecutive, error_rate, cooldown_remaining)`. A breaker trip without a log line is a bug.

### 6.2 Cost ceiling (D-08 — dollars per window, degrade-then-stop)

```go
type CostTracker struct {
    mu        sync.Mutex
    cfg       CostCeilingConfig
    spent     map[windowBoundary]float64   // fixed-window accumulation
    degraded  bool                          // true once the primary ceiling is breached
}

// Account records the estimated cost of one completed request.
func (c *CostTracker) Account(model string, inTokens, outTokens int) float64

// Check returns the action for the next request: Allow, Degrade(tier), HardStop.
func (c *CostTracker) Check() CostAction
```

**Cost estimation (D-08):** per request, `cost = (inTokens × Pricing.InputPerMToken + outTokens × Pricing.OutputPerMToken) / 1_000_000`. Token counts come from the provider response (`Response.Raw` → `usage` fields; Phase 1 adapter parses, Phase 3 reads). The pricing is the resolved model's `Pricing` from config. The estimate is **approximate** (it uses the posted pricing; actual billing may differ slightly) — documented; the ceiling is a guard rail, not an accounting system.

**Fixed window (D-08 "per time window"):** MVP uses a **fixed calendar-aligned window** (e.g. `24h` → resets at local midnight, or at the process-start-time + window modulo). Simpler than a sliding window (no ring buffer per request), deterministic for tests (inject `now`). `windowBoundary` is `now.Truncate(cfg.Window)`. A sliding window is a documented v2 option if operators need smoother behavior.

**Degrade-then-stop (D-08 verbatim):**
1. When `spent[thisWindow] >= amount_usd` AND not yet degraded: emit `CostCeilingWarn` event (warn → ACP session/update: "cost ceiling hit ($X/$Y), degrading to light tier"); set `degraded=true`; subsequent requests resolve to the `degrade_to` tier.
2. If `degraded=true` AND `spent[thisWindow] >= amount_usd` for the degraded tier's (lower) ceiling: **hard-stop** — return `&ProviderError{Kind: KindExhausted}`. The turn fails with a clear cost-exhausted error. (The degraded tier's ceiling defaults to the same `amount_usd`; an operator can set a lower per-tier ceiling via a `tiers.<t>.cost_ceiling` override — documented v2 unless trivial.)

**The `CostCeilingWarn` event (D-08):**

```go
type CostCeilingWarn struct {
    TurnID    string
    Window    string         // "24h ending 2026-08-09T00:00:00Z"
    Spent     float64        // USD spent this window
    Ceiling   float64        // the breached amount_usd
    DegradedTo string        // tier slug ("" on the hard-stop warning)
    HardStop  bool           // true when the degraded tier also hit its ceiling
}
func (CostCeilingWarn) Kind() string { return "CostCeilingWarn" }
```

**Window rollover:** when `now` crosses into a new window boundary, `spent` for the previous window is frozen (logged at Info for accounting) and the new window starts at 0; `degraded` resets to false. This is the "per window" semantics — a fresh window is a fresh budget.

## 7. Capability profiles + validation (D-09, D-10 — SCHED-06)

### 7.1 The capability profile (D-09 — declared per model, §1.2)

The `CapabilityProfile` struct (§1.3) is machine-readable: context window, max output, tool-calling, streaming, extended-thinking, limitations. It lives in `models.<slug>.capabilities`. This is what makes the tier abstraction honest: "heavy" on GLM-5.2 (200K context, tools, streaming, thinking) ≠ "heavy" on a different model (different limits). The resolver attaches the profile to every resolved `Target`.

### 7.2 Runtime capability gate (D-09 request-time)

The turn loop passes `CapabilityReq{NeedsTools, NeedsStreaming, NeedsThinking}` to `Scheduler.Dispatch` (derived from the turn: a turn with tools in the catalog sets `NeedsTools=true`; a streaming turn sets `NeedsStreaming=true`; a thinking-enabled profile sets `NeedsThinking=true`). The resolver:

1. Resolves the primary `Target` (§4).
2. If `Target.Capabilities` does not satisfy `capReq` (e.g. `capReq.NeedsTools && !Target.Capabilities.ToolCalling`) → skip the primary, walk the fallback for the first capable candidate (§5.1 step 2 applies the same gate to each fallback).
3. If no candidate satisfies `capReq` → return a structured error ("no candidate for tier heavy satisfies NeedsTools"). Logged at Warn.

This is the **request-time** guarantee; D-10 is the **load-time** guarantee. Together they make tier mismatch explicit at both authoring and request time (CONTEXT.md D-10 closing sentence).

### 7.3 Load-time validation (D-10 — reject inconsistent configs)

A pure function `Validate(cfg *Config) error` runs at viper load (and on config reload). It cross-references the config graph:

| Check | Severity | Example |
|---|---|---|
| Every `tiers.<t>.model` slug exists in `models` | **REJECT** | `tiers.heavy.model: glm-5.2` but no `models.glm-5.2` |
| Every `fallback` slug exists in `models` | **REJECT** | `fallback: [minimax-m3]` but no `models.minimax-m3` |
| Every `models.<m>.provider` exists in `providers` | **REJECT** | `models.glm-5.2.provider: anthropic` but no `providers.anthropic` |
| **Capability consistency (D-10 core):** for each tier, if the primary supports a capability, every fallback must support it too | **REJECT** | `tiers.heavy.model: glm-5.2` (tool_calling:true) but `fallback: [haiku-cheap]` (tool_calling:false) → REJECT with "tier 'heavy' primary glm-5.2 supports tool_calling but fallback haiku-cheap does not — incompatible chain" |
| Same capability check across `time_windows[].tiers` and `projects[].tiers` | **REJECT** | a window's heavy fallback lacks a capability the window's heavy primary has |
| `cost_ceiling.degrade_to` tier exists | **REJECT** | degrade_to: ultra but no tiers.ultra |
| Overlapping time-windows (same schedule) | **WARN** | two windows match the same time — first wins; operator should fix |
| Unknown shape (`providers.<p>.shape` not in {anthropic, openai}) | **REJECT** | shape: gemini |

The capability-consistency check is D-10's hard guarantee: **the operator cannot ship a config where a tool-using tier could fall back to a tool-less model.** The check examines the capability fields that affect correctness (`tool_calling`, `streaming`, `extended_thinking`) — a fallback may have a smaller context window (the turn's request still has to fit, but that's a request-time check) but MUST NOT drop a capability the primary has, because that would silently change model behavior mid-fallback. The validation names the exact conflict in the error message (investigate-and-fix-ready).

**On rejection:** the loader returns a typed `*ConfigError` listing all violations (collect-all, not fail-fast-on-first, so the operator sees every problem in one pass). ass-guard refuses to start (the `acp serve` / any command fails with the validation report to stderr). This is the fail-fast guarantee.

## 8. The Scheduler composite (the turn-loop seam)

### 8.1 The API the turn loop calls

```go
// Package scheduler is the model-scheduling layer. The turn loop selects a tier;
// the Scheduler resolves it to a concrete (provider, model), dispatches through
// the provider adapter, walks the fallback chain on transient failure, and
// enforces circuit breakers + cost ceilings — all transparent to the developer.
package scheduler

type Scheduler struct {
    cfg      *Config
    resolver *Resolver
    breakers map[providerModelKey]*Breaker   // per-(provider,model), D-07
    cost     *CostTracker                    // D-08
    bus      *event.Bus                      // Phase 2 bus (ProviderFallback, CostCeilingWarn)
    sem      *provider.Semaphore             // Phase 2 D-12 (default 6)
    providers map[string]provider.Provider   // slug → adapter (Phase 1)
    now      func() time.Time                // injectable for tests (D-03 lazy)
}

// Dispatch is the turn-loop seam. tier is heavy/good/light; project is the cwd
// project key ("" for the global default); capReq is the turn's capability need.
type CapabilityReq struct { NeedsTools, NeedsStreaming, NeedsThinking bool }

func (s *Scheduler) Dispatch(ctx context.Context, tier, project string,
    capReq CapabilityReq, messages []Message) (provider.Response, error)
```

`Dispatch` orchestrates: resolve → (capability gate) → (breaker check) → (cost check) → semaphore.Acquire → provider.Send/Stream → semaphore.Release → (on Transient: emit ProviderFallback, walk fallback) → cost.Account → return. The developer-facing surface is `tier` (heavy/good/light); everything else is operator config. This is the "developer never thinks about which concrete model is running" promise (phase goal).

### 8.2 The dispatch algorithm (the full picture, tying §4-7 together)

```
Dispatch(ctx, tier, project, capReq, messages):
  primary, fallbacks, err := resolver.Resolve(tier, project, s.now(), capReq)   # §4
  if err: return err                                                            # no capable candidate
  candidates := append([]Target{primary}, fallbacks...)
  var lastErr error
  for i, cand := range candidates:
    # capability gate (§5.1 step 2 / §7.2)
    if !satisfies(cand.Capabilities, capReq): log Info "skip ..."; continue
    # breaker (§6.1)
    b := s.breaker[key(cand)]; if !b.Allow(s.now()): log Info "skip breaker open"; continue
    # cost (§6.2)
    switch s.cost.Check():
      case HardStop: return &ProviderError{Kind: Exhausted, Reason: "cost ceiling exhausted"}
      case Degrade:  degrade tier; re-resolve (one step); continue   # §6.2 degrade-then-stop
    # dispatch through semaphore + provider
    s.sem.Acquire(ctx)
    resp, err := s.providers[cand.Provider].Send(ctx, cand.Model, messages)
    s.sem.Release()
    if err == nil:
       b.RecordSuccess(); s.cost.Account(cand.Model, resp.Tokens); return resp
    perr := asProviderError(err)        # D-04
    b.RecordTransient(perr)             # only Transient affects the breaker (§6.1)
    s.cost.Account(cand.Model, partialTokens)
    if perr.Kind == Structural: return perr                              # report, no retry (D-04)
    if perr.Kind == Exhausted:  return perr                              # hard stop
    # Transient: emit fallback event, continue walk (D-05, D-06)
    if i < len(candidates)-1:
       s.bus.Publish(ProviderFallback{FromModel: cand.Model, ToModel: candidates[i+1].Model, ...})
    lastErr = perr
  return lastErr   # all candidates failed transiently
```

### 8.3 Construction + lifecycle

The Scheduler is constructed once at process startup (in `cmd/ass-guard`, alongside the Phase-2 Session Core construction): load config (viper, §1) → validate (§7.3, reject-on-fail) → build breakers map (one per referenced `(provider, model)`) → build cost tracker → inject the event bus + semaphore + provider map. It's held by the Session Core. On config reload (SIGHUP or future file-watch), the config is re-loaded + re-validated; the resolver + breakers reset; the cost tracker carries over its window accumulation (operator cost continuity — a config reload mid-window does not reset the budget).

## 9. Package + file layout (the planner's map)

| Package | File | Provides | Plan |
|---|---|---|---|
| `internal/scheduler` | `config.go` | Config types (§1.3) | 03-01 |
| `internal/scheduler` | `load.go` | viper load + layering (§1.1) + `Validate` (§7.3) | 03-01 |
| `internal/scheduler` | `init.go` | `_ "time/tzdata"` blank import (§2.1) + defaults application | 03-01 |
| `internal/scheduler` | `resolver.go` | `Resolve(tier, project, now, capReq)` + window matching (§2.3, §4) | 03-01 |
| `internal/scheduler` | `events.go` | `ProviderFallback`, `CostCeilingWarn` event types (§5.2, §6.2) | 03-02 |
| `internal/provider` | `errors.go` | `ProviderError`, `ErrorKind`, `ClassifyHTTP` (§3) | 03-02 |
| `internal/scheduler` | `dispatch.go` | `Scheduler.Dispatch` + the fallback walker (§5, §8) | 03-02 |
| `internal/scheduler` | `breaker.go` | `Breaker` state machine (§6.1) | 03-03 |
| `internal/scheduler` | `cost.go` | `CostTracker` (§6.2) | 03-03 |
| `internal/scheduler` | `safety.go` | breaker+cost wiring into Dispatch (§8.2 breaker/cost branches) | 03-03 |
| `internal/scheduler` | `cli.go` (or `cmd/ass-guard/scheduling_*.go`) | `ass-guard scheduling validate` + `resolve` subcommands (operator tooling) | 03-04 |
| `cmd/ass-guard` | `scheduling_validate.go`, `scheduling_resolve.go` | cobra subcommands (operator-facing) | 03-04 |
| `internal/scheduler` | `testdata/*.yaml` | config fixtures (valid + invalid) | 03-01, 03-04 |
| `internal/provider` | `fake_test.go` (or `testing` package) | a fake Provider for scheduler tests (returns canned Response / ProviderError) | 03-02 |

**Dependencies Plan 03-01 adds to `go.mod`:** `github.com/spf13/viper`, `gopkg.in/yaml.v3`, `github.com/stretchr/testify` (already used by `internal/redact`'s test style per Phase 0 — confirm in T1). No provider SDK at tracer fidelity (the resolver is pure; the provider is stubbed).

## 10. Pitfalls (the planner must pre-empt)

1. **Conflating file-layering with resolution precedence.** Viper file-merge (project → global → embedded default) is NOT the D-02 resolution order (time-window → project → global). They are different axes. The config is FULLY merged first (viper); THEN the resolver applies D-02 over the merged config. A plan that mixes these will produce subtly wrong resolution. (§1.1 vs §4.)

2. **`time/tzdata` not imported.** If the blank import is missing, `time.LoadLocation("America/New_York")` works on the dev's Mac (host zoneinfo present) but FAILS on a minimal Linux deploy. The test suite MUST include a test that loads a non-host, non-UTC zone and asserts no error — this is the single-binary guarantee. (§2.1.)

3. **Overnight-window edge.** A window `22:00→06:00` must match 23:59 AND 01:00, but NOT 06:00 exactly (to-exclusive) or 21:59. The `to`-exclusive boundary is the common off-by-one. Table-test every boundary. (§2.3.)

4. **Structural errors feeding the breaker.** A 401 is NOT a breaker event — it's a config bug (bad auth key), not a provider outage. Recording structural failures into the breaker would trip it on every request when the key is wrong, masking the real cause. Only Transient failures feed the breaker. (§6.1 table.)

5. **Cost estimate without token counts.** If the Phase-1 adapter's `Response` doesn't expose token counts yet (it's Phase 1's job to parse `usage`), the cost tracker cannot account. Plan 03-03 MUST define a `TokenCounts` accessor on the response shape (or a scheduler-side fallback: zero-cost-if-unknown, logged at Warn "cost tracking disabled: no token counts from provider"). This is the intended-interface dependency surfaced concretely. (§6.2.)

6. **Breaker state under concurrent dispatch.** Parent + subagents (Phase 2 PARA) hit the SAME `(provider, model)` breaker concurrently. The breaker mutex MUST be held only for state read/write (not across the provider call), or it becomes a serialization point that defeats the semaphore. The `Allow`/`RecordSuccess`/`RecordTransient` methods take the mutex; the provider call happens lock-free. (§6.1.)

7. **Exhausted kind leaking from the adapter.** The adapter MUST NOT produce `KindExhausted` — only the cost tracker does. If the Phase-1 adapter ever returns Exhausted (e.g. misclassifying a 402), the scheduler's hard-stop would fire spuriously. `ClassifyHTTP` never returns Exhausted (§3.2 table). Unit-test this invariant.

8. **D-02 reversal forgotten.** The natural/cleaner order is project-first (most-specific wins). The user REVERSED this. A plan that implements project-first (because it "looks right") directly violates D-02. The resolver test table MUST assert the window-wins-outright cases explicitly. (§4.1.)

9. **Stdout pollution from the CLI subcommands.** `ass-guard scheduling validate` / `resolve` are operator commands — their diagnostic output goes to stderr; only machine-readable output (e.g. `resolve --json`) may go to stdout, and ONLY when explicitly requested (transport discipline, PROJECT.md). The default human-readable form goes to stderr. (§9, PROJECT.md.)

10. **Validation collecting vs fail-fast.** Collect ALL violations in one pass (the operator sees the whole picture), not fail-on-first. A fail-fast validator forces N reload cycles to fix N errors. (§7.3.)

## 11. Dependencies + integration points (what Phase 3 consumes/contributes)

| Concern | Source | Phase 3 relationship |
|---|---|---|
| `Provider` interface | Phase 1 PROV-01..03, Phase 2 §11.2 | **Consumes** (via fake in tests); Phase 3 adds `ProviderError` to `internal/provider` |
| Event bus | Phase 2 D-04, §2 (`event.Bus`) | **Extends** (adds `ProviderFallback`, `CostCeilingWarn` kinds) |
| Provider semaphore | Phase 2 D-12, §11.1 (`provider.Semaphore`, default 6) | **Consumes** (fallback walker reuses it) |
| Turn loop / Session Core | Phase 1 D-12 / Phase 2 D-17 (`internal/loop` → `internal/session`) | **Consumed by** (the turn loop calls `Scheduler.Dispatch` instead of `Provider.Send` directly) |
| `internal/redact` | Phase 1 (executed) | **Consumes** — cost-tracking logs (provider/model names, pricing) + ProviderError messages route through `redact.ScrubError` before logging (belt-and-suspenders; the provider/model slugs are not secrets but error strings may wrap request bodies) |
| ACP adapter | Phase 2 D-15, §8.2 | **Consumed by** — the adapter subscribes to the new event kinds and forwards as `session/update` (info for ProviderFallback, warn for CostCeilingWarn) |
| Config layout (`.ass-guard/`) | Phase 2 D-06/D-07 (self-gitignoring) | **Reuses** — per-project `scheduling.yaml` lives in `.ass-guard/` |

## 12. Source grounding

- **STACK.md §Focus 2 (Model Scheduling Layer)** — the LiteLLM/OpenRouter/Portkey/Requesty survey (all Python or hosted; NOT Go-embeddable); the internal config-table resolver recommendation; the LiteLLM router design template (fallbacks, cooldowns) studied but NOT depended on. §"What NOT to Use" (LiteLLM as runtime dep violates single-binary).
- **LiteLLM router docs** (`docs.litellm.ai/docs/routing` + `/docs/proxy/reliability`) — the fallback-chain + cooldown design template (design reference only; Python proxy, not a Go dep). The dual trip mechanism (consecutive + rate) and the cooldown/half-open recovery are distilled from LiteLLM's `Router` + sony/gobreaker's state machine.
- **sony/gobreaker** (`github.com/sony/gobreaker`) — the canonical Go circuit-breaker pattern (Closed/Open/HalfOpen + `ReadyToTrip`). Studied as the design template for §6.1; NOT a dependency (single-binary + custom dual-mechanism trip).
- **Go `time/tzdata`** (`pkg.go.dev/time/tzdata`) — the bundled IANA tz database; the documented single-binary timezone mechanism.
- **Phase 2 02-RESEARCH.md §2 (event bus), §11 (provider streaming + semaphore), §8.2 (ACP adapter)** — the intended interfaces Phase 3 consumes/extends (the dep note, §0).
- **Phase 1 01-CONTEXT.md D-09 (Shaper hybrid SDK driving), PROV-01..03 (provider adapter contracts)** — the provider adapters Phase 3 sits above.
- **VERIFIED-FACTS.md #1 (zcode → Z.ai GLM via Anthropic protocol, `builtin:zai-coding-plan`)** — the default provider the heavy tier resolves to; #2 (go-openai tool-calling schema) — the OpenAI-shape providers the scheduler may route to.
- **PROJECT.md "Constraints" (single static binary — scheduler in-process, no proxy/daemon), "Investigate-and-fix-ready logging" (every breaker trip / cost breach / fallback must be diagnosable from the transcript + stderr)** — load-bearing for §6 logging, §9 transport discipline.

## 13. Validation Architecture

> Phase 3 is logic-dense (resolver, breaker state machine, cost window, config-validation graph, fallback walker) with mostly **pure functions** — ideal for exhaustive table/property testing. The Nyquist sampling risk (untested state-space regions) is low because the state spaces are small and fully enumerable. This section maps the validation strategy per component.

### 13.1 Resolver (§4) — exhaustive cross-product table tests

The resolver is a pure function of `(config, tier, project, now, capReq)`. The test matrix enumerates:
- **D-02 precedence:** {window-maps-tier, window-active-but-silent-on-tier, no-window-active} × {project-maps-tier, project-silent} → assert the 4-line algorithm's outcome for each cell. Especially the **window-wins-outright** case (D-02 reversal — the cell where project AND window both map the tier must return the WINDOW's pick, not the project's).
- **D-03 time:** for each window kind (same-day, overnight-wrap, weekday-filtered, every-day), inject `now` values at boundaries (from, from+ε, to-ε, to, overnight-wrap-both-sides) and assert active/not-active.
- **Capability gate (D-09 runtime):** `capReq.NeedsTools` × {primary-has-tools, primary-lacks-tools-but-fallback-has, no-candidate-has-tools} → assert skip-to-fallback / error.

Coverage target: every cell in the precedence × time × capability matrix has at least one test. The matrix is small (≈30-50 cases) — fully enumerated, no sampling. This is the Nyquist "sample every state transition" guarantee for a pure function.

### 13.2 Circuit breaker (§6.1) — state-machine transition tests

The breaker is a 3-state machine (Closed/Open/HalfOpen) with the transitions in the §6.1 table. Each row is one test:
- Closed→Open via consecutive (N=5 Transient failures).
- Closed→Open via error-rate (>50% over M=20).
- Closed→Closed on success (counter reset).
- Closed→Closed on Structural (not a breaker event — invariant test).
- Open→skip (cand considered during cooldown → skipped, no provider call).
- Open→HalfOpen after cooldown (inject `now = openedAt + cooldown`).
- HalfOpen→Closed on probe success.
- HalfOpen→Open on probe failure (cooldown restarts).

Plus **concurrency test**: 100 goroutines hammering `RecordTransient`/`RecordSuccess` on one breaker → no race (`-race`), final state consistent. This is the load-bearing concurrency guarantee (pitfall 6).

### 13.3 Cost tracker (§6.2) — window-accumulation + breach tests

- Account N requests → `spent` accumulates correctly (token × pricing arithmetic).
- Spent crosses `amount_usd` → `Check()` returns Degrade; CostCeilingWarn published.
- Degraded + spent crosses ceiling again → HardStop; `KindExhausted` returned.
- Window rollover (`now` crosses boundary) → spent resets, degraded resets.
- Injected `now` makes the window deterministic.

### 13.4 Config validation (§7.3) — rejection tests per check

One test per validation-check row in the §7.3 table. Especially the **capability-consistency** reject (D-10 core): a fixture where primary has tool_calling and a fallback lacks it → `Validate` returns an error naming both models + the capability. Plus a valid-config fixture → `Validate` returns nil. Plus the collect-all behavior (a fixture with 3 violations → the error lists all 3).

### 13.5 Fallback walker (§5) — fake-provider scenario tests

A fake `Provider` returns canned outcomes per `(provider, model)`:
- Primary Transient → walker tries fallback 1 → success (ProviderFallback event published).
- Primary + fallback-1 both Transient → fallback-2 success (two events).
- All candidates Transient → last error returned.
- Primary Structural → walk stops immediately (no fallback attempted; no event).
- Primary success → no walk, no event.
- Breaker open on fallback-1 → skipped (walker proceeds to fallback-2).
- Capability gate skips a tool-less fallback.

These are integration tests of `Scheduler.Dispatch` using the fake provider + a real resolver + real breaker + real cost tracker (all in-process, deterministic, no network).

### 13.6 Test command surface

| Component | Test command | Flags |
|---|---|---|
| Resolver | `go test ./internal/scheduler -run TestResolve -race -v` | table tests |
| Breaker | `go test ./internal/scheduler -run TestBreaker -race -v` | state machine + concurrency |
| Cost | `go test ./internal/scheduler -run TestCost -race -v` | window/breach |
| Validation | `go test ./internal/scheduler -run TestValidate -race -v` | rejection matrix |
| Dispatch | `go test ./internal/scheduler -run TestDispatch -race -v` | fake-provider scenarios |
| ProviderError | `go test ./internal/provider -run TestClassify -race -v` | HTTP-status table |
| Full suite | `go test ./... -race` | the `-race` flag is load-bearing (breakers, cost, bus all concurrent) |

### 13.7 Sampling risk assessment

The Nyquist concern (under-sampled state space leading to a false "it works") is **low** for Phase 3: every component's state space is small and enumerable (resolver matrix ≈50 cells; breaker ≈10 transitions; cost ≈5 branches; validation ≈10 checks). The risk is **not** insufficient sampling but **incorrect precedence/time logic** (pitfalls 3, 8) — mitigated by exhaustive tables, not more samples. The one genuinely concurrent component (breaker under fan-out) gets a dedicated `-race` concurrency test (§13.2). There is no "unsampled unit" that could hide a whole class of failures.

---

*Phase: 3-Model Scheduling*
*Researched: 2026-08-09*
*Confidence: HIGH — every load-bearing claim grounded in STACK §Focus 2, Phase 1/2 intended interfaces, LiteLLM/sony-gobreaker design templates, and the Go stdlib (`time/tzdata`). Zero external runtime dependencies added (viper + yaml.v3 + testify only, all already in the project's stack).*
