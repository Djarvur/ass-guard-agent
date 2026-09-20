---
phase: 16-acp-wire-foundation
plan: 03
subsystem: acp-wire
tags: [acp, jsonrpc, registry, timeout-ladder, cancellation, capability-probe, metrics, tdd]
requires:
  - "16-01 TurnEmitter (fg/bg lanes, Barrier, single drain) — the cascade rides the fg lane; Writer is the outbound chokepoint"
  - "internal/session askBroker DefaultAskTimeout (10 min) — the D-17 HUMAN-ASK mirror target"
  - "acp Server dispatch loop (Serve) — the interception point before handler lookup"
provides:
  - "Registry: Call(ctx, method, params, TimeoutClass) — UUID v4 id'd outbound requests matched by id under concurrent notification traffic, resolution under the registry's OWN mutex with buffered(1) channels (criterion 3)"
  - "D-14 loud ladder: timeout -> exactly ONE same-id retry -> typed *RequestTimeoutError fallback + structured stderr log {id, method, elapsed_ms}; -32800 resolves cancelled immediately with zero retries"
  - "D-19 synthetic-cancel: ResolveCancelled(id) / caller-ctx cancel resolve cancelled AND emit one $/cancel_request {requestId} through the emitter foreground lane (Barrier-ordered before the prompt response); Pitfall-8-safe shutdown drain"
  - "Serve response interception BEFORE handler dispatch (zero spurious -32601) + inbound $/cancel_request fast no-op (A8)"
  - "D-13/D-18 capability negotiation: advertisement-first (clientCapabilities.elicitation.form), else one elicitation/create probe per connection; sticky ok/degraded cache; Server.Capability(key) accessor for Phase 17"
  - "D-16 metrics family (probe/timeout/fallback, registry cancels, writer stalls) with Snapshot() for Phase 20 /status"
affects:
  - "Phase 17 (interactive asks build directly on Call/ResolveCancelled/Capability — the D-19 costly-to-reverse contract)"
  - "16-05/16-06 (config wire surface consumes the registry error classes; simulator smoke)"
  - "Phase 20 (/status surfaces Metrics.Snapshot)"
tech-stack:
  added: [] # stdlib only (sync, sync/atomic, context, time, crypto/rand)
  patterns:
    - "resolution under the registry's OWN mutex, never the writer's; buffered(1) entries so Deliver never blocks; done-CAS exactly-once resolution guard"
    - "wire-visible cascade rides the emitter foreground lane via a pre-built-frame Notify route with full Barrier accounting"
    - "Call owns param marshaling — json.RawMessage passes verbatim (the []byte-would-base64 footgun is structurally closed)"
    - "capability cache rides acp Server: probe IS a registry Call; Server lifetime equals connection lifetime (D-13/D-18 scope)"
key-files:
  created:
    - internal/acp/request_registry.go
    - internal/acp/request_registry_test.go
    - internal/acp/metrics.go
  modified:
    - internal/acp/server.go
    - internal/acp/handlers.go
    - internal/acp/types.go
    - internal/acp/emitter.go
    - internal/acp/server_test.go
    - internal/runtime/emitter_e2e_test.go
    - internal/runtime/integration_test.go
key-decisions:
  - "Resolution NEVER touches the writer mutex: pending map under the registry's own mutex, buffered(1) resolution channels, done-CAS exactly-once guard (criterion 3 verbatim; duplicate/late deliveries after the D-14 retry are dropped quietly)"
  - "The D-19 cascade is the ONE outbound frame that bypasses direct-Writer writes: it rides EmitterHandle.Notify (new fg-lane route with Barrier accounting) so the turn-end barrier orders $/cancel_request before the prompt response"
  - "D-14 ladder retries the SAME id — a late answer to either send resolves the single entry; timeout-fallback returns an errors.As-able *RequestTimeoutError (distinct from cancellation) so callers pick degrade-vs-abort correctly"
  - "Capability cache deliberately rides internal/acp Server beside the registry (probe IS a registry Call; Server lifetime = connection lifetime); acpserve owns it transitively via composition"
  - "HUMAN-ASK mirrors session.DefaultAskTimeout via a local constant — acp stays session-free in production code; the cross-package equality test is the drift alarm (D-17)"
  - "Legacy acp/runtime test fixtures updated to Zed-like advertised initializes — real Zed always advertises elicitation.form (acp.rs:767-795), so fixtures mirror real traffic and the probe stays out of probe-unrelated tests"
