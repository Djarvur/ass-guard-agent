---
phase: 00-spike-re-verification
plan: 04
subsystem: infra
tags: [go, go-telegram, telegram, acp, stdio, stdout-collision, transport-discipline, goroutine, context-context, spikes, throwaway, redaction, sanitization]

# Dependency graph
requires:
  - phase: 00-spike-re-verification
    provides: "spikes module skeleton (github.com/djarvur/ass-guard-spikes, go 1.25, go-telegram/bot@v1.23.0 + go-openai@v1.42.0 pins) — Plan 00-01's output this spike `go run`s against"
provides:
  - "spikes/05-stdout-collision/main.go — throwaway Go spike proving go-telegram/bot@v1.23.0 (long-poll loop) and ACP stdio (newline-delimited JSON-RPC frames on stdout) coexist in ONE PROCESS with zero stdout collision (the ROADMAP's explicit integration test)"
  - "spikes/05-stdout-collision/main_test.go — RED-then-GREEN offline test suite asserting the framing rule + the clean-stdout byte-equality helper + the stderr-routing handler + the offline-disciplined bot construction"
  - "spikes/05-stdout-collision/RESULT.md — D-02-shaped item-#5 draft (Status: VERIFIED, Tier A favorable) for Plan 00-05 to fold into VERIFIED-FACTS.md; the silent-by-default + handler-routing recipe recorded for the Phase-5 Telegram frontend"
affects: [00-05, phase-1-mimicry-mvp, phase-5-ecosystem-compatibility, v2-telegram-peer]

# Actuals (#2632) — pairs with the plan's estimate (36000 tokens, 2 tasks, low confidence)
actuals:
  tokens: 12617   # chars/4 over the realized diff (50466 chars across 3 files)
  tasks: 2
  commits: 3       # RED test + GREEN impl + RESULT.md

# Tech tracking
tech-stack:
  added: []  # zero NEW deps — reuses the go-telegram/bot@v1.23.0 pin from Plan 00-01's spikes/go.mod
  patterns: ["multi-frontend stdout discipline proof: real os.Stdout captured via os.Pipe + concurrent go-telegram/bot goroutine + byte-equality assertion (captured == canned ACP frames, 0 extra bytes) — the empirical shape the ROADMAP's criterion #3 calls for", "go-telegram/bot@v1.23.0 is silent-by-default: zero os.Stdout/fmt.Print writes in library source; all output via 3 default handlers (defaultErrorsHandler/defaultDebugHandler/defaultHandler) all log.Printf→os.Stderr; debug path gated behind WithDebug(); no WithLogger, no io.Writer option — confirmed by direct source inspection + the spike's byte-clean assertion", "Tier-A doc refinement: go-telegram/bot@v1.23.0 callback signatures are ErrorsHandler func(err error) and DebugHandler func(format string, args ...any) — SIMPLER than RESEARCH.md §5 documented (no context.Context params); the load-bearing 'callbacks, no stdout, no io.Writer' claim is unchanged", "handler-routing mitigation recipe for ass-guard's v2 Telegram frontend: bot.New(token, WithErrorsHandler→slog→os.Stderr), NO WithDebug/WithDebugHandler, log.SetOutput(os.Stderr) belt-and-suspenders, go b.Start(ctx) in a goroutine", "context-first shutdown contract confirmed: bot goroutine exits within 2s of telegramCtx cancel (STACK's load-bearing reason for picking go-telegram/bot over gotgbot/telebot)", "D-03 redaction preserving JSON keys + method/field names + enum values + framing while replacing sessionId/messageId/text content + the dummy token"]

key-files:
  created:
    - spikes/05-stdout-collision/main_test.go
    - spikes/05-stdout-collision/main.go
    - spikes/05-stdout-collision/RESULT.md
  modified: []

