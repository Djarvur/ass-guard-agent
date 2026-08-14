---
phase: 01-mimicry-mvp-north-star-proof
status: passed
started: 2026-08-13
completed: 2026-08-13
total_tests: 5
passed: 5
failed: 0
---

# Phase 1 UAT — Mimicry MVP

## User-Flow Walk-Through

### Test 1: Send a prompt through ass-guard with the zcode profile
**Status:** PASS
**What happened:** Ran `ass-guard --profile zcode --prompt "Read the file go.mod and tell me the module name"`. The model returned a `Read` tool-call with `file_path: go.mod`. Exit code 0.

### Test 2: Profile artifact exists with coverage manifest
**Status:** PASS
**What happened:** `profiles/zcode/` contains 3 system blocks, 103 tools, `coverage.yaml` with `target_capture_ref`, `meta.yaml` with D-16 data-source strategy + RESEARCH-FLAG-01 held-out split. Extracted from real zcode rollout (not hand-written).

### Test 3: ass-guard profile check zcode (drift detector)
**Status:** PASS
**What happened:** `TestProfileCheck_NoDrift` and `TestProfileCheck_DriftDetected` both pass. The drift detector correctly identifies matching and mismatched captures.

### Test 4: Structural request indistinguishability (the thesis)
**Status:** PASS — THESIS PROVEN
**What happened:** ass-guard's shaped request carries: model=GLM-5.2, 3 system blocks (byte-identical to zcode), 103 tools, thinking={enabled,32000}, tool_choice={auto}, stream=true. All profile-driven fields match zcode's actual request.

### Test 5: Audit log captures verbatim shaped request (LOG-01)
**Status:** PASS
**What happened:** The tracer's debug output includes the full verbatim request body (model, system blocks, tools, messages, thinking, tool_choice). This is the Phase-1 audit foundation; Phase 2 expands it into the transcript=audit-log.

## Verdict

**5/5 PASS** — Phase 1 delivers its promised value. The mimicry thesis is empirically proven: ass-guard's shaped outgoing request is structurally indistinguishable from zcode's actual request on all profile-driven fields.
