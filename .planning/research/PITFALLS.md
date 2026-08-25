# Pitfalls Research

**Domain:** v1.2 Claude Code Parity for ass-guard — pitfalls of ADDING these features to the shipped v1.0/v1.1 Go agent (ACP completeness incl. request_permission + elicitation + tool_call/plan streaming + session family + editor config; built-in chat commands; slash-invocable skills; CC parity closures — compaction, subagents w/ background agents, hooks PreToolUse deny, AGENTS.md/CLAUDE.md injection, streamed thinking, rich prompt content, background Bash + persistent shell; SEED-004 gaps — shadow-git checkpoints, real sandbox, steering queue)
**Researched:** 2026-08-26
**Confidence:** HIGH overall — every pitfall is grounded in direct inspection of this repo's actual v1.0/v1.1 code (`internal/acp`, `internal/session`, `internal/coreexec`, `internal/checkpoint`, `internal/toolcat`, `internal/shaper`, `cmd/ass-guard/acp_serve.go`); externally-sourced facts are individually graded below (Context7-verified Anthropic API behavior = MEDIUM; web-search-derived ACP/Zed/Seatbelt/bwrap/git/pty facts = LOW-to-MEDIUM, flagged where load-bearing).

> Scope note: this **replaces** the 2026-08-14 v1.1 pitfalls doc (v1.1 shipped 2026-08-25; its pitfalls materialized or were absorbed into PROJECT.md Validated/Caveats). This doc covers ONLY the v1.2 feature set, with emphasis on **integration pitfalls** — the ways new features break a working shipped system. The v1.0 stub-vs-real lesson (old Pitfall 1) remains in force and is inherited here as a standing phase gate, not restated.

**Phase labels used below** (PROJECT.md v1.2 priority order; the roadmapper may reorder — every pitfall also names its feature area so mapping survives renumbering):

- **P1** — ACP completeness: request_permission, elicitation/create, tool_call+plan streaming, available_commands_update, session list/resume/close/delete, editor-driven configOptions
- **P2** — Built-in chat commands (/model /config /compact /clear /cost …) — compaction is a prerequisite
- **P3** — Slash-invocable skills + per-agent model frontmatter
- **P4** — CC parity audit closures: compaction on overflow, full subagents + background agents, hooks PreToolUse deny, AGENTS.md/CLAUDE.md auto-injection, streamed thinking blocks, rich prompt content (@-mentions/images), background Bash + persistent shell
- **P5** — SEED-004 gaps: shadow-git checkpoints, compaction verify-first, real sandbox (Seatbelt/bwrap), steering queue
- **P6** — SEED-001 kit extraction (library)
- **P7** — small tails (LSP docs, scheduler outcome store, nightly parity CI)

Known ordering tension to resolve at roadmap time: **compaction appears twice** (P2 lists /compact which "requires compaction"; P5 lists "compaction verify-first"). The research position (see Pitfalls 7–8): compaction design belongs BEFORE or WITH the earliest feature that depends on it, and the verify-first spike (does the zcode profile already capture zcode auto-compact + cache_control placement?) must precede ANY compaction implementation regardless of which phase builds it.

---

## Critical Pitfalls

### Pitfall 1: request_permission implemented as a blocking in-turn RPC while holding the session/mutating locks — one unanswered dialog hangs the world