key-decisions:
  - "Load-bearing library-behavior finding VERIFIED by direct source inspection: go-telegram/bot@v1.23.0 has ZERO os.Stdout/fmt.Print writes in its library source (only matches are in examples/ — separate sample programs, not the library). The only output surfaces are 3 default handlers in bot.go (defaultErrorsHandler/defaultDebugHandler/defaultHandler), all stdlib log.Printf → log.Default() → os.Stderr. No WithLogger, no io.Writer option. Debug path gated behind b.isDebug (set only by WithDebug()). This is exactly what RESEARCH.md §5 predicted; the spike confirms it empirically (assertion B) AND by source inspection (RESULT.md Evidence)."
  - "Tier-A doc refinement recorded, NOT a Tier-B halt. RESEARCH.md §5 documented the callback signatures as ErrorsHandler func(ctx, err) and DebugHandler func(ctx, message, err, data). The ACTUAL v1.23.0 signatures (bot.go:27-28) are simpler: ErrorsHandler func(err error) and DebugHandler func(format string, args ...any). Tier A (cosmetic doc drift — over-described signatures; the load-bearing 'no stdout, no io.Writer, callbacks-only' claim is unchanged). The spike uses the REAL signatures; Phase 5's Telegram frontend must too."
  - "Bot construction kept OFFLINE via WithSkipGetMe (skips the GetMe network call in New). Per critical_constraint #2, the spike must NOT require a real token or a live Telegram connection — the goroutine topology is the point. WithSkipGetMe lets New() succeed offline with the dummy token; Start() will still attempt getUpdates against api.telegram.org and fail — that error path is exactly what WithErrorsHandler routes to the stderr sink (assertion C determinizes it with a synthetic-marker probe that does not depend on network timing)."
  - "Handler-routing assertion grounded in DETERMINISTIC empirical proof, not structural reasoning. The spike captures the process's real os.Stdout via os.Pipe, runs the bot goroutine concurrently, writes 3 canned ACP frames (415 bytes), AND directly invokes the bot's configured WithErrorsHandler with a synthetic error carrying a unique marker — proving (a) the marker lands in the stderr-bound sink (132 bytes, present=true) and (b) is absent from the captured stdout. This makes assertion C observable behavior, not a closure-capture argument."
  - "No discrepancy vs STACK on the core claim. STACK item #5 ('integration test that both frontends never write a non-ACP frame to stdout') and the Version-Compatibility line ('Telegram frontend must never write to stdout') both hold for go-telegram/bot@v1.23.0 in-process. The transport discipline (stdout = ACP only) holds; ass-guard's Phase-5 Telegram frontend can proceed against this version."

patterns-established:
  - "TDD RED→GREEN for a stdout-collision spike: the offline-verifiable slice (framing rule + byte-equality helper + handler-routing closure + offline bot construction) is the testable behavior; the live 'bot goroutine + canned frames on captured real os.Stdout' assertion is proven by the binary itself via a structured stderr footer."
  - "Empirical stdout-discipline proof shape: capture os.Stdout via os.Pipe (not a TeeReader) so EVERY byte any goroutine writes to fd 1 is collected; write the canned ACP frames to the pipe's write-end; read back and byte-compare. The honest PASS requires captured > 0 AND captured == canonical AND extra-byte count == 0 (guards against a trivial 0==0 pass from an empty pipe)."
  - "Deterministic handler-routing probe: directly invoke the configured errors-handler with a synthetic-marker error WHILE stdout is captured; assert marker-in-stderr-sink AND marker-not-on-stdout. Avoids reliance on the library's network-dependent error path firing in a test window."
  - "D-03 redaction as a commit gate for committed evidence: redact sessionId/messageId/text → [REDACTED: id/content] tokens + the dummy token → [REDACTED: dummy token], preserving all JSON keys, method names, field names (agentCapabilities/protocolVersion/sessionUpdate/stopReason), enum values (end_turn/agent_message_chunk), and framing. 3-check self-grep CLEAN (no token literal, no real user paths, 7 D-02 fields)."

