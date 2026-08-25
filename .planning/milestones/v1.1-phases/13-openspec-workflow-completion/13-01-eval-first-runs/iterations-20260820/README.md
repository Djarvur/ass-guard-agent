# 13-01 Task 1 gate iterations — 2026-08-20 (post re-tuning)

Four gate runs after the stage-transition re-tuning (seeded.toml rows
`post-propose-handoff` / `post-apply-handoff`, this dir's parent README). The
re-tuning itself is PROVEN: the 12-08 apply-re-fire loop is gone and the chain
now fires explore→propose→apply→archive-injection. The gate stays RED on a
NEWLY DIAGNOSED failure class the pattern table cannot fix.

| iter | verdict | dur | shape |
|------|---------|-----|-------|
| 1 | RED | 211s | 3 continues, 3 msgs, provenance [explore,propose,apply,archive]; archive dir empty. Ask-class exit 28s after the 3rd decision (transcript reaped — poller not yet built; artifact + run.log only) |
| 2 | (killed) | 15s | process-group kill when the launching shell exited — no verdict; superseded |
| 3 | RED (infra) | 124s | TLS handshake timeout to api.z.ai on the propose turn's model call (artifact + run.log + transcript) |
| 4 | RED | 55s | 1 continue, 1 msg, provenance [explore,propose]; the PROPOSE turn ended mid-tool-loop with NO assistant message, NO engine decision, nil RunPrompt error (artifact + run.log + transcript, poll-copied pre-ask) |

## The diagnosis (iters 1 + 4, code-verified)

A mid-chain AskUserQuestion suspends the turn: `session.Prompt` returns the
internal `stopAsk` marker immediately (`session.go` suspension branch), the
engine's `Observe` loop exits on `stop != "end_turn"` WITHOUT deciding
(`observe.go` post-injection check), and the D-01 askTimeout=45s timer — the
dispositioned hands-off mode — fires ~45s LATER, detached: `resumeAskClaimed`
appends the non-answer and resumes the turn at the session layer, OUTSIDE the
engine. The eval harness asserts after `RunPrompt` returns (i.e. at the
suspension), before the timer even fires.

Evidence fits exactly:
- nil RunPrompt error + no assistant_message for the asking turn + no
  engine_decision for it — `stopAsk` is the only stop that skips
  `AppendAssistantMessage` and the post-injection decision while returning a
  nil error (max_tokens appends the message; cancelled carries ctx death).
- iter-4's transcript ends mid-tool-loop (`openspec new change` →
  `openspec status` → usage) 14s before the assertion.
- iter-1: ask at the archive stage = the 12-08 run-2 shape; iter-4: ask at
  the propose stage — the model asks nondeterministically at any stage
  (2 of 3 completed runs).

## Why this is STOP-and-surface (not a re-tuning miss)

- The pattern rows are proven discriminating on every observed closing
  (parent README): the chain works wherever no ask intervenes.
- The dispositioned config-only route ("askTimeout=45s resolves the
  suspension class") does not hold for CHAINED turns: the timer resume is
  asynchronous and engine-invisible — the chain is already dead when the
  non-answer arrives.
- No pattern-row change can make the chain survive an ask: the exit is in
  engine/session behavior, which Task 1's amendment holds byte-untouched
  (Pitfall 18 discipline; "a pattern that cannot be tuned green ... is a
  STOP-and-surface finding, not a pass").
- A runner-level ask-drain (wait for the pending ask + detached resume)
  would only green the iter-1 shape (ask at the LAST stage); the iter-4
  shape (ask mid-chain) still loses all remaining injections because Observe
  has exited. Re-driving stages from the harness would fake zero-continue —
  rejected.

## Fix-route options for the manager (architectural, Rule 4)

1. Engine-visible resume: `Observe` treats `stopAsk` on an injected turn as
   "block until the broker resolves, then re-read the turn" (the resume must
   re-enter the engine loop, not run detached). Touches engine + session
   resume plumbing; safety pins re-pinned.
2. Synchronous D-1 timer inside Prompt: the suspension branch blocks on the
   broker (reply or timer) instead of returning `stopAsk`, making the
   non-answer part of the SAME turn (the live-client flow keeps its async
   path). Touches session internals; ACP ask UX re-verified.
3. Re-disposition: accept "ask-capable model" and drop the flagship's
   zero-continue requirement to the stages before the first ask — weakens
   the assertion; operator call.

D-06 note: iter-4 was the single zero-delta re-run of the ask-class failure;
a third run would be best-of-N. Both ask-class failures and the infra flake
are recorded above.
