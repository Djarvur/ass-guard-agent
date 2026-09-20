# Phase 24: Documentation & Ops Tails - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-08-27
**Phase:** 24 - Documentation & Ops Tails
**Areas discussed:** LSP doc scope, Scheduler outcome store, Nightly parity CI, ECOS-04 mode matrix

**Context:** Discussed inline while Phase 16 executed in background. Research-before-questions enabled.

---

## LSP doc scope

Research basis: Serena-as-MCP was ECOSYSTEM-AUDIT's LSP-01 proposal (uvx install, symbol tools as `mcp__serena__*`); Zed hosts MCP "Context Servers" with ACP as the editor↔agent bridge ([Serena config](https://oraios.github.io/serena/02-usage/050_configuration.html), [Zed MCP docs](https://zed.dev/docs/ai/mcp), [ACP discussion #24028](https://github.com/zed-industries/zed/discussions/24028)).

| Option | Description | Selected |
|--------|-------------|----------|
| Zed example + generic | Zed's IDE-side MCP (LSP capabilities) as the worked example; generic pattern secondary | ✓ (operator choice) |
| Serena example + generic | ECOSYSTEM-AUDIT's original LSP-01 shape | |
| Generic only | No named server; nothing to rot | |
| Multiple examples | Serena + Zed + others | |

**User's choice:** Zed example — "we know there is an mcp inside zed providing lsp capabilities. we should provide zed example"

| Option | Description | Selected |
|--------|-------------|----------|
| docs/ file | Own file, linked from README index | ✓ |
| Install.md section | One doc, longer | |
| README section | Most visible, fattens front page | |

**User's choice:** docs/ file (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Followable dry-run | Author follows guide top-to-bottom during phase | ✓ |
| Review-only | Accuracy check, no live run | |

**User's choice:** Followable dry-run (Recommended)

---

## Scheduler outcome store

Research basis: bandit-feedback routers treat models as arms learning from observed outcomes; static routers degrade as prices/traffic shift; cascades = cheap-first escalate ([Palani](https://www.linkedin.com/pulse/model-routing-from-cost-saving-hack-production-layer-sam-palani-12a3c), [Pan](https://tianpan.co/blog/2025-11-03-llm-routing-model-cascades), [survey](https://arxiv.org/html/2603.04445v2)). The letter's "deterministic, zero LLM calls" rules out LLM-judged quality.

| Option | Description | Selected |
|--------|-------------|----------|
| Mechanical signals | (provider, model, tier), outcome class, latency, tokens, cost — no content, no LLM judgment | ✓ |
| Outcome bit only | Success/failure only | |
| Quality-judged | LLM scores — violates the letter | |

**User's choice:** Mechanical signals (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| JSONL log | Append-only under .ass-guard/, in-memory aggregation | ✓ |
| Aggregated file | learned.yaml family, rewritten on mutation | |
| SQLite | Queryable, heavier | |

**User's choice:** JSONL log (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Feed existing seams | Breaker, cost-degrade, fallback ordering | ✓ |
| New tier layer | Rolling-stats reordering independent of breakers | |
| Store only | No feedback — violates the letter | |

**User's choice:** Feed existing seams (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Test + CLI dump | Seeded-outcome routing test + stats subcommand | ✓ |
| Test only | Not surfaced to operator | |

**User's choice:** Test + CLI dump (Recommended)

---

## Nightly parity CI

Research basis: GH Actions scheduled-workflow cron drift/dropped-runs on peak hours — off-peak schedules reduce it; drift reports persist as artifacts; notify via issues not auto-fix ([cron drift discussion](https://github.com/orgs/community/discussions/196910), [drift-detection pattern](https://terrateam.io/blog/terraform-drift-detection-github-actions)). Repo facts: GitHub remote exists, no workflows dir, `mise eval-gate` needs real zcode + model, `profile check` is deterministic.

| Option | Description | Selected |
|--------|-------------|----------|
| GH Actions + self-host | Scheduled workflow; zcode-dependent job on self-hosted runner label | ✓ |
| Local launchd | Operator's Mac, invisible on GitHub | |
| Hybrid | GH for zcode-free parts, manual drift — splits the unattended promise | |

**User's choice:** GH Actions + self-host (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Drift-only | Version probe + structure hash + build/test; eval stays on change-class gate | ✓ |
| Drift + full eval | Nightly API spend | |
| Drift, eval on change | Automate 12-08's re-fire pattern | |

**User's choice:** Drift-only (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Artifact + issue | Artifact always; auto-open issue on drift | ✓ |
| Artifact only | Requires someone to look | |
| Commit report | Noisy history | |

**User's choice:** Artifact + issue (Recommended)

---

## ECOS-04 mode matrix

Research basis: cross-tool compatibility matrices are the established portability-verification pattern for skills/plugins ([Ayers — SKILL.md open standard + compat matrix](https://chris-ayers.com/posts/agent-skills-plugins-marketplace/), [Anthropic on agent skills](https://www.anthropic.com/engineering/equipping-agents-for-the-real-world-with-agent-skills)).

| Option | Description | Selected |
|--------|-------------|----------|
| Four origins | Interactive, subagent, background wake, automation/cron; steering/parked = mechanics within interactive | ✓ |
| Extended set | + steering, parked asks, persistent shell as distinct modes | |
| Interactive only | Weakest reading | |

**User's choice:** Four origins (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| All three surfaces | Commands + skills + plugin hooks, per mode | ✓ |
| Commands + skills | Hooks covered by Phase 21's verification | |
| Representative plugin | No systematic axis | |

**User's choice:** All three surfaces (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Scripted E2E | Functional-outcome assertions per mode, CI-runnable | ✓ |
| Manual checklist | One-time operator walk | |
| E2E + live pass | Both | |

**User's choice:** Scripted E2E (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Synthetic + spot | In-repo fixture + one real CC plugin spot-check | ✓ |
| Synthetic only | Hermetic, weaker "unchanged" claim | |
| Real only | Strongest claim, flaky | |

**User's choice:** Synthetic + spot (Recommended)

---

## Deferred Ideas

None — discussion stayed within phase scope.
