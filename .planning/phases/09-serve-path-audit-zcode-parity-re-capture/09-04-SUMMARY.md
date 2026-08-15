---
phase: 09-serve-path-audit-zcode-parity-re-capture
plan: 04
subsystem: zcode-parity-recapture
tags: [aud-05, harvest, thresholds, finding, delegated-checkpoint]
status: complete-with-finding   # 2026-08-16 overnight: the capture happened (autonomous app-server driver, operator-directed), all legs ran; the parity gate's recalibration (stale v1.0-era expectations, profile-independent) is the recorded finding awaiting disposition
---

# Phase 9 Plan 04: zcode parity re-capture Summary (delegated harvest run)

## COMPLETION (2026-08-16 overnight — supersedes the stopped-harvest state below)

The operator disproved the plan's "no CLI/API path" premise ("there IS a cli binary inside the app… talk to it using its stdio protocol") and directed the API route. The scripted workload ran AUTONOMOUSLY against `zcode.cjs app-server --stdio` (driver + mechanism findings: `/tmp/zcode-recapture/`, documented in the drift report): 5 scripted turns via `session/create`/`session/send`, and the mid-session MCP attach/detach via **config-change + `session/resume` boundaries** (resume re-reads config and continues the SAME rollout session file — the only headless-reachable mechanism; config hot-reload, `/mcp` slash commands, and server-death were all tested and ruled out).

**Result: SESSION QUALIFIES** — `sess_3cee56ae` (zcode 0.16.3): 13 request records, 81 distinct tools, 1 subagent file, catalog timeline 79 → 80 (`mcp__recapture_probe__recapture_probe`, tool invoked live) → 79. Legs completed in Pitfall-18 order: drift report committed first (`5a2c7f5`), extractor divergence-class fixes TDD (`d3fcfa1`: querySource gating + mcp__-only transition tolerance + the eternal-SKIP stability-test filename bug), profile re-pinned (`3a250b5`), stability test GREEN on the new pin, run record appended, parity fresh numbers recorded (curated 1/8 on BOTH profile bundles — control-proven stale v1.0-era expectations, profile-independent; from-rollout 1/13 with documented harness artifacts). Full `mise ci` green after refreshing four stale GLM-5.2/thinking test expectations. **Open: the parity recalibration disposition** (re-record curated expectations from current live zcode + fix ExtractTurnsFromRollout's delta/state-pollution artifacts) — see the drift report's follow-up section.

---

**The delegated harvest ran the mechanical thresholds over every existing session in `~/.zcode/cli/rollout/` and found NO session meeting all four selection criteria — every session has a STABLE tool catalog (zero mid-session MCP attach/detach events). Per the operator's delegation (STATE.md 4407710): finding recorded, capture-dependent legs stopped for the morning.**

## The harvest (2026-08-15, mechanical, evidence-backed)

Method: for each `model-io-sess_*.jsonl`, count request records carrying `request.body.{model,system,tools}` (turns), the union of distinct tool names, distinct complete tool-sets across the session (the catalog-change detector), and the file's role (filename + record shape).

| session (short) | role | turns | distinct tools | distinct tool-sets | verdict |
|---|---|---|---|---|---|
| `4440f5a7-3c7a-…` | main | 34 | 105 | **1** | fails the catalog-change threshold |
| `subagent_agent_19cf6ade-…` | subagent | 185 | 98 | **1** | subagent-role (cannot be the pinned MAIN session); stable catalog |
| `subagent_agent_8a003655-…` | subagent | 63 | 98 | **1** | subagent-role (this executor's own session); stable catalog |

Threshold scorecard (runbook §3, applied as SELECTION criteria per the delegation):
- **≥5 user turns** — MET by the main session (34).
- **≥10 distinct tools** — MET (105).
- **≥1 subagent dispatch** — MET in effect (two subagent session files on disk).
- **≥1 mid-session tool-catalog change (MCP attach AND detach)** — **NOT MET by any session.** Every request record in every file carries an identical tool-set (34/34, 185/185, 63/63 records agree).

## The finding

Natural zcode usage on this machine has never attached/detached an MCP server mid-session — the catalog is configured statically per workspace, so rollout sessions show one stable tool-set each. The divergence class the stability re-grounding NEEDS (the mid-session catalog change, Turn 4/5 of the scripted workload) does not occur in natural usage here and will not appear by waiting.

## Disposition (per the delegation's stop rule)

- The capture-dependent legs — drift report, re-extract/re-pin, stability-green-on-new-pin, parity re-baseline — are **stopped** (they have no qualifying subject; pinning the stable-catalog main session would be exactly the vacuous re-grounding Pitfall 17 forbids).
- Morning options for the operator: (a) run the scripted 5-turn workload per `docs/recapture-runbook.md` §3 (the original D-04 path — ~15 minutes interactive; Turn 4/5 are the only steps natural usage never produced), then the automated legs run end-to-end (all machinery green from 09-03); or (b) accept the Phase-1 pin as-is and re-scope AUD-05.

## What IS in place (09-03 output, verified this run)

- The pinned-consumption machinery: `TestStability_WithinSessionExtractionSource` reads coverage.yaml's pinned ID and skips clean while the capture is absent (verified: SKIP naming the runbook).
- The canary proves non-vacuity (green).
- `-zcode-version` flag + manifest/meta stamping + `profile check` provenance footer all green.
- The runbook carries the workload, thresholds, pinning procedure, drift-before-update ordering, and the empty Run record — the operator's 15 minutes completes the chain.

## Verification (re-runnable)

```
ls ~/.zcode/cli/rollout/
go test ./internal/profile/ -run TestStability_WithinSession -v   # SKIP (pinned capture absent) — expected
go test ./internal/profile/ -run TestStability_CanaryDetectsDivergence -v   # PASS
```

---
*Phase: 09-serve-path-audit-zcode-parity-re-capture*
*Harvest recorded: 2026-08-15 (overnight delegated run)*