requirements-completed: [ACP-03]
duration: 38 min
completed: 2026-08-27
status: complete
estimate:
  tokens: 42000
  raw_tokens: 42000
  tasks: 3
  confidence: low
actuals:
  tokens: 17980 # chars/4 over the realized diff (71,919 chars, 10 files, 1722 insertions)
  tasks: 3
  commits: 7
coverage:
  - id: D1
    description: "Id'd outbound request resolves by id while notification frames stream concurrently — resolution under the registry's own mutex, never the writer's (ROADMAP criterion 3, -race)"
    requirement: ACP-03
    verification:
      - kind: unit
        ref: "tests/internal/acp/request_registry_test.go#TestRegistryConcurrentResolve"
        status: pass
    human_judgment: false
  - id: D2
    description: "D-14 ladder: unanswered request re-sent exactly once (same id, fresh window), then typed *RequestTimeoutError fallback + exactly one structured stderr log carrying id/method/elapsed_ms"
    requirement: ACP-03
    verification:
      - kind: unit
        ref: "tests/internal/acp/request_registry_test.go#TestRegistryTimeoutFallback"
        status: pass
    human_judgment: false
  - id: D3
    description: "A -32800 error response resolves the pending Call as cancelled immediately with zero retries (exactly one outbound frame)"
    requirement: ACP-03
    verification:
      - kind: unit
        ref: "tests/internal/acp/request_registry_test.go#TestRegistryRequestCancelledResponse"
        status: pass
    human_judgment: false
  - id: D4
    description: "D-19 synthetic-cancel cascade: ResolveCancelled and caller-ctx cancellation resolve cancelled AND emit exactly one $/cancel_request {requestId} through the foreground emitter lane"
    requirement: ACP-03
    verification:
      - kind: unit
        ref: "tests/internal/acp/request_registry_test.go#TestRegistrySyntheticCancel"
        status: pass
      - kind: unit
        ref: "tests/internal/acp/request_registry_test.go#TestRegistryCallerCtxCancel"
        status: pass
    human_judgment: false
  - id: D5
    description: "Shutdown drain (Pitfall 8): Stop releases all pending waiters cancelled with the cascade suppressed, rejects new Calls without writes, idempotent"
    requirement: ACP-03
    verification:
      - kind: unit
        ref: "tests/internal/acp/request_registry_test.go#TestRegistryShutdown"
        status: pass
    human_judgment: false
  - id: D6
    description: "D-17 timeout classes: FAST-CONTROL 10s default; HUMAN-ASK default equals session.DefaultAskTimeout exactly (cross-package pin, production acp stays session-free)"
    requirement: ACP-03
    verification:
      - kind: unit
        ref: "tests/internal/acp/request_registry_test.go#TestHumanAskTimeoutMirrorsSessionDefault"
        status: pass
    human_judgment: false
  - id: D7
    description: "Serve intercepts response frames (id + no method + result-or-error) BEFORE dispatch: pending Call resolves, zero spurious -32601 frames, unknown ids logged once and dropped, serving continues (Pitfall 1)"
    requirement: ACP-03
    verification:
      - kind: unit
        ref: "tests/internal/acp/server_test.go#TestServeResponseRouting"
        status: pass
    human_judgment: false
  - id: D8
    description: "Inbound $/cancel_request notification (client cancelling ITS request) is a logged fast no-op with no response frame (Assumption A8)"
    requirement: ACP-03
    verification:
      - kind: unit
        ref: "tests/internal/acp/server_test.go#TestServeInboundCancelNoOp"
        status: pass
    human_judgment: false
  - id: D9
    description: "D-13 capability negotiation: advertisement-first (elicitation.form advertised -> NO probe, cached ok); absent -> exactly one elicitation/create probe with a minimal schema-valid v1 Form payload (no session content, T-16-08); result caches ok, -32601 caches degraded; initialize responds on every path"
    requirement: ACP-03
    verification:
      - kind: unit
        ref: "tests/internal/acp/server_test.go#TestInitializeProbe"
        status: pass
    human_judgment: false
  - id: D10
    description: "D-18 stickiness: after a degraded negotiation a second capability query returns degraded with NO new probe for the connection"
    requirement: ACP-03
    verification:
      - kind: unit
        ref: "tests/internal/acp/server_test.go#TestCapabilityStickiness"
        status: pass
    human_judgment: false
  - id: D11
    description: "Probe timeout follows the D-14 ladder (same-id retry) then degrades with counters probe=1/timeout=2/fallback=1 and structured stderr lines; initialize response still arrives"
    requirement: ACP-03
    verification:
      - kind: unit
        ref: "tests/internal/acp/server_test.go#TestProbeTimeoutFallback"
        status: pass
    human_judgment: false
  - id: D12
    description: "D-16 metrics family: Snapshot() exposes probe/timeout/fallback, registry-cancel (via the registry's onCancel hook, proven end-to-end on a -32800), and the adopted 16-01 writer-stall counter"
    requirement: ACP-03
    verification:
      - kind: unit
        ref: "tests/internal/acp/server_test.go#TestMetricsRegistryCancelCounter"
        status: pass
      - kind: unit
        ref: "tests/internal/acp/server_test.go#TestMetricsWriterStallFamily"
        status: pass
    human_judgment: false
  - id: D13
    description: "Registry + probe behavior against a REAL editor client (Zed cascades, real elicitation rendering, hostile-client withholding)"
    verification: []
    human_judgment: true
    rationale: "The plan's own verification note: a manual smoke in the simulator arrives in 16-06; nothing in this plan requires a live editor. In-repo proof is the full -race test set above."
