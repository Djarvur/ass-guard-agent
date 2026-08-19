# Compaction decision (EARLY-02) — evidence from the zcode rollout corpus

Decision artifact for requirement EARLY-02 (Phase 14, plan 14-02). Verify-first by
design: every row below is mechanical corpus evidence produced by the committed
scanner (`internal/profile/corpus_scan.go`, commit `7931d30`), never an
anticipation of the expected shape. No implementation happened in this plan —
every implementation need is routed (section 3).

**Bottom line:** the analyzed zcode wire corpus shows NO auto-compaction and NO
context eviction — no compact-continuation headers, no history shrinkage, no
window resets; the target manages context with a rolling last-64-message request
window plus `cache_control` breakpoints on every system block. ass-guard's
mimicry already delivers the window behavior (Projector 64-tail, no mid-turn
resets); it does NOT emit `cache_control` — that is the one routed gap.

## 1. Provenance

- **Scan date:** 2026-08-19 ~18:16 UTC (snapshot pinned below).
- **Scan commit:** `7931d30` (`feat(14-02): context-behavior corpus scan + fixtures`);
  scanner API `profile.ScanContextBehavior(io.Reader) (ContextBehaviorReport, error)`.
- **Pinned-session statement:** the Phase-9 coverage-pinned session
  **`sess_3cee56ae`-cc6a-43a6-8f00-a08eb266e1aa** (profiles/zcode/coverage.yaml) has
  rotated OFF `~/.zcode/cli/rollout/` (first verified 2026-08-19 at planning; still
  absent at execution). The fallback ladder's step 1 is therefore unavailable.
- **Ladder step used: step 2** — the main rollout session present at execution
  time. Planning-time check had found two larger main sessions (18MB/22MB,
  `model-io-sess_78b86dd2…` / `model-io-sess_888d29b8…`); those ALSO rotated off
  between planning and execution. The dir at execution time holds the live
  2026-08-19 wave sessions only.
- **Self-referential corpus note (honesty):** the analyzed sessions are the
  operator's active zcode sessions from today's work — including the sessions
  running this very phase. They are genuine zcode wire captures (the scan is
  content-agnostic); the counts are a snapshot of live-growing files, frozen by
  copying to `/tmp/14-02-corpus-snapshot/` immediately before scanning.
- **zcode version:** the sessions' own records carry `x-zcode-app-version: 3.8.1`
  (request headers). The coverage manifest's `0.16.3` is the `zcode --version`
  of the ROTATED pin era — cited here only as the manifest value; the analyzed
  corpus's header-carried version (3.8.1) is the one the census describes.
  Caveat: header-carried app version is not `zcode --version` output; both are
  recorded, neither was independently re-run.

Analyzed files (byte sizes are the scanned snapshot; live files kept growing):

| File (under `~/.zcode/cli/rollout/`) | Bytes | mtime (2026-08-19) | Role | Records |
|---|---|---|---|---|
| `model-io-sess_59ccd071-da6c-4ca2-abc5-f5dc4b80ffe3.jsonl` | 1,031,031 | 21:01:47 | main (`querySource=main_turn`) | 2 |
| `model-io-sess_subagent_agent_bdcb51f9-9350-490d-b995-8c8fdb652f7b.jsonl` | 40,360,206 | 21:01:02 | subagent | 164 |
| `model-io-sess_subagent_agent_e1f59824-0491-4ba6-b758-17f633d8373a.jsonl` | 19,328,245 | ~21:15 (snapshot) | subagent | 62 |

Per the plan, the PRIMARY corpus is the main session; the two subagent sessions
are analyzed separately (section 2b) because today's main session is young (2
requests — the conversation has not yet exceeded 64 messages), so the tail-window
form is only provable from the subagent sessions plus the 08-09 prior art.
No `model-io-no-session` files existed.

## 2. Census

Every context-management wire form the scan counted. Known forms absent from the
corpus appear as explicit `not observed in analyzed corpus` rows — the
completeness guarantee: no shape class is silently unexamined.

### 2a. Primary corpus — main session `sess_59ccd071`

