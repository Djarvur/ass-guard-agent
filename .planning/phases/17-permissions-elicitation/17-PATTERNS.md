# Phase 17: Permissions + Elicitation - Pattern Map

**Mapped:** 2026-08-27
**Files analyzed:** 10 (new/modified)
**Analogs found:** 9 / 10 (Phase 16 registry/config-surface analogs are planned-not-executed — coded against contracts)

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `internal/perm/rules.go` (NEW) | utility | transform (rule evaluation) | `internal/session/planmode.go` (stateless gate check) | role-match |
| `internal/perm/store.go` (NEW) | service | file-I/O (CRUD on yaml) | `internal/learning/store.go` | exact |
| `internal/session/gate.go` (NEW) | middleware (pre-execution gate) | request-response | `internal/session/session.go` `planModeBlocks` site (469-473) | exact |
| `internal/session/askqueue.go` (NEW) | service | event-driven (queue) | `internal/session/ask.go` AskBroker | role-match (queue is NEW) |
| `internal/session/ask.go` (MOD) | service | event-driven (suspension) | itself — resumeAskClaimed widened | exact |
| `internal/acpserve/ask_surface.go` (NEW) | component (surface renderer) | transform | `internal/coreexec/ask.go` RenderAskSurface + runtime.go:984-989 wiring | exact |
| `internal/acpserve/config_surface.go` (MOD/NEW) | config | request-response | 16-05 contract (not yet in tree) | no-analog (contract-only) |
| `internal/acp/types.go` (MOD) | model | transform | `internal/acp/handlers.go` wire structs (`//nolint:tagliatelle`) | exact |
| `internal/acp/request_registry.go` | (16-03 delivers) | — | — | no-analog (contract-only) |
| `*_test.go` (5 files, NEW) | test | — | `internal/session/ask_test.go`, `internal/learning/store_test.go`, `internal/sched/sched_test.go` | exact |

## Pattern Assignments

### `internal/session/gate.go` — the chokepoint (middleware, request-response)

**Analog:** `internal/session/session.go` lines 464-477 — the plan-mode gate is THE precedent for a per-call pre-execution gate in runTurn's tool-call loop. The chokepoint sits beside it (same loop, same identity available: turnID, callID, tc.Name, tc.Input).

```go
// internal/session/session.go:464-473 (verbatim — the shape to sit beside):
	// 12-04 plan-mode gate (CAPTURED, 12-05 re-record): while
	// plan mode is ON, gated calls (mutating + the captured
	// extra-refused set) return the captured refusal WITHOUT
	// executing — the target's runtime-level enforcement,
	// mirrored; read-only exploration continues.
	if s.planModeBlocks(tc.Name) {
		s.appendToolResultLoud(turnID, callID, tc.Name, planModeRefusal(), true)

		continue
	}
```

Key facts to copy:
- Deny path appends a tool result via `s.appendToolResultLoud(turnID, callID, tc.Name, payload, isErr)` + `continue` — never an error return.
- The loop sees subagent calls BEFORE the batch (`session.go:421-462`, `isSubagentTool` branch bypasses DispatchBatch) — the gate must be invoked in BOTH branches (research Open Question 2 recommends gating Task with its own specifier).
- The batch dispatch at `session.go:480` is the execute step: `toolexec.DispatchBatch(ctx, s.toolExecOrStub(), s.Catalog, batchCalls)`.
- The ask-suspension pattern at `session.go:507-553` shows the suspend-exit shape: record suspension, `return stopAsk, nil` from runTurn.

**Why NOT lower:** `internal/toolexec/batch.go:210-232` — `executeBounded` wraps every dispatched call in `context.WithTimeout(ctx, d)` where `d = timeoutFor(...)` defaults to `toolcat.DefaultToolTimeoutMS` (120000ms, `internal/toolcat/types.go:69`). A human-scale dialog wait inside the executor dies at 120s. The class derivation source is `isAloneInSlot` (`batch.go:174-185`) — `t.IsMutating() || !t.IsConcurrencySafe()` IS D-06's ask class:

