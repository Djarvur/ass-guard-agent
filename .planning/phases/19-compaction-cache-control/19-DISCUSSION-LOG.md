# Phase 19: Compaction + cache_control - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-08-27
**Phase:** 19 - Compaction + cache_control
**Areas discussed:** Trigger & measurement, Reset-point context shape, Summarizer execution, Manual /compact + cache scope

**Context:** Discussed inline while Phase 16 plans in background. Research-before-questions enabled.

---

## Trigger & measurement

Research basis: CC triggers on a reserved token buffer (~83–95% of window by version), usage estimated each turn ([buffer analysis](https://claudefa.st/blog/guide/mechanics/context-buffer-management), [vincentqiao](https://blog.vincentqiao.com/en/posts/claude-code-context/)); late-trigger failure known ([#66144](https://github.com/anthropics/claude-code/issues/66144)). ass-guard holds REAL per-request usage (16-D-21 records).

| Option | Description | Selected |
|--------|-------------|----------|
| Real usage + est delta | Last-request input_tokens + conservative new-content estimate | ✓ |
| Local estimate | Tokenizer/chars-4 over projected request | |
| Message count | Like the 64-window | |

**User's choice:** Real usage + est delta (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Pre-request + retry | Check before every send incl. tool-loop iterations; overflow retry backstops | ✓ |
| Post-turn only | Simpler; next request can overflow before any check | |

**User's choice:** Pre-request + retry (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| configOptions menu | threshold_pct + enabled under 16's advertise-all rule | ✓ |
| File only | Not editor-switchable | |

**User's choice:** configOptions menu (Recommended)

---

## Reset-point context shape

Research basis: canonical `[summary] + [recent tail]` with logical-boundary cuts ([platform docs](https://platform.claude.com/docs/en/build-with-claude/compaction), [pitfall](https://medium.com/@reliabledataengineering/claude-compaction-the-secret-to-infinite-length-conversations-03b6ee607f2d), [MS Agent Framework](https://learn.microsoft.com/en-us/agent-framework/concepts/agents/conversations/compaction)).

| Option | Description | Selected |
|--------|-------------|----------|
| Summary + tail | [summary] + [verbatim recent tail, pair-bounded] | ✓ |
| Summary only | Maximum shrink; loses verbatim grip | |

**User's choice:** Summary + tail (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Fixed N messages | Predictable cut, easy tests | |
| Budget-fill | Fill remaining budget with as much tail as fits | ✓ (operator override) |
| Claude decides | Pinned at planning | |

**User's choice:** Budget-fill — NOT fixed-N; maximize retained context.

| Option | Description | Selected |
|--------|-------------|----------|
| Seed = summary only | Durable seed is the summary; tail follows normal boundary discipline | ✓ |
| Seed = summary + tail | Tail also immune; blurs boundary semantics | |

**User's choice:** Seed = summary only (Recommended)

---

## Summarizer execution

Research basis: separate smaller model recommended for summarization; async-vs-blocking is the core trade-off ([MS Agent Framework](https://learn.microsoft.com/en-us/agent-framework/concepts/agents/conversations/compaction), [Bedrock AgentCore](https://pubs.towardsai.net/long-context-compaction-for-ai-agents-part-2-implementation-and-evaluation-d708d2d2a6e5)).

| Option | Description | Selected |
|--------|-------------|----------|
| Blocking pre-request | Summarize → marker → request; no races | ✓ |
| Async background | Non-blocking; overflow risk during gap | |

**User's choice:** Blocking pre-request (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Since last marker | Full span since previous compaction; chained summaries | ✓ |
| Window only | Cheaper; pre-window content lost to later summaries | |

**User's choice:** Since last marker (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Skip + retry next | Proceed un-compacted; one warning + counter | ✓ |
| Fail turn | Typed error; user intervenes | |

**User's choice:** Skip + retry next (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Same pipeline | Breaker, cost, local records — visible in /cost | ✓ |
| Shadow path | Minimal call path; invisible spend | |

**User's choice:** Same pipeline (Recommended)

---

## Manual /compact + cache scope

| Option | Description | Selected |
|--------|-------------|----------|
| Same machinery | One implementation, two triggers; runs regardless of threshold | ✓ |
| Separate path | Divergent semantics, two test surfaces | |

**User's choice:** Same machinery (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| System-only, locked | Corpus-proven 910/910; probe baseline unchanged | ✓ |
| System + tools | Beyond corpus evidence | |

**User's choice:** System-only, locked (Recommended)

---

## Deferred Ideas

None — discussion stayed within phase scope.
