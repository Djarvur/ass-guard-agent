# Phase 19: Compaction + cache_control - Pattern Map

**Mapped:** 2026-08-27
**Files analyzed:** 9 (4 new, 5 modified)
**Analogs found:** 9 / 9

> **CRITICAL verified finding — CURRENT-STATE-ONLY (measured 2026-08-27, BEFORE Phase 16 executed).**
> At mapping time, Phase 16's `AppendCompaction` / `TypeCompaction` transcript kind had **NOT landed** —
> `grep -rn "Compaction" internal/` found only `internal/profile/corpus_scan.go`
> (corpus census counters, unrelated); `internal/session/transcript.go` line
> constants stopped at 16 kinds (no `"compaction"`), and `Manager` had no
> `AppendCompaction`.
>
> **RESOLUTION (orchestrator-confirmed, revision 1):** the milestone roadmap locks
> "Phase 19 Depends on: Phase 16" and Phase 16 executes first — the transcript kind
> + Manager append + configOptions menu land via Phase 16 (16-02/16-04/16-05 plan
> specs carry the contracts). The original "this phase must add the transcript kind
> itself" directive is **SUPERSEDED**: Phase 19 consumes the landed kind (19-03
> extends it additively with the Summary field) under an explicit
> `<execution_precondition>` gate — halt-and-reconcile if the landed artifacts
> diverge from the named contracts. The measurements above remain valid as the
> pre-Phase-16 baseline; they are NOT a Phase 19 TODO list.

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `internal/session/compaction.go` (NEW) | service | request-response (LLM) + transcript I/O | `internal/session/subagent.go` (nested turn w/ profile copy) + `internal/session/session.go` runTurn | role-match |
| `internal/session/compaction_test.go` (NEW) | test | unit | `internal/session/projector_test.go` / `subagent_test.go` | exact |
| `internal/session/transcript.go` (MOD) | model | file-I/O (append-only JSONL) | existing `TypeAskSuspended`/`TypeCommandProvenance` kind additions | exact |
| `internal/session/manager.go` (MOD) | model | file-I/O | `AppendUsage` / `AppendBoundary` | exact |
| `internal/session/projector.go` (MOD) | service/transform | transform (transcript → window) | itself: `splitAtResetBoundary` + `boundMidTurn` | exact |
| `internal/provider/streaming.go` (MOD) | middleware/adapter | request-response (streaming) | existing `sseWatchdog` error-chunk path + `Send`'s non-2xx handling | role-match |
| `internal/provider/errors.go` (MOD) | utility | request-response | `isNetOrContextError` matcher family | exact |
| `internal/shaper/shaper.go` (MOD) | transform | transform | itself: `Shape` systemBlocks loop (lines 99-102) | exact |
| `internal/profile/types.go` + `loader.go` (MOD) | model/config | file-I/O | `TextBlock` + `readSystemBlocks` | exact |
| `internal/paritycli/parity.go` (MOD) | CLI/utility | transform | itself: `composeCacheProbeInput` (lines 57-87) | exact |

### Addendum (revision 1 — cross-phase deferral for 19-05's files)

19-05's modified files were NOT scannable at mapping time (their substrates land in Phase 16); their analogs defer to the 16-04/16-05 contracts, verified at execution under 19-05's `<execution_precondition>`:

| 19-05 Modified File | Role | Data Flow | Closest Analog (Phase 16 contract) | Match Quality |
|---------------------|------|-----------|-----------------------------------|---------------|
| `internal/modelrouting/config.go` (MOD) | model/config | file-I/O (layered yaml) | 16-04's `Config.SessionTier` layered-key pattern | exact |
| `internal/modelrouting/defaults/config.yaml` (MOD) | config defaults | file-I/O | 16-04's defaults application site (SessionTier heavy default) | exact |
| `internal/modelrouting/config_test.go` (MOD) | test | unit | 16-04's session_tier load/round-trip cases | exact |
| `internal/acpserve/config_surface.go` (MOD) | surface/adapter | request-response (menu/set) | 16-05's ConfigSurface (menu construction, effective values, persist-then-apply) | exact |
| `internal/acpserve/config_test.go` (MOD) | test | unit | 16-05's surface test harness | exact |
| `internal/session/compaction.go` (MOD by 19-05) | service | setter wiring | 16-05's `SetTurnModel` serialized live-apply landing | exact |

