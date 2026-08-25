# ass-guard-agent

**ass-guard** is a mimicry-first coding agent: an ACP server that speaks the
Agent Client Protocol (so it plugs into editors like
[Zed](https://zed.dev)), driven by any Anthropic-API-shaped model provider —
with every outgoing request shaped to be structurally indistinguishable from
the agent it mimics (zcode). The mimicry contract is validated continuously by
a behavioral A/B parity gate, not asserted.

- Single static Go binary (macOS/Linux, amd64/arm64), zero runtime dependencies
- Zero-config first launch in a project; operator config optional
- Install + editor setup: **[docs/install.md](docs/install.md)**

## What works today (v1.1)

Everything below is verified — most of it by live end-to-end runs against real
models during the v1.1 UAT round (2026-08-22…25), which drove the product over
stdio ACP exactly like an editor would.

### Driving the agent

| Capability | Notes |
|---|---|
| ACP server (`acp serve`) | stdio transport, streamed responses; works from Zed's agent panel |
| Ask & approve flows | model asks a question mid-task → turn suspends → your reply lands as the tool result → same turn resumes. Unanswered asks time out (`--ask-timeout`, default 10m) and hand back a "proceed or decline on your own" form |
| Plan mode | model-declared explore-then-plan phases. While ON the runtime refuses mutating tools ("Plan mode only allows read-only, non-destructive tools"); `ExitPlanMode` suspends on an approval question before any code changes |
| Background work | `Bash run_in_background` returns immediately with a task id + log path; `TaskOutput` retrieves (blocking or snapshot); `TaskStop` kills the whole process group |
| Cron automations | model-created schedules persist per-project; due prompts fire as engine-driven turns (serialized behind active turns, exactly-once catch-up with missed-window notes); no daemon, no port |
| Agent messaging | subagents are addressable (`agent_<uuid>` ids); `SendMessage` delivers per-session mailboxes; unknown ids error structurally, never broadcast |
| Session context reads | `ReadSessionContext` serves relevant/handoff excerpts from the project's own transcripts |
| Slash commands | zcode-semantics expansion (`$ARGUMENTS`, `$1..$N`) over `.claude/commands/` incl. namespaced layouts; the full OpenSpec command matrix runs end-to-end hands-off (zero manual continues) |
| Subagents | Task/Agent dispatch with restricted tool subsets; discovered agent definitions become spawnable types |

### Safety & observability

| Capability | Notes |
|---|---|
| Workspace checkpoints | shadow-git snapshots before mutating turns; restore via CLI or the model |
| Audit trail | per-session redacted audit mirror under `.ass-guard/audit/` — what the agent did AND why the engine continued (decision provenance) |
| Transcripts | durable JSONL under `<workdir>/.ass-guard/`; secrets redacted at the chokepoint |
| Engine | unified continue/stop decision layer with pattern-table chaining, re-fire budget caps, hook-DAG boundaries |
| Behavioral eval net | scenario suites with pass@k scoring behind gate flags (`mise eval-gate`); opening runs have already caught real drift |

### Model routing (formerly "scheduling")

Tier-based model selection resolved per request: named tiers (the default
config ships `heavy`; operators add more, e.g. a `light` tier for subagent
routing — config-conditional), fallback chains on transient failure, circuit
breaker, cost ceiling with a degrade path, time-window overrides.

```
ass-guard model-routing validate          # load-time config guarantee
ass-guard model-routing resolve heavy     # which (provider, model) answers right now
```

Config layers (later wins): embedded defaults → `~/.config/ass-guard-agent/config.yaml`
→ `<project>/.ass-guard/config.yaml`. Schema: `providers` / `models` / `tiers` /
`circuit_breaker` / `cost_ceiling`. Live models need `ZAI_API_KEY` (or the
provider's env) — the handshake works without it.

## Known rough edges

Honest list, so adoption is not a leap of faith:

- **No session resume yet.** Reopening a thread starts fresh;
  `session/load` returns unsupported. Resume is the top v1.2 item.
  Transcripts persist for human investigation either way.
- Asks render as plain text in the client (single-line), not clickable option
  buttons — the captured target's wire form has no rich ask surface; answering
  by typing the option text works.
- WebFetch/WebSearch route through a built-in DuckDuckGo-default backend;
  operators can swap backends by config.
- The newline transport guard was relaxed (2026-08-19): model text chunks now
  reach the wire byte-faithful.

## CLI overview

| Command | Purpose |
|---|---|
| `ass-guard acp serve` | run the ACP server (the editor entrypoint). Flags: `--ask-timeout` (D-01 wait; 0 = block forever), `--work-dir`, `--profile`, `--max-concurrent`, `--no-engine`, `--audit-log` |
| `ass-guard model-routing validate \| resolve <tier>` | inspect/validate routing config; resolve a tier now |
| `ass-guard checkpoint list \| restore <sessionID-turn-NNN>` | workspace checkpoints |
| `ass-guard learning list \| revert <id>` | learned-config store |
| `ass-guard parity` | behavioral mimicry A/B gate (the north-star check) |
| `ass-guard profile check <name>` | profile drift detection vs a fresh capture |
| bare `ass-guard "<prompt>"` | tracer — one turn, tool-calls printed to stderr |
| `ass-guard --version` | build-injected version (stderr) |

All diagnostics go to stderr; `acp serve` stdout carries only protocol frames.

## Development

```sh
mise ci            # vet + lint + build + race tests — the merge gate
mise eval-gate     # behavioral-eval suites at k=1 (real binary + real model)
go build -o ass-guard ./cmd/ass-guard   # dev snapshot
goreleaser build --snapshot --clean     # release-matrix binaries
```

Docs: [install](docs/install.md) ·
[recapture runbook](docs/recapture-runbook.md) ·
[tool contract inventory](docs/tool-contract-inventory.md) ·
[compaction decision](docs/compaction-decision.md) ·
[shaper↔pi audit](docs/shaper-pi-audit.md)

Planning artifacts (milestones, phase verification reports, UAT records):
`.planning/`.

## License

MIT — see `agent.json`.
