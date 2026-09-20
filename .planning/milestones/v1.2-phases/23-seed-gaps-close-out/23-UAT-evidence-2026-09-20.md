---
audit_acknowledged:
  milestone: v1.2
  at: 2026-09-20
  gap_snapshot: "unknown::scenarios=0"
---

# Phase 23 UAT Evidence — 2026-09-20 (automated operator run)

Operator of record: Daniel Podolsky (verdict pending). Execution: ZCode automation
driving the live Zed editor (XWayland, synthetic input) plus a Zed-shaped ACP driver
against the same shipped binary. Binary under test: `~/go/bin/ass-guard` (mtime
2026-09-12 00:36 = fix commit `fa370f2`). Client: **real Zed 1.20.2** (auto-updated
mid-session from 1.19.2). Workspace for 1b/1c/2: throwaway clone `/tmp/uat-ws`
(created so Zed's per-workspace poisoned dock state could not block the run; same
binary, same wire protocol, same checkpoint store semantics). Wire capture:
`strace` wrapping the agent process (`/tmp/gsd-zed-uat/acp-trace.txt`).

## G-23-1 regression surface — wire evidence with real Zed (session e7071f0f)

Frame order from the live Zed↔agent session (timestamps from strace):

| ts (local) | dir | frame |
|---|---|---|
| 17:58:09.244462 | agent→Zed | `session/new` **response** (sessionId `e7071f0f-…`, configOptions) |
| 17:58:09.245002 | agent→Zed | `session/update` `available_commands_update` — full catalog, **15 commands incl. `undo`** |
| 17:58:09.252122 | Zed→agent | `session/set_config_option` (session registered and active client-side) |

The advertisement now lands **after** the response frame — the exact ordering the
G-23-1 fix (`fa370f2`, plan 23-07) established. Pre-fix this update was emitted
before the response and Zed dropped it ("Available commands for ass-guard: none").

## Test 1 — Live-Zed steering + /undo operator UAT (WINDOWS #23): PASS

**Steering leg (driven through the live Zed UI):**

- Prompt sent mid-flow: create `uat-scratch.txt`, read go.mod, list top-level dirs, reply DONE.
- Steering note sent while the turn was live. Thread rendering (screenshot 54): the note
  renders as a distinctly-styled queued note; turn **not cancelled** (6 tool calls completed).

- Transcript (`/tmp/uat-ws/.ass-guard/transcript_e7071f0f-….jsonl`): steering
  `user_message` recorded mid-flow; final reply contains the steering's requested
  directory count and ends **"DONE STEERED"** — boundary delivery confirmed.

**Idle /undo (1b) — driver leg:**

- Turn created `uat-1b.txt` → `/undo` resolved as class-B command, control-plane-fast,
  no model turn; reply: `undo complete — restored checkpoint …-turn-001 (pre-restore
  snapshot: …-pre-001); run /undo again to walk back further`.

- `uat-1b.txt` **reverted** (gone) after restore.

**Mid-turn /undo (1c) — driver leg:**

- Multi-step turn launched; `/undo` sent mid-turn → responded in **0.1 s**
  (`end_turn`); interrupted turn's stopReason = **`cancelled`**; `uat-1b.txt`
  reverted. Cancel-then-restore ordering visible in `driver2-frames.jsonl`
  (undo response before the cancelled turn's response).

## Test 2 — Two-live-session cross-session restore check (WINDOWS #24): PASS

Driver legs, two sessions (A `64c1127c-…`, B `d2250c19-…`) on one workspace:

- B setup turn created `uat-b.txt`.
- A's turn live → B sent `/undo` → **refused in 0.1 s**, verbatim:
  `undo refused: checkpoint: restore refused: a client turn is active for session
  64c1127c-1f39-4a63-a255-c0c472784477 — the worktree and checkpoint store are shared
  by every session of this process; cancel that session's turn or wait for it to end`
  (names the busy session; `uat-b.txt` untouched).

- After A's turn ended, B retried `/undo` → `undo complete — restored checkpoint
  d2250c19-…-turn-001`; `uat-b.txt` reverted.

## Observations (non-blocking, recorded for follow-up)

1. **stdout discipline violation (real bug, minor).** At startup the agent writes a
   git-checkpoint log line to **stdout**, which the transport reserves exclusively
   for ACP frames: frames 0–1 of `acp-trace.txt` — `[refs/checkpoints/last
   (root-commit) e5f44b9] ass-guard checkpoint store init`. Zed tolerated it
   (skipped unparseable lines) but this violates the LSP-style stdout discipline
   (PROJECT.md constraint). Route to stderr.

2. **Zed 1.20.2 renders no slash-command menu** on `/` in the agent input even with
   the catalog accepted; Zed validates commands at send time (the original failure
   surfaced there). The legs above prove the catalog is delivered and accepted at
   the wire level; the send-time UI validation path was not exercised (see method note).

3. **Zed does not re-spawn a dead ACP agent server via panel interaction** (the
   "Agent server exited — Restart" banner was unresponsive to synthetic input);
   a full Zed restart re-spawned it at startup. Environment finding, not an agent defect.

## Method note (scope of evidence)

Legs 1b/1c/2 were executed by a Zed-shaped ACP client (`acp-driver.py`,
`acp-driver2.py` — initialize/session/new/session/prompt frames byte-equivalent in
shape to Zed's own, captured in the same strace). Agent-side behavior is therefore
evidenced exactly as a live editor would observe it. The Zed-specific half of G-23-1
(catalog acceptance) is evidenced separately on the wire with the real client (table
above). What was NOT exercised: Zed's send-time `/undo` forwarding UI (dock/panel
automation was blocked by Zed's workspace-scoped dock state under synthetic input).

## Artifacts

`/tmp/gsd-zed-uat/` (ephemeral — copy retained until reboot): screenshots
`01…89-*.png`, wire captures `acp-trace.txt` / `driver-frames.jsonl` /
`driver2-frames.jsonl`, drivers `acp-driver*.py`, Zed log excerpts. Clone workspace
`/tmp/uat-ws` (transcripts under `.ass-guard/`). The real repo's working tree was
untouched (verified identical to session start).
