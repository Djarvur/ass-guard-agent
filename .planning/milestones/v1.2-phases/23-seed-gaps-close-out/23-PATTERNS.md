# Phase 23: SEED Gaps Close-out - Pattern Map

**Mapped:** 2026-08-28
**Files analyzed:** 12 (2 new, 8 modified, 2 test files whose extension targets do not yet exist)
**Analogs found:** 10 / 12 (2 files have no in-repo analog — see "No Analog Found" for the contract source)

## Codebase-State Discrepancies Found During Mapping (planner MUST read)

Two files the RESEARCH.md structure lists as `MOD` do **not exist** in the codebase as of this mapping (verified by `find` + `grep` across `internal/`):

1. **`internal/runtime/commands.go` does not exist.** There is no class-B command intercept table and no RESERVED 13-name set anywhere in Go code (grepped for `RESERVED`, `"help"`, `"compact"`, `handleCommand`, `intercept` — zero command-table hits). Phase 20's plans were authored (commits `49c4e03`/`c89dbff`) but the class-B machinery has **not landed on branch `gsd/v1.2-claude-code-parity`**. The research's 20-01/20-02 citations are PLAN-file contracts, not shipped code.
2. **`internal/runtime/commands_test.go` does not exist** — the "extends TestClassB battery" instruction targets a file that is not there.

Everything else the research cites was verified accurate against source this session (line numbers below are from fresh reads): transcript kinds, `AppendLocalCommand`, the Projector anchor rule, `runTurn`'s loop top, all five store extension points, the config-surface pending-option pattern, `routeAskReply` ordering, and the cancel contract.

**Planning consequence:** Phase 23 either (a) sequences after Phase 20 execution, or (b) creates `commands.go` fresh under the 20-01/20-02 contract with `/undo` as a founding member. Decide in planning; do not assume the file exists.

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `internal/session/steerqueue.go` | utility (concurrency primitive) | event-driven (ticket queue) | `internal/session/ask.go` (AskBroker) | role-match |
| `internal/session/steerqueue_test.go` | test | event-driven | `internal/session/checkpoint_seam_test.go` + `internal/checkpoint/store_test.go` helpers | role-match |
| `internal/session/session.go` | service (turn loop) | streaming + event-driven | self — `runTurn` (349-583) | exact |
| `internal/session/projector.go` | transform | batch | self — `accumulateMidTurn` (170-227) | exact |
| `internal/session/transcript.go` | model/types | batch (log schema) | self — 16-D-20 additive-kind precedents (48-76) | exact |
| `internal/session/manager.go` | service (transcript sole owner) | file-I/O append | self — `AppendLocalCommand` (362-367) | exact |
| `internal/runtime/runtime.go` | controller | event-driven + request-response | self — `Run` ingress (479-566), `sessionFor` (976-1268) | exact |
| `internal/runtime/commands.go` | controller (command intercept) | request-response | **none in-repo** — 20-01/20-02 plans + `routeAskReply` | none-in-repo |
| `internal/runtime/commands_test.go` | test | request-response | `internal/runtime/checkpoint_session_test.go` (fixture style) | none-in-repo (target) |
| `internal/checkpoint/store.go` | service (git subprocess) | file-I/O | self — Snapshot/Restore/prune/withLock | exact |
| `internal/checkpoint/store_test.go` | test | file-I/O | self — `treeMap`/`seedWorkspace`/`snap` helpers | exact |
| `internal/acpserve/config_surface.go` | config | request-response | self — pending-select-option pattern (`optCompactionThresh`) | exact |

## Pattern Assignments

### `internal/session/steerqueue.go` (utility, event-driven) — NEW

**Analog:** `internal/session/ask.go` — the AskBroker is the codebase's established shape for per-session, mutex-guarded queue-like state with racing claimants. No ticket-queue exists in-repo; the ticket/cutoff protocol is defined in `.planning/research/PITFALLS.md:329` (quoted in RESEARCH.md Pattern 2).

**Structural pattern to copy — single mutex, no channels, atomic first-claimant-wins** (`internal/session/ask.go:97-110, 169-224`):

