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

## Freshness record (12-05 Task 1, 2026-08-20)

- **Protocol drift: NONE.** Current `zcode.cjs --version` is still `0.16.3` (==
  `zcode-version.txt`); `session/create` + `session/send` + config-change +
  `session/resume` boundaries all worked unmodified.
- **Kit-freshness fixes required after the archival** (the kit moved
  `/tmp/zcode-recapture/` → `tools/zcode-recapture/`, and /tmp reaped the originals):
  - `scripted-workload.mjs` pointed the probe server at `/tmp/zcode-recapture/probe-server.mjs`
    (GONE) — now KIT-RELATIVE (`import.meta.url`).
  - The config restore read `/tmp/zcode-recapture/cli-config-backup.json` (GONE) inside a
    `catch {}` — a silent no-op that would have LEFT the probe entry in the operator's
    `~/.zcode/cli/config.json`. Now the workload takes a FRESH backup to
    `/tmp/zcode-recapture-run/cli-config-backup.json` BEFORE any touch and restores +
    diff-verifies it in a process-`exit` handler (non-zero exit on diff — T-12-05-01).
  - `zcode-driver.mjs --selftest` added: side-effect-free readiness probe (binary present,
    enabled provider with key in `~/.zcode/v2/config.json`, kit files intact).
  - The workload now SEEDS the scratch workspace (`/tmp/zcode-recapture-ws`: README.md,
    calc.js, test.js, package.json) — the original run seeded it by hand.
  - The archived mechanism probes (`hotreload*.mjs`, `kill-test.mjs`, `resume-test.mjs`)
    still carry the dead `/tmp/zcode-recapture/` paths — left VERBATIM (archival copies,
    not operative; do not run them as-is).
- **Freshness proof run (2026-08-20, session `sess_0f01a95d-d56f-4cc4-9ba3-414597a0b853`):**
  the 5-turn workload ran end-to-end against live zcode under the operator's logged-in
  `builtin:zai-coding-plan`; RESULT: SESSION QUALIFIES — 14 request records (>=5), 81
  distinct tools (>=10), 1 subagent file, tool-set timeline mechanically verified
  79 → 80 (`mcp__recapture_probe__recapture_probe` attached) → 79 (detached); config
  restore diff-empty. Log: `/tmp/zcode-recapture-run/task1-freshness-2026-08-20.txt` (in-kit copy; the live
  rollout pair: main `model-io-sess_0f01a95d…jsonl` + subagent
  `model-io-sess_subagent_agent_aabeb358…jsonl`). NOTE: one aux single-tool request
  record appears inside T1 (harness artifact, same class as the drift report's); T5's
  model SELF-REPORT claimed the probe was still attached — the wire timeline disproves
  it (models cannot introspect their catalog; the request records are ground truth).
- **Side-dirs salvage check (12-CONTEXT Discretion item, read-only):**
  `~/.zcode/cli/{agents,artifacts,exec}/sess_3cee56ae…` survive but hold only the OLD
  pin's ordinary machinery — `artifacts/` = 2 Edit-tool diff JSONs (scratch calc.js /
  test.js before/after), `agents/` = 1 subagent transcript (5,908 streaming records,
  Bash/find calls only), `exec/` = EMPTY. ZERO deferred-tool content (the old workload
  never exercised them — the very gap 12-05's tour closes). Nothing folded into the
  harvest; the fresh capture is the sole ground truth.

## Runbook addendum: the deferred-tools tour + cleanup (12-05 Task 2)

The scripted workload's tour legs (run AFTER the base 5 turns, same session — the
capture then carries both the divergence thresholds AND the deferred-tool forms):

1. **ask answered** — a turn instructing the model to ask a question before proceeding;
   the driver answers via a follow-up `session/send` (or the app-server's ask
   interaction) while the turn holds its question state.
2. **ask non-answer (yolo)** — the same shape with NO answer sent; the runtime's
   ask-timeout path produces the non-answer result.
3. **plan mode** — EnterPlanMode then ExitPlanMode with a real plan text.
4. **subagent + SendMessage** — a fan-out turn, then SendMessage to the spawned agent id.
5. **ReadSessionContext** — targeting the capture session itself.
6. **cron quartet** — CronCreate (delayMinutes ONLY — never recurring; bounded operator-env
   footprint) → CronList → CronUpdate → CronDelete.
7. **background bash** — Bash `run_in_background` of a sleep → TaskOutput (block + non-block)
   → TaskStop.
8. **Bash timeout leg** — timeout 10000 on a `sleep 30` (the timeout form hunt).
9. **truncation hunt** — a command emitting ~200KB (yes/seq loop).
10. **cleanup proof** — the final step proves CronList EMPTY after the deletes (the
    workload exits non-zero otherwise), and the config restore diff-empty check runs on
    every exit path.

## Capture record (12-05 Task 3, 2026-08-20)

The forms harvest ran as four live passes (logs in-kit below; every config
restore diff-verified clean; no automation ever created in the operator env —
verified: no `recapture tour marker` hit outside this agent's own artifacts):

1. **Primary** (`task3-tour-2026-08-20.txt`): full base workload + 10-leg tour,
   session `sess_6e4b5cc7` — QUALIFIES (38 recs / 81 tools / subagent /
   79→80→79). Ask/plan-approval interactions initially failed on the wrong v4
   answer shape (`Permission request failed`), and a never-exited plan mode
   poisoned the session's mutating legs (zcode's runtime-level plan-mode gate).
2. **Retries** (`task3-retry2-2026-08-20.txt`): same session — the corrected
   `{action:'accept',content:{answer}}` shape landed the ANSWERED ask form +
   the ExitPlanMode exit; the held-pending non-answer leg proved NO self-timeout
   in 600s (honest corpus-absent); plan-mode re-entry kept blocking crons.
3. **Fresh session** (`task3-retry5-2026-08-20.txt`): clean session, answerer on
   every leg — captured the CronList zod-error family (a zcode 0.16.3 headless
   bug: automation/list fails its own schema on EVERY call, 3 sessions) and
   re-observations of every shared family; the model still re-entered plan mode
   and the cron/message/stop SUCCESS forms stayed uncaptured (honest
   corpus-absent — the full hunt is in the fixture's corpus_absent list).

**Rolled out mid-harvest:** `sess_6e4b5cc7`'s rollout file rotated off
`~/.zcode/cli/rollout/` BETWEEN harvests (the D-04 loss class repeating, live).
Its unique families are pinned VERBATIM in
`internal/coreexec/testdata/zcode-recaptured-2026-08.json` (the committed
fixture is the surviving ground truth — exactly what the convention is for).
The supplementary session's snapshot lived at
`/tmp/zcode-recapture-run/surviving-bc155bc5.jsonl` during curation (ephemeral).

**Executor upgrades landed from the capture** (late-harvest markers per the
08-08 precedent): the Bash timeout form, the `<persisted-output>` truncation
envelope (budget 15000 / preview 2000, KB ÷1024 one-decimal — target-source
verified + capture-consistent), and the answered-ask pairing form
(`session.RenderAskAnswered`).

## Provenance

- Produced 2026-08-15→16 during Phase 9 AUD-05 (the operator-directed app-server stdio
  driver deviation, accepted at UAT).
- Archived verbatim (`cp -p`, permissions preserved) by quick task `260817-uv3`.
