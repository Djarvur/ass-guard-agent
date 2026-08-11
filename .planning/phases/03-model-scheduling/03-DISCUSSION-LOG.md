# Phase 3: Model Scheduling - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-08-10
**Phase:** 3-Model Scheduling
**Areas discussed:** Resolver config shape, Fallback chain mechanics, Circuit breakers & cost ceilings, Capability profiles

---

## Area selection

| Option | Description | Selected |
|--------|-------------|----------|
| Resolver config shape | SCHED-01/02/03 — config table format, precedence, time evaluation | ✓ |
| Fallback chain mechanics | SCHED-04 — failure classification, chain walk, degradation UI | ✓ |
| Circuit breakers & cost ceilings | SCHED-05 — trip triggers, cost units, response on hit | ✓ |
| Capability profiles | SCHED-06 — profile content, mismatch surfacing | ✓ |

**User's choice:** All four selected.

---

## Resolver config shape

### Config format

| Option | Description | Selected |
|--------|-------------|----------|
| Declarative YAML (rec.) | Viper-loaded, layered global→project; readable, handles nested rules | ✓ |
| Structured TOML | Consistency with OpenSpec but awkward for nested time-windows/fallbacks | |
| Programmatic Go API only | Type-safe but violates config-not-code; requires rebuild | |

**User's choice:** Declarative YAML.

### Resolve precedence

| Option | Description | Selected |
|--------|-------------|----------|
| Project → time-window → global (rec.) | Most specific context first | |
| Time-window → project → global | Time-window is primary structural signal; project narrows within | ✓ |
| Return all candidates, caller picks | Max flexibility but leaks scheduler logic into caller | |

**User's choice:** Time-window → project → global (USER REVERSED the recommended order).

**Notes:** Time-window reflects operational reality (peak hours = capacity/cost constraints) applying to everyone; project override refines within that, doesn't replace it entirely.

### Time evaluation

| Option | Description | Selected |
|--------|-------------|----------|
| Lazy per-request (rec.) | Resolver checks current wall-clock each request; live, not precomputed | ✓ |
| Eager session-bound | Bind once at session start; cached; simpler but no mid-session switch | |
| Per-turn cached | Middle ground; consistent within a turn, switches between turns | |

**User's choice:** Lazy per-request.

---

## Fallback chain mechanics

### Failure signal

| Option | Description | Selected |
|--------|-------------|----------|
| Typed ProviderError with .Kind (rec.) | Transient/Structural/Exhausted enum; adapter classifies; scheduler pattern-matches | ✓ |
| Standard error + status inspection | Scheduler reaches into HTTP details it shouldn't own | |
| Adapter retries internally | Hides failure type from scheduler; breaks SCHED-04 | |

**User's choice:** Typed ProviderError with .Kind.

### Chain walk

| Option | Description | Selected |
|--------|-------------|----------|
| Explicit per-tier fallback list (rec.) | Operator-authored ordered array; auditable; no silent mismatch | ✓ |
| Auto-degrade tier (heavy→good→light) | Automatic but risks silent capability mismatch across providers | |
| Hybrid: explicit + auto-degrade tail | Belt-and-suspenders; research validates if tail needed | |

**User's choice:** Explicit per-tier fallback list.

### Degradation UI

| Option | Description | Selected |
|--------|-------------|----------|
| Info session/update (rec.) | Transparent; developer sees the switch; not blocked | ✓ |
| Silent fallback | Cleaner output but hides unexpected switches | |
| Warn session/update | More prominent; may be noisy if fallbacks common | |

**User's choice:** Info session/update.

---

## Circuit breakers & cost ceilings

### Breaker trip

| Option | Description | Selected |
|--------|-------------|----------|
| Consecutive failures + cooldown (rec.) | Fast trip on hard outage; simple; hard to misconfigure | |
| Error-rate threshold | Smooths blips but needs statistical mass; slow on hard outage | |
| Both (consecutive + error-rate) | Most robust; fast + slow trip; two mechanisms to tune | ✓ |

**User's choice:** Both (consecutive + error-rate).

**Notes:** Most robust option. Consecutive for fast trip on hard outage; error-rate for slow trip on degraded performance. Research defines exact thresholds.

### Cost ceiling

| Option | Description | Selected |
|--------|-------------|----------|
| Dollars/window, degrade-then-stop (rec.) | Operator-intuitive unit; graceful degradation; hard-stop only if cheap tier exhausted | ✓ |
| Tokens/window | Direct usage tracking but abstract for operators | |
| Requests/window | Simplest but ≠ cost; doesn't prevent surprises | |

**User's choice:** Dollars/window, degrade-then-stop.

---

## Capability profiles

### Profile content

| Option | Description | Selected |
|--------|-------------|----------|
| Structured capability declaration (rec.) | Machine-readable; scheduler consults at fallback time; makes tier abstraction honest | ✓ |
| Freeform doc per model | Human-readable but not machine-actionable | |
| No profiles (defer SCHED-06) | SCHED-06 explicitly requires them | |

**User's choice:** Structured capability declaration.

### Mismatch surfacing

| Option | Description | Selected |
|--------|-------------|----------|
| Reject at config-load (rec.) | Fail-fast; operator can't ship inconsistent config; primary/fallbacks must be compatible | ✓ |
| Skip-mismatched at request time | Flexible but validation spread across requests | |
| Both: config-load structural + request-time contextual | Layered validation | |

**User's choice:** Reject at config-load.

---

## Claude's Discretion

None — every Phase-3 decision was explicitly user-answered. The user reversed the recommended precedence in D-02 (time-window first) with a clear rationale.

## Deferred Ideas

None raised as new capabilities. Implementation details for research/planning:
- Exact circuit-breaker thresholds (N, M, error-rate%, cooldown)
- Per-project override discovery mechanism (cwd-based?)
- Pricing table update workflow
- Cross-process cost tracking (out of scope — single process)

---

*Phase: 3-Model Scheduling*
*Discussion date: 2026-08-10*
