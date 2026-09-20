---
status: partial
phase: 22-background-execution-sandbox-reality
source: [22-VERIFICATION.md]
started: 2026-09-10T04:10:00Z
updated: 2026-09-11T17:36:26Z
audit_acknowledged:
  milestone: v1.2
  at: 2026-09-20
  gap_snapshot: "partial::scenarios=0"
---

## Current Test

[testing complete]

## Tests

### 1. Darwin live seatbelt battery (22-05 T5 / behavior_unverified)

expected: On a macOS host, GOOS=darwin go test ./internal/sandbox/ -run 'TestSeatbelt|TestPolicySymmetry_SeatbeltShape' -v passes — sandbox-exec -p confinement denies curl connect and outside writes; the rendered profile is allow-default with targeted denies. (The linux landlock leg is live-proven on this host; only the darwin enforcement leg needs macOS hardware.)
result: blocked
blocked_by: physical-device
reason: "blocked, я переехал с mac на linux, mac проверим в другой раз (user moved from mac to linux; darwin live leg explicitly deferred by operator to a future macOS host)"

### 2. Human resolution of the 8 judgment-tier prohibitions (ADR-550 P1-P9)

expected: A human confirms the non-authoritative HELD verdicts for the must-NOT invariants recorded in 22-VERIFICATION.md's Prohibition Disposition section: ids never model-controlled; client turn never preempted; sweep never silently deletes; ask-class decline never bypassed; PTY output never to stdout; never fail open silently; never confine ass-guard itself; green result never implies confinement; no exec site skips wrap while sandbox on.
result: pass

## Summary

total: 2
passed: 1
issues: 0
pending: 0
skipped: 0
blocked: 1

## Gaps
