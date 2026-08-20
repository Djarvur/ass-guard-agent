# 13-01 eval first runs — the 12-08 drift evidence (in-repo copy)

Provenance: copied 2026-08-20 from `/tmp/eval-net-evidence/12-08-first-runs/` (the
12-08 opening runs' evidence, independently corroborated by the Phase-12 verifier).
Preserved in-repo per the 13-01 Task 1 amendment so `/tmp` reaping can never make
the re-tuning input unavailable — `/tmp` is no longer load-bearing.

Contents:

- `README-EVIDENCE.txt` — the original three-run failure-shape notes (verbatim):
  run 1 stalled after explore (decisions=[nothing]); run 2 SUSPENDED on a real
  AskUserQuestion at the archive stage (pre-askTimeout; the landed askTimeout=45s
  D-01 config resolves this class); run 3 chained explore→propose→apply, then the
  apply turn's ending re-matched the →apply injection 8 times until the re-fire
  budget cap stopped it.
- `eval-20260820-130906-k1.json` — suite artifact, 205s pass (3 continues, ask
  suspension at archive; no archived dir).
- `eval-20260820-131257-k1.json` — suite artifact, 78.8s pass (0 continues;
  stalled after explore).
- `run3-apply-loop-scratch/.ass-guard/transcript_sess-opsx-e2e.jsonl` — the run-3
  loop transcript (323.8s, askTimeout=45s): 9 assistant messages, 9 engine
  decisions (1 provenance continue + 7 text:post-propose-handoff re-fires + 1
  budget-cap nothing). This transcript is the re-tuning's ground truth for the
  CURRENT model's stage-closing phrasings. Only the transcript was carried over
  from the scratch (the scratch's code/artifacts are reproducible; the transcript
  is not).

The 13-01 Task 1 re-tuning input derived from this evidence:

- propose closings mention `/opsx:apply` exactly once, as
  `Run \`/opsx:apply\` … to start …` (both the 2026-08-15 captures and run 3).
- apply closings mention `/opsx:apply` ONLY inside the apply command's own banner
  echo `(override with \`/opsx:apply <other>\`)` — the drifted bare-`/opsx:apply`
  anchor matched THIS, causing the re-fire loop — and consistently close with
  `… archiv(e|ed)( this change)? with \`/opsx:archive\``.
- the archive closing says "Archive Complete" (the terminal shield's anchor holds).