---

# Phase 16 Plan 03: ACP Wire Foundation — Outbound Request Registry Summary

The outbound-request half of the ACP wire is live and raced: UUID-keyed requests resolve under concurrent notification traffic, degrade loudly through D-14's one-retry ladder, cascade $/cancel_request on turn death, and negotiate client capability exactly once per connection — the transport Phase 17's asks ride.

## Performance

- **Duration:** 38 min
- **Started:** 2026-08-27T15:43:34Z
- **Completed:** 2026-08-27T16:21:46Z
- **Tasks:** 3 (all TDD: RED + GREEN each)
- **Files modified:** 10 (3 created, 7 modified)

## Accomplishments

- **Registry core (Task 1):** `Registry.Call` issues UUID v4 id'd frames straight through the Writer (id'd frames are unconstrained by notification ordering) and blocks on a buffered(1) resolution channel — Deliver resolves under the registry's OWN mutex, never the writer's, with a done-CAS exactly-once guard (criterion 3, proven under -race with 100 interleaved notifications). D-14's ladder re-sends the SAME id once with a fresh window, then returns an `errors.As`-able `*RequestTimeoutError` plus one structured stderr line {id, method, elapsed_ms}; `-32800` resolves cancelled immediately. `ResolveCancelled(id)` and caller-ctx cancellation resolve cancelled AND emit exactly one `$/cancel_request {requestId}` through the emitter's foreground lane — the documented cascade — while `Stop()` drains every waiter with the cascade suppressed (Pitfall 8). HUMAN-ASK mirrors `session.DefaultAskTimeout` via a local constant pinned by a cross-package test (D-17).
- **Serve interception (Task 2):** response-shaped frames (id set, method empty, result-or-error) route to `registry.Deliver` BEFORE the dispatch branch — zero spurious `-32601`, unknown ids logged once and dropped (never dispatched, T-16-06); inbound `$/cancel_request` notifications are a logged fast no-op (A8); teardown order is handlerWG → registry.Stop → emitter.Stop → Writer close.
- **Capability probe + telemetry (Task 3):** initialize reads `clientCapabilities.elicitation.form` FIRST (advertisement → cached ok, no probe — Zed never triggers one); absent/ambiguous → exactly one `elicitation/create` probe with a minimal static v1 Form payload (no session content, T-16-08) riding the FAST-CONTROL window and the full D-14 ladder inside the request's own goroutine; result → ok, `-32601`/cancel/ladder → degraded, and degradation is STICKY for the connection (D-18); initialize ALWAYS responds with agentCapabilities unchanged. `metrics.go` lands the D-16 counter family (probe/timeout/fallback, registry cancels via the onCancel hook, 16-01's writer-stall counter adopted) behind `Snapshot()` for Phase 20's /status.

## TDD Gate Compliance

