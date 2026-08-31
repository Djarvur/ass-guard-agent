---
status: complete
phase: 16-acp-wire-foundation
source: [16-VERIFICATION.md]
started: 2026-08-28T15:10:00Z
updated: 2026-08-31T22:15:14Z
---

## Current Test

[testing complete]

## Tests

### 1. Live-Zed chip==wire confirmation
Connect a real Zed client with the project tier model differing from the profile slug; read the Model chip BEFORE any editor stamp; then send an initialize _meta blob with a model fill and read the chip again and the next provider request's model.
expected: Pre-stamp chip shows the tier-resolved config model; after a blob fill, chip and wire model move TOGETHER.
result: pass
evidence: |
  Pre-stamp: headless probe (initialize+session/new, no editor push) against the live binary advertised model=glm-5.2 (tier-resolved; profile slug is GLM-5.3) and _global/model=GLM-5.3; config untouched. Live: Zed re-push (remembered GLM-5.3) landed 1s after spawn and was persisted (config mtime 00:25:59); operator prompt turn-001 wire model GLM-5.3 == chip; operator picked glm-5.2 in the panel (persisted 00:28:29); turn-002 wire model glm-5.2 == chip. Transcript e9e47c41: turn-001 "model":"GLM-5.3", turn-002 "model":"glm-5.2". Deviation note: live Zed sends its fill via set_config_option (research-verified: no initialize blob in real Zed); the literal initialize-blob channel is simulator-pinned (TestZedSimulatorE2E). chip==wire held on all observed paths.

### 2. CR-02 blob-tier edge (product intent)
With an editor stamp set under tier A, deliver an initialize _meta blob whose tier fill resolves to tier B with a different model; observe the effective model of the next turn.
expected: The hook overwrites the prior stamp so chip==wire holds under the NEW tier — confirm this product intent (a layer-backed stamp would make the fill inert instead; only in-memory stamps are overwritten).
result: pass
evidence: Product intent confirmed by operator (2026-08-31): a later initialize-blob tier fill overrides an earlier in-memory editor stamp; chip==wire holds under the new tier.

### 3. CR-01(new) turn-scoped cancel semantics (product intent)
Start a turn, cancel it mid-stream, then re-prompt the SAME session id; then logout and re-check.
expected: Cancel keeps the session registered and promptable (re-prompt succeeds; MCP host/transcript/forwarder stay live); reaping happens only on logout or serve teardown — confirm this product intent.
result: pass
evidence: Product intent confirmed by operator (2026-08-31): cancel-and-continue is the intended semantic; reaping only on logout/serve teardown.

### 4. WR-01 per-turn hook executor (behavior change)
Run the same hook chain concurrently from two DIFFERENT sessions.
expected: Both run (HOOK-04 in-flight reentrancy guard is now per-turn-instance; loop prevention within one chain unchanged) — confirm cross-session concurrency is the intended behavior change.
result: pass
evidence: Product intent confirmed by operator (2026-08-31): cross-session concurrent hook chains are the intended behavior change; per-turn reentrancy guard accepted.

## Summary

total: 4
passed: 4
issues: 0
pending: 0
skipped: 0
blocked: 0

## Gaps

[none]
