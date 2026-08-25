# Phase 13: OpenSpec Workflow Completion - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-08-17
**Phase:** 13-OpenSpec Workflow Completion
**Areas discussed:** Fixable-path coverage, Dead-end routing, Seed-harvest procedure, Eval suite granularity, Advisory surface & repeat policy, Gate flake policy, Verify chain role, Onboard boundaries

**Mode:** Default interactive (manager-dispatched inline discuss). All decisions user-selected in live discussion. Research-before-questions applied to the two externally-researchable areas.

---

## Fixable-path coverage

Research context presented: happy-path-only as the classic blind spot; curated failure scenarios over blanket matrices ([Autonoma](https://getautonoma.com/blog/happy-path-testing-beyond-the-happy-path), [QA Skills 20 rules](https://qaskills.sh/blog/end-to-end-testing-best-practices), [arXiv evaluation-driven dev](https://arxiv.org/html/2411.13768v2)); Phase-8 evidence (2P/1F fixable tally).

| Option | Description | Selected |
|--------|-------------|----------|
| Full 2-path matrix | All 6 commands × happy + fixable (12 legs); evidence over gate time | ✓ |
| Concentrate on mutating cmds | Fixable legs only on apply/archive/bulk-archive | |
| Happy in gate, fixable local-only | Weakest OS-01 fidelity | |

**User's choice:** Full 2-path matrix
**Notes:** Diverges from recommendation — operator prioritizes proof coverage.

## Dead-end routing

| Option | Description | Selected |
|--------|-------------|----------|
| Model-initiated only | Seeds teach the tool; engine untouched | |
| Model-initiated + advisory | Plus audit-logged advisory on unmatched question endings; never holds continuation | ✓ |
| Engine-forced conversion | Closest to the rejected Phase-8 design | |

**User's choice:** Model-initiated + advisory
**Notes:** The Phase-8 auto-continue rejection was framing; the advisory is observability + next-turn nudge, not continuation.

## Seed-harvest procedure

| Option | Description | Selected |
|--------|-------------|----------|
| Harvest pass, then verify | Unseeded pass records stage-ends → seed → gate re-run asserts zero-continue | ✓ |
| Opportunistic harvest | Seed from gate runs; verify on next change-class trigger | |
| Phase-8 corpus extrapolation | Weakest D-12 fidelity | |

**User's choice:** Harvest pass, then verify

## Eval suite granularity

Research context presented: pass^k consistency as the gating-relevant metric; curated standard+edge suites with per-trajectory attribution ([Braintrust regression gates](https://www.braintrust.dev/articles/ai-agent-evaluation-framework), [LangChain readiness checklist](https://www.langchain.com/blog/agent-evaluation-readiness-checklist), [pass^k discussion](https://medium.com/@vinodkrane/chapter-8-agent-evaluation-for-llms-how-to-test-tools-trajectories-and-llm-as-judge-788f6f3e0d52)).

| Option | Description | Selected |
|--------|-------------|----------|
| Per-command suites (6) | Exact attribution into the D-03 gate | ✓ |
| Grouped by family | Fewer suites, one-hop attribution | |
| One matrix suite | Worst attribution | |

**User's choice:** Per-command suites (6)

## Advisory surface & repeat policy

| Option | Description | Selected |
|--------|-------------|----------|
| session/update note | Visible in-editor; transcript fidelity intact | ✓ |
| ACP user-message | Prominent but breaks replay fidelity | |
| Audit-log only | Safest but invisible | |

| Option | Description | Selected |
|--------|-------------|----------|
| Dedupe per session+class | First occurrence visible; repeats audit-only | ✓ |
| Fire every occurrence | Spam risk | |
| Once per session | Coarser attribution | |

**User's choice:** session/update note; dedupe per session + pattern-class

## Gate flake policy

| Option | Description | Selected |
|--------|-------------|----------|
| One re-run, no-delta proof | Phase-8 evidence class; two failures = hard fail | ✓ |
| Hard fail, no re-run | Every variance flake becomes investigation | |
| Best-of-N re-runs | Approximates pass-any-of-k | |

**User's choice:** One re-run, no-delta proof

## Verify: chain-link or terminus

| Option | Description | Selected |
|--------|-------------|----------|
| Chain terminus | Report, human reads, chain over | |
| Chain into fix workflow | Seeded handoff (verify → continue/new change) when report finds unimplemented specs | ✓ |
| Terminus + one opt-in seed | Middle ground | |

**User's choice:** Chain into fix workflow
**Notes:** Diverges from recommendation — ambitious route; mechanism constrained to the standard seeded-pattern machinery (no freehand report interpretation).

## Onboard boundaries

| Option | Description | Selected |
|--------|-------------|----------|
| Full-tilt in scratch | Real writes; idempotent re-run as fixable class | ✓ |
| Reconciliation-only | Dodges the primary flow | |
| Full-tilt + blast-radius assert | Prove the blast radius | |

**User's choice:** Full-tilt in scratch

---

## Claude's Discretion

Onboard's D-11 mutability classification; advisory wording/field shape; verify-handoff target command (harvest-driven); fixture shape and leg ordering; seed-row syntax details.

## Deferred Ideas

Engine-forced conversion (safety-rejected, do not revisit); best-of-N re-runs; ACP user-message advisory; onboard blast-radius assertion (v1.1 candidate); cross-command mega-suite.