| Task | RED | GREEN | REFACTOR | Status |
|------|-----|-------|----------|--------|
| Task 1 (registry core) | ✓ `2c1bbfa` (build-failure RED — Registry API absent) | ✓ `c3f9e8f` | — (none needed) | Pass |
| Task 2 (Serve interception) | ✓ `6b0a262` (build-failure RED — Server.registry absent) | ✓ `eb51873` | — (none needed) | Pass |
| Task 3 (probe + metrics) | ✓ `289db57` (build-failure RED — metrics/capability API absent) | ✓ `4cb4bc6` | — (none needed) | Pass |

## Task Commits

1. **Task 1 RED** — `2c1bbfa` (test): failing registry core tests
2. **Task 1 GREEN** — `c3f9e8f` (feat): registry core — pending map, D-14 ladder, D-19 cascade
3. **Task 2 RED** — `6b0a262` (test): failing Serve response-interception tests
4. **Task 2 GREEN** — `eb51873` (feat): response interception before dispatch + inbound cancel no-op
5. **Task 3 RED** — `289db57` (test): failing capability-probe and metrics tests
6. **Task 3 GREEN** — `4cb4bc6` (feat): initialize capability probe + sticky degradation + D-16 metrics
7. **Deviation fix** — `76091ff` (test): runtime fixtures advertise elicitation (D-13 consequence)

## Files Created/Modified

- `internal/acp/request_registry.go` — Registry: Call/Deliver/ResolveCancelled/Stop, timeout classes, D-14 ladder, D-19 cascade
- `internal/acp/request_registry_test.go` — 7 registry behavior tests (-race)
- `internal/acp/metrics.go` — D-16 atomic counter family + Snapshot()
- `internal/acp/server.go` — registry/metrics/capabilities fields, Serve interception, capabilityCache + exported Capability()
- `internal/acp/handlers.go` — advertisement-first initialize, probeElicitationCapability, cancelRequest no-op, uuidV4 generalization
- `internal/acp/types.go` — CodeRequestCancelled (-32800), $/cancel_request + elicitation/create method constants
- `internal/acp/emitter.go` — EmitterHandle.Notify (pre-built-frame fg/bg route with Barrier accounting); stall counter adoption into Metrics
- `internal/acp/server_test.go` — harness stderr exposure + interception/probe/metrics tests
- `internal/runtime/emitter_e2e_test.go`, `internal/runtime/integration_test.go` — Zed-like advertised initializes (deviation 3)

## Decisions Made