| Form | Count | Detail |
|---|---|---|
| `cache_control` occurrences | 6 | `system:index=0/1/2` × 2 requests — every `request.body.system` block carries it |
| `cache_control` value form | all `{"type":"ephemeral"}` | no other value observed |
| `cache_control` on tools decls | 0 | not observed in analyzed corpus |
| `cache_control` on message content blocks | 0 | not observed in analyzed corpus |
| `cache_control` at other nestings (other bucket) | 0 | not observed in analyzed corpus |
| compact-continuation-header | 0 | not observed in analyzed corpus |
| system-reminder (user-carried `<system-reminder>` wrapper) | 4 | context-INJECTION form (agentsMd/env delivery), not compaction |
| inline-system-message (role=system inside request.messages) | 10 | context stack (agentsMd, role, notes, env, skills), 5/request |
| messagesKind census | full=2 | conversation below the 64-message window threshold |
| request message-count distribution | 45×1, 47×1 | growing full-history requests |
| MonotonicOffsets / ResetObserved | true / false | no window regression, no history shrink |

### 2b. Separate analysis — subagent sessions `bdcb51f9` + `e1f59824`

| Form | bdcb51f9 | e1f59824 |
|---|---|---|
| `cache_control` occurrences | 656 (`system:index=0..3` × 164) | 248 (`system:index=0..3` × 62) |
| `cache_control` non-system placements | 0 (not observed in analyzed corpus) | 0 (not observed in analyzed corpus) |
| compact-continuation-header | 0 (not observed in analyzed corpus) | 0 (not observed in analyzed corpus) |
| system-reminder | 24 | 28 |
| inline-system-message | 115 | 135 |
| messagesKind census | full=22, delta=1, tail=141 | full=26, delta=1, tail=35 |
| message-count distribution | full-growing 5→62 (one each), then **64×142** | full-growing 3→62 (one each), then **64×36** |
| MonotonicOffsets / ResetObserved | true / false | true / false |

The window pattern across ALL 176 tail records: `messagesKind=full` requests grow
the whole history until it reaches 64 messages, then `messagesKind=tail` requests
hold a rolling **last-64-message** window with `messageOffset` advancing
(2, 4, 6, …) as `messageCount` grows — continuation, never reset. This is the same
form as the 08-09 prior art (session 4440f5a7: 46/46 tail records at len 64,
advancing offsets, zero mid-turn resets). `messagesKind`/`messageOffset`/
`messageCount` are request-level bookkeeping (the rollout stores the normalized
messages array OUTSIDE `request.body`); the wire-observable fact is the message
array itself: full history up to 64, then the rolling 64 tail.

### 2c. Corpus-wide "not observed in analyzed corpus" completeness rows

| Requirement-named form | Status |
|---|---|
| Auto-compact marker (a message announcing context compression) | not observed in analyzed corpus (0 across 228 records) |
| Explicit eviction marker (system notice of dropped context) | not observed in analyzed corpus (ResetObserved=false in all three sessions; zero messageCount regressions) |
| Cross-turn window span | not observable in analyzed corpus — all three analyzed sessions are single-turn (`turnId` constant per file); the 08-09 prior art recorded the span; re-check rides the 12-05 re-record |

### Reproducing the census

From the repo root, with any dir inside the module (the scanner is an
`internal/` package — the driver must live in-module), e.g. `.tmp-scan/main.go`:

```go
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
)

func main() {
	for _, path := range os.Args[1:] {
		f, err := os.Open(path)
		if err != nil { fmt.Fprintln(os.Stderr, err); continue }
		rep, err := profile.ScanContextBehavior(f)
		_ = f.Close()
		if err != nil { fmt.Fprintln(os.Stderr, err); continue }
		out, _ := json.MarshalIndent(rep, "", "  ")
		fmt.Printf("=== %s ===\n%s\n", path, out)
	}
}
```

```sh
go run ./.tmp-scan ~/.zcode/cli/rollout/model-io-sess_<id>.jsonl [...]
```

The committed fixtures (`internal/profile/testdata/context-behavior/*.jsonl`)
pin the same forms redacted; `go test ./internal/profile/... -run
TestScanContextBehavior -count=1` proves the classifier. Live-dir re-runs yield
the same SHAPES with counts that drift as sessions grow (see Provenance).

## 3. Dispositions

One row per behavior, exactly one disposition each.

