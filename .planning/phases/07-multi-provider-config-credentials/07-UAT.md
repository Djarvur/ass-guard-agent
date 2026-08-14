---
phase: 07-multi-provider-config-credentials
status: testing
started: 2026-08-14
total_tests: 1
passed: 0
failed: 0
---

# Phase 7 UAT — Multi-Provider Config & Credentials

Source: 07-VERIFICATION.md (status: human_needed — all automated gates pass; 1 operator-gated item)

## User-Flow Walk-Through

### Test 1: Live editor-spawned zero-env model turn (operator-gated, OPTIONAL per 07-02-PLAN surfaced assumption)
**Status:** PENDING — awaiting operator
**What to do:** Put a real `api_key` literal in `.ass-guard/scheduling.yaml` (or export the key into the editor's launch environment), spawn `ass-guard acp serve` from an editor (Zed) with ZERO shell env vars, and make one real model turn.
**Expected:** The turn authenticates with the file credential (Source=config); the request goes to the configured base_url with the resolved key; no credential appears on stdout or in logs.
**Why human:** External service integration requires a real key (operator setup). The autonomous proof — `TestEditorZeroEnv_LiteralInConfig` (Source=config, zero env, real adapter) + `TestProviderFactory_WireRoundTrip` (httptest wire: host + X-Api-Key + path) — proves the full config→factory→adapter→wire path; the live end-to-end model turn against a real provider is the only element no automated test can exercise.

## Verdict

0/1 PASS — awaiting the operator-gated live turn. All 14 automated must-have truths verified against the actual codebase (07-VERIFICATION.md); this single item is optional operator verification per the plans' surfaced assumptions, not an autonomous gate.
