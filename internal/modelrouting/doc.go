// Package modelrouting is the model-routing layer (Phase 3; renamed from
// "scheduler" 2026-08-25 — the old name collided with the cron engine in internal/sched).
//
// The turn loop (Phase 2 Session Core) selects a tier — heavy / good / light —
// instead of a concrete model. The Scheduler resolves that tier to a concrete
// (provider, model) at request time, dispatches through the provider adapter,
// walks a configured fallback chain on transient failure, and enforces circuit
// breakers + a cost ceiling — all transparent to the developer (the phase-goal
// promise: "the developer never thinks about which concrete model is running;
// the operator manages cost/reliability via config").
//
// Layering: the operator authors a declarative config.yaml (D-01); the
// loader (load.go) merges it layered (embedded default → global → per-project)
// and validates it at load time (D-10 — inconsistent configs are REJECTED with
// a named, collect-all *ConfigError before any request is served). The resolver
// (resolver.go) is a pure function of (config, tier, project, now, capReq)
// applying the D-02 precedence — time-window → project → global (the user's
// explicit reversal: a structural time-window wins outright; a project narrows
// the gap the window leaves). The dispatcher (dispatch.go) orchestrates resolve
// → capability gate → breaker → cost → semaphore → provider.Send → fallback
// walk. The safety nets (breaker.go, cost.go) stop cascading outages (D-07:
// consecutive OR error-rate trip, cooldown + half-open probe) and cost surprises
// (D-08: dollars per fixed window, degrade-then-stop).
//
// Phase-3 decisions implemented: D-01 (declarative YAML), D-02 (window-first
// precedence), D-03 (lazy per-request IANA time, time/tzdata bundled), D-04
// (typed ProviderError classified by the adapter, pattern-matched by the
// scheduler), D-05 (explicit per-tier fallback list), D-06 (ProviderFallback
// event → ACP session/update), D-07 (dual-mechanism circuit breaker), D-08
// (dollars-per-window cost ceiling), D-09 (structured CapabilityProfile +
// request-time gate), D-10 (load-time capability-mismatch rejection).
package modelrouting
