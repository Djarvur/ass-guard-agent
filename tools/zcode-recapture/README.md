# zcode-recapture driver kit (archived 2026-08-17)

Archived from `/tmp/zcode-recapture/` before /tmp reaping, per the accepted Phase-9 UAT
disposition (2026-08-16/17) and Phase-12 discuss decision D-04
(`.planning/phases/12-product-functional-completeness/12-CONTEXT.md`).

**Why this kit matters:** it is the only proven mechanism for producing a qualifying
divergence-prone zcode capture session (app-server stdio driver + scripted workload +
MCP probe). The Phase-9 pin (`sess_3cee56ae`, zcode 0.16.3) was produced with it, and
Phase 12's ACP-07 re-record route depends on running it again against current live zcode.

**Data-loss fact (2026-08-17):** the pinned rollout file
`model-io-sess_3cee56ae-cc6a-43a6-8f00-a08eb266e1aa.jsonl` has rotated off
`~/.zcode/cli/rollout/` and is absent from `/tmp` — it could NOT be archived. Only the
session side-dirs survive under `~/.zcode/cli/{artifacts,agents,exec}/sess_3cee56ae…`.
The surviving request-record ground truth is `capture-line-base.json` /
`capture-line-probe.json` below (redacted at capture — `REDACTED` markers present).

## Contents

| File | What it is |
|------|------------|
| `zcode-driver.mjs` | The app-server stdio driver (spawns zcode headless, drives a scripted workload; reads the API key from env — no credentials embedded) |
| `scripted-workload.mjs` | The divergence-prone 5-turn workload (the runbook's scripted procedure, automated) |
| `probe-server.mjs` | The MCP `mcp__recapture_probe` server (mid-session attach/detach) |
| `hotreload-test.mjs`, `hotreload-test2.mjs`, `kill-test.mjs`, `resume-test.mjs`, `smoke.mjs` | Mechanism probes used while building the driver (config hot-reload, kill, session/resume boundary, smoke) |
| `capture-line-base.json`, `capture-line-probe.json` | Full request-record dumps from the capture legs (base + probe lines; redacted) — the surviving partial ground truth |
| `parity-results.json`, `parity-curated-results.json`, `parity-oldprofile-results.json` | Fresh parity numbers recorded in `profiles/zcode/drift-reports/2026-08-16-recapture.md` (new, curated, old-bundle control) |
| `cli-config-backup.json` | Driver-run backup of zcode CLI config (skills/plugins only — verified no credentials) |
| `rollout-watermark.txt` | The rollout-dir file listing at capture time (provenance for what existed) |
| `ci-final.log`, `zcode-version.txt` | Final gate log + captured zcode version (0.16.3) |

## Re-running a capture

Follow `docs/recapture-runbook.md`; this kit automates its scripted procedure. Secrets
discipline: the driver reads `ANTHROPIC_API_KEY`/`ZAI_API_KEY` from the environment;
capture dumps pass through the redaction pipeline — never commit unredacted captures.

## Provenance

- Produced 2026-08-15→16 during Phase 9 AUD-05 (the operator-directed app-server stdio
  driver deviation, accepted at UAT).
- Archived verbatim (`cp -p`, permissions preserved) by quick task `260817-uv3`.
