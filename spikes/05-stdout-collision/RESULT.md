# #5. go-telegram/bot + ACP stdout collision

**Status: VERIFIED**

- **Fact:** STACK item #5 (verbatim): "`go-telegram/bot` + ACP stdout-collision test — integration test that both frontends running in one process never write a non-ACP frame to stdout." Reinforced by the go-telegram/bot Version-Compatibility line (STACK ~line 480): "`go-telegram/bot` + ACP stdio in one process | Both share `log/slog` → stderr | **Critical:** Telegram frontend must never write to stdout (stdout is ACP's). Enforce via review/lint." And the Stack-Pattern line (STACK ~line 291): "Telegram logging MUST go to stderr (stdout is ACP's)."

- **Source:** `.planning/research/STACK.md` §"Open Verification Items (Phase-0 spike checklist)" item #5 (line 492); STACK table rows `go-telegram/bot` (~lines 50, 63, 248, 480, 192); `.planning/ROADMAP.md` Phase 0 Success Criterion #3 ("the `go-telegram/bot` + ACP stdout-collision integration test demonstrates the two transports don't fight over stdout"); AGENTS.md "Transport discipline" (stdout reserved exclusively for ACP JSON-RPC frames; all logging/diagnostics to stderr).

- **Verified:** 2026-08-09

- **Verified against:** `github.com/go-telegram/bot@v1.23.0` (released 2026-08-03), via the integration spike at `spikes/05-stdout-collision/main.go` — a real `go-telegram/bot` long-poll loop running in a goroutine alongside canned ACP frames written to a captured `os.Stdout`, with a byte-equality assertion proving zero non-ACP bytes. Also verified by direct inspection of the `go-telegram/bot@v1.23.0` package source in the module cache (the load-bearing library-behavior claim — see Evidence + Notes (a)).

- **Status:** `VERIFIED`

