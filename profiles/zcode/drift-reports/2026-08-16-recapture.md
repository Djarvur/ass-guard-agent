# Drift Report — zcode profile re-capture (AUD-05, plan 09-04)

**Date:** 2026-08-16
**New pinned session:** `sess_3cee56ae-cc6a-43a6-8f00-a08eb266e1aa` (main) + `subagent_agent_26954a13-f6f1-4028-bfa6-764d66cb7577` (subagent)
**zcode version at capture:** `0.16.3` (`zcode --version`, verbatim: `zcode 0.16.3`)
**ass-guard commit at capture:** `7b9a551`
**Old pin superseded:** `sess_016eee8a-f2f0-44c3-abf9-9d57a2cf04a1` (extracted 2026-08-11, extractor `extract-profile/01-02`, zcode version **unrecorded** — the provenance gap this re-capture closes)

## Capture method (operator-authorized deviation from `checkpoint:human-action`)

Plan 09-04 locked the scripted workload as operator-run on the premise "there is no CLI/API path" to drive zcode. That premise is false and the operator directed the API route on 2026-08-16 ("there IS a cli binary inside the app … you can run it and talk to it using its stdio protocol"). The workload was driven autonomously against `/Applications/ZCode.app/Contents/Resources/glm/zcode.cjs app-server --stdio` (the internal line-delimited JSON-RPC documented by github.com/william0wang/zcode-acp):

- The 5 scripted turns (read/search variety → mutation variety → subagent fan-out → MCP-attached turn invoking the probe tool → mixed finale) ran via `session/create` + `session/send` in `mode:"yolo"` in a scratch workspace (`/tmp/zcode-recapture-ws`).
- The mid-session MCP **attach/detach** — unreachable via config hot-reload (config is read once at process start), `/mcp` connect/disconnect (TUI-client-side interception — via `session/send` it reaches the model as plain text), server-process death (catalog stays frozen), or any session/* RPC — was produced via **config-change + `session/resume` boundaries**: resume in a fresh process re-reads config and continues the SAME rollout session file. The probe server (`recapture_probe`, a 30-line stdio MCP server) was added to `~/.zcode/cli/config.json` `mcp.servers` before the attach resume and removed before the detach resume; the config was restored from backup afterward.
- Driver + probe artifacts: `/tmp/zcode-recapture/` (zcode-driver.mjs, probe-server.mjs, scripted-workload.mjs + the mechanism tests that ruled out hot-reload/kill/`/mcp`).

## Threshold verification (runbook §3 rejection thresholds — all PASS)

| Threshold | Observed | Verdict |
|---|---|---|
| ≥5 user turns | 5 scripted prompts → 13 request records | PASS |
| ≥10 distinct tools | 81 distinct tool names in `request.body.tools` union | PASS |
| ≥1 subagent dispatch | `subagent_agent_26954a13…` file on disk (T3 fan-out) | PASS |
| ≥1 mid-session tool-catalog change | 3 distinct tool-sets, timeline below | PASS |
| attach AND detach | both transitions present | PASS |

Tool-set timeline (per request record, ground truth from the session file):

```
records  1–9:  79 tools, no probe          (T1 read/search, T2 mutation, T3 subagent; record 2 = 1-tool lite request)
records 10–11: 80 tools, mcp__recapture_probe__recapture_probe present   (T4 — ATTACH; model invoked the tool, live output confirmed)
records 12–13: 79 tools, probe gone        (T5 — DETACH)
```

## Drift classification (old shipped profile vs new capture) — zcode-changed vs extractor-changed

`ass-guard profile check zcode --capture-file <line>` against the OLD manifest (`extract-profile/01-02`, pinned `sess_016eee8a`):

| Finding | Old manifest | New capture | Classification |
|---|---|---|---|
| `request.body.tools` count (TIER-1) | 103 | 79 (base) / 80 (probe-attached) | **zcode-changed** — target moved: zcode 0.16.3 bundles a different base catalog than the machine state at the 2026-08-11 capture (plugin/MCP config differs; the old pin recorded no zcode version, so the delta cannot be attributed to a version we can name — which is itself the superseded pin's provenance gap) |
| `request.body.thinking` missing (TIER-1) | present | absent in base-catalog records | **zcode-changed** — current zcode 0.16.3 + GLM-5.3 requests emit no `thinking` field in these records (the probe-attached record carries it — field presence varies by turn state in 0.16.3) |

No extractor-changed drift: the old extractor (`extract-profile/01-02`) and the current one agree on field paths; the counts moved because the target moved. Per Pitfall 18 this report is committed BEFORE the profile update commit.

## Parity re-baseline

**PENDING** — appended below after re-pin + the `ZAI_API_KEY`-gated parity run (never gate extraction on the key).