## Pattern Assignments

### `internal/session/compaction.go` (NEW — threshold, summarizer, marker, CompactNow)

**Analogs:** `internal/session/session.go` runTurn/streamAndEmit + `internal/session/subagent.go` profile-copy discipline.

**Loop-head insertion site** — `internal/session/session.go:356-369` (verbatim):
```go
const maxIterations = 64 // bound the tool loop (runaway guard)
for range maxIterations {
    err = ctx.Err()
    ...
    // Step 1: project the lean window (D-01/D-02).
    messages, err := s.Projector.Project(turnID)
```
The pre-request compaction check (D-02) goes between the ctx check and `s.Projector.Project(turnID)`. `ctx.Err()` shadowing: note the local `err` vs outer named return — copy the existing shadow discipline exactly.

**Usage capture site (Pitfall 3 fix)** — `internal/session/session.go:675-681` (verbatim):
```go
case "usage":
    if chunk.Usage != nil && s.Bus != nil {
        s.Bus.Publish(event.UsageUpdate{
            TurnID:      turnID,
            InputTokens: chunk.Usage.InputTokens, OutputTokens: chunk.Usage.OutputTokens,
        })
    }
```
Add `s.lastInputTokens = chunk.Usage.InputTokens` here (in-memory read model; the bus/transcript line stays the audit record).

**Error-on-stream handling** — `session.go:390-400`: `s.appendError(turnID, "provider", streamErr, false)` then `return "", fmt.Errorf("session turn stream: %w", streamErr)` — the retry-once branch (criterion 3) intercepts an overflow-classified error before this return.

**Model override without write-back** — `internal/session/subagent.go:265-277` (per RESEARCH, verified quote): assign `prof.Model` on a **copy** of `s.Profile` — "assigning prof.Model never writes back to s.Profile". Use the same copy-per-call shape for the light-tier summarizer profile.