```go
// internal/toolexec/batch.go:174-185 (verbatim — D-06's default ask set):
func isAloneInSlot(name string, catalog *toolcat.Catalog) bool {
	if catalog == nil {
		return false
	}
	t, ok := catalog.Get(name)
	if !ok {
		return false
	}
	return t.IsMutating() || !t.IsConcurrencySafe()
}
```

(Reuse via a session-visible equivalent — session owns a `*toolcat.Catalog` as `s.Catalog`; do not import toolexec's private helper, mirror it against the same `toolcat` mutability API.)

### `internal/session/ask.go` widening + permission resume variant (service, event-driven)

**Analog:** itself — `resumeAskClaimed` (ask.go:368-416) is the exact structural template for the permission resume variant. Copy: the nil-ctx fallback, the kind-switch result rendering, `appendToolResultLoud`, `runTurn(ctx, p.TurnID)` re-entry, and the settle-channel close AFTER the resumed turn returns.

```go
// internal/session/ask.go:403-415 (verbatim — the resume tail to mirror):
	s.appendToolResultLoud(p.TurnID, p.CallID, "ask", marshalAskForm(form), isErr)

	stop, _ := s.runTurn(ctx, p.TurnID)

	if p.settle != nil {
		close(p.settle)
	}
	return stop
```

The permission variant inserts BEFORE `runTurn`: (persist rule if `*_always` →) `out, isErr := exec gated call` via the batch/deadline path → `appendToolResultLoud(turnID, callID, tool, boundedToolResult(out), isErr)`. Note `boundedToolResult` already exists in the session package (used at session.go:452, 520).

Also copy:
- `PendingAsk` struct (ask.go:68-83) — add `PendingAskKindPermission` beside `PendingAskKindPlanApproval` (ask.go:87); carry the gated call's Input + tool name.
- `settle` channel discipline (ask.go:176-181, 407-413): Surface always arms fresh; the winning resume closes exactly its own.
- Structured-reply widening: `ResolveAsk(ctx, reply string)` (ask.go:350) is the plain-string seam; the elicitation `accept.content` map must render through `RenderAskAnswered`-family (`ask.go:281-297`) — string-valued single-field replies must stay byte-identical (golden fixture `internal/coreexec/testdata/zcode-interactive-results.json`).

### `internal/session/askqueue.go` (service, event-driven — NEW pattern, no direct analog)

**Analog:** `internal/session/ask.go` AskBroker (ask.go:97-110) — copy the mutex-guarded single-slot discipline, but extend to a slice of entries (one FIRED, N queued). The claim-race shape (Surface/Claim, ask.go:169-224) is the concurrency template.

```go
// internal/session/ask.go:97-104 (verbatim — the guard discipline to extend):
type AskBroker struct {
	mu        sync.Mutex
	pending   *PendingAsk
	onSurface func(PendingAsk)
	onTimeout func(PendingAsk)
```

- Enqueue note emission: copy the bus-publish + timing discipline from `runtime.go:984-989` (the onSurface callback pattern) — publish `event.AgentMessageChunk{TurnID:…, MessageID:…, Content:…}`. CAUTION the 13-03 timing hazard (`runtime.go:528-548`): a post-turn bus publish is DROPPED when no subscriber exists — queue notes for suspended turns must ride the in-hand emitter or the collector-drain shape shown there.
- Turn-death drain hooks: `handleSessionCancel` (`internal/acp/handlers.go:153-182`) calls `st.cancelTurn()` then `closeSessionIfPossible` — the drain function hangs off the same teardown sites (cancel, session close, serve shutdown; research Open Question 1).

### `internal/perm/rules.go` + `internal/perm/store.go` (utility + service, file-I/O)

**Analog (store):** `internal/learning/store.go` — exact match for the yaml single-writer 0600 atomic store.

```go
// internal/learning/store.go:220-241 (verbatim — the atomic save to copy):
func (s *Store) save(entries []Entry) error {
	env := fileEnvelope{Entries: entries}
	raw, err := yaml.Marshal(env)
	if err != nil {
		return fmt.Errorf("learning: marshal: %w", err)
	}
	tmp := s.path + ".tmp"
	err = os.WriteFile(tmp, raw, filePermOwner)   // filePermOwner = 0o600 (store.go:17)
	if err != nil {
		return fmt.Errorf("learning: write tmp %q: %w", tmp, err)
	}
	err = os.Rename(tmp, s.path)
	if err != nil {
		return fmt.Errorf("learning: rename %q → %q: %w", tmp, s.path, err)
	}
	return nil
}
```

Also copy from the same file: `Open` (store.go:46-65) — MkdirAll parent with `dirPerm = 0o750`, write an empty-valid envelope when absent (the zero-config floor); mutex-serialized mutations; copy-on-read returns.

**Analog (yaml loading):** `internal/modelrouting/load.go:43-76` — yaml.v3 direct use (NOT viper; dotted-key hazard documented there), layered overlay.

**Analog (rules evaluation):** `internal/session/planmode.go` `planModeBlocks` — a stateless name-class check consulted per call from the loop. The grammar table (deny→ask→allow first-match, `:*` wildcard, mcp__ namespace, compound split) is fully specified in 17-RESEARCH §CC Rule Model — hand-roll ~40 LOC prefix matching, no glob library (repo zero-dep precedent).

**MCP namespace mapping:** the per-session catalog registers MCP tools at `runtime.go:950` (`mcpHost.Register(sCatalog)`) under advertised names — the evaluator needs the mapping to/from `mcp__<server>__<tool>` via the MCP host (research A7/Pitfall 7; pin with a table test using a fake registered MCP tool).

### `internal/acpserve/ask_surface.go` (component, transform)

**Analog:** `internal/coreexec/ask.go` `RenderAskSurface` (ask.go:100-133) — the current plain-text surface that becomes the FALLBACK. The elicitation builder is the D-08 mapping table applied to `session.AskQuestion{Question, Header, Options[]{Label, Description}, MultiSelect}` (ask.go:56-63).

```go
// internal/runtime/runtime.go:984-989 (verbatim — the surface wiring site to branch):
	askBroker := session.NewAskBroker(r.askTimeout, func(p session.PendingAsk) {
		r.bus.Publish(event.AgentMessageChunk{
			TurnID: p.TurnID, MessageID: p.TurnID,
			Content: coreexec.RenderAskSurface(p.Questions),
		})
	})
	coreexec.RegisterAsk(sCatalog, askBroker)
```

The dispatcher chooses: elicitation/create (form) when the sticky capability probe says supported (16-03/16-D-13/16-D-18 — consume, don't rebuild), else the above path verbatim.

**RegisterAsk discipline** (coreexec/ask.go:70-85): Execute-only override on the catalog entry — captured Name/Description/InputSchema/Mutability stay byte-identical. Copy this for any executor changes; the AskUserQuestion executor's suspension contract (`ErrSuspended` + questions in Output, coreexec/ask.go:51-68) is UNCHANGED — only the surface differs.

### `internal/acp/types.go` wire structs (model, transform)

**Analog:** `internal/acp/handlers.go:36-41, 60-63, 88-96` — the wire-struct convention: camelCase json tags each marked `//nolint:tagliatelle // ACP wire field`.

```go
// internal/acp/handlers.go:36-41 (verbatim — the convention):
type initializeResponse struct {
	ProtocolVersion   int            `json:"protocolVersion"`   //nolint:tagliatelle // ACP wire field
	AgentCapabilities map[string]any `json:"agentCapabilities"` //nolint:tagliatelle // ACP wire field
	AgentInfo         map[string]any `json:"agentInfo"`         //nolint:tagliatelle // ACP wire field
	AuthMethods       []any          `json:"authMethods"`       //nolint:tagliatelle // ACP wire field
}
```

The RequestPermissionFrame / ElicitationFormFrame Go shapes are already drafted verbatim in 17-RESEARCH §Code Examples — use those; they carry the same tag convention.

## Shared Patterns

### UUID v4 ids (request/registry/message ids)
**Source:** `internal/acp/handlers.go:227-239` (`newSessionID`)
**Apply to:** every registry request id the queue/registry fires.
```go
func newSessionID() string {
	var b [16]byte
	_, err := rand.Read(b[:])
	if err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	b[6] = (b[6] & versionMask) | asciiDelete
	b[8] = (b[8] & variantMask) | uuidVariantSet
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
```

### 0600 atomic file writes
**Source:** `internal/learning/store.go:220-241` (above); same constant discipline at `internal/checkpoint/store.go:93`, `internal/sched/sched.go:28,534-539`.
**Apply to:** permissions.yaml (persist-then-execute for allow_always — a write failure downgrades to allow_once with a loud structured log, never silently widens).

### Session-teardown / cancel wiring
**Source:** `internal/acp/handlers.go:153-182` — `s.mu`-guarded sessionState lookup, `st.cancelTurn()`, `closeSessionIfPossible`.
**Apply to:** the D-13 turn-death drain (open dialog ResolveCancelled + queued-entry drain) hooked into all three teardown paths (cancel notification, session close, serve ctx shutdown).

### Mutex discipline / lock-free human waits
**Source:** `internal/runtime/cron_wiring.go:28-38` — `sessionTurnMu` is THE serialization point; a suspended turn must never hold it (AskBroker Claim discipline proves the shape: Surface records + returns; the resolution drives a fresh resume under the serve-lifetime ctx via `SetAskBroker(resumeCtx, ...)` ask.go:315-322).
**Apply to:** the permission suspension — gate verdict `gateSuspend` ends the turn (stop marker) exactly like `session.go:536-541`; the resolution resumes under `s.askResumeCtx`.

### Structured error results convention
**Source:** `internal/session/session.go:544-551` — errors appended as `{"error": "…"}` JSON tool results with `isErr=true`; marshal-failure fallback literal.
**Apply to:** deny results, automation-decline results, degraded no-broker paths.

### Post-turn emitter timing (13-03 hazard)
**Source:** `internal/runtime/runtime.go:528-548` — subscribe BEFORE the turn, drain AFTER, emit through the in-hand emitter (a post-turn bus publish is lost).
**Apply to:** D-12 queue notes and D-07 automation-decline notes that fire while no turn is active.

## No Analog Found

| File | Role | Data Flow | Reason |
|------|------|-----------|--------|
| `internal/acp/request_registry.go` | service | request-response (id'd outbound) | 16-03 planned-not-executed; CONSUME its contract (Call/Deliver/ResolveCancelled, HUMAN-ASK class) — do not rebuild |
| `internal/acpserve/config_surface.go` permissions.mode handler | config | request-response | 16-05 planned-not-executed; register the real handler behind the locked advertisement shape (persist-then-apply per 16-D-07, sibling of tier/model, reuse WriteLayerOption) |
| priority ask queue | service | event-driven | AskBroker is single-pending, no queue/priority/drain exists — new design per D-11..D-13, built on the AskBroker concurrency template above |

## Metadata

**Analog search scope:** internal/{session,acp,acpserve,coreexec,toolexec,toolcat,runtime,learning,modelrouting,sched,checkpoint}
**Files read:** session/ask.go (full), session/session.go (400-590), toolexec/batch.go (160-250), runtime/runtime.go (520-570, 940-1010), runtime/cron_wiring.go (1-50), coreexec/ask.go (full), acp/handlers.go (full), learning/store.go (1-90, 220-245), modelrouting/load.go (1-60)
**Pattern extraction date:** 2026-08-27
