12-08 first gated runs — the eval net's opening catches (all evidence real, real binary + real model via the operator's repo config creds):

RUN 1 (suite, 78.9s): explore chained; stopped after explore (decisions=[nothing]).
  -> eval-20260820-130906-k1.json (first suite artifact ever emitted)
RUN 2 (suite, 205s then re-run 78s): archive stage SUSPENDED on a REAL
  AskUserQuestion (ask_suspended line; 12-01 made asks real — the Phase-8
  proof predates that, asks were then a no-implementation stub).
  -> eval-20260820-131257-k1.json
RUN 3 (runtime product-proof, 323.8s, with the D-01 askTimeout=45s config):
  explore->propose->apply chained, then the apply turn's ending RE-MATCHED
  the ->apply injection 5 more times (8 total, budget cap stopped the loop:
  "re-fire budget cap reached (8 continue-injections)"). No archive.
  -> run3-apply-loop-scratch/.ass-guard/transcript_sess-opsx-e2e.jsonl

DIAGNOSIS: the pattern table's stage-transition signals have drifted against
the current model's phrasing (three runs, three chain shapes). The budget-cap
and no-chain-suspension safety pins held everywhere. The first GREEN of the
flagship gate is blocked on pattern-table re-tuning (capture-informed — the
08-06 seeded patterns vs current model output), routed to the operator.
