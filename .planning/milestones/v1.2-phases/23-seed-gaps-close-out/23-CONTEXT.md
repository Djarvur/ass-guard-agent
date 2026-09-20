# Phase 23: SEED Gaps Close-out - Context

**Gathered:** 2026-08-27
**Status:** Ready for planning

<domain>
## Phase Boundary

The remaining SEED-004 gaps close: steering/input queue applied at model-request boundaries during a running turn (transport-neutral — the Telegram prerequisite), parked-ask disambiguation (visible, cancelable, never blocking), checkpoint restore hardened (active-chain refusal, pre-restore snapshots, nested-repo refusal, expiry GC, git-exclude), and /undo as a class-B command walking the checkpoint stack.

</domain>

<decisions>
## Implementation Decisions

### Steering Injection
- **D-01:** Steering text enters the NEXT request as a user-role message wrapped in a steering marker (system-reminder-shaped — the captured wire convention): the model sees the user speaking mid-turn, distinguishable from turn-start prompts; replay can tell steering from prompts.
- **D-02:** All steering queued since the last boundary COALESCES into one delivery at the next boundary — arrival order preserved, one marker block. Bounded request growth.
- **D-03:** Each boundary drain emits a session/update note ("steering applied: N inputs") + a typed `steering_delivery` transcript record — visible live, reconstructable on replay (18-D-01).
- **D-04:** Steering NEVER cancels or interrupts: the running turn absorbs it at the boundary and continues. Cancellation stays the separate explicit ticket/cutoff protocol (SEEDG-01 letter). One knob each.
- **D-05:** A parked ask is VISIBLE immediately: one session/update note when parked ("ask waiting behind running turn: \<summary\>"), then fires in full when its queue position is reached.
- **D-06:** A parked ask is CANCELABLE while waiting (cancel surface on the parked note; a subsequent input can parse as cancel): resolves cancelled-normal WITHOUT killing the turn — 17-D-13's drain semantics applied proactively.
- **D-07:** Parking never blocks: parked asks belong to their own (or origin) turns; parking affects delivery order only. The active turn's progress is untouched.
- **D-08:** Checkpoint expiry = AGE + COUNT, whichever binds first: default 7d age AND a max-per-session count (count default Claude's discretion); GC sweeps at session start (18-D-09's grace-family precedent).
- **D-09:** Pre-restore snapshots ARE checkpoint objects: they enter the same age+count GC and are restorable via /undo — a botched restore is undoable by the same machinery. One store, one lifecycle.
- **D-10:** `checkpoint.expiry_days` (7) and `checkpoint.max_per_session` join the configOptions menu (Phase 16's rule — advertise all, apply-as-landed).
- **D-11:** Stack walk: /undo restores the last checkpoint; repeated /undo keeps walking back (each restore snapshots current state first — D-09 makes the walk reversible); `/undo N` jumps N steps. Depth bounded naturally by the age+count GC.
- **D-12:** /undo with an ACTIVE turn AUTO-CANCELS then restores (operator override of the refuse option): cancel via the existing cancel contract (clean drain — 18-D-12 family), then pre-restore snapshot, then restore. Nested-repo refusal still refuses outright (no auto path — gitlink contents would be silently unprotected). — **Reversibility:** costly — auto-cancel-then-restore is a compound destructive action on one keystroke; the pre-restore snapshot is the only undo, so the snapshot MUST precede the cancel's state changes where ordering permits.

### Claude's Discretion
- The steering marker's exact wrapper syntax (existing system-reminder shape family).
- Parked-note wording and the cancel-input grammar (what parses as "cancel the parked ask").
- max_per_session default value; GC sweep logging shape.
- /undo N argument parsing edge cases (0, negative, beyond-depth).
- The transport-neutral steering API's concrete interface shape (Go API consumable by a non-ACP frontend — TG-02's contract).

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements & Roadmap
- `.planning/REQUIREMENTS.md` §SEED Gap Close-out — SEEDG-01 (boundary-drain only, never mid-in-flight-request, never splitting tool_use/result pairs, ticket/cutoff protocol, parked-ask disambiguation, transport-neutral — do NOT descope to queue-behind), SEEDG-02 (five restore guards), SEEDG-03 (/undo class-B) verbatim
- `.planning/REQUIREMENTS.md` §Telegram — TG-02 (consumes the steering core) — the transport-neutrality consumer contract
- `.planning/ROADMAP.md` §Phase 23 — goal, 5 success criteria

### Prior Phase Contracts (hard dependencies)
- `.planning/phases/20-built-in-commands-skills-per-agent-model/20-CONTEXT.md` — D-05 (class-B output shape /undo follows), D-06 (boundary discipline analog)
- `.planning/phases/17-permissions-elicitation/17-CONTEXT.md` — D-13 (turn-death ask drain — D-06 extends proactively), D-11/D-12 (ask serialization + queue visibility — parked asks join the same queue discipline)
- `.planning/phases/18-session-family/18-CONTEXT.md` — D-09 (grace-GC precedent for D-08), D-12 (cancel-and-drain — D-12's auto-cancel rides it), D-01 (replay reconstructs steering deliveries)
- `.planning/phases/16-acp-wire-foundation/16-CONTEXT.md` — D-20 (additive kinds — steering_delivery, parked_ask records), D-22 (local_command record for /undo)

### Code Anchors
- `internal/runtime/runtime.go` checkpointerAdapter + checkpoint store wiring (248–256, 881) — the store /undo and GC extend
- Turn serialization mutex (sessionTurnMu, 12-07 D-02) — today's queue-behind; SEEDG-01 upgrades to boundary steering
- AskBroker (12-01) — parked asks ride the suspension pattern
- Shaper's user-role system-content mapping (corpus census #5/#6) — D-01's marker form

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- Checkpoint store + Checkpointer seam (14-01): snapshots exist per turn; /undo and restore guards extend, not replace.
- Turn serialization (sessionTurnMu): the queue-behind behavior becomes the boundary-drain discipline's substrate.
- AskBroker suspension + reply routing: parked asks are queued asks with delivery positions.
- Cancel contract (v1.1 ACP + 18-D-12): D-12's auto-cancel composes existing pieces.

### Established Patterns
- Graceful degradation + loud notes — parked-ask notes, steering-applied notes, GC sweep logs.
- Grace-period GC family — checkpoint expiry joins 18-D-09's tombstone GC shape.
- Transcript-as-truth — steering deliveries and parked-ask records reconstruct on replay.

### Integration Points
- Model-request boundary: the seam between tool-loop iterations where steering drains (planner locates it in the runner).
- session/update note machinery — parked + applied notes.
- configOptions registration — checkpoint GC keys.
- /undo joins Phase 20's class-B command table.
- `.git/info/exclude` append at store init (SEEDG-02 letter).

</code_context>

<specifics>
## Specific Ideas

Operator framing that shaped decisions:
- /undo auto-cancels rather than refusing — convenience over caution, with the pre-restore snapshot as the safety net.
- Age+count dual expiry over age-only — disk hygiene on both axes.
- Everything else endorsed as recommended.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 23-seed-gaps-close-out*
*Context gathered: 2026-08-27*
