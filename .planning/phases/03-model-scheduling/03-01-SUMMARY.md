---
phase: 03-model-scheduling
plan: 01
status: complete
requirements: [SCHED-01, SCHED-02, SCHED-03, SCHED-06]
key-files:
  created:
    - internal/scheduler/doc.go
    - internal/scheduler/init.go
    - internal/scheduler/config.go
    - internal/scheduler/load.go
    - internal/scheduler/load_test.go
    - internal/scheduler/resolver.go
    - internal/scheduler/resolver_test.go
    - internal/scheduler/defaults/scheduling.yaml
    - internal/scheduler/testdata/valid.yaml
    - internal/scheduler/testdata/invalid_cap_mismatch.yaml
    - internal/scheduler/testdata/invalid_dangling_slug.yaml
    - internal/scheduler/testdata/invalid_multiple.yaml
    - internal/scheduler/testdata/minimal.yaml
    - internal/scheduler/testdata/base_layering.yaml
    - internal/scheduler/testdata/overlay_layering.yaml
  modified:
    - go.mod
    - go.sum
---

# Plan 03-01 SUMMARY — Tracer: scheduling config + resolver + load-time validation

## What was built

The "honest config" slice of the model-scheduling layer (the resolution half of
the scheduling seam; the runtime half lands in 03-02):

- **`internal/scheduler/config.go`** — all 13 types from RESEARCH §1.3
  (`Config`, `ProviderConfig`, `ModelConfig`, `Pricing`, `CapabilityProfile`,
  `TierBinding`, `TimeWindow`, `Schedule`, `ProjectOverride`,
  `CircuitBreakerConfig`, `CostCeilingConfig`) plus the hand-off `Target` and
  `CapabilityReq` shapes. Every field carries a `yaml:"..."` tag matching the
  operator-facing schema verbatim.
- **`internal/scheduler/init.go`** — blank import `_ "time/tzdata"` (the
  single-binary IANA guarantee, D-03, ~450KB pure-Go tz database bundled into
  the binary) + the documented D-07 breaker defaults (N=5, M=20, 50%, 60s, 1
  probe) and D-08 cost window (24h) applied when config fields are zero.
- **`internal/scheduler/load.go`** — `Load(paths...)` layered YAML merge
  (embedded default → global → per-project) + `Validate` (collect-all D-10) +
  `*ConfigError`. Custom `UnmarshalYAML` on the breaker/cost configs parses
  human-friendly `60s`/`24h` durations.
- **`internal/scheduler/resolver.go`** — `Resolver.Resolve` implementing the
  D-02 4-line algorithm (time-window → project → global), lazy IANA-zone window
  matching (same-day + overnight-wrap + weekday-filter, to-exclusive boundary),
  capability enrichment, and the `satisfies` helper (defined + unit-tested;
  applied in 03-04). Pure function — zero `sync`, zero logging, zero network.
- **7 testdata fixtures + an embedded zero-config default** exercising the
  valid config, both D-10 rejection paths, the collect-all behavior, the
  defaults-applied path, and viper-free deep-merge layering.

## Decisions honored

- **D-01** declarative YAML, layered (embedded default → global → per-project).
- **D-02** time-window → project → global precedence (the user's explicit
  reversal) — asserted by `TestResolveWindowWinsOutright`.
- **D-03** lazy per-request IANA time, `time/tzdata` bundled
  (`TestTzdataBundled` proves a non-host zone loads).
- **D-09** structured `CapabilityProfile` attached to every resolved Target.
- **D-10** load-time capability-mismatch rejection (collect-all
  `*ConfigError`); the core check rejects a primary that has a capability
  (`tool_calling`/`streaming`/`extended_thinking`) a fallback lacks.

## Notable deviations / investigate-and-fix-ready

1. **Loader uses `gopkg.in/yaml.v3` directly, not `spf13/viper`** (the RESEARCH
   §1.1 / D-01 wording said "viper-loaded"). Viper's internal config flatten
   splits map keys on `.` (its key-path delimiter), which mangles dotted model
   slugs — `glm-5.2` decodes as `glm-5` (the `.2` is dropped). Every model slug
   in the zcode/GLM ecosystem is dotted, so this is load-bearing. `yaml.v3`
   preserves dotted map keys verbatim. **The operator-facing contract from D-01
   is preserved exactly** (declarative YAML, layered global → per-project,
   embedded zero-config floor); only the parsing library changes. Recorded as
   the principal plan deviation.

2. **`valid.yaml` heavy-fallback models given `extended_thinking: true`.** The
   D-10 capability-consistency check correctly caught that the RESEARCH §1.2
   example config had `glm-5.2` (thinking:true) falling back to `glm-4.6` /
   `minimax-m3` (thinking:false) — a real mismatch. The fixture's heavy-fallback
   models were bumped to `extended_thinking: true` so the heavy chain is
   consistent (the whole point of the heavy tier is full capability). This is a
   fixture correction the validator surfaced; it does not change behavior.

## Self-Check: PASSED

- `go test ./internal/scheduler -race -count=1` — 18 tests green (8 load+tzdata
  + 10 resolver/active-window/satisfies, including 5 overnight-boundary
  subtests and the D-02 window-wins-outright cell).
- `go build ./...` + `go vet ./...` clean.
- `grep -c "sync\." internal/scheduler/resolver.go` returns 0 (the resolver is
  a pure function — concurrency lives in 03-02/03-03).

## Evidence

- D-02 reversal: `TestResolveWindowWinsOutright` — Mon 10:00 NY resolves heavy
  to the peak window's `minimax-m3`, not myproj's or the global `glm-5.2`.
- Overnight-wrap boundary (pitfall 3): `TestActiveWindowOvernightBoundary`
  subtests assert 23:59 + 01:00 active, 06:00 (to-exclusive) + 21:59 inactive.
- D-10: `TestLoadInvalidCapMismatch` asserts the error names `glm-5.2`,
  `haiku-cheap`, and `tool_calling`; `TestLoadCollectAll` asserts two distinct
  violations are reported in one pass.
- Defaults: `TestLoadDefaults` asserts the 5 D-07 breaker defaults + 24h window.
