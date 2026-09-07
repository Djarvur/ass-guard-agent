---
status: complete
phase: 19-compaction-cache-control
source: [19-VERIFICATION.md]
started: 2026-09-06T20:30:00Z
updated: 2026-09-07T00:10:00Z
---

## Current Test

[testing complete]

## Tests

### 1. Architect decision — same-turn marker carve-out vs next-turn semantics (CR-01 / deferred-items.md)
expected: One of: (a) accept next-turn recovery semantics (criterion 3's retry rescues the session, not the producing turn) and re-pin the criterion wording; or (b) rule the producing turn must be rescuable and schedule the same-turn carve-out (per-turn override projecting the retry turn as post-marker) with a content-sensitive regression test.
result: issue
reported: "(b) — operator ruling 2026-09-07: the producing turn must be rescuable; schedule the same-turn carve-out with a content-sensitive regression test."
severity: major

### 2. Operator live-LLM compaction check — run a real long session past the 80% threshold (or force a low threshold via set_config_option), let it compact, continue working
expected: Conversation continues coherently with earlier-turn context preserved via the summary (where v1.1 silently lost turns); note that compaction is currently INVISIBLE to the user (the compacting note degraded to stderr+counter — no session/update status frame exists in the landed v1 vocabulary), which itself may need a UX decision.
result: issue
reported: "Operator 2026-09-07: could not cause context overflow — setting the threshold to 5 did not produce any observable compaction in a live session. Could not tell whether compaction fired invisibly (stderr-only note) or never fired (live-apply / threshold logic in a real session)."
severity: major

## Summary

total: 2
passed: 0
issues: 2
pending: 0
skipped: 0
blocked: 0

## Gaps

- gap_id: G-19-1
  truth: "On a provider overflow error, the producing turn completes after compaction + a single retry — the retry request projects post-marker (carries the summary), not a byte-identical resend"
  status: failed
  reason: "Operator ruling (b) 2026-09-07: the producing turn must be rescuable. As implemented, 19-03's test-pinned position rule (compactionMarkerIdx — projector.go:308-320, markers reset only turns whose user message follows the marker) makes the retry window byte-identical to the rejected request, so a deterministic overflow fails the turn after exactly one retry; recovery reaches the next turn only. Found independently by the 19-04 executor (deferred-items Rule-4 entry) and code review CR-01; the green TestCompaction_OverflowRetryOnce 'recovery' leg cannot detect it because its fake provider (compaction_test.go:311-338) is content-blind."
  severity: major
  test: 1
  artifacts:
    - internal/session/session.go:548-553 (overflow retry path — force-compact + continue)
    - internal/session/projector.go:308-320 (compactionMarkerIdx position rule: i < userIdx)
    - internal/session/compaction_test.go:311-338 (content-blind fake provider)
    - .planning/phases/19-compaction-cache-control/19-REVIEW.md (CR-01; WR-03 rider)
    - .planning/phases/19-compaction-cache-control/deferred-items.md (same-turn carve-out sketch: TurnID == projected turn, no pre-user marker present, subagent-safe by TurnID key)
  missing:
    - per-turn override on the retry path projecting the producing turn as post-marker (engine sets it; projector honors it only for the matching TurnID with no pre-user marker present)
    - content-sensitive regression test: fake provider rejecting by payload size, proving the retry window shrank and the turn completes
    - WR-03 rider: bound the summarize input near-limit and stop the per-loop-head re-fire (up to 64 guaranteed-failing summarize calls per turn)
- gap_id: G-19-2
  truth: "A live session at a forced-low threshold (e.g. 5%) observably compacts — or at minimum, the operator can determine from the editor whether compaction happened"
  status: failed
  reason: "Operator 2026-09-07: setting compaction-threshold to 5 produced no observable compaction in a real session. Indistinguishable from the user's seat whether (i) the live-applied threshold didn't govern the real session's comparison, (ii) the threshold check didn't fire (usage estimate / limit base — see 19-REVIEW WR-02 writer-lag double-count and WR-06 runner-level limit), or (iii) it fired invisibly (the compacting note degraded to stderr+counter by pre-authorized design — no session/update status frame exists in the landed vocabulary). The designed invisibility alone makes the feature unverifiable and undiscoverable from the editor."
  severity: major
  test: 2
  artifacts:
    - internal/session/compaction.go (maybeCompact loop-head check, usage estimate, D-09 degrade)
    - internal/session/session.go (loop-head wiring)
    - internal/acpserve/config_surface.go (set_config_option live-apply chain; WR-01 initialize-blob relay gap)
    - .planning/phases/19-compaction-cache-control/19-REVIEW.md (WR-01, WR-02, WR-06)
    - internal/session/compaction_test.go (TestCompactionLive_MenuThresholdRoundTrip — proves live-apply in the harness, not in a real Zed session)
  missing:
    - diagnosis: root cause among (i) live-apply not reaching the real session, (ii) threshold check not firing (estimate/limit base), (iii) fired-but-invisible
    - observability: a user-visible compaction surface (session/update frame of an existing update class, or an explicit note) — needs a UX decision given the landed v1 vocabulary


