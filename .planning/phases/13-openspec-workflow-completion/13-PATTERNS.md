# Phase 13: OpenSpec Workflow Completion - Pattern Map

**Mapped:** 2026-08-19
**Files analyzed:** 12 (new + modified)
**Analogs found:** 9 / 12 (3 land on planned-but-not-yet-executed or genuinely new ground — see "No Analog Found")

> **CRITICAL GROUND-TRUTH FINDING (verified 2026-08-19, read the planner note):** the 6
> Phase-13 commands — `/opsx:new /opsx:continue /opsx:ff /opsx:verify /opsx:bulk-archive
> /opsx:onboard` — are **NOT installed by the pinned openspec v1.5.0 default install**.
> `openspec init --tools claude --force` (probed into `/tmp/opsx-probe-13`) installs only
> 5 commands: `explore / propose / apply / archive / sync` (matching the committed fixture
> `internal/ecosys/testdata/opsx-real/.claude/commands/opsx/`). The 6 new commands are the
> **opt-in expanded profile** (`.planning/research/FEATURES.md` line 41: "Expanded profile
> (opt-in via `openspec config profile`)"). The installed binary exposes
> `openspec config profile [preset]` — and config is **global-scope only**
> (`~/.config/openspec/config.json`; current machine state: `profile: core`). Consequences
> the planner MUST carry: (1) every Phase-13 E2E bootstrap enables the expanded profile
> BEFORE `openspec init`, and (2) because the switch is global, the harness must
> save/restore the operator's `~/.config/openspec/config.json` around runs — no existing
> harness code mutates global config (no analog). The preset name is to be probed
> (`openspec config profile --help` accepts a preset shortcut; FEATURES.md names the
> profile concept). The Phase-8 `newOpsxRunner` bootstrap cannot be cloned unchanged.

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `internal/openspec/seeded.toml` (new rows) | config | event-driven (chaining seeds) | the file's own 08-06 rows, lines 48-100 | exact |
| `internal/engine/decide.go` (advisory decision class) | service | event-driven | `askSuspendedDecision` in same file, lines 6-17 | exact |
| `internal/engine/types.go` (advisory Signal/Decision vocab) | model | event-driven | `SignalAskSuspended` + `Decision` struct, lines 101-173 | exact |
| `internal/engine/observe.go` (advisory emit, dedupe gate) | service | event-driven | `applyDispatcher` ask branch + `emit`, lines 255-334 | exact |
| `internal/engine/advisory_test.go` (NEW) | test | event-driven | `internal/engine/chain_provenance_test.go` (fake-table battery) | exact |
| `cmd/ass-guard/acp_serve.go` (advisory session/update note, post-turn) | controller adapter | request-response | ask-broker `onSurface` bus publish, lines 971-985 | role-match |
| `cmd/ass-guard/e2e_opsx_matrix_test.go` (NEW; or extend `e2e_opsx_test.go`) | test | batch (12-leg matrix, 2 passes) | `TestOpsxEndToEnd_Gated` + `TestOpsxFixableRecovery_Gated` | exact |
| `cmd/ass-guard/testdata/opsx-e2e/` (new per-command captures) | testdata | file-I/O | `stage-<n>-output{-capture,}.txt` + `captureStageOutputs` | exact |
| `internal/evalsuite/scenarios/opsx-{new,continue,ff,verify,bulk-archive,onboard}.json` (NEW, 6) | config (eval scenario) | batch | 12-08-PLAN's `opsx-flagship.json` target shape | planned-not-on-disk |
| `.mise.toml` (only if D-06 needs task wiring) | config | batch | 12-08-PLAN Task 2 mise task surface | planned-not-on-disk |
| Question-shaped-ending classifier (new engine file) | utility | transform | `compiledPattern` regex rows, `internal/openspec/patterntable.go` lines 13-18 | partial (regex form only) |
| Advisory dedupe state (per session + pattern class) | state | event-driven | — none; Engine is stateless by design (`observe.go` line 76) | no analog |

Likely **unmodified**: `internal/openspec/patterntable.go` + `config.go` — the existing
`[[patterns]]` / `[[command_patterns]]` / `[command_mutability]` row forms already cover
D-07's seeded handoffs (13-CONTEXT discretion item says "existing forms in
`internal/openspec/seeded.toml`"). Only extend them if the expanded-profile surface
demands new CLI `[commands]` rows (re-probe first — see seeded.toml lines 43-46 for the
re-probe procedure).

## Pattern Assignments

### `internal/openspec/seeded.toml` (config, chaining seeds for the 6 commands + verify→fix)

**Analog:** the same file's 08-06 re-seeded block — the row forms AND the mandatory
capture-provenance comment discipline.

**Capture-provenance comment + text row form** (lines 48-77 — copy this structure for the
expanded-matrix rows; D-03 pass-1 harvest output is the new source):

```toml
# ---- 08-06 T3 chaining rows — RE-SEEDED FROM THE REAL CAPTURE (D-12) ----
# Source: cmd/ass-guard/testdata/opsx-e2e/stage-{1,2,3}-output-capture.txt
# Capture run: 2026-08-15T13:29Z, gated E2E capture mode (real openspec 1.5.0
# binary + real GLM-5.2 via the Z.ai Anthropic endpoint), full
# explore→propose→apply→archive scenario, 505s. [...]

[[patterns]]
id = "post-propose-handoff"
regex = "/opsx:apply"
action = "continue"
next = "/opsx:apply"
```

**Command-provenance row form** (lines 96-100 — the deterministic fallback for free-form
closings; D-07's verify→fix handoff and any question-shaped matrix closings chain here):

```toml
[[command_patterns]]
id = "post-explore-handoff"
command = "opsx:explore"
action = "continue"
next = "/opsx:propose"
```

**Terminal-shield convention** (lines 62-65) — legit terminus rows use `action = "wait"`
(no injection); D-07's note that verify's own report-ending "is a legitimate turn end"
lands as either a wait-row or the absence of a row:

```toml
[[patterns]]
id = "post-archive-terminal"
regex = "(?i)archive complete"
action = "wait"
```

**Mutability rows** (lines 260-266) — the D-11 discretion item adds the 6 new keys here
(onboard likely `"mutating"`; boundary opens AT EXPANSION via
`expandUserBlocks` → `AppendBoundary(mutatingCommandCause+key, ...)`):

```toml
[command_mutability]
"opsx:explore" = "read-only"
"opsx:propose" = "mutating"
```

**Seed-row rules that bind (from 08-06/13-CONTEXT):** regexes come from CAPTURED
stage-end output only, never command-file prose (D-12); row order among text rows is
first-match-wins with terminal shields leading; duplicate `command_patterns` keys: LAST
row wins (`patterntable.go` lines 96-99); `next` empty ⇒ generic "continue" prompt.

---

### `internal/engine/decide.go` + `types.go` + `observe.go` (service, the D-02/D-05 advisory)

**Analog:** `askSuspendedDecision` — the most recent decision-class addition, added the
same way (12-01): a small pure predicate + vocabulary constant + dispatch branch, with
the safety contract stated in the doc comment.

**Decision-class pattern** (`decide.go` lines 6-17 — the advisory predicate follows this
shape; NOTE the advisory fires on the UNMATCHED path and NEVER mutates Action away from
`ActionNothing`):

```go
func askSuspendedDecision(out TurnOutput) (Decision, bool) {
	if !out.AskSuspended {
		return Decision{}, false
	}
	return Decision{
		TurnID: out.TurnID,
		Action: ActionAsk,
		Signal: SignalAskSuspended,
		Reason: "turn suspended on AskUserQuestion — surface the ask, stop the loop (never chain)",
	}, true
}
```

**Where the advisory hooks Decide** (`decide.go` lines 127-132 — the unmatched cell is
the advisory's trigger point; the returned Action stays `ActionNothing`):

```go
	return Decision{
		TurnID: out.TurnID,
		Action: ActionNothing,
		Signal: "unmatched",
		Reason: "no pattern or handoff tool matched",
	}
```

**Signal vocabulary pattern** (`types.go` lines 169-173 — add the advisory signal the
same way; per-class identity rides the signal string, e.g. `advisory:<class>`):

```go
// SignalAskSuspended is the Decision.Signal for an ask-suspended turn
// (12-01, ACP-01): ...
const SignalAskSuspended = "ask:suspended"
```

**Audit emit — the observability rail the advisory rides** (`observe.go` lines 315-334;
every decision including Nothing is emitted, so the advisory's audit line + repeat counts
land through `emit`; dedupe gates ONLY the client-visible note, never the audit):

```go
func (e *Engine) emit(dec *Decision) {
	if e.Bus != nil {
		e.Bus.Publish(event.EngineDecision{
			TurnID: dec.TurnID, Action: dec.Action.String(), Signal: dec.Signal,
			MatchedSpan: dec.MatchedSpan, ConfigSource: dec.ConfigSource, Reason: dec.Reason,
		})
	}
	if e.Manager != nil {
		werr := e.Manager.AppendEngineDecision(dec.TurnID, dec.Action.String(), dec.Signal,
			dec.MatchedSpan, dec.ConfigSource, dec.Reason)
		// ... failure logged, never blocks the loop
	}
}
```

**Transcript-line shape** (`internal/session/manager.go` lines 259-272 — the audit side;
no Manager change needed, `AppendEngineDecision` already carries signal + reason):

```go
func (m *Manager) AppendEngineDecision(
	turnID, action, signal, matchedSpan, configSource, reason string,
) error {
	return m.appendLine(&Line{
		Type: TypeEngineDecision, TurnID: turnID, Timestamp: now(),
		Name: action, Input: json.RawMessage(`"` + signal + `"`),
		MatchedSpan: matchedSpan, ConfigSource: configSource, Text: reason,
	})
}
```

**Design tension the planner must resolve:** `Engine` is deliberately stateless
(`observe.go` line 76: "The engine holds no per-turn mutable state"), but D-05's dedupe
(per session + pattern class, first-per-class client-visible, repeats audit-only) needs
mutable state. Options with precedent: a field on the ACP-side wrapper
(`sessionTurnRunner` already holds per-process state like `reg`/`patternTable`), or a
small dedupe struct passed to Observe. There is NO existing per-session dedupe analog.

---

### `cmd/ass-guard/acp_serve.go` (controller adapter, D-05 client-visible note)

**Analog:** the ask-broker surface callback (12-01) — the most recent "engine-side event
becomes a client-visible agent-message-style session/update" wiring.

**Bus-publish surface pattern** (lines 971-985 — the advisory's note reuses the same
event kind + renderer seam; wording is capture-informed per 13-CONTEXT discretion):

```go
	askBroker := session.NewAskBroker(r.askTimeout, func(p session.PendingAsk) {
		r.bus.Publish(event.AgentMessageChunk{
			TurnID: p.TurnID, MessageID: p.TurnID,
			Content: coreexec.RenderAskSurface(p.Questions),
		})
	})
```

**Wire shape** (`internal/acp/server.go` lines 304-318 — `agent_message_chunk`
session/update; this is the "agent-message-style" D-05 names):

```go
func (a *adapter) AgentMessageChunk(messageID, text string) error {
	update := map[string]any{
		"sessionUpdate": "agent_message_chunk",
		"messageId":     messageID,
		"content":       ContentBlock{Type: blockText, Text: text},
	}
	// ... written as a session/update notification (no id)
}
```

**Timing hazard (load-bearing):** the ask surface works because Run's chunk forwarder
(`acp_serve.go` lines 707-714, `startChunkForwarder` lines 767-803) is subscribed DURING
the turn — but D-05's advisory fires AFTER the turn ends, when the forwarder has
unsubscribed. The emission point must be after `runOneTurn`/`Observe` returns where the
ACP `emit ChunkEmitter` is still in hand (`Run`, lines 697-744), or a dedicated post-turn
publish+emit. Do not reuse the mid-turn forwarder blindly.

**Forbidden surfaces (D-05, deferred list):** never an ACP user-message
(transcript/session-load replay fidelity, 08 D-02); never audit-log-only. The advisory
must read as routing help, never as the chain being held open — the Phase-8
question-ending auto-continue REJECTION is untouched (no code exists for it; it is a
decision invariant — see 13-CONTEXT canonical_refs).

---

### `cmd/ass-guard/e2e_opsx_matrix_test.go` (test, D-01 12-leg matrix + D-03 harvest-then-verify)

**Analog:** `cmd/ass-guard/e2e_opsx_test.go` — the proven gated harness, wholesale.

**Double env gate + FAIL LOUD** (lines 53-67):

```go
func e2eGates(t *testing.T) {
	t.Helper()
	if os.Getenv("ASSGUARD_OPENSPEC_BIN") != "1" || os.Getenv("ASSGUARD_E2E_LLM") != "1" {
		t.Skip("set ASSGUARD_OPENSPEC_BIN=1 (real openspec binary on PATH) AND " +
			"ASSGUARD_E2E_LLM=1 (live model; creds from the repo's " +
			".ass-guard/config.yaml) to run the real /opsx E2E")
	}
	_, lerr := exec.LookPath("openspec")
	if lerr != nil {
		t.Fatalf("BLOCKER: ASSGUARD_OPENSPEC_BIN=1 but no openspec binary on PATH: %v", lerr)
	}
}
```

**Scratch bootstrap with the REAL binary** (lines 112-164 — `newOpsxRunner`; the Phase-13
clone INSERTS the expanded-profile enable + global-config save/restore before
`openspec init`, the one place it cannot be copied verbatim):

```go
	scratch := t.TempDir()
	initCmd := exec.CommandContext(context.Background(), "openspec", "init", "--tools", "claude", "--force")
	initCmd.Dir = scratch
	out, err := initCmd.CombinedOutput()
	// ... then seedScratchCodebase, load real profile, wire sessionTurnRunner
	// (bus, profile, workDir, maxConc, makeProvider), r.setupEngine(),
	// r.loadCommandRegistry()
```

**Capture mode → seeds → proof mode** (lines 262-290 — D-03's two-pass procedure extends
exactly this: pass 1 unseeded matrix capture, derive seeds, pass 2 zero-continue):

```go
	if os.Getenv(e2eCaptureStage) == "1" {
		// CAPTURE MODE (pre-re-seed): drive the four stages manually so each
		// stage's real output lands in testdata; the chaining patterns are
		// seeded FROM THIS capture (D-12) ...
		for _, cmd := range []string{ /* the 6 commands, happy legs */ } {
			runStageTyped(t, r, sessionID, cmd)
		}
		texts := stageAssistantTexts(t, r, sessionID)
		captureStageOutputs(t, texts[:e2eStageCount], "-capture")
		return
	}
	// PRODUCT-PROOF MODE: ONE typed prompt; the engine chains the rest.
```

**Zero-continue + provenance assertions** (lines 293-340 — the gate evidence per happy
leg: engine_decision continue counts + command_provenance keys for every chained stage):

```go
	for i := range lines {
		if lines[i].Type == session.TypeEngineDecision {
			actions = append(actions, lines[i].Name)
		}
		if lines[i].Type == session.TypeCommandProvenance {
			provenance = append(provenance, lines[i].Name)
		}
	}
	// continues >= stages-1; every chained key present in provenance
```

**Fixable-leg pattern** (lines 369-417 — `seedIncompleteChange` (deterministic
fixable trigger) + `scanFixableRecovery` (fail-then-recover scan over tool results) +
the real-artifact assertion (archive dir exists); each command's fixable leg clones this
triple with a command-appropriate trigger — onboard's is D-08's idempotent re-run):

```go
func scanFixableRecovery(lines []session.Line) (int, int) {
	// first fixable-failure index + first recovery AFTER it (-1 when absent)
	// signatures: "force closed the prompt" || "archive_tasks_incomplete";
	// recovery: a result containing "archived"
```

**Execution-layer classification leg** (lines 504-538 — direct tool invocation asserting
the structured `{classification: "fixable", exit_code: non-zero}` result; reuse for any
new fixable-classified `[commands]` rows).

**D-06 flake policy precedent** (evidence class from
`.planning/phases/08-slash-command-kickoff/08-08-PLAN.md` Task 4 + 08-08-SUMMARY): both
runs recorded, re-run allowed ONCE with zero source/config commits since the failure,
preserve transcripts "to /tmp before t.TempDir reaps them". No best-of-N.

**Scenario-subject forwarding (chaining context)** (`acp_serve.go` lines 1314-1328): a
BARE injected command inherits the first invocation's args — the 6 new chained commands
rely on this already; no change needed, but harvest legs must type the subject on the
first invocation.

---

### `internal/engine/advisory_test.go` (NEW test battery)

**Analog:** `internal/engine/chain_provenance_test.go` — the fake-table battery shape.

**Fake-table + pure-Decide test pattern** (lines 30-45, 58-87 — copy: tiny in-memory
fake, `t.Parallel()`, assert Action/Signal/Span/TurnID/ConfigSource):

```go
type provenanceTable struct{}
func (provenanceTable) MatchText(string) engine.MatchDetail { return engine.MatchDetail{} }
func (provenanceTable) MatchTool(string) engine.MatchDetail { return engine.MatchDetail{} }
func (provenanceTable) MatchCommand(key string) engine.MatchDetail { ... }

func TestDecide_CommandProvenanceSignal(t *testing.T) {
	t.Parallel()
	out := engine.TurnOutput{TurnID: "t1", StartedBy: exploreKey, Text: "..."}
	dec := engine.Decide(out, provenanceTable{})
	// assert Action, Signal, MatchedSpan, TurnID, ConfigSource
}
```

Required battery contents (from D-02/D-05): advisory fires ONLY on unmatched
question-shaped endings; never converts Action away from Nothing (the no-hold invariant);
does NOT fire on verify's matched-seed report-ending (D-07 note); dedupe first-per-class
client-visible + repeat audit-only; ACP-level wiring test in
`cmd/ass-guard/ask_wiring_test.go` style (`TestAskWiring_*`, lines 68-418) for the
session/update note + replay fidelity (no user-message line in the transcript).

---

### `internal/evalsuite/scenarios/opsx-*.json` (6 NEW eval suites, D-04)

**Analog:** planned only — `.planning/phases/12-product-functional-completeness/12-08-PLAN.md`
(the net: `internal/evalharness` + `internal/evalsuite` + `opsx-flagship.json`). **12-08
is NOT executed** (no SUMMARY; no `internal/evalsuite/` on disk; no eval tasks in
`.mise.toml`). Phase 13 hard-depends on it landing first (ROADMAP: "Depends on: Phase 12
and Phase 8"). The 12-08 contract Phase 13 extends: "adding a scenario is adding a JSON
scenario file — no harness change"; scenarios carry prompts, expected stage progressions,
named assertion keys (assertions are named KEYS resolved by the harness — free-form
assertions are forbidden, T-12-08-01); pass@k=1 in the gate behind
`ASSGUARD_EVAL_GATE=1`; result artifacts under `.ass-guard/eval/`. Note 12-08-PLAN says
the E2E tests move to `internal/runtime/` — the move is v1.2-deferred (STATE.md re-scope:
"the `internal/runtime` refactor move to v1.2"); today they live in `cmd/ass-guard/`.
Planner should treat file locations as current-disk truth, plan-doc shapes as the schema
source.

## Shared Patterns

### Structural safety (applies to every seed row + the advisory)
**Source:** `internal/engine/decide.go` lines 79-133 + `acp_serve.go` lines 1303-1331.
Assistant-role-only matching; `StartedBy` sourced EXCLUSIVELY from the prompt-side
invocation (`invocationFor`, never assistant/tool content); unmatched ⇒ nothing. The
advisory MUST NOT hold continuation (D-02: "the advisory NEVER holds continuation").

### Gates + FAIL LOUD (applies to all E2E/eval legs)
**Source:** `e2e_opsx_test.go` lines 53-67, 72-98. Double env gate; missing binary/creds
are `t.Fatalf("BLOCKER: ...")`, never silent skips; scratch roots via `t.TempDir()` +
`findRepoRoot` for creds.

### Audit-log-every-decision (applies to the advisory + all matrix legs)
**Source:** `observe.go` `emit` (lines 315-334) + `manager.go` `AppendEngineDecision`
(lines 259-272). The phase gate requires "chaining decisions evidenced in the audit
trail" (ROADMAP Phase-13 gate) — matrix legs assert on `TypeEngineDecision` +
`TypeCommandProvenance` transcript lines exactly as `e2e_opsx_test.go` lines 293-340 do.

### Evidence preservation (applies to D-03 pass 1 + D-06 re-runs)
**Source:** 08-08-PLAN Task 4 — poll scratch session transcripts to `/tmp` before
`t.TempDir()` reaps; both runs recorded; re-run ONCE only at zero source/config delta.

### Captured-output provenance comments (applies to every new seed row + testdata capture)
**Source:** `seeded.toml` lines 48-58 + `captureStageOutputs` (`e2e_opsx_test.go` lines
212-235): every capture file/row carries a header naming the run, date, binary + model.

## No Analog Found

| File / Mechanism | Role | Data Flow | Reason + Planner Guidance |
|------------------|------|-----------|---------------------------|
| Expanded-profile bootstrap (global config enable + save/restore) | test infra | file-I/O | No harness code mutates global config; `openspec config` is global-scope ONLY (`~/.config/openspec/config.json`, currently `profile: core`). The 6 commands do not exist on the default install (verified: `init --tools claude` installs explore/propose/apply/archive/sync only). Bootstrap must probe the preset name, enable it, and restore the operator's config afterward. |
| Question-shaped-ending classifier | utility | transform | No existing detector (the Phase-8 question-ending handling is a decision invariant, not code). Closest regex-class form: `compiledPattern` (`patterntable.go` lines 13-18). Per-class identity feeds the dedupe key + advisory signal. |
| Advisory dedupe state (per session + class) | state | event-driven | `Engine` is stateless by design (`observe.go` line 76); no per-session dedupe state exists anywhere. Needs a new home (ACP-side wrapper field or passed-in struct) — planner decides. |
| The 6 eval suite JSONs + any `.mise.toml` additions | config (eval) | batch | `internal/evalsuite/` does not exist yet — 12-08 planned but NOT executed. Hard dependency; schema comes from 12-08-PLAN (named assertion keys, pass@k=1, `ASSGUARD_EVAL_GATE=1`). |

## Metadata

**Analog search scope:** `internal/engine`, `internal/openspec`, `internal/session`,
`internal/ecosys`, `internal/acp`, `cmd/ass-guard` (incl. `testdata/opsx-e2e`),
`.planning/phases/{08,12}-*/`, `.planning/{REQUIREMENTS,ROADMAP,STATE}.md`,
`.planning/research/FEATURES.md`; live probe of the installed openspec v1.5.0 binary
(`/tmp/opsx-probe-13` scratch: command set + `config profile` surface + global config
state — read-only except the /tmp scratch).
**Files scanned:** ~25 source/plan/doc files + live binary probe
**Pattern extraction date:** 2026-08-19