- **Evidence:**

  The spike runs the two transports concurrently in ONE PROCESS and asserts the captured stdout is byte-equal to the canned ACP frames the spike wrote (zero extra bytes from the bot). Run output (`go run ./05-stdout-collision/` from `spikes/`, stderr footer):

  ```
  === SPIKE 05 RESULT ===
  claim: go-telegram/bot@v1.23.0 + ACP coexist in one process; stdout stays byte-clean
  library: github.com/go-telegram/bot@v1.23.0
  topology: main goroutine writes canned ACP frames to os.Stdout; bot goroutine runs b.Start(ctx)
  token: [REDACTED: dummy token] (no real Telegram connection; WithSkipGetMe keeps New() offline)
  canned_frame_count: 3
  canonical_byte_count: 415  (the bytes the spike wrote to captured stdout)
  captured_byte_count: 415  (the bytes read back from captured stdout; must == canonical)
  stdout_extra_byte_count: 0  (0 = byte-clean; the load-bearing number)
  stderr_sink_byte_count: 132  (handler-routing evidence sink; marker "SPIKE05_HANDLER_PROBE_MARKER" present=true)
    PASS | clean stdout (captured == canned ACP frames; byte-equal, 0 extra) — captured 415 bytes == canonical 415 bytes; extra-byte count 0 (byte-equal)
    PASS | default silence (WithDebug NOT set → no stdout writes) — captured stdout byte-equal to canned frames — debug path silent by default
    PASS | handler routing (errors → stderr-bound sink, never stdout) — synthetic error marker found in stderr sink and absent from captured stdout
    PASS | shutdown (bot goroutine exits within 2s of context cancel) — bot goroutine exited within 2s of cancel (context-first contract holds)
  overall_status: VERIFIED
  === END ===
  ```

  The canned ACP frames written to stdout (D-03 redacted; 3 frames, newline-delimited JSON-RPC 2.0 per Plan 00-03 — an initialize response, a session/update notification, a session/prompt response):

  ```
  {"id":1,"jsonrpc":"2.0","result":{"agentCapabilities":{"loadSession":true},"agentInfo":{"name":"ass-guard-spike-acp","version":"0"},"protocolVersion":1}}
  {"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"[REDACTED: id]","update":{"content":{"text":"[REDACTED: content]","type":"text"},"messageId":"[REDACTED: id]","sessionUpdate":"agent_message_chunk"}}}
  {"id":2,"jsonrpc":"2.0","result":{"stopReason":"end_turn"}}
  ```

  Load-bearing library-behavior verification (direct source inspection of `go-telegram/bot@v1.23.0`, module cache path `~/go/pkg/mod/github.com/go-telegram/bot@v1.23.0/`): a recursive grep for `os.Stdout` / `fmt.Print` / `fmt.Fprint(os.Stdout…` across the package's non-test, non-`examples/` Go files returns **ZERO matches**. The only output surfaces are three default handlers in `bot.go`, all using stdlib `log.Printf` (which writes to `log.Default()` → `os.Stderr`):

  ```
  // bot.go:147-157  (go-telegram/bot@v1.23.0)
  func defaultErrorsHandler(err error)  { log.Printf("[TGBOT] [ERROR] %v", err) }
  func defaultDebugHandler(format string, args ...any) { log.Printf("[TGBOT] [DEBUG] "+format, args...) }
  func defaultHandler(_ context.Context, _ *Bot, update *models.Update) { log.Printf("[TGBOT] [UPDATE] %+v", update) }
  ```

  The `defaultDebugHandler` is gated behind `b.isDebug` — it only fires when `WithDebug()` is set (`get_updates.go:40`, `raw_request.go:61,135`). With `WithDebug()` NOT set (the spike's configuration), the debug path is silent by default. The `defaultErrorsHandler`/`defaultHandler` are overridable via `WithErrorsHandler`/`WithDefaultHandler`. There is **no `WithLogger`, no `io.Writer` option, no unconditional stdout write** anywhere in the v1.23.0 API surface.

  Redaction note (D-03): the dummy bot token (`dummyTokenForSpike()`) is replaced with `[REDACTED: dummy token]` throughout; the canned frames redact `sessionId`/`messageId` → `[REDACTED: id]` and `text` → `[REDACTED: content]`, preserving all JSON keys, method names, field names (`agentCapabilities`, `protocolVersion`, `sessionUpdate`, `stopReason`), enums (`end_turn`, `agent_message_chunk`), and framing. Module-cache file paths in this Evidence field are reduced to the generic `~/go/pkg/mod/...` form. Plan 00-05 re-runs the grep over the merged VERIFIED-FACTS.md as a second barrier.

- **Notes:**

  (a) **The load-bearing finding — `go-telegram/bot@v1.23.0` is SILENT BY DEFAULT and never writes to stdout.** Direct source inspection confirms zero `os.Stdout`/`fmt.Print` writes in the package (the only matches are in `examples/` — separate sample programs, not the library). All library output funnels through three default handlers in `bot.go` (`defaultErrorsHandler`, `defaultDebugHandler`, `defaultHandler`), each using stdlib `log.Printf` → `log.Default()` → `os.Stderr`. The library has **no `WithLogger` and no `io.Writer` option** — all output routing is via the `WithDebugHandler`/`WithErrorsHandler`/`WithDefaultHandler` callbacks. The debug path is gated behind `b.isDebug` (set only by `WithDebug()`); without `WithDebug()`, `defaultDebugHandler` never fires. This is exactly what RESEARCH.md §5 predicted; the spike confirms it empirically (assertion B: captured stdout byte-equal to the canned frames with `WithDebug` NOT set).

  (b) **Tier-A doc refinement vs RESEARCH.md §5 (handler signatures).** RESEARCH.md §5 documented the callback signatures as `ErrorsHandler func(ctx context.Context, err error)` and `DebugHandler func(ctx context.Context, message string, err error, additionalData any)`. The **actual** `go-telegram/bot@v1.23.0` signatures (bot.go:27-28) are simpler: `type ErrorsHandler func(err error)` and `type DebugHandler func(format string, args ...any)`. This is Tier A (cosmetic doc drift — a slightly over-described signature; the load-bearing "callbacks, no stdout, no io.Writer" claim is unchanged). The spike uses the REAL v1.23.0 signatures; Phase 5's Telegram frontend must use them too.

  (c) **The mitigation recipe (propagation rule for the real Phase-5 implementation).** ass-guard's v2 Telegram frontend follows this exact pattern to keep stdout ACP-only:
    1. Construct the bot with `bot.New(token, bot.WithErrorsHandler(makeStderrErrorsHandler(os.Stderr)))` — route errors to `slog` → `os.Stderr`.
    2. Do **NOT** register `WithDebug()` or a stdout-writing `WithDebugHandler` (default silence).
    3. Belt-and-suspenders: call `log.SetOutput(os.Stderr)` at process start so even the library's default handlers (if one is accidentally left un-overridden) stay off stdout.
    4. Run the long-poll loop as `go b.Start(telegramCtx)` in a goroutine; cancel `telegramCtx` on editor-initiated shutdown to drain the loop.

  (d) **Context-first contract confirmed (assertion D).** The bot goroutine exits within 2 seconds of `telegramCtx` cancellation — the context-first shutdown contract STACK picked the library for (Focus 3: "idiomatic `context.Context` throughout — load-bearing for clean cancellation/shutdown when the ACP server owns process lifecycle"). `b.Start(ctx)` blocks on the long-poll loop and returns cleanly on `ctx.Done()`.

  (e) **Empirical proof shape.** The spike captures the process's real `os.Stdout` via `os.Pipe()`, writes 3 canned ACP frames (415 bytes) to the pipe's write-end, runs the bot goroutine concurrently, then reads the bytes back from the read-end and asserts `captured (415) == canonical (415)` with `stdout_extra_byte_count: 0`. Assertion C additionally determinizes the handler-routing path: the spike directly invokes the bot's configured `WithErrorsHandler` with a synthetic error carrying a unique marker, and proves the marker lands in the stderr-bound sink (132 bytes, `present=true`) and is absent from the captured stdout. This is observable behavior, not structural reasoning.

  (f) **Tier disposition: VERIFIED (Tier A — the favorable outcome).** The library is silent by default and routes all output through stderr-bound callbacks; the transport discipline (stdout = ACP only) holds with `go-telegram/bot` in-process. No Tier-B (architectural) finding: no stdout-write default that can't be silenced, no library contract bug on shutdown. The only correction is the Tier-A handler-signature doc refinement in (b). ass-guard's Phase-5 Telegram frontend can proceed against `go-telegram/bot@v1.23.0` with the recipe in (c).