| # | Behavior (evidence) | Disposition | Mechanism / route |
|---|---|---|---|
| 1 | `cache_control {"type":"ephemeral"}` on EVERY system block (910/910 placements across 228 records; 3 blocks main-role, 4 subagent-role; other bucket empty) | **gap** | Routed requirement (post-adoption queue): *"emit cache_control breakpoints where the target does — every request.body.system block carries `{"type":"ephemeral"}`; scope: profile TextBlock gains the captured value (TIER-1), shaper maps it onto `anthropic.TextBlockParam.CacheControl`"* — see Consequences. The in-phase consumer is **14-04 (EARLY-03 cache probe)**, which uses this census's placement classes as its expected-placement baseline; EMISSION is not in 14-04's scope (probe-only, verify-first) and is routed above. |
| 2 | Rolling last-64-message request window (176/176 tail records at len 64; full-history growth before that) | **already-delivered-by-mimicry** | `internal/session/projector.go`: `MidTurnWindowMessages = 64` mid-turn tail with pair-safety bounding (`boundMidTurn`) — pinned to the captured form 08-09. |
| 3 | No mid-turn resets on tool results; window bookkeeping only ever advances (MonotonicOffsets=true, zero regressions, all sessions) | **already-delivered-by-mimicry** | Projector within-turn accumulation survives boundary lines (SESS-04 revised 08-09); the 08-09 re-scope was corpus-grounded on exactly this absence. |
| 4 | Cross-turn window span — the target's 64-tail carries prior turns' exchanges into later turns (08-09 prior art; not re-observable today, single-turn corpus) | **gap** | Routed requirement (post-adoption queue): *"carry prior turns' tail into the next turn's request window if the 12-05 multi-turn re-record confirms the span; ass-guard's D-01/D-02 between-turn lean seed deliberately resets — the known, documented divergence (STATE: routed to 12-05/AUD-05 since 08-09)"*. |
| 5 | Inline role=system context-stack messages in request.messages (260 occurrences; agentsMd/role/notes/env/skills stack) | **already-delivered-by-mimicry** | `internal/shaper/shaper.go` `toMessageParamRole("system")` maps mid-conversation system content to the wire convention (user-role text block) — the claude-code-compat convention pinned by the 08-07 fixture; the stack's content is runtime-composed per session (analogous to `CaptureWorkDir` substitution), not profile content. |
| 6 | `<system-reminder>` context injections (56 occurrences) | **already-delivered-by-mimicry** | Structure level: a user-role text message — the shaper's ordinary user-message emission. The reminder content is target-side runtime state (context injection), not a static wire form; no compaction semantics ride it in this corpus. |
| 7 | Auto-compact marker | **absent-in-target** | not observed in analyzed corpus (0 across 228 records) — see the honesty clause in Consequences; absence is a property of THIS corpus, not a forever-claim. |
| 8 | Explicit eviction marker | **absent-in-target** | not observed in analyzed corpus (ResetObserved=false everywhere; zero messageCount regressions) — same honesty clause. |

## 4. Consequences

1. **Nothing is built in-phase (EARLY-02 verify-first honored).** The only
   production code committed by 14-02 is the scanner itself; `git diff` over
   `internal/session/` and `internal/shaper/` for this plan is empty.
2. **12-05 re-record must re-check (corpus-absent honesty clause):**
   - auto-compact markers and eviction markers under a LONG multi-turn session
     (today's corpus is single-turn per session — compaction forms, if the target
     ever emits them, surface in long-lived sessions, not young ones);
   - the cross-turn window span (disposition #4's open evidence question);
   - `cache_control` placement stability across zcode versions (analyzed corpus:
     3.8.1 header-carried; manifest pin era: 0.16.3 — same placement classes,
     but the pin-era observation was line-count-based, not placement-classed).
3. **14-04 (EARLY-03) consumes this census:** its cache probe's expected
   placement = system blocks only, every block, value `{"type":"ephemeral"}`
     (disposition #1's evidence column is the probe's baseline).
4. **12-05/12-08 (re-record + eval net)** inherit the reproduction driver shape
   above for re-running the census mechanically on any future capture.

*Artifact: docs/compaction-decision.md — plan 14-02, requirement EARLY-02, phase
14 (adoption-readiness-analysis-dispositions), milestone v1.1 ACP Early Adoption.*