**Summarizer stream discipline (Pitfall 4):** consume chunks in a private loop; do NOT call `streamAndEmit` and do NOT `s.Bus.Publish(event.AgentMessageChunk{...})` (the emit at session.go:659-663 is the exact leak to avoid). Only the usage line is appended (via `Manager.AppendUsage`, matching `internal/session/transcript_writer.go:109-116`'s bus→AppendUsage path — call the Manager directly for the synchronous summarizer).

---

### `internal/session/transcript.go` + `manager.go` (MOD — new `compaction` kind)

> **SUPERSEDED by the resolution above:** Phase 16 (16-02) owns the kind's creation. Phase 19 verifies the landed kind against the contracts below and extends it additively (Summary field per 19-03 Task 1). The patterns that follow are the verification/extension analogs, not creation steps.

**Analogs:** the existing kind-constant block and the Append family.

**Kind constant** — `transcript.go:18-49` pattern (verbatim example):
```go
const (
    TypeSessionStart = "session_start"
    ...
    TypeAskSuspended = "ask_suspended"
)
```
Add `TypeCompaction = "compaction"` with a doc comment in the `TypeAskSuspended` style (purpose + what the Projector does with it).

**Flat Line extension** — `transcript.go:61-115`: the `Line` struct is flat with `omitempty` per-type field subsets grouped by comment. Add a `// compaction` field group (`Summary string`, pre/post pointers, usage snapshot fields can reuse `InputTokens`/`OutputTokens`). Reuse existing fields where shapes match (`TurnID`, `Timestamp`).

**Append method** — `manager.go:233-234` pattern:
```go
// AppendUsage records a token-usage update.
func (m *Manager) AppendUsage(turnID string, input, output int64) error {
```
Copy this shape for `AppendCompaction` (sole-owner mutex append, `0600` discipline — see `openTranscript` transcript.go:123-150). For the marker id: the crypto/rand UUID pattern lives in `internal/shaper/shaper.go:423-435` (`uuidV4`) — follow that implementation style (or extract/share).

---

### `internal/session/projector.go` (MOD — durable reset-point class + budget-fill cut)

**Analog:** itself. Exact sites:

**Reset-point scan to extend** — `projector.go:141-147` (verbatim):
```go
boundaryIdx := -1

for i := range lines {
    if lines[i].Type == TypeBoundary && (turnUserIdx < 0 || i < turnUserIdx) {
        boundaryIdx = i
    }
}
```
Extend the scan to also accept `TypeCompaction`; when the winning reset point is a compaction marker, seed = marker summary (DURABLE — later `TypeBoundary` must not displace it, D-06), tail = budget-fill from the marker forward. Note RESEARCH Open Question 3: recommendation is summary-only seed after a later boundary.

**Seed assembly site** — `projector.go:92-101`: the single-user-message lean seed (`"Task summary (mechanical, post-boundary):\n" + summary + "\n\n--- Current request ---\n" + currentIntent`) is where the compaction summary seed lands — same one-user-message shape.

**Pair-safe cut to generalize to budget-fill** — `projector.go:261-276` (verbatim):
```go
func boundMidTurn(mid []provider.Message) []provider.Message {
    if len(mid) <= MidTurnWindowMessages {
        return mid
    }

    cut := len(mid) - MidTurnWindowMessages
    for cut < len(mid) && mid[cut].Role == roleToolMsg {
        cut++ // advance to the next group head (assistant message)
    }
    ...
}
```
Budget-fill (D-05) replaces the fixed `MidTurnWindowMessages` count with a token bound but MUST keep the advance-past-tool-role group discipline verbatim.

**No-marker backward compat:** `splitAtResetBoundary`'s default paths (transcript.go:149-156 switch) must remain byte-identical when no compaction line exists — pin in tests.

**Subagent safety (Pitfall 5):** `accumulateMidTurn` filters by `TurnID` (projector.go:195-199 `if l.TurnID != turnID { continue }`); the marker must reset only turns that START after it, exactly like `TypeBoundary`.

---

### `internal/provider/streaming.go` (MOD — non-2xx surfacing)

**Analog:** the existing watchdog error-chunk path + `Send`'s capture hook.

**Insertion site** — `streaming.go:231-253` (verbatim):
```go
resp, err := httpClient.Do(req) //nolint:bodyclose // closed in drain goroutine
if err != nil {
    return nil, fmt.Errorf("anthropic provider stream send: %w", err)
}

ch := make(chan StreamChunk, mnd8)
```
The status check goes immediately after the `err != nil` block: non-2xx → read the JSON error envelope, `ClassifyHTTP`, emit an error chunk on `ch` (not a done chunk), close, return. Do NOT let the body reach `drainSSE` (which skips non-`data:` lines and defaults empty finish to `"end_turn"` — streaming.go:497-501, the Pitfall 1 swallow). The error-chunk shape already exists (drainSSE emits one on non-EOF read errors; the turn loop consumes it at session.go:688-695 `case chunkErrorType`).

### `internal/provider/errors.go` (MOD — overflow predicate)

**Analog:** `isNetOrContextError` matcher family, errors.go:118-135. Add `IsOverflow(err error) bool` as a message-class matcher over `*ProviderError` (`errors.As` + case-insensitive `"prompt is too long"` prefix, per A1) — NOT a new `ErrorKind` (KindStructural already covers 400 via `structuralStatuses`, errors.go:148-151).

---

### `internal/shaper/shaper.go` (MOD — cache_control emission, D-12)

**Analog:** itself. **Site** — `shaper.go:99-102` (verbatim):
```go
systemBlocks := make([]anthropic.TextBlockParam, 0, len(p.System))
for _, b := range p.System {
    systemBlocks = append(systemBlocks, anthropic.TextBlockParam{Text: b.Text})
}
```
Replace with the profile-gated form (RESEARCH Code Examples, verified against SDK v1.63.0):
```go
tb := anthropic.TextBlockParam{Text: b.Text}
if b.CacheControl {
    tb.CacheControl = anthropic.NewCacheControlEphemeralParam()
}
```
Zero value (no flag) marshals byte-identically to today. The 4-breakpoint cap degrade (Pitfall 2: keep the LAST 4) is decided at this same loop — count placements, stop emitting past 4, document at site, pin in test. `ComposeRuntimeWorkDir` (shaper.go:82-90) shows the profile-copy discipline if compaction needs any per-request profile mutation.

### `internal/profile/types.go` + `loader.go` (MOD — captured value)

**Analog:** `TextBlock` (types.go:44-48) and `readSystemBlocks` (loader.go:83-126, blocks load as `TextBlock{Type: "text", Text: string(raw)}` with no per-block metadata channel). Since the corpus value is 910/910 uniform, a presence flag (`CacheControl bool` with yaml/json tags matching the existing style) applied to every block carries full fidelity; the loader populates it once. No per-block sidecar file needed.

### `internal/paritycli/parity.go` (MOD — probe flip)

**Analog:** itself, `composeCacheProbeInput` lines 57-87 (verbatim seam var with its own routing comment at lines 53-56: "when the routed emission fix lands, this seam derives the flags from the profile's captured values instead"). Change `comp.System[i] = parity.ProbeSystemBlock{Block: b}` to also mirror the new profile field into the probe's `CacheControl` flag; `AssertPlacementAgainstPin`/`PinClasses` (internal/parity/cacheprobe.go:261-281, compares placement CLASSES not counts) stay untouched.

## Shared Patterns

### Transcript appends (all new Manager methods)
**Source:** `internal/session/manager.go` Append family + `openTranscript` (`transcript.go:123-150`): sole-owner mutex, `O_APPEND|O_CREATE|O_WRONLY` mode `0600`, flat `Line` with `omitempty`, `fmt.Errorf("call: %w", err)` wrapping.

### Error handling (Session core)
**Source:** `session.go` runTurn: `s.appendError(turnID, "<component>", err, false)` + `return "", fmt.Errorf("session turn <phase>: %w", err)`. D-09's degrade path (skip + ONE warning + counter) follows the existing graceful-degradation counter family — but does NOT return.

### Provider error typing
**Source:** `internal/provider/errors.go:85-114` `ClassifyHTTP` — every surfaced HTTP failure becomes `*ProviderError` with a `Kind`; new predicates are matchers over that type, never new Kinds, never raw string checks on the turn loop.

### Profile copies are never written back
**Source:** `subagent.go:265-277` + shaper.go:82-90 doc ("Callers own the profile COPY they compose into... the shared loaded profile is never mutated") — applies to the summarizer's light-tier model override.

## No Analog Found

| File | Role | Data Flow | Reason |
|------|------|-----------|--------|
| (none) | | | Every seam has an in-repo analog; the two genuine new design acts (durable-seed semantics, budget-fill math) extend projector.go sites quoted above |

## Metadata

**Analog search scope:** internal/session, internal/provider, internal/shaper, internal/profile, internal/paritycli, internal/parity
**Files read:** projector.go (full), shaper.go (full), transcript.go (full), errors.go (full), parity.go (53-87), session.go (330-480, 655-690), manager.go (grep-indexed Append family), profile types.go/loader.go (relevant ranges)
**Pattern extraction date:** 2026-08-27