**What goes wrong:**
`session/request_permission` is a JSON-RPC request the AGENT sends to the client and then waits on. The natural implementation — call it synchronously inside the tool executor right before a mutating action — deadlocks the shipped concurrency architecture: the whole turn holds the **per-session turn mutex** (`sessionTurnMu` in `cmd/ass-guard/acp_serve.go` — "the whole turn (ask-reply resumes included) holds the session mutex"), and mutating tools serialize behind the mutability gate (`toolcat.EffectiveMutability` / read-only-concurrent-mutating-serialized discipline). An in-line blocking permission ask therefore parks the turn mutex for as long as the human takes to answer — minutes, hours, overnight (this agent's whole point is hands-off unattended runs). Every queued client prompt, automation firing, and hook-DAG step piles up behind it. Worse: if the ask is placed inside the *tool-execution* path while the mutating slot is held, cross-session mutation throughput collapses too. And there is **no protocol-level timeout** to save you: ACP defines no auto-deny-after-N; the request stays open until the user answers, cancels, or the connection dies (web-search derived, LOW confidence but consistent across ACP schema page and community reports). Zed renders a native picker and simply waits — the "expectational behavior": the protocol expects the agent to sit there blocked.

**Why it happens:**
Request-permission *feels* like a pre-execution guard (check-then-run), and check-then-run wants to live inside the executor where the tool name and args are at hand. The repo already has the correct pattern — `AskUserQuestion` returns `session.ErrSuspended`; the session tool loop records the suspension in the per-session `AskBroker` and ENDS THE TURN; the reply resumes the same turn later. Permission asks are a second instance of the same suspension class, and the temptation is to "just await it inline" because unlike AskUserQuestion it arrives with tool identity attached.

**How to avoid:**
- Follow the ErrSuspended/AskBroker precedent exactly: on a gated tool, emit the `tool_call` update (status pending) + `session/request_permission`, record the pending permission in a broker keyed by (sessionId, toolCallId), end the turn with a suspension marker, release the turn mutex. The outcome resumes the SAME turn with the tool result (allowed → execute; denied → structured denial result; cancelled → treat as denial-with-cancelled-note).
- NEVER hold the mutating-execution slot across a human-wait. Acquire it only when the resumed turn actually executes.
- Decide the watchdog deliberately: an optional agent-side deny-on-timeout (mirroring the existing D-01 ask-timeout flag) is reasonable; silence is not — an unconfigured system must have an explicit default (recommend: no timeout, matching ACP/Zed expectations, surfaced clearly in /permissions and doctor output).
- Handle the cancel contract precisely: on `session/cancel` the client MUST respond with `outcome: cancelled` as a NORMAL response (not a JSON-RPC error — verified against agentclientprotocol.com schema page, MEDIUM). Treating cancelled-as-error would spam the transcript with fake failures and break the engine's cancel-drain accounting.
- Persist "allow_always" decisions with an explicit scope (per-project pattern-table entry, operator-inspectable via /permissions) — see Security Mistakes.

**Warning signs:**
- A `select { case resp := <-permCh }` sitting anywhere inside `toolexec`/`coreexec` execution paths.
- Tests for permission flow that don't assert the turn mutex is released during the wait.
- Any code path where a goroutine awaits a human while holding `turnMu`, `Store.mu`, or the mutating semaphore.

**Phase to address:** P1 (ACP completeness — request_permission). This is the single highest-risk P1 item.

---

### Pitfall 2: Streaming frames out of order — concurrent emitters (main turn + parallel subagents + async engine firings) violate per-session sequence monotonicity

**What goes wrong:**
ACP `session/update` notifications carry a monotonically increasing per-session `sequenceNumber`; clients may buffer on gaps and treat reordering as an agent bug (web-search derived, LOW-MEDIUM). Today the chunk-forwarder in `acp_serve.go` emits from ONE turn under the turn mutex. v1.2 adds many concurrent emitters: `tool_call`/`tool_call_update` from the tool loop, `plan` updates, `agent_thought_chunk` from streamed thinking, background-agent completion notifications arriving mid-other-turn (an explicit v1.2 target), and elicitation forms. Emitting from multiple goroutines through the mutex-guarded `Writer` guarantees no *byte-level* line interleaving (the framer's channel + mutex handles that) but does NOTHING for *logical* ordering: a `tool_call_update(completed)` can land before the `tool_call(pending)` that introduces it, a plan update can interleave mid-message-chunk, and background completions can jump ahead of the foreground narrative. Zed drops or misrenders such frames.

**Why it happens:**
The framer solves the transport problem (atomic lines) while the sequencing problem (application-level total order per session) is invisible until a second concurrent emitter exists. Subagents already run as goroutine turn-loops today; they simply didn't emit client-visible frames. The moment they do (full-subagents parity work, P4), every unsynchronized `emit()` becomes a reordering bug.

**How to avoid:**
- Build ONE per-session outbound sequencer now (P1, when tool_call/plan streaming lands): a single goroutine owning a queue; every producer appends `(update, seq)`; the sequencer assigns monotonically increasing numbers and writes frames. All emitters — turn loop, subagents, engine, background notifications — go through it. This is also the natural place to persist `lastSeq` for resume (see Pitfall 6).
- Define the interleaving POLICY deliberately: e.g., plan updates and tool_call frames from the foreground turn take priority; background-agent completions are emitted at safe points (turn boundaries or between tool calls) unless the protocol demands immediacy. Document it — silent policies rot.
- Sequence-number continuity across the serve lifetime per session (not per turn) — confirm against the SDK/spec whether reset-on-load is legal before shipping resume (LOW-confidence area; verify in `zed-industries/agent-client-protocol` source during P1 planning).

**Warning signs:**
- More than one call site reaching the ACP `Writer` outside the sequencer.
- A test suite that only streams from a single goroutine (add a race-detector stress test with N concurrent emitters asserting strict sequence order).
- `sequenceNumber` fields absent from emitted updates (clients that rely on them will degrade silently).

**Phase to address:** P1 (establishes the sequencer with tool_call/plan streaming); P4 (subagents/background agents MUST consume it, not bypass).

---

### Pitfall 3: Client backpressure stalls the agent — slow/paused editor freezes turns, or the "fix" (unbounded buffering) eats memory

**What goes wrong:**
ACP rides stdio. If Zed stops reading (modal dialog up, tab in background, OS pipe buffer full), the agent's stdout writes block once the pipe buffer fills. The framer's buffered channel (`make(chan *Message, writeBuffer)`) delays but does not remove this: when the buffer fills, the writer goroutine blocks, the channel fills, and then every producer calling the blocking send blocks — including, transitively, the turn loop and (if not careful) the sequencer that everything shares. Symptom: the model keeps generating but the agent stops accepting/processing anything, or the turn appears hung exactly when a permission dialog (which itself may be why the client is busy) is open — a self-deadlock loop with Pitfall 1. The naive fix — grow the buffer unboundedly or spill to memory without cap — trades hang for OOM on chatty turns (thinking streams + tool outputs can be MBs).

**Why it happens:**
Backpressure is invisible in dev (the editor always reads promptly) and only appears with a slow/paused/minimized client or a huge burst (background bash emitting continuously — the TaskRegistry already tees unbounded accumulated output into `bgTask` buffers plus a file). Nobody tests "client stops reading mid-stream."

**How to avoid:**
- Bound every outbound queue; decide the overflow policy EXPLICITLY per frame class: droppable progress deltas (intermediate `tool_call_update` rawOutput chunks, redundant plan ticks) may be coalesced/dropped with a "truncated" marker; state transitions (`pending→completed/failed`) and permission/elicitation requests must NEVER be dropped (coalesce by keeping only the latest state per toolCallId — the update shape is designed for this: fields "only applied if present").
- Decouple producers from the socket with a bounded spill: cap in-memory backlog per session; beyond the cap, persist overflow to the session artifact directory and replay on demand (the progressive-log pattern `coreexec/background.go` already uses for bash output generalizes here).
- Add a regression test: a mock client that stops reading for N seconds mid-turn; assert the turn completes, no frames lost that matter, memory bounded.
- Never let a *notification* write path hold the turn mutex while blocked on the socket (another Pitfall 1/3 composite).

**Warning signs:**
- Unbounded channels/slices in any emit path; `select` with `default: continue` silently discarding state transitions.
- Turn duration correlating with client responsiveness (visible in usage records).
- Memory growth proportional to streamed bytes rather than conversation size.

**Phase to address:** P1 (backpressure policy ships WITH the sequencer, not after).

---

### Pitfall 4: Tool_call frames left open — pending/in_progress entries never reach completed/failed, poisoning Zed's UI and the resume path

**What goes wrong:**
The ACP tool_call lifecycle (`tool_call` → `tool_call_update`(status) → terminal `completed`/`failed`) is advisory-looking but client-visible: Zed renders each open call as an in-flight row. Paths that forget the terminal update: cancelled turns (cancel-drain skips the "close open calls" bookkeeping), suspended turns (ErrSuspended for asks/permissions leaves the call pending — correct mid-flight, wrong if never resumed), panics in executors, and — the big one — **process restarts**: after resume, calls that were in-flight at death are permanently open in the client's view unless the agent actively closes them. A related trap: emitting `tool_call` frames for tools whose results are purely internal (engine steps, hook-DAG nodes) clutters the UI and multiplies the close-out burden.

**Why it happens:**
Terminal-frame emission sits on the success path; every abnormal exit (and hands-off SDD runs are ABNORMAL-path factories: timeouts, cancels, suspensions, provider structural errors) skips it. Nothing fails loudly when a frame is missing — the UI just quietly accumulates zombies.

**How to avoid:**
- Centralize frame lifecycle in the tool-execution wrapper (where AppendToolCall/AppendToolResult transcript lines already pair): guarantee exactly-one terminal `tool_call_update` per emitted `tool_call` via defer, covering panic/cancel/suspend paths. Suspended calls stay `in_progress` with the suspension reason in the title/rawInput — and resume/close/delete must reconcile them (below).
- On session/load (resume), replay the durable transcript's tool-call records and emit reconciling updates: completed/failed for finished calls, failed("interrupted by restart") for orphaned in-flight ones. Do NOT re-emit full history blindly — Zed surfaces what you send.
- Be selective about WHAT becomes a client-visible tool_call: catalog tools yes; internal engine/hook steps no (log to transcript/stderr instead).

**Warning signs:**
- Zed showing spinner rows after a cancelled or crashed turn.
- Transcript lines AppendToolCall without matching AppendToolResult after cancel tests (there is already a cancel-drain seam — extend its invariant to frames).
- No test asserting "every tool_call id receives exactly one terminal update" across the E2E matrix.

**Phase to address:** P1 (lifecycle invariant established with streaming); P1/P4 (resume reconciliation); revisit at every new emitter.

---

### Pitfall 5: Session resume treated as transcript replay only — in-memory state (task registry, parked asks, pending permissions, subagents, sequence counters) silently lost or duplicated

**What goes wrong:**
v1.0 shipped durable-transcript + lean-projection, and `acp_serve.go` documents `session/load` as a no-op under the old D-09 (since reversed — resume is now the operator must-have). The pitfall: implementing resume as "rebuild the projection from the transcript" and stopping there. The transcript cannot represent: (1) the in-memory `TaskRegistry` of background bash tasks — their process groups die with the agent (editor owns lifecycle), yet the transcript references `exec_<uuid>` ids that `BashOutput`/`KillShell` calls will now hit as unknown; (2) parked AskUserQuestion suspensions and pending permission requests — the D-01 timer, the parked-ask resume plumbing (`askResumeCtx`), all evaporate; (3) dispatched-but-unfinished subagent goroutines — the transcript shows `AppendSubagentDispatch` with no result (the projector already tolerates orphaned results, but the WORK is lost and the model believes it launched); (4) the outbound sequence counter (Pitfall 2) and any "allow_always" session-scoped grants. Each produces a specific post-resume pathology: unknown-id structured errors (fine), model waiting forever on a tool result that will never come (NOT fine — must synthesize an interrupted-result on load), duplicate re-dispatch of work the model repeats because it saw no result (wasteful but survivable), or client-side dropped/misordered frames (invisible until reported).

ID stability compounds this: turn ids (`<sessionID>-turn-<NNN>` grammar validated by `checkpoint.validateTurnID`), toolCallIds, and the checkpoint refs are all minted per-process. Resume must CONTINUE the numbering (read last N from transcript) rather than restarting at turn-001 — otherwise checkpoint ids collide with existing refs and `update-ref` overwrites old snapshots (silent history loss).

**Why it happens:**
Replay logic naturally keys off the transcript (it's the mandated artifact), and everything in-memory is invisible to it. The failure only shows on the second process lifetime — which UAT rarely exercises because it requires killing the editor mid-run and reloading.

**How to avoid:**
- Enumerate the live-state inventory explicitly at P1 planning: turn counter, seq counter, task registry, brokers (ask + permission), subagent latches, config overrides (/model, editor configOptions). For each, decide: reconstruct-from-transcript, persist-to-disk, or declare-lossy-with-synthetic-closure.
- Rule: **no dangling expectation survives load.** Every transcript record that implies future output (dispatch without result, suspension without settle, background start without exit) gets a synthetic closing record at resume ("interrupted by agent restart") so the model never waits on a ghost.
- Continue id sequences from transcript maxima; add a test that snapshots a checkpoint, restarts, and asserts new turn ids don't collide with stored refs.
- `available_commands_update` must be re-sent after load (command set is session-scoped state the client forgot).
- E2E test: kill -9 the agent mid-turn (with a background task and a parked ask), relaunch, session/load, drive the turn — assert no hang, no ghost waits, coherent UI.

**Warning signs:**
- `session/load` handler shorter than ~100 lines with no broker/registry reconciliation.
- Post-resume transcript containing dispatch lines whose results appear with a NEW turn's timestamp (re-dispatch instead of closure).
- Any map/slice of live state not enumerated in the resume design doc.

**Phase to address:** P1 (session family — this IS the phase's hard part; budget accordingly).

---

### Pitfall 6: Compaction triggered too late and defeated by the single-oversize-result trap — "prompt is too long" arrives before summarization can help

**What goes wrong:**
Threshold-triggered auto-compaction (community-documented CC behavior: trigger around ~92% of window; LOW confidence on exact numbers) fails in a predictable way for a coding agent: ONE tool result (a huge log read, a minified bundle, a directory dump) can exceed the remaining budget by itself. The request dies with a provider `invalid_request_error` before any summarization could run — and a naive retry-after-compact loop then tries to summarize a history CONTAINING the oversized result, which the summarizer request itself exceeds. Second-order trap: the overflow error arrives MID-TURN (after partial tool execution), so recovery must preserve executed-state consistency (tool calls made, checkpoints taken, boundary markers appended) while rebuilding the window.

**Why it happens:**
Token counting is estimated pre-request; tool outputs are unbounded; the repo already truncates (`session/truncate.go`) and persists oversize output (`bash.persistOversize`), but truncation limits are per-tool-output, not per-window-budget, and nothing ties them to the compactor's threshold.

**How to avoid:**
- Enforce a per-result budget ceiling at the tool-result layer (hard cap + persist-and-reference pattern already present for bash: store full output under `.ass-guard/outputs/`, hand the model a preview + path). Make the cap a fraction of the smallest configured model window across the routing tiers (heavy/good/light differ! A result fine for the heavy tier can overflow when /model reroutes the session to light).
- Pre-flight estimation per request: estimate tokens (existing estimator + margin); if over threshold → compact BEFORE the request, not after the error. Keep the error-path recovery anyway (estimate drifts): on provider overflow error, compact aggressively (drop to last-N + summary) and retry ONCE, with the transcript recording the compaction event (investigate-and-fix-ready constraint).
- Verify-first (the P5 spike, pulled EARLY): inspect whether the zcode profile/corpus already encodes zcode's own auto-compact behavior and cache_control placement — mimicry-era assets retained; if zcode compacts in-band, copying its observable behavior beats inventing our own thresholds (behavioral-eval net can then pin it).
- Never compact mid-suspension (parked asks/permissions reference tool calls that must remain resolvable — see Pitfall 8 for the thinking-block variant of this).

**Warning signs:**
- Any compaction trigger expressed only in the error handler (reactive-only).
- Per-tier routing tests absent from the compactor's test matrix.
- Oversize tool results flowing into the summarizer request verbatim.

**Phase to address:** P5 spike FIRST (verify-first), implementation landing before-or-with P2 (/compact depends on it). Roadmap should pull this ahead of its nominal priority-5 slot.

---

### Pitfall 7: Compaction fights the two-layer context — summary placement vs boundary reset, projector assumptions, and recursive-compaction debt

**What goes wrong:**
The shipped context model is delicate: the durable transcript is append-only truth; the lean projected window resets BETWEEN turns at boundary markers (`MaybeAppendBoundary`, SESS-04 revised — the reset never touches the producing turn's own mid-turn window) and bounds mid-turn accumulation to the rolling last-64-messages (`MidTurnWindowMessages`, pinned to the zcode corpus tail — a mimicry-era constant now load-bearing). Compaction can break every invariant: (1) a summary injected as an assistant/user message confuses `splitAtResetBoundary` (which keys on "the turn's user message is the last one carrying…") and `accumulateMidTurn` pairing (orphaned tool results are DROPPED — a summarized-away tool_use leaves its result orphaned and silently discarded, or vice versa: the model references a tool result the summary replaced — the classic "summarization lost what the model still cites" failure); (2) compacting by REWRITING the transcript violates its append-only diagnostic contract (PROJECT.md investigate-and-fix constraint — the transcript is the primary forensic surface); (3) boundary resets wipe the window INCLUDING the freshly-injected summary if the summary is recorded as window content rather than durable preamble — the very next mutating command erases the compaction; (4) recursive compaction (summarizing summaries) compounds loss ("compaction debt") and hands-off long-running SDD workflows hit dozens of compactions per day.

**Why it happens:**
Compaction is usually prototyped against a flat message array; this codebase is NOT flat — it's transcript-lines → mechanical projection. Every compaction feature that manipulates "messages" directly is operating at the wrong layer.

**How to avoid:**
- Compact ONLY by appending: new transcript line types (e.g., `AppendCompact{summary, coversThroughTurnID}`) that the PROJECTOR interprets (summary becomes durable preamble; covered lines become invisible to the window but remain on disk). The transcript stays complete; projection stays mechanical; diagnostics survive.
- Pair-preservation rule: a tool_use/result batch is atomic under compaction — summarize the PAIR (or keep the pair verbatim and drop surrounding prose); never split them across the compaction boundary (projector orphan-drop makes splits silent).
- Summary placement: durable-layer preamble, immune to boundary resets; pin projector tests asserting a summary survives a mutating-command boundary.
- Reference preservation: teach the summarizer prompt to carry forward file paths, command ids, checkpoint ids, todo state VERBATIM (externalized-state pattern: the repo's todo/openspec artifacts are natural anchors — point the summary at them rather than restating contents).
- Cap recursion: a summary that summarizes summaries gets flagged in the transcript; consider re-grounding from artifacts (todo list, diff stats) instead of re-summarizing prose.
- Behavioral evals: extend the existing evalharness net with a "post-compaction continuation" scenario — the regression net is the project's proven safety mechanism.

**Warning signs:**
- Compaction code importing provider.Message types instead of session.Line types.
- Tests that never combine compaction with a mutating-boundary reset.
- Projector changes that special-case "compact" strings rather than typed lines.

**Phase to address:** P5 design (with the verify-first spike), implementation before P2's /compact. The projector/type-line requirement makes this a session-package change — schedule with awareness that P1 resume (Pitfall 5) touches the same package.

---

### Pitfall 8: Thinking blocks break replay, compaction, and multi-provider shaping — signature-exactness vs the redactor, and OpenAI-shape providers that don't have them

**What goes wrong:**
Four distinct failure modes converge here. (1) **Signature corruption on replay**: Anthropic requires every `thinking`/`redacted_thinking` block accompanying the current tool_use exchange to be passed back BYTE-EXACT including the cryptographic `signature`; edited, reordered, filtered, or reconstructed blocks yield 400 `invalid_request_error` (Context7-verified, MEDIUM). This repo's pipeline is full of transformation layers: the REDACTOR scrubs transcript/request content (a redaction pass over thinking text destroys the signature), the projector mechanically rebuilds messages from transcript lines (JSON round-tripping must preserve the raw block verbatim — `plainContent`-style flattening of content arrays would strip signatures), and compaction (Pitfall 7) loves to drop "verbose" assistant content — dropping the thinking block that accompanies the LAST assistant tool_use is an immediate 400. (2) **Provider skew**: the daily-driver providers ride the OpenAI shape (MiniMax M3, DeepSeek) where thinking arrives as `reasoning_content`-style fields or not at all — no signatures, different replay rules; the zcode profile carries an Anthropic-shape thinking config (`shaper.toThinking`). A replay layer that assumes signatures exist breaks on OpenAI-shape; one that assumes they don't breaks on Anthropic-shape. (3) **Streaming duality**: streamed thinking must simultaneously update the transcript (for replay), feed the outbound sequencer as `agent_thought_chunk` (Zed supports the thinking kind — added alongside elicitation in v0.202.0, LOW confidence), and respect the redactor — three consumers, one stream, and backpressure on any one must not corrupt the others. (4) **Config conflicts**: extended thinking constrains `tool_choice` (forced tool choice is incompatible with thinking on Anthropic-shape) and requires budget_tokens < max_tokens — the capability-profile machinery (D-09/D-10 load-time + request-time enforcement) must learn these constraints or the shaper emits invalid requests.

**Why it happens:**
Thinking blocks are invisible in v1.0's text-oriented transcript content model, so every layer treats them as opaque text — and opaque-text transformations are exactly what signatures forbid. Provider skew hides because tests run against one shape.

**How to avoid:**
- Persist thinking/redacted_thinking blocks as RAW JSON on the transcript line (json.RawMessage passthrough — no re-marshal, no redaction, no prettification). The redactor's scope must exclude thinking-block interiors by construction; add a redactor unit test with a signed block.
- Projector: pass raw blocks through verbatim; compaction rule: NEVER drop or alter the thinking accompanying the latest assistant tool_use exchange (drop-older-only).
- Capability profiles gain thinking-support + signature-semantics flags; the shaper branches on shape (anthropic: preserve signatures; openai-shape: map/drop reasoning fields per provider doc; unknown: strip safely and log).
- Stream with fan-out: one reader, three typed consumers (transcript writer, sequencer emitter, redaction-aware logger); backpressure policy from Pitfall 3 applies per consumer.
- Add shaper conformance tests per provider shape asserting the replayed assistant message round-trips byte-identical thinking blocks (golden fixtures from real rollout logs — the Phase-1 discipline).

**Warning signs:**
- `string` (not json.RawMessage) anywhere a thinking block is stored.
- Redactor tests lacking a signed-thinking fixture.
- A single provider shape exercised in the thinking tests.
- 400 errors mentioning thinking/signature in any manual run (immediate stop-and-fix).

**Phase to address:** P4 (CC parity — streamed thinking blocks), but the TRANSCRIPT/RAW-STORAGE decision must land first (it's a schema change the P1 resume work and P7 compaction both depend on — sequence it early in P1 or as its own thin phase).

---

### Pitfall 9: Background Bash orphans — Setpgid detachment means agent death strands process groups, and the editor (not ass-guard) owns the kill

**What goes wrong:**
The TaskRegistry correctly launches each task in its OWN process group (`Setpgid: true`, "the REGISTRY owns this lifecycle") with Stop/ReapAll doing group kills. The subtlety cuts the other way at shutdown: because tasks are detached into their own groups, the agent's own process-group signals (what an editor typically sends on teardown) DO NOT reach them. The distribution constraint says "no daemon — the editor owns process lifecycle," so the common death is SIGKILL/SIGHUP to the agent from Zed — no graceful `Close()`, no `ReapAll`, and (macOS has no PR_SET_PDEATHSIG) no kernel parent-death signal. Result: build servers, watchers, `tail -f`, dev servers started by the model keep running headless on the developer machine after the editor quits — the exact zombie/orphan plague the feature was supposed to manage. Linux can partially compensate (Pdeathsig via SysProcAttr), macOS cannot, and CGO-free static builds get no help from libc tricks.

Second failure mode: **completion-notification races.** Background agents/tasks completing mid-other-turn must notify the client and append results — but the turn mutex serializes turns (queue-behind-active-turn). A naive notifier that emits frames directly violates the sequencer discipline (Pitfall 2); one that grabs turnMu may wait arbitrarily long; one that appends transcript lines concurrently with an active turn races the Manager's mutex (appendLine is guarded, but SEMANTIC interleaving — a background result appearing between a tool_call and its result — confuses projection pairing).

**Why it happens:**
Process-group discipline solves the in-lifetime problem (targeted kills) and creates the at-death problem (nobody's group includes the orphans). Editor-owned lifecycle is a stated constraint, so "we'll clean up on exit" is structurally unavailable in the kill case.

**How to avoid:**
- Accept orphan risk on SIGKILL but shrink the window and the blast radius: (1) install signal handlers for every catchable signal (SIGHUP/SIGTERM/SIGINT) that run ReapAll before exiting — editors usually try TERM first; (2) on Linux set `Pdeathsig: syscall.SIGKILL` per task (pure-Go, CGO-free); (3) on macOS document the limitation in /doctor output; (4) prefer TERM-then-KILL escalation in ReapAll (currently KILL-first) so well-behaved children can shut down.
- Startup sweep: at agent start (and at session/load), detect and optionally adopt/kill stale task logs (`.ass-guard/outputs/*.log` whose owning pid is gone) — cheap, best-effort hygiene.
- Completion notifications route through the outbound sequencer at safe points (between tool calls / at turn end), and their transcript appends happen under a dedicated small lock with a defined interleave policy (append BEFORE the next foreground tool_call, never inside a call/result pair). Never hold turnMu across the notification — enqueue it.
- Registry durability: persist minimal task metadata (id, command, log path, start time) so resume can synthesize "interrupted" closures (joins Pitfall 5).

**Warning signs:**
- Orphaned processes visible after `kill -9` of the agent in tests (add this assertion to the E2E harness).
- Any notification path writing to the ACP writer without going through the sequencer.
- ReapAll not invoked in at least one signal path.

**Phase to address:** P4 (background Bash + persistent shell + background agents land together — shared lifecycle phase).

---

### Pitfall 10: Persistent shell via PTY — master-close doesn't kill children, EOF is a lie (EIO), and ANSI pollution enters the model's context

**What goes wrong:**
A persistent-shell option (CC parity) means one long-lived shell per session, likely via `creack/pty`. Verified failure modes (web-search, MEDIUM-LOW): closing the PTY master does NOT reliably terminate children (grandchildren holding slave fds keep the pty alive; interactive shells ignore/re-parent around SIGHUP); reads on the master return EIO rather than EOF on Linux when the peer exits (bare `io.Copy` reports spurious errors); shells spawn grandchildren that survive naive kills; closing the tty file concurrently with `Wait()` races; output written between `Start()` and the first read is lost. Beyond the library issues: PTY output carries ANSI escapes, cursor moves, and prompt codes — feeding that verbatim into model context wastes tokens and confuses the model (it "sees" terminal garbage as tool output); shell STATE (cwd, env, venv activation, exports) diverges from what the transcript records — after resume (Pitfall 5) the transcript replays `cd src && make` but the fresh shell sits at the workspace root, and subsequent non-shell tools (which use explicit cmd.Dir) disagree with the shell about cwd; and each session's persistent shell is a second long-lived child subject to ALL of Pitfall 9's orphan problems PLUS interactive-job-control (a shell in its own session with Setsid changes signal semantics vs the plain Setpgid used today).

**Why it happens:**
PTY semantics differ from pipes in exactly the places developers assume pipe semantics; and "persistent" state is invisible to a transcript-based replay design.

**How to avoid:**
- Lifecycle: `Setsid` + negated-pid group kills with HUP→TERM→KILL escalation; drain-with-deadline then close AFTER Wait; treat EIO as EOF; coordinate tty close with Wait via done-channel (all verified patterns from creack/pty issues).
- Output hygiene: strip/translate ANSI escapes before they reach BOTH the model window and (ideally) the client frames; cap the returned delta like any bash result (reuse persistOversize).
- State tracking: intercept `cd`/`export` best-effort (shell-integration markers like CC uses, or a wrapping prompt-hook) and RECORD effective cwd/env in the transcript so resume can re-prime the shell; accept and document that full state fidelity is impossible — the transcript records what the model BELIEVED the state was.
- One shell per session, capped count globally; shell joins the TaskRegistry-style lifecycle (signals, sweep, metadata persistence).
- Keep the plain-pipe Bash as default; the persistent shell is opt-in per the parity audit — don't regress the working path.

**Warning signs:**
- Model outputs containing `\x1b[` sequences in tool results (grep transcripts in UAT).
- `io.Copy` error handling without an EIO exemption.
- Any test asserting the shell dies from master-close alone.

**Phase to address:** P4 (with background Bash — same lifecycle infrastructure; do them in one phase, not two).

---

### Pitfall 11: Sandbox "made real" breaks more than it sandboxes — Seatbelt implicit dependencies, Ubuntu's userns wall, and CGO purity

**What goes wrong:**
Three platforms of pain in one feature. **macOS/Seatbelt**: `sandbox-exec` is officially deprecated (still functional through Sequoia; LOW-MEDIUM confidence) with cryptic SBPL failures — `(deny default)` profiles fail on basic spawns because process-exec needs companion `file-read*` on the interpreter AND dyld/libSystem paths; network and mach-lookup must be explicit; Apple provides no support. A profile tuned on one macOS version breaks on the next (operations get renamed/deprecated). **Linux/bwrap**: Ubuntu 23.10+/24.04 restricts unprivileged user namespaces via AppArmor (`kernel.apparmor_restrict_unprivileged_userns=1` default) — plain bwrap fails with "setting up uid map: Permission denied" unless the CALLING binary has an AppArmor profile with `userns` (profiling bwrap itself is wrong when scripts invoke it — the caller needs the profile) or the sysctl is disabled; Debian/Fedora/openSUSE unrestricted today, Arch considering (LOW confidence, distro-dependent and moving). **Build gate**: the mandatory `CGO_ENABLED=0` static build rules out cgo-linked seccomp/libseccomp wrappers; anything pulled in for sandboxing (seccomp BPF generation, namespace libs) must be pure Go or the `mise ci` gate fails. And the meta-pitfall: sandboxing BREAKS LEGITIMATE TOOLS — git needs `.git` writes, openspec/node/python need their install-tree reads and HOME caches, MCP subprocess spawns need exec allows — an over-tight profile converts working features into mysterious EPERMs, violating the investigate-and-fix-ready constraint unless denials are diagnosable.

There's also a product-level trap: v1's Out-of-Scope explicitly listed "no confirmation tier" and the safety model is pattern/hook + manual cancel. Making the sandbox flag REAL changes the documented safety posture — flipping default-on silently changes behavior for existing sessions.

**How to avoid:**
- Ship per-OS reference profiles as DATA (go:embed, consistent with firstrun's embed pattern): macOS = deny-file-write-outside-(workspace,tmp,HOME-cache-read) with broad read + explicit network-deny-list approach rather than deny-default (deny-default is where Seatbelt pain lives); Linux = bwrap argv builder (--ro-bind /, --bind workspace+tmp, --dev --proc) rather than hand-rolled profiles.
- Capability probing at startup with DEGRADE-AND-REPORT: probe bwrap availability + userns permission (cheap dry-run), probe sandbox-exec presence; if unavailable → run unsandboxed with a LOUD stderr warning + /doctor entry + session configOption surfacing, never a hard failure (the flag is "real," not "mandatory").
- Pure-Go only: audit any candidate dep for cgo (seccomp BPF generators exist in pure Go; libseccomp bindings do not qualify). Add a CI assertion that the sandbox packages compile under CGO_ENABLED=0 (already covered by the gate — just don't introduce the dep).
- Denial diagnosability: wrap sandboxed execution so failures surface "sandbox denied: <op/path>" hints (Seatbelt denials appear in the child's errno; log the profile name + offending op path in the structured error), and keep a `--sandbox=off` escape hatch per invocation.
- Default OFF with explicit opt-in (parity with zcode tool semantics per the milestone text); document the posture change in PROJECT.md at ship, don't flip silently.
- Test matrix: real Seatbelt run on macOS CI runner (GitHub mac runners permit sandbox-exec), real bwrap on a Linux runner WITHOUT the userns sysctl disabled (assert the graceful-degrade path fires) — this is the standing real-dependency gate inherited from the v1.0 lesson.

**Warning signs:**
- `(deny default)` in any shipped profile (choose targeted denies instead).
- Any dependency whose build tags mention cgo for seccomp/namespaces.
- Sandbox failures surfacing as bare "operation not permitted" without sandbox context in the error.
- CI testing bwrap only on a sysctl-relaxed runner (tests the happy path that doesn't exist on stock Ubuntu 24.04).

**Phase to address:** P5 (SEED-004 sandbox). Profile authoring deserves its own plan within the phase.

---

### Pitfall 12: Shadow-git checkpoints — nested repos silently unprotected, storage blowup, restore destroying user work, and the store leaking into the USER's git

**What goes wrong:**
The shipped store (`.ass-guard/shadow.git`, isolated env, own `ass-guard.lock`, prune with keep-cap) is solid, but scaling it to "workspace snapshot at EVERY turn boundary + rollback surface" hits five walls: (1) **Nested repositories**: `git add -A` in the shadow repo records nested `.git` directories as GITLINKS (commit pointers, not content) — files inside any nested repo (vendored deps, generated subprojects, the user's OTHER worktrees) are silently NOT snapshotted; restore then force-checkouts the tree and `clean`s "files created after the snapshot" — potentially deleting untracked files INSIDE nested repos that were never protected. Silent data loss shaped like a feature. (2) **Storage growth**: every-turn snapshots of a workspace with binaries/build artifacts bloat `.ass-guard/shadow.git` fast; the prune caps REF count but loose objects accumulate until prune/GC; a single large asset re-committed each turn multiplies. (3) **Restore vs user's live edits**: rollback restores PRE-TURN state — if the user has been editing concurrently (the editor is RIGHT THERE — this is the IDE surface), restore obliterates their unsaved/uncommitted work with no undo; Zed buffers may also conflict with on-disk reverts (stale-buffer overwrite). (4) **Leakage into the user's repo**: `.ass-guard/` inside the worktree — if not git-ignored, the user's `git add -A` ingests the shadow store (objects, possibly secrets in snapshots); IDE git integrations may see `.ass-guard/shadow.git` as a nested repo and warn/churn; conversely the SNAPSHOT scan walks the ENTIRE workspace including `node_modules`-scale trees — snapshot latency at every turn boundary can stall turns on monorepos (the store serializes via withLock, so a slow snapshot also delays the NEXT turn's checkpoint). (5) **Index-lock interactions**: the store's own lock prevents INTERNAL races, but any FUTURE feature running git against the USER's repo (none today — T-14-04 forbids it; keep it that way) would collide with the IDE's git operations on `index.lock`; the moment someone adds "checkpoint the user's HEAD too," they inherit git's fail-fast O_EXCL contention (verified: git retries ~15ms then fails; blind lock deletion corrupts).

**Why it happens:**
Gitlinks-vs-content and loose-object growth are git internals invisible in the happy path (small text-only workspaces); restore semantics are designed around "undo the AGENT's turn" while the actual hazard is "undo while a HUMAN holds the pen."

**How to avoid:**
- Nested repos: DETECT nested `.git` dirs during snapshot (walk or `git ls-files` heuristics) and either (a) refuse-and-report (structured warning naming the path, transcript-recorded) or (b) snapshot them via a second shadow pass with GIT_DIR pointed INSIDE each — pick (a) for v1.2, explicit > silent. Restore must skip nested-repo interiors entirely (never `clean` inside them).
- Storage: extend prune to also expire OBJECTS (`reflog expire + gc --prune=now --aggressive` periodically inside the shadow store, or keep-N-then-gc); add a size ceiling with degrade-to-warning (stop snapshotting, tell the model/operator) rather than filling the disk; consider excluding obvious heavy dirs (respect .gitignore as a floor, add built-in excludes for `node_modules`, target/, dist/ — configurable).
- Restore safety: pre-restore snapshot (checkpoint the CURRENT state before reverting — makes restore reversible); refuse restore when the working tree differs from the checkpoint's post-turn expectation WITHOUT --force, listing dirty paths; document the Zed-stale-buffer caveat in the ACP command surface (emit a client message advising reload after restore).
- Leakage: ensure `.ass-guard/` is in `.git/info/exclude` (local, doesn't touch the user's tracked .gitignore — consistent with "strictly read-only on .claude/" spirit) at store init if not already ignored; verify IDE-facing footprint.
- Latency: measure snapshot time on a realistic tree during the phase; if slow, move snapshots off the critical path (async post-turn with the turn-end marker recorded after completion, or incremental add via `-mtime`/index diff) — but keep serialization (withLock) intact.
- Standing rule reaffirmed: NO checkpoint feature ever invokes git against the user's `.git` (T-14-04). Any proposal to "also track user HEAD" is a design review red flag.

**Warning signs:**
- Checkpoint E2E tests using flat text-only fixtures only (add a fixture with a nested repo + binary file).
- Shadow store size growing linearly with turn count in a soak test.
- Any restore path without a pre-restore safety snapshot.
- `git status` in the user's worktree showing `.ass-guard/` as untracked.

**Phase to address:** P5 (SEED-004 checkpoints/undo). The store exists; this phase is about scale + safety edges, so budget for fixture realism, not core build.

---

### Pitfall 13: Steering queue semantics chosen wrong — mid-turn injection vs queue-behind, and the cancel/enqueue race

**What goes wrong:**
"Steering/input queue during a running turn" has TWO defensible semantics and picking by accident is expensive: (a) QUEUE-BEHIND (today's D-02 discipline — everything queues on turnMu; trivially safe but NOT steering: the user's correction arrives after the agent finished the wrong thing); (b) MID-TURN INJECTION (CC-like: the user message is appended into the running conversation at the next model boundary so the model can course-correct — this is what "steering" MEANS, and it's the Telegram prerequisite per the milestone text). Implementing (b) naively breaks invariants: injecting into the provider request mid-stream conflicts with the projector's pairing logic (a user message appearing between a tool_call and its result — the projector's orphan-drop and split-at-boundary logic assume user messages only START turns), and with the shaper's cache_control placement (mid-conversation insertion invalidates prefix caching — real dollar cost). The cancel race: `session/cancel` drains queued items while a producer enqueues — an item accepted just after the drain sweep begins is orphaned (acknowledged by the client, never processed) or worse, processed AFTER the cancel completed (zombie turn).

Interaction minefield: steering input arriving while an ASK or PERMISSION is parked — is the text an ANSWER to the parked ask or a NEW instruction? The ask-reply path already special-cases prompts during suspension ("lost race" handling in acp_serve.go); steering widens that ambiguity to every turn.

**Why it happens:**
Queue-behind exists and works, so the pressure is to call IT steering. True injection requires touching the turn loop's model-boundary points (between LLM responses / before each next request), which crosses four subsystems (projector, shaper, transcript, sequencer).

**How to avoid:**
- Decide explicitly and DOCUMENT: recommended shape is bounded injection at MODEL REQUEST BOUNDARIES only (input received while the model is generating or tools are executing is held; at the next request-build, queued inputs are appended as user content before the request). Never inject INTO an in-flight HTTP request; never split a tool/result pair.
- Projector: extend (don't hack) — a queued-steering line type appended in order, participating in pairing; add eval scenarios with mid-turn injections.
- Cache economics: acknowledge prefix-cache invalidation on injection; prefer injecting at boundaries where the prefix is already changing (post-tool-result), and note the tradeoff in the design doc.
- Cancel protocol: assign monotonically increasing queue tickets; cancel marks a cutoff ticket — items with ticket ≤ cutoff are drained with acknowledgment (synthetic "cancelled" closure in transcript), items > cutoff survive to the next turn. Test the race with a producer hammering enqueue during cancels under `-race`.
- Parked-ask disambiguation: during a parked ask, incoming TEXT routes to the ask broker FIRST (current behavior preserved); a `/steer`-style escape hatch or non-text content routes to the queue — mirror the existing non-text-prompt rule, write it down.
- Transport-neutral core: the queue API lives in the session/engine layer (Telegram rides it later) — no ACP types in the queue interface (kit-extraction friendly, P6).

**Warning signs:**
- Steering implemented as "write to a channel the turn loop selects on" with no boundary discipline.
- No ticket/cutoff concept in the cancel path.
- Tests never combining steering + parked ask + cancel in one scenario.

**Phase to address:** P5 (SEED-004 steering queue). Design note: pi/strands reference semantics were named in PROJECT.md — consult during discuss-phase (research did not verify those references this round; LOW confidence on their exact contracts).

---

### Pitfall 14: Rich prompt content (@-mentions, images) — token blowups, capability mismatches, and path trust

**What goes wrong:**
ACP ContentBlock is currently text-only in this codebase (`types.go`: Type + Text, "forward-compatible"); v1.2 fills in images and @-file mentions. Failure modes: (1) **Token cost**: images are token-expensive (hundreds-to-thousands of tokens EACH depending on size/model), and screenshots pasted repeatedly in a hands-off debugging loop multiply silently — no confirmation tier means no natural friction point; @-mentions expanding full file contents re-pay tokens on every mention UNLESS prefix caching absorbs them (cache_control placement is load-bearing and the zcode-profile capture pins where zcode puts it — moving blocks around to "optimize" breaks both mimicry-era assumptions AND cache hits). (2) **Capability mismatch**: providers differ in supported image formats (Anthropic-shape: jpeg/png/gif/webp — HEIC/tiff rejected; OpenAI-shape: its own set; MiniMax/DeepSeek vary further) and size/base64 limits; passing an unsupported block through yields a PROVIDER 400 mid-turn with a confusing error instead of an upfront rejection. The capability-profile machinery (load-time + request-time enforcement, D-09/D-10) is the designed home for this and must learn image capabilities, or the enforcement promise is hollow for the new content kinds. (3) **Path trust**: @-mention expansion is an implicit file READ — path traversal (`@../../secrets`), absolute paths outside the workspace, and symlinked escapes all become one keystroke away; the tool layer gates reads, mention-expansion must inherit the SAME gating, and expanded content must be transcript-recorded with provenance (which path, resolved where) per the investigate-and-fix-ready constraint. (4) **Missing/binary**: mentioning nonexistent paths, or binary files (mentioning a .png should attach an image block IF supported, else refuse — silently injecting base64 garbage into text breaks requests).

**Why it happens:**
ContentBlock extension feels like plumbing; the token, capability, and trust dimensions only surface with real usage. The redactor must also LEARN the new shapes (base64 image payloads in transcripts are huge — redact/truncate policy needed for logs).

**How to avoid:**
- Validate at ingress: on session/prompt, check each non-text block against the ROUTED model's capability profile (format, size cap, per-turn image-count cap with a clear structured error); downscale or refuse early, never forward-and-fail.
- Mention expansion goes through the read-tool gate (same allowlist/workspace-root rules as Read); record expansion provenance lines; cap expandable size (large mentions become a reference + truncated preview, consistent with the oversize-output pattern).
- Cache-aware placement: keep user-content ordering stable relative to the captured profile; don't reorder blocks for optimization without re-validating against the zcode corpus (behavioral eval).
- Transcript policy: store image blocks with size/format metadata and the payload (needed for replay fidelity — the model must re-see images after resume) BUT bound total; define the compaction story for images (older images are prime DROP candidates — cheaper than summarizing; pair with Pitfall 8's latest-turn preservation rule).
- Redactor: extend patterns to scrub base64 blobs from AUDIT mirrors while preserving them in the replay transcript (two sinks, two policies — the audit mirror already exists separately).

**Warning signs:**
- Prompt-turn tests with text blocks only.
- No capability-profile field for image support.
- Mention expansion bypassing the toolcat read gate.
- Audit mirrors carrying megabyte base64 strings.

**Phase to address:** P4 (rich prompt content). Ingress validation should land with the P1 session/prompt touchpoints if timeline allows (same file, cheap then, annoying later).

---

### Pitfall 15: Elicitation/create — version skew with older Zed and validation-on-receive

**What goes wrong:**
`elicitation/create` landed in Zed v0.202.0 (2025-07-30, LOW-MEDIUM confidence) in Zed CORE — meaning the skew axis is simply the user's Zed version, not extension APIs. Older Zed replies to the unknown method with JSON-RPC `-32601 method not found` (or ignores, depending on version — verify empirically during the phase); an agent that treats that as fatal fails the TURN for a client-capability reason. Second half: elicitation FORMS are agent-defined schemas answered by a client UI — the returned values can violate the declared schema (missing required, wrong types, extra fields, cancellation disguised as empty submission); trusting client-side validation is the MCP-elicitation lesson repeated. There's also integration debt: elicitation is semantically another SUSPENSION (human-wait) — it must join the AskBroker/permission broker family (Pitfall 1's rules: no locks held, timeout policy explicit, resume-safe) or it becomes a third inconsistent ask mechanism.

**Why it happens:**
Method-not-found handling is untestable without an old client, so it ships broken; schema trust feels safe because "the editor validates."

**How to avoid:**
- Probe-and-degrade: attempt elicitation; on -32601 (or timeout-of-no-capability), fall back to the existing plain-text ask path (AskUserQuestion-shaped) automatically, once per session, with a stderr/transcript note. Record client capability at initialize if ACP advertises one; don't re-fail per ask.
- Re-validate on receive: agent-side schema validation of answers (required present, types coerce-or-reject); on invalid answers, re-ask ONCE with narrowed fields, then fall back to text ask — bounded loop, logged.
- Join the broker family: one suspension abstraction, three surfaces (AskUserQuestion, request_permission, elicitation) sharing timeout/resume/cancel semantics — this is also the cleanest kit-extraction boundary (P6).
- Empirical matrix in UAT: current Zed (form works), one pre-0.202 Zed if obtainable (fallback works), cancel mid-form (settles cleanly).

**Warning signs:**
- A dedicated elicitation code path not sharing the broker.
- No -32601 test (mock client omitting the method).
- Answers consumed without schema validation.

**Phase to address:** P1 (elicitation/create).

---

### Pitfall 16: Hooks PreToolUse deny path — the first SYNCHRONOUS hook breaks the async observer architecture and the "unmatched ⇒ nothing" safety proof

**What goes wrong:**
The unified engine is an OBSERVER on the event bus, explicitly not in the turn's critical path (validated predecessor fact, carried as architectural ground). PreToolUse hooks with a DENY outcome are the opposite: synchronous, in-path, and DECISIONAL — the turn must WAIT for the hook verdict before executing a tool. Naive retrofit options all hurt: running the hook inline in the executor couples tool execution to hook-subprocess latency and failure modes (a hanging hook script now hangs the tool — the Pitfall-1 class again, human-timescales replaced by script-timescales); making deny ASYNC (execute-then-maybe-undo) is not a deny and breaks the safety promise; widening the engine's authority erodes the structural safety property ("unmatched ⇒ nothing runs") that the eval suite proves — a deny-path bug now blocks legitimate work silently, the inverse failure of the injection concern. Additional edges: deny verdicts must produce a TOOL RESULT the model can act on (structured denial, not a hang or a raw exit-code), the deny decision must be transcript-recorded with the matching rule's provenance, and PreToolUse hooks arriving from PROJECT `.claude/settings.json` are REPO-SHIPPED config — the v1.1 prompt-injection concern (repo-controlled content steering an ungated agent) now gets an EXECUTION AUTHORITY it never had (deny others' tools = griefing; allow-others = privilege escalation if hooks can grant).

**Why it happens:**
"Claude Code has PreToolUse hooks" invites porting the FEATURE without respecting that this codebase's engine deliberately sits OUT of the critical path, and its safety model deliberately has no confirmation tier.

**How to avoid:**
- Implement deny as a bounded, synchronous PRE-EXECUTION CHECK in the tool wrapper: pattern/table match first (cheap, deterministic, preserves unmatched⇒nothing), subprocess hook SECOND with a HARD timeout (kill on expiry ⇒ fail-open or fail-closed — DECIDE AND DOCUMENT; recommend fail-open with loud transcript note for v1.2, since the safety model's backbone remains the pattern table, and fail-closed turns a broken user script into a full tool outage).
- Verdicts are structured: {decision: allow|deny, reason} — deny renders as a structured tool result (model-visible) + transcript record with provenance (rule id, source file). Never exit-code archaeology.
- Authority scoping: hooks from project `.claude/` may DENY (restrictive, low abuse value) but the question "can hooks ALLOW what the pattern table gates" must be answered NO for v1.2 (allow-authority from repo-shipped files = the injection escalation). Keep allow-authority config/operator-only.
- Extend the eval suite: matched-deny, hook-timeout-failopen, hook-crash, unmatched-noop — each pinned, preserving the structural-safety proof.
- This dovetails with request_permission (Pitfall 1): define ONE gate pipeline (hook verdict → permission ask → execute) with documented precedence BEFORE building either, so P1 and P4 don't bolt on two incompatible gates.

**Warning signs:**
- Hook execution reachable from inside `toolexec` stubs without a timeout context.
- Any code path where a hook can flip a gated tool to allowed.
- Eval suite unchanged after the deny path lands.

**Phase to address:** P4 (hooks full lifecycle), with the gate-pipeline precedence decision made in P1 (request_permission) design — one pipeline, two consumers.

---

## Technical Debt Patterns

Shortcuts that seem reasonable but create long-term problems.

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| Blocking permission/elicitation ask inline in the executor | Skips the broker/suspend/resume machinery | Human-timescale lock holds; unusable hands-off; rework when it deadlocks (Pitfall 1) | Never |
| Emitting ACP frames from feature code directly instead of the sequencer | Faster feature delivery | Ordering bugs that only appear with concurrent subagents; every later feature re-audited (Pitfall 2) | Never (sequencer exists from day one of P1) |
| Rewriting/compacting transcript lines in place | Simple mental model | Violates the append-only forensic contract; breaks resume replay; investigation impossible (Pitfall 7) | Never — append compact-marker lines instead |
| String-typing thinking/content blocks instead of raw JSON | Easier rendering code | Signature corruption → provider 400s; replay infidelity (Pitfall 8) | Only for display copies; storage is RawMessage |
| Session-scoped "always allow" kept in memory only | Trivial to ship | Decisions vanish on restart; users re-prompted or, worse, re-implemented ad hoc per feature | Only if documented as session-scoped BY DESIGN |
| Ad-hoc second gate pipeline for hooks beside request_permission | Parallel work possible | Two precedence semantics; security-review surface doubles (Pitfall 16) | Never — one pipeline decided in P1 |
| Skipping the real-Zed UAT for protocol features (mock-client-only) | Fast CI | Mocks encode OUR assumptions; Zed's actual rendering/expectations differ (v1.0 stub lesson) | Never for P1 surfaces; mocks supplement, never replace |
| Building compaction before the verify-first zcode-corpus spike | Starts sooner | Reinvents behavior the profile may already encode; behavioral-eval net can't pin invented semantics (Pitfall 6/7) | Never — spike is hours, not days |
| Extracting the kit library (P6) concurrently with feature phases | Feels efficient | Abstractions frozen against churning internals; every feature pays extraction tax twice | Never — P6 stays last per PROJECT.md priority |

## Integration Gotchas

Common mistakes when connecting these features to the existing system and externals.

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| Zed request_permission | Assuming an error arrives on user dismissal | Cancel arrives as a NORMAL response with outcome=cancelled; handle as a first-class outcome, not an error path |
| Zed available_commands_update | Sending incremental diffs of the command list | Updates REPLACE the full command set — send the complete list every time (LOW-confidence web detail; verify against SDK before coding) |
| Zed tool_call streaming | Emitting tool_call for internal engine/hook steps | Catalog tools only; internal steps live in transcript/stderr — keeps UI legible and close-out obligations bounded |
| session/load (resume) | Replay-only implementation | Reconcile ALL live state: brokers, task registry, seq counters, id sequences, command advertisement (Pitfall 5) |
| Anthropic thinking replay | Redacting/round-tripping thinking text freely | Byte-exact passthrough incl. signature; redactor excluded; latest-turn blocks never dropped (MEDIUM, Context7-verified) |
| OpenAI-shape providers | Applying Anthropic thinking/signature rules universally | Shape-branched shaper; capability flags per provider; strip-and-log for unknown shapes |
| Seatbelt profiles | Copying deny-default examples from security blogs | Targeted denies (file-write outside workspace/tmp); explicit read/exec/network allowances; per-version smoke test |
| bwrap on Ubuntu 24.04 | Assuming bwrap present ⇒ bwrap works | Probe with a dry-run container at startup; AppArmor userns restriction makes stock systems fail; degrade loudly |
| User's git worktree | Running any git against the user's `.git` for checkpoint convenience | Never (T-14-04 stands); shadow store is fully isolated; put `.ass-guard/` in `.git/info/exclude` |
| creack/pty persistent shell | Trusting master-close to reap children; treating read errors as failures | Group-kill escalation, EIO-as-EOF, close-after-Wait coordination (MEDIUM-LOW, issue-verified) |
| Editor-driven configOptions | Accepting credential-ish settings from the editor | PROJECT.md already rules: API keys stay env/file, never editor settings — enforce at the configOption whitelist, don't filter later |
| Background agents (subagents) | Giving subagent turn-loops their own frame-writing path | Subagent events funnel through the per-session sequencer with a defined interleave policy (foreground priority) |

## Performance Traps

Patterns that work at small scale but fail as usage grows.

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| Snapshot-every-turn on large worktrees | Turn-end latency spikes; withLock queues next checkpoint | Measure on realistic tree; incremental staging; async snapshot with completion marker | Workspaces with tens of thousands of files / binary assets |
| Shadow-store object accumulation | Disk creep; slow List/for-each-ref | Ref-prune + periodic gc --prune=now inside shadow store; size ceiling with warning | Weeks of daily hands-off use |
| Unbounded bgTask in-memory accumulation | RSS growth proportional to child output | Cap in-memory ring, spill to the progressive log file, report truncation | Long-running watchers/builds (minutes) |
| Frame emission per token/chunk without coalescing | Pipe saturation; client CPU; backpressure stalls (Pitfall 3) | Coalesce deltas per tick; bound queues; drop-and-mark intermediate chunks | Chatty turns: thinking + big tool outputs |
| Full-history projection on resume of long sessions | Load takes seconds-minutes; Zed appears hung | Incremental projection from last boundary; lazy history; show progress | Sessions past ~hundreds of turns / multiple compactions |
| Image-heavy transcripts replayed wholesale | Token costs explode post-resume; slow projection | Compaction drops old images first; per-turn image caps | Debugging loops with screenshots, 10+ images/session |
| Per-request token estimation too coarse | Late compaction → overflow errors (Pitfall 6) | Calibrate estimator against provider counts; safety margin; pre-flight check every request | Immediately after first oversized tool result |

## Security Mistakes

Domain-specific security issues beyond general web security.

| Mistake | Risk | Prevention |
|---------|------|------------|
| "allow_always" permission grants stored project-wide from repo-influenced dialogs | A cloned repo's activity can social-engineer permanent grants; grant scope unclear | Grants keyed per-project + per-tool-pattern, stored operator-inspectable (/permissions), never committed into shared config by the agent |
| Hooks gaining allow-authority from project `.claude/` | Repo-shipped files escalate tool privileges (injection → execution) | Deny-only from project scope for v1.2; allow-authority stays operator config (Pitfall 16) |
| @-mention expansion bypassing read gates | Path traversal reads outside workspace with zero friction | Mentions route through the same workspace-rooted read gate as the Read tool; provenance lines in transcript (Pitfall 14) |
| Base64 images/payloads mirrored unredacted into audit logs | Secret-bearing screenshots/logs persist in plaintext mirrors; mirror bloat | Redactor learns new content kinds; audit mirror truncates blobs with metadata-only placeholders |
| Sandboxed child still inheriting agent's stdout/stderr fds | Child writes corrupt the JSON-RPC stream (transport discipline breach) | Every spawned process (bash, PTY, sandbox wrapper, MCP) gets explicit redirected fds — audit ALL new spawn sites; never rely on inheritance defaults |
| Elicitation answers trusted as validated | Malformed/hostile client data flows into prompts/config paths | Agent-side schema re-validation; coercion bounds; length caps (Pitfall 15) |
| Restore/checkpoint ids built from unvalidated strings | Refspec injection into shadow git | Already handled (validateTurnID strict grammar) — keep the discipline for NEW id surfaces (task ids, permission ids) |

## UX Pitfalls

Common user experience mistakes for these editor-native features.

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| Permission dialog storm (ask per tool call in hands-off runs) | The "hands-off" promise dies by a thousand clicks | Granular patterns (allow-class rules) surfaced at FIRST ask ("always allow Bash git status"-style); /permissions to audit; seeded sensible defaults from the hook table |
| Zombie tool rows in Zed after cancel/crash | UI lies about running work | Terminal-update invariant + resume reconciliation (Pitfall 4) |
| Silent compaction | User's carefully pasted context vanishes; agent "forgets" | Announce compaction as a client-visible event (frame + transcript line); /cost and /context-style visibility of window state |
| Resume that loads but looks empty/wrong | Operator distrusts the must-have feature | Re-send available_commands_update, reconcile tool rows, synthesize interrupted-closures, surface a "resumed N turns" note (Pitfall 5) |
| Restore destroying concurrent user edits | Data loss with the editor OPEN — worst possible surface | Dirty-tree refusal + pre-restore snapshot + advise buffer reload (Pitfall 12) |
| Sandbox failures as bare EPERM | Undiagnosable tool failures blamed on the agent | Structured errors naming sandbox/op/path; /doctor shows sandbox status per backend |
| Slash-command autocomplete missing newly installed skills | Users think skills are broken | Re-send available_commands_update on ecosystem discovery changes mid-session (the method exists for exactly this) |

## "Looks Done But Isn't" Checklist

Things that appear complete but are missing critical pieces.

- [ ] **request_permission:** often missing the cancel-as-normal-response path and the mutex-release-during-wait proof — verify with a test where the client NEVER answers and other turns still proceed
- [ ] **tool_call streaming:** often missing terminal updates on cancel/suspend/panic — verify "exactly one terminal frame per tool_call id" across the E2E cancel matrix
- [ ] **Plan streaming:** often missing updates when the plan changes mid-turn (only sent once at turn start)
- [ ] **session/load:** often missing broker/task/id-sequence/command-update reconciliation — verify by killing -9 mid-turn with a parked ask + background task, then loading
- [ ] **available_commands_update:** often missing re-emission after skill/MCP discovery changes AND after resume
- [ ] **/compact:** often missing the oversized-single-result pre-cap and the post-boundary survival of the summary — verify compaction followed immediately by a mutating command
- [ ] **Compaction + thinking:** often missing the latest-turn signature preservation — verify a thinking-enabled session compacts then continues without provider 400
- [ ] **Background Bash:** often missing signal-handler ReapAll coverage and the orphan assertion after kill -9 — verify no surviving children in the process table post-test
- [ ] **Persistent shell:** often missing EIO-as-EOF and ANSI stripping — grep transcripts for escape sequences during UAT
- [ ] **Sandbox:** often missing the stock-Ubuntu-24.04 bwrap degradation path in CI — verify the probe-and-warn path fires on a non-relaxed runner
- [ ] **Checkpoints:** often missing nested-repo refusal and restore-reversibility — verify with a nested-repo fixture and a restore-of-restore
- [ ] **Steering queue:** often missing the cancel/enqueue race test under -race and the parked-ask disambiguation rule
- [ ] **Thinking blocks:** often missing redactor exclusion and OpenAI-shape branch — golden-fixture round-trip per provider shape
- [ ] **Rich content:** often missing ingress capability checks and mention-expansion provenance — paste a HEIC and a `@../outside` path in UAT
- [ ] **Elicitation:** often missing the -32601 fallback — mock a client without the method
- [ ] **Editor configOptions:** often missing the credential-field whitelist rejection (keys must never arrive via editor settings)

## Recovery Strategies

When pitfalls occur despite prevention, how to recover.

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| Permission deadlock shipped (P1) | MEDIUM | Add broker+suspend path behind the existing seam; migrate incrementally per tool class; transcript already records suspension markers to find affected sessions |
| Frame reordering in production (P2/P4) | MEDIUM | Route all emitters through sequencer (mechanical); add seq-gap telemetry to stderr to quantify historical damage; clients recover on next session |
| Ghost waits after resume (P1) | LOW | Synthetic-closure sweep is additive; run a one-shot repair pass over affected transcripts appending interrupted-results |
| Compaction data loss (P2/P5) | HIGH | Original transcript lines persist by design (append-only) — re-project with fixed projector; no model-facing undo exists, but forensics survive |
| Thinking-signature 400s (P4) | LOW | Strip thinking blocks from the failing request (safe fallback), log, fix storage to RawMessage; provider retries succeed |
| Orphaned background processes (P4) | LOW | Manual cleanup (ps/pgrep by log-file pid records); startup sweep prevents recurrence |
| Seatbelt/bwrap breakage on OS update (P5) | LOW | Degrade path already runs unsandboxed; update embedded profile in next release; /doctor tells the operator |
| Checkpoint store bloat (P5) | MEDIUM | One-shot gc/prune CLI (`ass-guard checkpoint` maintenance verb); size-ceiling warning already halts growth |
| Restore destroyed user work (P5) | HIGH | Pre-restore snapshot (if shipped) reverses it; otherwise editor local history / user's own git — prevention is the only real fix |
| Elicitation failing on old Zed (P1) | LOW | Text-ask fallback is automatic by design; nothing to recover |

## Pitfall-to-Phase Mapping

How roadmap phases should address these pitfalls.

| Pitfall | Feature Area | Prevention Phase | Verification |
|---------|--------------|------------------|--------------|
| 1. Permission ask holds locks | request_permission | P1 | No-answer test: client silent, other turns proceed; mutex released during wait (race test) |
| 2. Out-of-order frames | All streaming | P1 | N-emitter stress test under -race asserting strict sequenceNumber order |
| 3. Backpressure stall | All streaming | P1 | Stop-reading mock client mid-turn; turn completes, memory bounded, state frames intact |
| 4. Open tool_call frames | tool_call streaming | P1 (+P4 resume) | Exactly-one-terminal-frame invariant across cancel/suspend/panic E2E matrix |
| 5. Lossy resume | session family | P1 | kill -9 mid-turn (parked ask + bg task + subagent) → load → no ghosts, continued ids, commands re-advertised |
| 6. Late/defeated compaction | compaction | P5 spike → before P2 | Oversized-result fixture compacts pre-request and recovers post-error once; per-tier window tests |
| 7. Compaction vs two-layer context | compaction | P5 design → P2 | Summary survives boundary reset (projector test); pair-atomicity under compaction; append-only transcript preserved |
| 8. Thinking replay/provider skew | streamed thinking | Early-P1 storage, P4 streaming | Signed-block redactor test; per-shape golden round-trips; compact-then-continue no-400 test |
| 9. BG process orphans | background Bash | P4 | Signal-handler ReapAll coverage; kill -9 orphan assertion; notification interleave test |
| 10. PTY lifecycle/state | persistent shell | P4 | EIO-as-EOF test; group-kill escalation test; ANSI-free transcript assertion; resume re-priming test |
| 11. Sandbox platform walls | sandbox | P5 | Real Seatbelt on mac runner; bwrap on NON-relaxed Ubuntu runner exercising degrade; CGO_ENABLED=0 build green |
| 12. Checkpoint scale/safety edges | shadow-git | P5 | Nested-repo + binary fixture; soak-test store size; restore-of-restore; user-git untouched (status clean of .ass-guard) |
| 13. Steering semantics/races | steering queue | P5 | Injection-at-boundary projector evals; cancel/enqueue race under -race; parked-ask disambiguation test |
| 14. Rich content costs/trust | @-mentions/images | P4 (ingress early-P1) | Capability-reject tests (HEIC, oversize); traversal mention blocked; cache-stable ordering eval |
| 15. Elicitation skew/validation | elicitation | P1 | No-method mock → text fallback; invalid-answer re-ask loop bounded; broker-family integration test |
| 16. Sync hook deny path | hooks PreToolUse | P4 (pipeline decided P1) | Matched-deny/hook-timeout/hook-crash/unmatched evals; no allow-from-project-scope test; gate-precedence doc exists |

Cross-cutting standing gates (apply to EVERY phase): `mise ci` (vet + golangci-lint v2 + CGO_ENABLED=0 build + `go test -race`) at phase close — the -race detector is the primary early-warning for Pitfalls 1/2/3/9/13; real-Zed UAT round for every P1 protocol surface (stub lesson); behavioral-eval extension for every change touching request shape (compaction, thinking, rich content).

## Sources

**Repo-grounded (HIGH — direct inspection, 2026-08-26):**
- `cmd/ass-guard/acp_serve.go` — per-session turn mutex (queue-behind-active-turn), parked-ask resume plumbing, session/load no-op comment (pre-D-09-reversal), WINDOWS #3 forwarder split
- `internal/session/ask.go` — AskBroker suspension/timeout/settle machinery (the pattern request_permission and elicitation must join)
- `internal/session/boundary.go`, `projector.go` — boundary reset semantics, MidTurnWindowMessages=64 rolling tail, orphaned-result drop, splitAtResetBoundary
- `internal/session/manager.go` — transcript line vocabulary (Append* family), mutex-guarded appends
- `internal/coreexec/bash.go` — Setpgid + group-kill + reapGroup straggler discipline, oversize persist pattern
- `internal/coreexec/background.go` — TaskRegistry lifecycle (registry-owned, no ctx cancel), progressive log tee, concurrent cap
- `internal/checkpoint/store.go` — shadow-git isolation (own env, ass-guard.lock O_EXCL, validateTurnID grammar, prune/DefaultKeep, T-14-04 user-git prohibition)
- `internal/toolcat/mutability.go` — more-mutating-wins formula, boundary classification
- `internal/acp/server.go`, `framer.go` — reader/dispatch goroutine model, mutex-guarded buffered Writer, sessionState cancel
- `internal/acp/types.go` — ContentBlock text-only today (forward-compatible shape)
- `internal/shaper/shaper.go` — thinking config shaping (anthropic union), profile seam

**External (graded):**
- ACP schema / request_permission / session/update semantics — https://agentclientprotocol.com/protocol/schema (fetched 2026-08-26; MEDIUM for fetched-page claims: cancel → outcome=cancelled as normal response; no timeout guidance in spec). Community/SDK details (sequenceNumber buffering, available_commands_update full-replacement, session/list-delete not in core protocol) — web-search derived, LOW, verify against https://github.com/zed-industries/agent-client-protocol source during P1 planning.
- Zed elicitation/create landing (v0.202.0, 2025-07-30, Zed core not extension API) — release-notes derived, LOW-MEDIUM.
- Anthropic thinking-block signature rules (exact pass-back; 400 on edit/reorder/filter/reconstruct; accompany tool_use; context-window doc) — Context7 `/llmstxt/platform_claude_llms_txt` (platform.claude.com docs), MEDIUM (verified fetch).
- sandbox-exec deprecation + SBPL pitfalls (implicit deps, process-exec/file-read pairing, network/mach-lookup explicitness) — eclecticlight.co (2025-10), newosxbook.com SB guide, redcanari.com, chromium osx_sandboxing docs — LOW-MEDIUM aggregate.
- Ubuntu unprivileged-userns AppArmor restriction (23.10+/24.04 default-on; bwrap uid-map failures; per-binary profile remedy; Debian/Fedora unaffected) — ubuntu.com blog + discourse release notes + manpages — LOW-MEDIUM aggregate; distro positions are MOVING, re-verify at P5.
- git index.lock mechanics (O_EXCL, ~15ms internal fail-fast, stale-lock hazards, worktree isolation) — git source docs + SO threads — LOW-MEDIUM aggregate.
- creack/pty failure modes (master-close vs child death, EIO-not-EOF, Setsid/group-kill escalation, close/Wait races) — GitHub issues #96/#65/#115/#118 + SO — MEDIUM-LOW aggregate (issue-consistent).

---
*Pitfalls research for: ass-guard-agent v1.2 Claude Code Parity*
*Researched: 2026-08-26 — supersedes the 2026-08-14 v1.1 pitfalls document*