```go
// AskBroker holds the per-session pending ask + the D-01 timeout timer. It is
// deliberately Await-free: Surface records + notifies; Claim atomically hands
// the pending ask to exactly ONE resume driver (the operator reply and the
// timeout timer race; the loser observes nothing pending and no-ops).
type AskBroker struct {
	mu        sync.Mutex
	pending   *PendingAsk
	...
}

func (b *AskBroker) Claim() (PendingAsk, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.pending == nil {
		return PendingAsk{}, false
	}

	p := *b.pending

	b.pending = nil

	b.stopTimerLocked()

	return p, true
}
```

**Copy for SteerQueue:** `mu sync.Mutex` + `items []SteerItem` + `next uint64` (tickets) + `cutoff uint64`; `Enqueue -> ticket`, `Drain -> coalesced batch in arrival order (D-02)`, `Cancel(cutoff) -> resolve items <= cutoff cancelled-normal (D-06)`. Producer enqueues never block the drain (single mutex, no chans). Transport-neutral by construction: **no imports from `internal/acp`** (TG-02's compile-level contract).

### `internal/session/steerqueue_test.go` (test, event-driven) — NEW

**Analog 1 — ordering without sleeps:** `internal/session/checkpoint_seam_test.go:13-32` — mutex-guarded event log:

```go
type seamEventLog struct {
	mu     sync.Mutex
	events []string
}

func (l *seamEventLog) add(ev string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.events = append(l.events, ev)
}
```

Use it to pin "drain lands after the previous iteration's result appends, before Project". For the `-race` producer/cancel hammer (milestone prescription), combine with a plain goroutine loop of `Enqueue` during `Cancel`.

**Analog 2 — fs fixture helpers:** `internal/checkpoint/store_test.go:24-129` (`treeMap`, `writeTestFile`, `seedWorkspace`, `openStore`, `snap` — all `t.Helper()` style, `t.Fatalf` on setup error).

### `internal/session/session.go` (service, streaming) — MOD

**Analog:** self — the drain seam is the `runTurn` loop top. Verified lines (`internal/session/session.go:349-394`):

```go
func (s *Session) runTurn(ctx context.Context, turnID string) (stop string, err error) {
	...
	const maxIterations = 64 // bound the tool loop (runaway guard)
	for range maxIterations {
		err = ctx.Err()
		if err != nil {
			s.recordCanceled(turnID, "context cancelled before turn step")

			return stopCancelled, nil
		}
		// Step 1: project the lean window (D-01/D-02).
		messages, err := s.Projector.Project(turnID)
		...
		resp, textBuf, streamErr := s.streamAndEmit(ctx, turnID, messages)
```

**Drain-point insertion rule:** drain between the `ctx.Err()` check (367-371) and `Project` (373). Appending steering AFTER the previous iteration's result appends (529-536) and BEFORE `Project` makes pair-safety structural — never split a `tool_call`/`tool_result` pair (Pitfall 2).

**Loud-degrade pattern to copy** (`session.go:197-204`, the AUD-03 discipline — snapshot failure is loud but never turn-fatal):

```go
	if s.Checkpointer != nil {
		cerr := s.Checkpointer.SnapshotTurn(ctx, s.SessionID, turnID)
		if cerr != nil {
			slog.Error("checkpoint: snapshot failed (turn continues without a checkpoint)",
				"turnID", turnID, "error", cerr.Error())
			s.appendError(turnID, "checkpoint", cerr, true)
		}
	}
```

Apply to: steering-delivery append failures, GC sweep failures, exclude-append failure (structured stderr note, never a turn/serve failure).

**Wiring pattern to copy** — `SetAskBroker` (`internal/session/ask.go:315-325`): a nil-safe setter storing the broker + a serve-lifetime ctx is how per-session components attach to Session. Add `SetSteerQueue` the same way; the Session struct field pattern is `ask *AskBroker` (session.go:113-116, "Nil = not wired").

### `internal/session/projector.go` (transform, batch) — MOD

**Analog:** self — `accumulateMidTurn` is the exact function gaining the `steering_delivery` case. The anchor hazard is real and verified (`internal/session/projector.go:170-227`):

```go
func accumulateMidTurn(lines []Line, turnID string) []provider.Message {
	// The anchor is the current turn's user_message ONLY ...
	anchor := 0

	for i := range lines {
		if lines[i].Type == TypeUserMessage && lines[i].TurnID == turnID {
			anchor = i + 1
		}
	}
	...
	for i := anchor; i < len(lines); i++ {
		l := &lines[i]
		if l.TurnID != turnID {
			continue // only the CURRENT turn's lines fold (subagent turns have their own)
		}

		switch l.Type {
		case TypeToolCall:
			...
		case TypeToolResult:
			flushBatch() // the batch is closed once its results start arriving
			...
		case TypeAssistantMessage:
			flushBatch()

			out = append(out, provider.Message{Role: roleAssistant, Content: l.Text})
		}
	}
```

**Change shape:** add one `case TypeSteeringDelivery:` folding `l.Text` as `provider.Message{Role: roleUserMsg, Content: l.Text}` in arrival position (after `flushBatch()` — steering is not part of a tool batch). **The anchor loop (174-180) must NOT match steering lines** — it matches only `TypeUserMessage`, which is exactly why a new kind is required (Pitfall 1: a `user_message` steering line would move the anchor and wipe the turn's accumulated window). `findCurrentIntent` (336-360) and `extractSummary` (281-332) switch on `TypeUserMessage`/`TypeAssistantMessage`/`TypeToolCall` only — they are safe by construction once steering is its own kind.

**Pair-safety precedent for review:** `boundMidTurn` (261-276) advances the cut past tool-role messages so the window never starts with an orphaned result — the invariant shape the drain point must preserve.

### `internal/session/transcript.go` (model/types, batch) — MOD

**Analog:** self — the additive-kind family. Four prior precedents exist, each documented in-place (`internal/session/transcript.go:48-76`):

```go
	TypeAskSuspended = "ask_suspended"
	...
	TypeRawThinking = "raw_thinking"
	...
	TypeLocalCommand = "local_command"
	...
	TypeCompaction = "compaction"
```

**Copy:** add `TypeSteeringDelivery = "steering_delivery"` and `TypeParkedAsk = "parked_ask"` with the same doc-block discipline (what the line carries, which append path, how the Projector treats it). **Readers must tolerate unknown kinds** — 16-D-20 additive-only weak schema (documented at transcript.go:144-148).

**Line struct fields to reuse** (`transcript.go:89-156`): the flat struct already carries `TurnID`, `Text`, `Content`, `Args`, `Input` — steering deliveries need `TurnID` + `Text` (coalesced marker text) + optionally `Input` (ticket/count JSON); parked asks need `TurnID` + `Text` (summary). Do NOT add new struct fields unless nothing fits — the existing additive-fields block (149-156) is the extension site.

### `internal/session/manager.go` (service, file-I/O append) — MOD

**Analog:** self — `AppendLocalCommand` is the append-template the research explicitly names (`internal/session/manager.go:362-367`):

```go
func (m *Manager) AppendLocalCommand(turnID, key, args, expansion string, sourceChain []string) error {
	return m.appendLine(&Line{
		Type: TypeLocalCommand, TurnID: turnID, Timestamp: now(),
		Name: key, Args: args, Expansion: expansion, SourceChain: sourceChain,
	})
}
```

**Copy:** `AppendSteeringDelivery(turnID, text string, count int)` and `AppendParkedAsk(turnID, summary string)` — one-line methods over `m.appendLine(&Line{...})`. Steering is model-visible untrusted user content: it goes through the **REDACTED** path (`appendLine`, manager.go:77-104) — never `appendLineUnredacted`, whose exemption is type-scoped to `raw_thinking` and must never widen (T-16-04, manager.go:106-113).

### `internal/runtime/runtime.go` (controller, event-driven) — MOD

**Analog:** self — three distinct patterns live here.

**1. Ingress classification order** (the disambiguation core, Pattern 6). Verified current ordering (`internal/runtime/runtime.go:479-566`):

```go
func (r *Runner) Run(
	ctx context.Context, sessionID string,
	emit acp.ChunkEmitter, prompt []acp.ContentBlock,
) (string, error) {
	sess := r.sessionFor(ctx, sessionID)

	turnMu := r.sessionTurnMu(sessionID)
	turnMu.Lock()

	defer turnMu.Unlock()
	...
	r.markClientTurn(sessionID, true)

	defer r.markClientTurn(sessionID, false)
	...
	ch := r.bus.Subscribe("AgentMessageChunk", event.BufAgentMessageChunk)
	...
	// 12-01 reply routing (ACP-01): a prompt arriving while an ask is pending
	// is the OPERATOR'S ANSWER, not a new turn (see routeAskReply).
	if stop, handled := r.routeAskReply(ctx, sess, blocks); handled {
		...
		return stop, nil
	}

	if !r.engineEnabled || r.eng == nil || r.patternTable == nil {
		blocks = r.expandUserBlocks(sess, blocks)
	}
	...
	stop, err := r.runOneTurn(ctx, sess, blocks)
```

**The classifier extends THIS ordering:** pending-ask route (incl. the new parked-cancel grammar) → class-B resolve (`invocationFor` — single parse of the first text block) → steering enqueue when turn/chain active → ordinary new turn. Critical mutex fact (D-12 / Pitfall 4): everything after `turnMu.Lock()` (490) **holds the session mutex** — an /undo auto-cancel that must wait for an active turn can never be classified there. Classify BEFORE `turnMu.Lock()`; only the restore itself acquires the mutex afterwards.

**2. Active turn/chain state to classify against** — already race-tested, do not rebuild (`runtime.go:181-182, 926-969`):

```go
turnActive           sync.Map // sessionID -> *atomic.Bool
...
func (r *Runner) chainCount(sessionID string) int { ... }

// WaitChainIdle blocks until no engine chain is active for the session ...
func (r *Runner) WaitChainIdle(ctx context.Context, sessionID string) bool {
	for {
		if r.chainCount(sessionID) == 0 {
			return true
		}
		...
	}
}
```

Parked chains hold NO mutex while `chainCount > 0` (`runOneTurn` returns at suspension, 849-855) — that is the state the "refuse with active chains" guard and D-12's cancel compose over.

**3. Note emission** — steering/parked notes ride the `AgentMessageChunk` note family. The construction-site precedent (`runtime.go:1120-1125`, the ask surface callback):

```go
	askBroker := session.NewAskBroker(r.askTimeout, func(p session.PendingAsk) {
		r.bus.Publish(event.AgentMessageChunk{
			TurnID: p.TurnID, MessageID: p.TurnID,
			Content: coreexec.RenderAskSurface(p.Questions),
		})
	})
```

The live-emitter precedent (`runtime.go:561-563`):

```go
	if adv := <-advDone; adv != nil && r.advisoryNoteDue(sessionID, adv.class) {
		_ = emit.AgentMessageChunk(adv.turnID, adv.text)
	}
```

**Timing hazard (documented at 543-549):** a bus publish after the per-Run subscriber drains is LOST. Boundary notes fire while the turn runs (subscriber live) — safe; but any post-turn emission must go through the in-hand `emit`, never a deferred bus publish.

**4. Checkpoint store promotion (SEEDG-02 prerequisite).** Today the store is built inside `sessionFor` and only the adapter lands on the session (`runtime.go:1004-1011, 1198-1203`):

```go
	var ckptStore *checkpoint.Store

	st, cerr := checkpoint.Open(dir)
	if cerr == nil {
		ckptStore = st
	} else {
		log.Printf("ass-guard: checkpoint store disabled for %s (%v) — turns run WITHOUT undo snapshots", dir, cerr)
	}
	...
	if ckptStore != nil {
		s.Checkpointer = checkpointerAdapter{store: ckptStore}
	}
```

The Runner never retains `ckptStore` — /undo, the restore guard, and the session-start GC need a Runner-level field (built once per workspace). **Degrade loudly** the same way (Pitfall 9): a nil store means /undo reports "unavailable", never silent. The `checkpointerAdapter` (256-268) is the seam pattern keeping `internal/session` import-free of `internal/checkpoint`.

### `internal/runtime/commands.go` (controller, request-response) — NEW (target absent)

**No in-repo analog.** Closest in-repo shapes:
- `routeAskReply` (`runtime.go:1611-1629`) — the pre-turn intercept + fall-through-to-ordinary-turn contract (`handled=false`):

```go
func (r *Runner) routeAskReply(
	ctx context.Context, sess *session.Session, blocks []session.ContentBlock,
) (string, bool) {
	if !sess.HasPendingAsk() {
		return "", false
	}

	idx := firstTextBlockIndex(blocks)
	if idx < 0 || blocks[idx].Text == "" {
		return "", false
	}

	stop, rerr := sess.ResolveAsk(ctx, blocks[idx].Text)
	if rerr != nil {
		return "", false // the D-01 timer won the race — an ordinary turn
	}

	return mapAskStop(stop), true
}
```

- `invocationFor` (`runtime.go:458-476`) — the single-parse discipline: resolve the first text block ONCE (`ecosys.ParseInvocation` + registry lookup); every consumer acts on that one resolution.
- The durable-record append: `sess.Manager.AppendLocalCommand(...)` (see manager.go analog) — /undo's `/undo N` args go in VERBATIM.

**Authoritative contract (read before planning):** `.planning/phases/20-built-in-commands-skills-per-agent-model/20-01-PLAN.md` and `20-02-PLAN.md` — intercept placement (in `Runner.Run` after `routeAskReply`, before the engine branch), RESERVED 13-name set (/undo joins as the 14th), D-05 output shape (echo `user_message_chunk` → `agent_message_chunk` → stopReason end_turn, zero provider calls), 20-D-01 shadow-check warning.

### `internal/runtime/commands_test.go` (test, request-response) — NEW (target absent)

**Analog:** `internal/runtime/checkpoint_session_test.go` (317 lines) — the established runtime-package integration-test style (temp workspace fixtures, fake providers, Runner construction). The class-B battery itself lands with Phase 20 per its plans; /undo cases join it.

### `internal/checkpoint/store.go` (service, file-I/O + subprocess) — MOD

**Analog:** self — all five SEEDG-02 extensions compose verified primitives.

**1. Id grammar — three coupled enforcement sites** (Pitfall 6; all three must move together for the pre-restore id family):
- `store.go:114` — `idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+-turn-(\d{3,})$`)`
- `store.go:364-380` — `validateTurnID` (also enforces the `<sessionID>-turn-` prefix)
- `store.go:454-488` — `parseRefLine` (`TrimSuffix(id, "-turn-"+m[1])` derives SessionID)

**2. Snapshot/restore composition under one lock** (`store.go:218-237, 252-279`):

```go
func (s *Store) Snapshot(ctx context.Context, sessionID, turnID string) error {
	err := validateTurnID(sessionID, turnID)
	if err != nil {
		return err
	}

	return s.withLock(ctx, func() error {
		sha, err := s.commitSnapshot(ctx, turnID)
		...
		err = s.updateRef(ctx, turnID, sha)
		...
		return s.prune(ctx)
	})
}
```

Pre-restore snapshots ARE `Snapshot` calls with a new id family (D-09); a GC sweep is a new `withLock` body (delete refs via `update-ref -d` — the `prune` idiom at 494-511 — then `reflog expire --expire=now --all` + `gc --prune=now` so objects actually shrink).

**3. Every git call funnels through the isolation wrapper** (`store.go:331-359`) — GC git args are no exception:

```go
func (s *Store) git(ctx context.Context, args ...string) ([]byte, error) {
	full := append([]string{
		"--git-dir=" + s.gitDir,
		"--work-tree=" + s.workDir,
		"-c", "core.hooksPath=" + hooksPathOff,
	}, args...)
	...
}
```

**4. Refs never enter git unvalidated** (`store.go:246-256`): the restore validates the id against `idPattern` BEFORE any subprocess (T-14-01). The new id family inherits this gate; never bypass it.

**5. Exclude-file write pattern** (`store.go:305-317` — the SHADOW store's own exclude; the USER-repo append at `<workDir>/.git/info/exclude` is the new sibling, append-only + idempotent, verified by the research's experiment).

**6. Nested-repo detection** — no analog exists; research verified the failure shape (gitlink 160000, `clean -fd` never descends). Detection walks the workspace for `.git` entries (dir OR file — worktrees/submodules), skipping `.ass-guard/`; refusal is outright (D-12: no auto path).

### `internal/checkpoint/store_test.go` (test, file-I/O) — MOD

**Analog:** self — the fixture helpers at `store_test.go:24-129` (`treeMap` fingerprints the tree skipping `.ass-guard/` — byte-identical-restore assertions; `seedWorkspace` — the flat text+binary tree that Pitfall 7 says must gain a nested-repo + `.git`-FILE fixture; `openStore`/`snap`). The GC/guard/nested/exclude/pre-restore tests extend this file with the same `t.Helper()` discipline.

### `internal/acpserve/config_surface.go` (config, request-response) — MOD

**Analog:** self — the pending-select-option pattern is the exact template for D-10's two new menu entries (advertise all, apply-as-landed). The compaction-threshold numeric-as-select precedent, verified (`config_surface.go:601-625`):

```go
func isMenuOption(bare string) bool {
	return bare == optModel || bare == optTier || bare == optPermissionsMode || bare == optCompactionThresh
}

func isPendingOption(bare string) bool {
	return bare == optPermissionsMode || bare == optCompactionThresh
}

func pendingValues(bare string) []string {
	if bare == optPermissionsMode {
		return []string{permModeUngated, permModeGated}
	}

	return []string{compactionOff, compactionMidLower, compactionMid, compactionDefault, compactionMidHigh}
}
```

**Menu construction** (`config_surface.go:501-531`) — the `build` closure; every option is `acp.ConfigOptionTypeSelect` (the ONLY pinned wire type — `checkpoint.expiry_days`/`max_per_session` must ship as enumerated selects):

```go
	build := func(id, name, desc, category, current string, values []string) acp.ConfigOptionFrame {
		opts := make([]acp.ConfigOptionValue, 0, len(values))
		for _, v := range values {
			opts = append(opts, acp.ConfigOptionValue{Value: v, Name: v})
		}

		return acp.ConfigOptionFrame{
			ID: id, Name: name, Description: desc, Category: category,
			Type: acp.ConfigOptionTypeSelect, CurrentValue: current, Options: opts,
		}
	}
```

**Accept-and-log set path** (`setPendingLocked`, 352-371) — validates membership against `pendingValues`, logs one structured line, persists nothing. **Read-back for `checkpoint:` keys** must use the generic layer-map pattern (`readLayerMap`/`mapHasPath`, 629-668) — the typed `modelrouting.Config` decode silently DROPS unknown top-level keys (verified `load.go`), so a `checkpoint:` key is load-safe but invisible to the typed config. **`isMenuOption` (608-610) must learn the new ids or Set rejects them with "unknown option id"** (Pitfall 10).

## Shared Patterns

### Loud, never-fatal degradation (AUD-03)
**Source:** `internal/session/session.go:197-204` (slog.Error + transcript error line, turn continues); `internal/runtime/runtime.go:1010` (store-open failure log, session runs without checkpoints); `internal/runtime/runtime.go:426, 434` (transcript-write failures degrade to stderr logs, never ACP errors).
**Apply to:** GC sweep failures, exclude-append failure, steering-delivery append failure, parked-note failure. Everything in this phase degrades loudly to stderr/slog (transport discipline: stdout is ACP-only) and never fails the turn or the serve.

### Transcript append template (REDACTED path)
**Source:** `internal/session/manager.go:77-104` (`appendLine` — marshal → redact → mutex → write) + `AppendLocalCommand` (362-367).
**Apply to:** `AppendSteeringDelivery`, `AppendParkedAsk`. Never `appendLineUnredacted` (type-scoped to raw_thinking, T-16-04).

### Additive transcript kinds (16-D-20)
**Source:** `internal/session/transcript.go:48-76` (four precedents) + tolerance rule at 144-148.
**Apply to:** `steering_delivery`, `parked_ask`. Readers tolerate unknown kinds until their owning phase parses; replay folds via the Projector (same path live and on replay — 18-D-01).

### Note emission via AgentMessageChunk + the timing hazard
**Source:** `internal/runtime/runtime.go:1120-1125` (construction-site bus publish), `runtime.go:561-563` (in-hand emitter), hazard documented at 543-549.
**Apply to:** "steering applied: N inputs" (D-03), "ask waiting behind running turn: \<summary\>" (D-05), queued notes. Fire at the boundary while the subscriber is live or through the in-hand `emit` — never a post-turn bus publish (it is LOST).

### Whole-store lock discipline
**Source:** `internal/checkpoint/store.go:518-563` (`withLock` — in-process mutex + cross-process O_EXCL + stale theft).
**Apply to:** GC sweep, pre-restore snapshot, restore — all store mutations run inside `withLock`; GC's `reflog expire` + `gc --prune=now` must sit in the SAME critical section as its ref deletions.

### Strict id validation before git refspec (T-14-01)
**Source:** `internal/checkpoint/store.go:114` (grammar), `364-380` (validateTurnID), `454-488` (parseRefLine), `252-256` (restore gate).
**Apply to:** the pre-restore snapshot id family — update ALL THREE grammar sites together (Pitfall 6); a snapshot failure must ABORT the restore (fail-closed, D-09/D-12).

### Per-session mutex-guarded component wiring
**Source:** `internal/session/ask.go:97-263` (AskBroker), `315-325` (SetAskBroker nil-safe setter); `internal/runtime/runtime.go:181-182` (turnActive/turnMus sync.Map fields).
**Apply to:** SteerQueue (same structural shape: one mutex, per-session, nil = not wired) and its Runner-side wiring in `sessionFor`.

### Cancel contract (D-12's auto-cancel substrate)
**Source:** `internal/acp/handlers.go:382-452` — `handleSessionPrompt` stores the cancel func, `handleSessionCancel` calls `st.cancelTurn()`; the turn observes the cancelled ctx, records `canceled` (session.go:367-371 → `recordCanceled`), and the handler maps it to stopReason "cancelled" (D-16) after the emitter `Barrier` (updates-before-response).
**Apply to:** /undo's auto-cancel-then-restore — cancel via this existing contract, THEN pre-restore snapshot, THEN restore. Order the snapshot before the cancel's state changes where ordering permits (D-12 reversibility note). Classify /undo BEFORE `turnMu.Lock()` (Pitfall 4 — holding turnMu while waiting for the turn self-deadlocks; `-race` will NOT catch it).

### Config pending-select-option (advertise all, apply-as-landed)
**Source:** `internal/acpserve/config_surface.go:601-625, 352-371, 501-531, 629-668`.
**Apply to:** `checkpoint.expiry_days`, `checkpoint.max_per_session` — enumerated selects (numeric-as-select precedent), membership-validated accept, persist via `WriteLayerOption` under a `checkpoint:` key path, read back via `readLayerMap`/`mapHasPath`.

### Test conventions
**Source:** `internal/session/checkpoint_seam_test.go:13-32` (seamEventLog — ordering assertions without sleeps); `internal/checkpoint/store_test.go:24-129` (`t.Helper()` fixtures, `treeMap` byte-identical assertions, flat `seedWorkspace` tree that must gain nested-repo + `.git`-file variants); package-level note `//nolint:testpackage // internal package test`.
**Apply to:** all Wave 0 test files. Concurrency tests run under `-race` (milestone prescription: producer hammering enqueue during cancels).

## No Analog Found

| File | Role | Data Flow | Reason | Pattern source instead |
|------|------|-----------|--------|------------------------|
| `internal/runtime/commands.go` | controller | request-response | Phase 20 class-B machinery never landed on this branch (no commands.go, no RESERVED set anywhere in `internal/`) | `.planning/phases/20-built-in-commands-skills-per-agent-model/20-01-PLAN.md` + `20-02-PLAN.md` contracts; in-repo shapes `routeAskReply` + `invocationFor` |
| `internal/session/steerqueue.go` (core semantics) | utility | event-driven | No ticket/cutoff queue exists in-repo; channels cannot express cutoff-cancel + inspection cleanly | RESEARCH.md Pattern 2 + PITFALLS.md:329 protocol; AskBroker as the structural shell |

## Metadata

**Analog search scope:** `internal/session/`, `internal/runtime/`, `internal/checkpoint/`, `internal/acpserve/`, `internal/acp/` (plus repo-wide greps for command-table symbols)
**Files read in full:** transcript.go, manager.go, projector.go, ask.go, session.go, store.go, config_surface.go, runtime.go, checkpoint_seam_test.go, handlers.go (targeted 352-481), store_test.go (targeted 1-130)
**Files scanned (grep/find):** all of `internal/` for command/RESERVED/local_command/turnActive symbols
**Discrepancies flagged:** 2 (commands.go + commands_test.go absent vs. research "MOD")
**Pattern extraction date:** 2026-08-28
