---
status: complete
phase: 15-internal-runtime-carve-step-0
source: [15-VERIFICATION.md]
started: 2026-08-26T00:00:00Z
updated: 2026-08-27T13:25:59Z
---

# Phase 15 UAT — Human Verification Tests

## Current Test

[testing complete]

## Tests

### 1. Live-Zed editor-session identity
expected: Spawn ass-guard acp serve from Zed exactly as in daily use; send a prompt; exercise a tool call (file read/edit); restart Zed mid-session. Native streaming, native tool diffs, restart replay matches v1.1-close behavior.
result: pass
reason: "Parity with v1.1-close PROVEN after diagnosis: internal/acp/ byte-identical since v1.1 (empty git diff), loadSession:false + session/load -32601 is exactly what v1.1 shipped — Zed aborts client-side before sending session/load, so explicit history-resume failed identically at v1.1 close. Streaming/tool-diff legs operator-confirmed (test 2); resume UX gap tracked under Deferred Follow-Ups, fix owned by Phase 18 (18-01/18-04). Diagnosis: .planning/debug/zed-session-resume-unsupported.md"

### 2. Native streaming + tool diffs (live-Zed checklist steps 1–3)
expected: Prompt streaming renders natively (session/update chunk-forwarder path); tool call diffs render natively and tool result returns (catalog executor path) after the carve.
result: pass

## Summary

total: 2
passed: 2
issues: 0
pending: 0
skipped: 0
blocked: 0

## Gaps

[none — G-15-1 reclassified as deferred follow-up after root-cause diagnosis (#1921): behavior is v1-by-design, fix owned by Phase 18]

## Deferred Follow-Ups

- test: 1
  idea: "Session resume from Zed history errors ('Failed to Launch — Loading or resuming sessions is not supported by this agent.') — by-design v1 scope (original D-09 'NO REPLAY IN v1'); fix is Phase 18 (18-01 loadSession:true + reconcile-then-replay, 18-04 sessionCapabilities), already planned READY TO EXECUTE. Parity with v1.1-close verified (empty git diff v1.1..HEAD -- internal/acp/; live-wire reproduction in .planning/debug/zed-session-resume-unsupported.md)."
  deferred_at: 2026-08-27
