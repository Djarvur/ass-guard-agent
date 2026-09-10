---
status: testing
phase: 22-background-execution-sandbox-reality
source: [22-VERIFICATION.md]
started: 2026-09-10T04:10:00Z
updated: 2026-09-10T04:10:00Z
---

## Current Test

number: 1
name: Darwin live seatbelt battery (22-05 T5 / behavior_unverified)
expected: |
  On a macOS host (amd64 or arm64), run GOOS=darwin go test ./internal/sandbox/ -run 'TestSeatbelt|TestPolicySymmetry_SeatbeltShape' -v.
  Live seatbelt battery passes: sandbox-exec -p confinement denies curl connect and outside
  writes; the rendered profile is allow-default with targeted denies (never deny-default).
  While there, consider WR-06 (unescaped quotes in seatbelt paths break the filter syntax).
awaiting: user response

## Tests

### 1. Darwin live seatbelt battery (22-05 T5 / behavior_unverified)
expected: On a macOS host, GOOS=darwin go test ./internal/sandbox/ -run 'TestSeatbelt|TestPolicySymmetry_SeatbeltShape' -v passes — sandbox-exec -p confinement denies curl connect and outside writes; the rendered profile is allow-default with targeted denies. (The linux landlock leg is live-proven on this host; only the darwin enforcement leg needs macOS hardware.)
result: [pending]

### 2. Human resolution of the 8 judgment-tier prohibitions (ADR-550 P1-P9)
expected: A human confirms the non-authoritative HELD verdicts for the must-NOT invariants recorded in 22-VERIFICATION.md's Prohibition Disposition section: ids never model-controlled; client turn never preempted; sweep never silently deletes; ask-class decline never bypassed; PTY output never to stdout; never fail open silently; never confine ass-guard itself; green result never implies confinement; no exec site skips wrap while sandbox on.
result: [pending]

## Summary

total: 2
passed: 0
issues: 0
pending: 2
skipped: 0
blocked: 0

## Gaps