- Registry resolution never touches the writer mutex; entries are buffered(1) with a done-CAS guard — duplicate/late deliveries after the retry are dropped quietly.
- The cascade is the one outbound notification routed through the emitter (new `EmitterHandle.Notify`, full Barrier accounting) so the turn-end barrier orders it before the prompt response; all other request frames go through the Writer directly.
- `Registry.Call` owns param marshaling: `json.RawMessage` passes verbatim, nil → null, structs marshal once — closing a base64 double-marshal footgun caught by the Task-3 tests.
- Capability cache rides acp Server beside the registry (probe IS a registry Call; Server lifetime = connection lifetime); acpserve owns it transitively.
- `WithRegistryOnCancel` seam added in Task 1 so Task 3's file set stayed as planned (metrics wiring happens only in server.go).
- Legacy acp + runtime test fixtures now send Zed-like advertised initializes — real Zed always advertises elicitation.form, so fixtures mirror real traffic and the eager probe stays out of probe-unrelated tests.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] EmitterHandle.Notify added to emitter.go (outside Task 1's file list)**
- **Found during:** Task 1 GREEN
- **Issue:** the D-19 cascade must ride the emitter's foreground lane (plan key_links), but EmitterHandle only had session/update frame methods — no route for a pre-built notification, and none preserved Barrier accounting.
- **Fix:** `EmitterHandle.Notify(msg *Message)` mirrors enqueueUpdate's accounting and routes by handle class; the cascade is its sole production caller.
- **Files modified:** internal/acp/emitter.go
- **Verification:** TestRegistrySyntheticCancel/TestRegistryCallerCtxCancel assert exactly one cascade frame; TestTurnEmitterBarrier suite stays green.
- **Committed in:** c3f9e8f

**2. [Rule 1 - Bug] Registry.Call double-marshaled pre-encoded params**
- **Found during:** Task 3 GREEN (probe params arrived base64-encoded on the wire)
- **Issue:** passing an already-marshaled `[]byte` into `Call(ctx, m, params any, ...)` re-marshaled it as a base64 JSON string — the probe frame was protocol garbage.
- **Fix:** Call owns marshaling with a type switch: `json.RawMessage` verbatim, nil → `null`, everything else marshaled once; the probe passes its struct directly.
- **Files modified:** internal/acp/request_registry.go, internal/acp/handlers.go
- **Verification:** TestInitializeProbe's payload assertions pass (mode/form/schema verbatim JSON).
- **Committed in:** 4cb4bc6

**3. [Rule 1 - Bug] Runtime e2e/integration fixtures met the eager probe**
- **Found during:** `mise ci` (TestTurnEmitterEndToEnd, TestIntegration_RealStreamingThroughACP failed)
- **Issue:** both fixtures send a bare initialize; with D-13 live, the probe frame correctly arrived where they expected the initialize response.
- **Fix:** fixtures now advertise `elicitation.form` like real Zed (acp.rs:767-795) — advertisement-first keeps them on the probe-free path; same treatment applied to the legacy acp suite's bare initializes.
- **Files modified:** internal/runtime/emitter_e2e_test.go, internal/runtime/integration_test.go, internal/acp/server_test.go
- **Verification:** `mise ci` green end-to-end.
- **Committed in:** 76091ff (acp fixture part rode 4cb4bc6)

**4. [Plan-structure note] WithRegistryOnCancel seam landed in Task 1**
- Task 3's file list excluded request_registry.go, but the registry-cancel counter needs a registry-side hook. The one-line-seam option landed with Task 1's GREEN (exercised there); Task 3 only wired it in server.go. Same file set otherwise.

**Total deviations:** 3 auto-fixed (2 Rule 1, 1 Rule 3) + 1 structural note. **Impact:** low — all within the plan's own architecture; the marshaling fix and fixture updates were required by the plan's own no-regression gates.

## Issues Encountered

- Legacy initialize tests in internal/acp and internal/runtime needed the Zed-like advertisement once D-13 went live (behavior working as designed; fixtures updated rather than the probe delayed — delaying would contradict D-13's eager-at-initialize contract). Suite green after the update; no production behavior reverted.

## Authentication Gates

None.

## Known Stubs

None. Every landed path is wire-real; the probe payload is intentionally minimal per A3/CONTEXT discretion (Phase 17 replaces it with real forms) — that is the designed shape, not a stub.

## Verification Results

- `go test -race ./internal/acp/ -run 'TestRegistryConcurrentResolve|TestRegistryTimeoutFallback|TestRegistrySyntheticCancel|TestRegistryShutdown' -count=1` — green
- `go test -race ./internal/acp/ -run 'TestServeResponseRouting|TestServeInboundCancelNoOp' -count=1 && go test ./internal/acp/ -count=1` — green
- `go test -race ./internal/acp/ -run 'TestInitializeProbe|TestCapabilityStickiness|TestProbeTimeoutFallback' -count=1 && go test ./internal/acp/ ./internal/acpserve/ -count=1` — green
- `go test -race ./internal/acp/ -count=1` — green (16-01 emitter suite unchanged)
- `mise ci` (vet + golangci-lint strict + build + `go test -race ./...`) — green, exit 0
- `go vet ./internal/acp/` — clean

## Next Phase Readiness

- Ready for 16-05 (config wire surface) and 16-06 (simulator smoke): the registry, capability cache (`Server.Capability`), and metrics family (`Metrics.Snapshot`) are the Phase 17/20 substrates; `ResolveCancelled` is the load-bearing D-19 contract Phase 17 calls directly.

---
*Phase: 16-acp-wire-foundation*
*Completed: 2026-08-27*

## Self-Check: PASSED

- request_registry.go, request_registry_test.go, metrics.go, server.go, handlers.go, types.go, emitter.go exist on disk ✓
- Commits 2c1bbfa, c3f9e8f, 6b0a262, eb51873, 289db57, 4cb4bc6, 76091ff, 32a0262 present in git log ✓
- All task acceptance criteria re-run and passing; plan-level verification (`go test -race ./internal/acp/ -count=1`, `mise ci`) green ✓
- ACP-03 NOT marked complete in REQUIREMENTS.md — shared-ID gate: sibling plans (16-05/16-06) also declare it and are still pending ✓
