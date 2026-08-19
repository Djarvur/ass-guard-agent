# Phase 12 — Deferred / Out-of-Scope Items

## 2026-08-19 — 12-01 execution

**Pre-existing, discovered by the 12-01 live witness (do not lose):** the ACP
Writer's decoded-newline transport guard (`internal/acp/framer.go`
`containsDecodedNewline`, v1.0) silently DROPS any session/update frame whose
decoded text contains a literal `\n` — the write error is swallowed by the
chunk forwarder's `_ = emit`. Live witness session `d9f98023`: 450 transcript
chunks vs 440 wire frames = 10 dropped (9 of the model's own newline-carrying
text chunks + the multi-line ask surface, pre-fix). The 12-01 fix made the ASK
surface single-line (within plan scope); the model-chunk drop class is
PRE-EXISTING and out of 12-01's scope (relaxing the guard reverses a documented
v1.0 transport decision — `transports.md` reading — pinned by its own tests).
Recorded in `.planning/WINDOWS.md` (open). Suggested disposition route: a small
dedicated fix task or fold into 12-04/12-05 verification with the operator's
call on the guard's spec reading (the raw-byte marshal check already enforces
the actual wire invariant).

**Known limitation (12-01, by-design v1 scoping):** the D-01 timer-driven
resume runs the resumed turn server-side (transcript + engine path fully
recorded — live-proven in witness session `e253bfbd`), but its streamed chunks
are NOT mirrored to the ACP client (no active `Run` chunk-forwarder
subscription at timer-fire time). The reply path IS fully client-visible
(live-proven leg 1). Recorded in `.planning/WINDOWS.md` (open). Natural home
for the fix: a serve-lifetime session-update forwarder — candidate for 12-07's
cron-firing machinery (the same engine-driven-turn visibility problem).