requirements-completed: [STACK-Phase0-#5]

# Coverage metadata (#1602) — the deliverable is the #5 finding, auto-proven by the
# passing offline test suite + the spike's VERIFIED footer with byte-clean evidence.
# No human judgment needed — the transport-discipline contract is a mechanical
# assertion (D-07 Tier A), fully asserted by automation. Routes to auto-pass
# (human_judgment: false).
coverage:
  - id: D1
    description: "STACK item #5 finding in spikes/05-stdout-collision/RESULT.md — go-telegram/bot@v1.23.0 + ACP stdout-collision integration recorded (D-02 seven fields, Status VERIFIED): library silent-by-default (zero stdout writes in source; all output via log.Printf→stderr handlers), WithDebug-gated debug path, no WithLogger/io.Writer option, Tier-A handler-signature refinement, the 4-step mitigation recipe for the v2 Telegram frontend, context-first shutdown confirmed."
    requirement: "STACK-Phase0-#5"
    verification:
      - kind: unit
        ref: "spikes/05-stdout-collision/main_test.go#TestWriteACPFrameProducesNewlineDelimitedJSON"
        status: pass
      - kind: unit
        ref: "spikes/05-stdout-collision/main_test.go#TestExtraByteCountZeroOnExactMatch"
        status: pass
      - kind: unit
        ref: "spikes/05-stdout-collision/main_test.go#TestExtraByteCountDetectsLeakedBytes"
        status: pass
      - kind: unit
        ref: "spikes/05-stdout-collision/main_test.go#TestCompareStdoutReportsCleanOnMatch"
        status: pass
      - kind: unit
        ref: "spikes/05-stdout-collision/main_test.go#TestStderrErrorsHandlerRoutesToSinkNotStdout"
        status: pass
      - kind: unit
        ref: "spikes/05-stdout-collision/main_test.go#TestBuildACPFramesProducesOrderedSequence"
        status: pass
      - kind: unit
        ref: "spikes/05-stdout-collision/main_test.go#TestBuildBotConfiguresDiscipline"
        status: pass
      - kind: integration
        ref: "spikes/05-stdout-collision/ — `go run ./05-stdout-collision/` structured footer: 4 assertions PASS (clean stdout 415==415 bytes 0 extra; default silence; handler routing marker-in-stderr-sink; shutdown within 2s), overall_status: VERIFIED; external stdout byte-empty (transport discipline)"
        status: pass
    human_judgment: false

# Metrics
duration: 10min
completed: 2026-08-09
status: complete
---

# Phase 0 Plan 04: #5 go-telegram/bot + ACP stdout-collision Integration Spike Summary

**Throwaway Go spike proves go-telegram/bot@v1.23.0 and ACP stdio coexist in ONE PROCESS with zero stdout collision — the library is silent-by-default (zero os.Stdout writes in source; all output via log.Printf→stderr handlers; no WithLogger/io.Writer option) — confirming the transport discipline (stdout = ACP only) holds for the multi-frontend case; VERIFIED with a captured-os.Stdout byte-equality assertion (415==415, 0 extra) + a deterministic handler-routing probe; Tier-A handler-signature refinement recorded.**

## Performance

- **Duration:** ~10 min
- **Started:** 2026-08-09T18:09:36Z
- **Completed:** 2026-08-09T18:19:05Z
- **Tasks:** 2
- **Files modified:** 3 (all created; 0 modified)

## Accomplishments

- Built the throwaway spike `spikes/05-stdout-collision/main.go` (package main, module `github.com/djarvur/ass-guard-spikes`, **zero new deps** — reuses the `go-telegram/bot@v1.23.0` pin from Plan 00-01's `spikes/go.mod`) that runs the two transports concurrently in ONE PROCESS: a `go-telegram/bot` long-poll loop in a goroutine (`go b.Start(telegramCtx)`) alongside canned ACP frames written to a captured real `os.Stdout`. All 4 assertions PASS; `overall_status: VERIFIED`; exit code 0.
- **Empirically proved the transport discipline holds** (the ROADMAP's explicit integration test, criterion #3): captured `os.Stdout` (415 bytes via `os.Pipe`) is byte-equal to the 3 canned ACP frames the spike wrote (initialize response, session/update notification, session/prompt response — newline-delimited JSON-RPC per Plan 00-03), with `stdout_extra_byte_count: 0`. The bot goroutine, while live, leaked ZERO bytes onto stdout.
- **Confirmed the load-bearing library-behavior claim by direct source inspection** of `go-telegram/bot@v1.23.0` in the module cache: a recursive grep for `os.Stdout`/`fmt.Print` across the package's non-test, non-`examples/` Go files returns **ZERO matches**. The only output surfaces are 3 default handlers in `bot.go` (`defaultErrorsHandler`/`defaultDebugHandler`/`defaultHandler`), all using stdlib `log.Printf` → `log.Default()` → `os.Stderr`. There is **no `WithLogger`, no `io.Writer` option, no unconditional stdout write** anywhere in the v1.23.0 API surface. The debug path is gated behind `b.isDebug` (set only by `WithDebug()`); without it, `defaultDebugHandler` never fires.
- **Determinized the handler-routing assertion** (assertion C): rather than rely on the library's network-dependent `getUpdates` error firing in a test window, the spike directly invokes the bot's configured `WithErrorsHandler` with a synthetic error carrying a unique marker (`SPIKE05_HANDLER_PROBE_MARKER`), then asserts the marker lands in the stderr-bound sink (132 bytes, `present=true`) and is absent from the captured stdout. This is observable behavior, not structural reasoning — the honest form of the proof.
- **Recorded the Tier-A handler-signature refinement** vs RESEARCH.md §5: the actual `go-telegram/bot@v1.23.0` signatures (`ErrorsHandler func(err error)`, `DebugHandler func(format string, args ...any)`) are simpler than RESEARCH.md documented (no `context.Context` params). Tier A (cosmetic doc drift); the load-bearing "callbacks, no stdout, no io.Writer" claim is unchanged. Phase 5's Telegram frontend must use the real signatures.
- Honored the security contract (T-00-08 mitigate, T-00-09 mitigate): the byte-equality assertion IS the protocol-integrity check (T-00-08); the dummy token (`dummyTokenForSpike()`) is redacted to `[REDACTED: dummy token]` in RESULT.md and the footer (T-00-09, D-03). D-03 self-grep CLEAN (3 checks: no token literal, no real user paths, 7 D-02 fields present). Transport discipline modeled from day one: only the canned ACP frames ever touch `os.Stdout`; everything else (`slog`, the error handler, the result footer, PASS/FAIL) goes to `os.Stderr` via `log.SetOutput(os.Stderr)` + `slog.NewTextHandler(os.Stderr, ...)`.
- Wrote `spikes/05-stdout-collision/RESULT.md` as the D-02 seven-field draft for Plan 00-05 to fold into VERIFIED-FACTS.md item #5: **Status: VERIFIED**; Evidence embeds the spike footer (415==415 bytes, 0 extra, stderr-sink marker present=true) + the direct source-inspection grep (3 default handlers, WithDebug-gated); Notes carry the 6 load-bearing findings (silent-by-default, Tier-A signature refinement, mitigation recipe, context-first shutdown, empirical proof shape, Tier-A VERIFIED disposition).
- Did NOT create or modify `.planning/research/VERIFIED-FACTS.md` (Plan 00-05 is the single writer — constraint #1 honored; zero `files_modified` overlap with sibling plans).

## Task Commits

Each task was committed atomically. Task 1 is `tdd="true"` under MVP+TDD (ROADMAP Phase 0 `Mode: mvp` + config `tdd_mode: true`), so it follows the RED→GREEN gate sequence:

1. **Task 1 (RED): Write failing test for #5 stdout-collision integration** - `7608540` (test) — 7 tests referencing undefined helpers (`writeACPFrame`/`buildACPFrames`/`extraByteCount`/`compareStdout`/`makeStderrErrorsHandler`/`buildBot`/`dummyTokenForSpike`) → compile failure (RED bar)
2. **Task 1 (GREEN): #5 stdout-collision integration spike (go-telegram/bot + ACP)** - `783eada` (feat) — `main.go` provides the framing + clean-stdout helpers + handler-routing mitigation + offline-disciplined bot construction + the live captured-stdout integration run; `go test -race` passes all 7 tests; module `go vet` clean; spike exits 0 with `overall_status: VERIFIED` (4/4 assertions PASS)
3. **Task 2: Record STACK #5 finding in RESULT.md (D-02 shape, VERIFIED)** - `14d9c2c` (feat)

**TDD Gate Compliance:** RED commit `7608540` (`test(00-04):`) precedes GREEN commit `783eada` (`feat(00-04):`) for the same plan — the RED→GREEN gate sequence is satisfied. The MVP+TDD runtime gate (Task 1 is `tdd="true"` with `<behavior>` + source `<files>`, so `task.is-behavior-adding` is true) did NOT trip because the RED commit was written before the implementation step, exactly as the gate requires. The RED test was genuinely failing (compile failure from undefined helpers) before the GREEN implementation.

## Files Created/Modified

- `spikes/05-stdout-collision/main_test.go` - RED-then-GREEN offline test suite (7 tests): ACP frame writer (newline-delimited JSON, no Content-Length, no embedded newlines), clean-stdout byte-equality helper (extraByteCount zero on match, >0 on leak), the verdict wrapper (compareStdout), the ordered canned-frame sequence builder, the stderr-routing errors handler (go-telegram/bot@v1.23.0 `ErrorsHandler func(err error)` signature), the offline-disciplined bot construction (WithSkipGetMe + WithErrorsHandler, NO WithDebug).
- `spikes/05-stdout-collision/main.go` - Throwaway spike (package main, zero new deps): captures real `os.Stdout` via `os.Pipe`, runs the `go-telegram/bot` long-poll loop in a goroutine tracked by a `sync.WaitGroup`, writes 3 canned ACP frames to the captured stdout, byte-compares captured vs canonical (415==415, 0 extra), deterministically probes the handler-routing path (synthetic-marker error), asserts bot exit within 2s of `telegramCtx` cancel. Offline bot construction (`WithSkipGetMe`, `WithErrorsHandler`→slog→stderr, NO `WithDebug`). Transport discipline: `log.SetOutput(os.Stderr)` + `slog.NewTextHandler(os.Stderr, ...)`; structured `=== SPIKE 05 RESULT ===` footer on stderr; only the canned ACP frames ever touch `os.Stdout`.
- `spikes/05-stdout-collision/RESULT.md` - D-02 seven-field item-#5 draft for Plan 00-05: Fact (STACK quote), Source, Verified (2026-08-09), Verified against (go-telegram/bot@v1.23.0 + integration spike + direct source inspection), Status (VERIFIED), Evidence (spike footer + redacted canned frames + source-grep of the 3 default handlers), Notes (6 findings: silent-by-default, Tier-A signature refinement, mitigation recipe, context-first shutdown, empirical proof shape, Tier-A VERIFIED disposition).

## Decisions Made

- **Offline bot construction via `WithSkipGetMe`.** Per critical_constraint #2 (the spike must NOT require a real token or a live Telegram connection; the goroutine topology is the point). `bot.New()` calls `GetMe()` synchronously unless `WithSkipGetMe()` is set; with a dummy token and the default `https://api.telegram.org` URL, that would make a real network call that fails. `WithSkipGetMe` skips it, keeping `New()` offline and deterministic. `Start()` still attempts `getUpdates` (which fails against the real API) — that error path is exactly what `WithErrorsHandler` routes to the stderr sink, and is the case assertion C's synthetic-marker probe determinizes (independent of network timing).
- **Captured real `os.Stdout` via `os.Pipe`, not a `TeeReader`/buffer.** Per the plan's `<action>` step 1: redirect real stdout to a pipe so EVERY byte any goroutine writes to fd 1 is collected, then byte-compare against the canned frames. The bot goroutine is NEVER given the write-end — it has no stdout handle at all (the library has no stdout write anyway, but the spike does not rely on that alone; the pipe captures everything). The honest PASS requires `captured > 0 AND captured == canonical AND extra-byte count == 0` (guards against a trivial 0==0 pass from an empty pipe).
- **Tier-A handler-signature refinement, not Tier-B.** RESEARCH.md §5 over-described the callback signatures (added `context.Context` params that the real v1.23.0 API does not have). Per D-07 this is Tier A (cosmetic doc drift): record the corrected signatures + continue. Research predicted Tier A at most; confirmed. The three Tier-B options (revise-and-continue / halt-and-replan / accept-and-document) are NOT triggered because the load-bearing "no stdout, no io.Writer, callbacks-only" claim is fully intact.
- **`RESULT.md` `**Status: VERIFIED**` annotation placed directly under the `# #5.` heading** (mirroring Plan 00-01's FINDING.md, Plan 00-02's RESULT.md, and Plan 00-03's RESULT.md patterns) so the plan's `RESULT5_OK` verify heuristic — which greps for `VERIFIED|FAILED` — passes regardless of the canonical D-02 field ordering. Both the annotation and the `- **Status:** VERIFIED` field agree; no information drift.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] `spikes/go.sum` missing the `go-telegram/bot` h1 entry (first import of the dep)**
- **Found during:** Task 1 (GREEN implementation, first `go vet`)
- **Issue:** Plan 00-01 pinned `go-telegram/bot@v1.23.0` in `spikes/go.mod`, but no prior spike imported it (00-02 used `go-openai`; 00-03 was zero-dep). The `go.sum` only carried the `go.mod` hash line, not the module-content h1 line, so `go vet`/`go test` failed with "missing go.sum entry for module providing package github.com/go-telegram/bot".
- **Fix:** Ran `go mod download github.com/go-telegram/bot` (from `spikes/`) to populate the h1 line. Per D-05, `spikes/go.sum` is gitignored (not committed) — the download populated it locally only; the dep was already pinned in `go.mod` from Plan 00-01, so this is build-config population, not a new dependency.
- **Files modified:** `spikes/go.sum` (gitignored — not committed)
- **Verification:** `go vet ./05-stdout-collision/` clean; `go test -race ./05-stdout-collision/` passes.
- **Committed in:** (no commit — gitignored artifact, D-05)

**2. [Rule 1 - Bug] Clean-stdout assertion initially trivially passed on an empty pipe (0==0)**
- **Found during:** Task 1 (GREEN implementation, first spike run analysis)
- **Issue:** The original `extraByteCount` returned `len(got) - len(canned)`; if both were 0 (e.g. a hypothetical capture bug where the pipe lost the frames), the delta would be 0 and assertion A would PASS spuriously. The spike redirected real `os.Stdout` to a pipe, so the external stdout showed 0 bytes — I needed to confirm the INTERNAL `captured` actually contained the 415-byte canonical sequence, not just that `extra == 0`.
- **Fix:** Strengthened assertion A to require BOTH (1) `captured > 0 AND captured == canonical` (the frames really were written and captured — guards against a trivial 0==0 pass) AND (2) `extra == 0` (byte-equal). Added `canonical_byte_count`/`captured_byte_count` lines to the footer so the byte-equality is visible to RESULT.md/reviewers. The run now reports `captured 415 == canonical 415; extra-byte count 0 (byte-equal)` — an honest, debuggable PASS.
- **Files modified:** `spikes/05-stdout-collision/main.go`
- **Verification:** `go run ./05-stdout-collision/` reports `captured_byte_count: 415` (== canonical), `stdout_extra_byte_count: 0`; assertion A PASS with the byte counts printed.
- **Committed in:** `783eada` (Task 1 GREEN commit)

**3. [Rule 2 - Missing Critical] Handler-routing assertion was structural-only, not empirical**
- **Found during:** Task 1 (GREEN implementation, footer analysis)
- **Issue:** The original assertion C argued structurally ("the handler closure captures os.Stderr only") — but a skeptic could note the bot's network-dependent `getUpdates` error did NOT actually fire in the 150ms test window (no `[TGBOT]` line appeared in stderr), so the routing path was never exercised in the run. Structural reasoning is weaker than the spike's whole point (empirical proof).
- **Fix:** Refactored assertion C to a DETERMINISTIC empirical probe: the spike holds a reference to the bot's configured `WithErrorsHandler` (via the new `buildBotWithHandler`), directly invokes it with a synthetic error carrying a unique marker (`SPIKE05_HANDLER_PROBE_MARKER`) WHILE stdout is still captured, then asserts (a) the marker lands in the stderr-bound sink (132 bytes, `present=true`) AND (b) is absent from the captured stdout. This makes assertion C observable behavior independent of network timing. Added `stderr_sink_byte_count` + marker-presence line to the footer.
- **Files modified:** `spikes/05-stdout-collision/main.go`
- **Verification:** `go run ./05-stdout-collision/` reports `stderr_sink_byte_count: 132 (marker present=true)`; assertion C PASS — "synthetic error marker found in stderr sink and absent from captured stdout".
- **Committed in:** `783eada` (Task 1 GREEN commit)

---

**Total deviations:** 3 auto-fixed (1 Rule-3 blocking gitignored-artifact; 2 Rule-1/2 assertion-strengthening in the GREEN implementation, both caught by analyzing the run output before commit; no architectural changes, no scope creep)
**Impact on plan:** All three were correctness/robustness improvements caught before the GREEN commit — the assertion strengthening (deviations 2 & 3) is what makes the spike's empirical proof honest rather than trivially-passing. No plan-scope changes; the offline construction, source-inspection finding, and handler-routing recipe were all made per the plan's `<action>` and `<read_first>` guidance.

## Issues Encountered

None beyond the three deviations above. The `go-telegram/bot@v1.23.0` source was already in the module cache from Plan 00-01's `go mod download`, so the direct source inspection (the load-bearing "zero stdout writes" claim) was a read-only check on first try. The library's `bot.New` + `Start(ctx)` + `WithErrorsHandler`/`WithSkipGetMe` API surface matched the plan's `<read_first>` (RESEARCH.md §5) exactly modulo the Tier-A signature refinement recorded in Notes (b).

## User Setup Required

None — no external service configuration required. This spike uses an obviously-fake dummy token (`dummyTokenForSpike()`) and `WithSkipGetMe` to keep `bot.New()` offline (no real Telegram connection, critical_constraint #2). The `TELEGRAM_BOT_TOKEN` env var documented in `spikes/README.md` is unused by this spike (the dummy token is hardcoded via the redactable helper); it remains in the README for the general Telegram-spike pattern. (No USER-SETUP.md generated — the plan's frontmatter `user_setup` is `[]`.)

## Threat Flags

The spike introduces security-relevant surface fully covered by the plan's `<threat_model>` (no NEW surface beyond it):

| Flag | File | Description |
|------|------|-------------|
| (covered) T-00-08 | spikes/05-stdout-collision/main.go | Tampering / protocol violation — stdout integrity. Mitigated: this IS the assertion — captured `os.Stdout` byte-equal to the canned ACP frames (415==415, 0 extra). The bot goroutine, while live, leaked zero bytes onto stdout. The `WithErrorsHandler` routes to a non-stdout sink; no stdout-writing debug handler is registered (`WithDebug` NOT set). |
| (covered) T-00-09 | spikes/05-stdout-collision/RESULT.md | Information disclosure — dummy bot token + RESULT.md. Mitigated: the dummy token is a hardcoded placeholder via `dummyTokenForSpike()` (no real token, no real chat IDs — D-03); redacted to `[REDACTED: dummy token]` in RESULT.md and the footer. D-03 self-grep CLEAN (no token literal, no real user paths). Plan 00-05 re-runs the grep over the merged VERIFIED-FACTS.md as a second barrier. |

No threat flags beyond the plan's register — both registered threats (T-00-08/09) are mitigated per the plan.

## Next Phase Readiness

- **Plan 00-04 (this plan) is complete.** The throwaway spike compiles (`go vet` clean), all 7 offline tests pass (`go test -race`), the spike runs end-to-end with `overall_status: VERIFIED` (4/4 assertions PASS with deterministic evidence), and the D-02-shaped RESULT.md is written with the Tier-A handler-signature refinement recorded.
- **Plan 00-05 (Wave 3)** reads this plan's `spikes/05-stdout-collision/RESULT.md` + the three other evidence files (`01-jsonl-capture/FINDING.md` from Plan 01 + `RESULT.md` from Plans 02/03) and authors VERIFIED-FACTS.md item #5 from it. It should record item #5 as **VERIFIED** with the silent-by-default finding + the 4-step mitigation recipe + the Tier-A signature refinement called out for Phase 5's Telegram adapter. With Plan 00-04 done, Wave 2 is complete (4/4 spike plans) and Plan 00-05 can fold all four evidence files into VERIFIED-FACTS.md in a single integration pass.
- **Phase 5 (Ecosystem Compatibility)** consumes the confirmed library-behavior + the mitigation recipe directly: the v2 Telegram frontend follows the exact pattern (`bot.New(token, WithErrorsHandler→slog→os.Stderr)`, NO `WithDebug`, `log.SetOutput(os.Stderr)`, `go b.Start(ctx)` goroutine) to keep stdout ACP-only. The Tier-A signature refinement prevents Phase 5 from using the wrong (over-described) callback signatures from RESEARCH.md.

## Self-Check: PASSED

- **Files created (3/3 FOUND):** `spikes/05-stdout-collision/main.go`, `spikes/05-stdout-collision/main_test.go`, `spikes/05-stdout-collision/RESULT.md`, `.planning/phases/00-spike-re-verification/00-04-SUMMARY.md`
- **Commits (3/3 FOUND):** `7608540` (test, RED), `783eada` (feat, GREEN), `14d9c2c` (feat, RESULT.md)
- **VERIFIED-FACTS.md NOT created/modified** (constraint #1 honored — Plan 00-05 owns it; `ls` confirms file does not exist)
- **No file deletions** in any of the three commits
- **Task 1 `<verify>`:** `SPIKE05_OK` (vet + footer + extra.byte.*0|stdout.*clean|byte.equal) — PASS
- **Task 2 `<verify>`:** `RESULT5_OK` (heading + Status + VERIFIED|FAILED + v1.23.0 + stderr|stdout.*clean|extra.byte.*0) — PASS
- **D-03 self-grep over RESULT.md:** CLEAN (3 checks: no dummy token literal, no real user paths, 7 D-02 fields present)
- **stdout discipline:** external stdout byte-empty (0 bytes); internal captured stdout 415 bytes == canonical 415 bytes, 0 extra; all diagnostics to stderr — PASS
- **Full-module regression check:** `go vet ./...` and `go build ./...` clean (no sibling-spike breakage)

---

*Phase: 00-spike-re-verification*
*Plan: 04*
*Completed: 2026-08-09*
