---
status: testing
phase: 16-acp-wire-foundation
source: [16-VERIFICATION.md]
started: 2026-08-28T15:10:00Z
updated: 2026-08-28T15:10:00Z
---

## Current Test

number: 1
name: Live-Zed chip==wire confirmation
expected: |
  Pre-stamp chip shows the tier-resolved config model (no longer the profile slug — the operator-observed turn-001 GLM-5.3-vs-glm-5.2 divergence class); after a blob fill, chip and wire model move TOGETHER
awaiting: user response

## Tests

### 1. Live-Zed chip==wire confirmation
Connect a real Zed client with the project tier model differing from the profile slug; read the Model chip BEFORE any editor stamp; then send an initialize _meta blob with a model fill and read the chip again and the next provider request's model.
expected: Pre-stamp chip shows the tier-resolved config model; after a blob fill, chip and wire model move TOGETHER.
result: [pending]

### 2. CR-02 blob-tier edge (product intent)
With an editor stamp set under tier A, deliver an initialize _meta blob whose tier fill resolves to tier B with a different model; observe the effective model of the next turn.
expected: The hook overwrites the prior stamp so chip==wire holds under the NEW tier — confirm this product intent (a layer-backed stamp would make the fill inert instead; only in-memory stamps are overwritten).
result: [pending]

### 3. CR-01(new) turn-scoped cancel semantics (product intent)
Start a turn, cancel it mid-stream, then re-prompt the SAME session id; then logout and re-check.
expected: Cancel keeps the session registered and promptable (re-prompt succeeds; MCP host/transcript/forwarder stay live); reaping happens only on logout or serve teardown — confirm this product intent.
result: [pending]

### 4. WR-01 per-turn hook executor (behavior change)
Run the same hook chain concurrently from two DIFFERENT sessions.
expected: Both run (HOOK-04 in-flight reentrancy guard is now per-turn-instance; loop prevention within one chain unchanged) — confirm cross-session concurrency is the intended behavior change.
result: [pending]

## Summary

total: 4
passed: 0
issues: 0
pending: 4
skipped: 0
blocked: 0

## Gaps
