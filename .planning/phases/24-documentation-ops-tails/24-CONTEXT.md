# Phase 24: Documentation & Ops Tails - Context

**Gathered:** 2026-08-27
**Status:** Ready for planning

<domain>
## Phase Boundary

Four small independent tails close out the milestone: LSP support documented as an IDE-side MCP configuration guide (Zed worked example — operator decision 2026-08-25 honored: documentation only, no agent-side implementation), the model-routing Scheduler gains a deterministic outcome store + feedback loop (zero LLM calls), nightly upstream-parity drift CI runs unattended on GH Actions, and plugins/skills prove functional unchanged in every interaction mode (ECOS-04 end-to-end matrix).

</domain>

<decisions>
## Implementation Decisions

### LSP Documentation (DOC-01)
- **D-01:** The guide's worked example is ZED's IDE-side MCP providing LSP capabilities (operator: "we know there is an mcp inside zed providing lsp capabilities. we should provide zed example"). The generic pattern for any LSP-providing MCP server is secondary. The exact Zed feature surface (which LSP capabilities it exposes to agents) is pinned at research time. Scope statement included: agent-side LSP explicitly OUT (the 2026-08-25 operator decision, stated in the doc itself).
- **D-02:** The guide lives in `docs/` as its own file (`docs/lsp-setup.md` family), linked from the README docs index — alongside `docs/install.md`, not inside it.
- **D-03:** Success criterion 1 verification = FOLLOWABLE DRY-RUN: the doc's author follows it top-to-bottom during the phase and reaches working LSP-symbol tools in an ass-guard session. Review-only accuracy does not satisfy the criterion.

### Scheduler Outcome Store (TAIL-01)
- **D-04:** The store records MECHANICAL signals per dispatch: (provider, model, tier), outcome class (ok / error class / fallback-used), latency, tokens, cost. No request content, no LLM-judged quality — the zero-LLM letter is a hard boundary.
- **D-05:** Storage = append-only JSONL under `.ass-guard/` (git-excluded like checkpoints), aggregated in memory at read time. No SQLite, no rewritten aggregate file.
- **D-06:** Outcomes feed the EXISTING deterministic seams: breaker state, cost-degrade decisions, fallback ordering. NO new tier-preference layer. Feedback is observable as changed routing decisions after enough evidence accumulates.
- **D-07:** Observability proof = deterministic test (seed the store with synthetic outcomes → assert the routing decision changes: breaker opens / degrade fires) + a CLI subcommand dumping aggregated stats.

### Nightly Parity CI (TAIL-02)
- **D-08:** Vehicle = GitHub Actions scheduled workflow with an OFF-PEAK cron time (not :00/:30 — the documented cron jitter/dropped-runs problem), the zcode-dependent job labeled for a SELF-HOSTED runner so the corpus stays local.
- **D-09:** The nightly executes DRIFT-ONLY: zcode version probe + profile structure hash vs pinned capture + build/test. Zero nightly API spend; the full LLM behavioral eval stays on the change-class gate (12-08's `ASSGUARD_EVAL_GATE` machinery, unchanged).
- **D-10:** Drift report = run artifact always; on drift, AUTO-OPEN a GitHub issue with the diff summary. The unattended signal survives and notifies without a human watching run logs.

### ECOS-04 Mode Matrix (TAIL-03)
- **D-11:** "Interaction mode" = turn ORIGIN. Exactly FOUR modes: interactive ACP prompt, subagent dispatch, background wake-turn, automation/cron turn. Steering delivery and parked asks are mechanics WITHIN interactive turns, not separate matrix rows.
- **D-12:** Per mode, ALL THREE surfaces must work: commands resolve + dispatch, skills load + execute, plugin hooks fire. Full matrix: 4 modes × 3 surfaces.
- **D-13:** Proof = scripted E2E harness per mode (fixture plugin/skill, assert FUNCTIONAL outcome — not just discovery/loading), CI-runnable and repeatable. Not a one-time manual checklist.
- **D-14:** Fixtures = synthetic in-repo plugin/skill (deterministic, no network) + ONE real installed Claude Code plugin spot-checked per surface to anchor the "unchanged" claim.

### Claude's Discretion
- The exact Zed MCP feature names/config keys the guide cites (research pins them).
- JSONL schema field names; aggregation window semantics (rolling vs fixed) for breaker/cost feedback.
- Nightly cron hour; workflow file naming; issue title/body template.
- Synthetic fixture plugin's name/content; which real CC plugin gets the spot-check.
- CLI subcommand name for the stats dump (`ass-guard routing stats` family).

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements & Roadmap
- `.planning/REQUIREMENTS.md` §Documentation & Tails — DOC-01 (IDE-side MCP configuration requirement, no agent-side implementation), TAIL-01 (deterministic outcome store + feedback loop, zero LLM calls), TAIL-02 (nightly upstream-parity gate CI unattended, reports zcode version/structure drift), TAIL-03 (ECOS-04: plugins/skills working unchanged in every interaction mode) verbatim
- `.planning/ROADMAP.md` §Phase 24 — goal, 4 success criteria; dependency note: Phases 15–23 settle the surfaces these tails verify

### Prior Phase Contracts (hard dependencies)
- `.planning/phases/16-acp-wire-foundation/16-CONTEXT.md` — D-20..D-23 (transcript additive kinds — outcome records ride this), config layer (D-05's store config if any)
- `.planning/phases/20-built-in-commands-skills-per-agent-model/20-CONTEXT.md` — D-01..D-04 (resolver chain: builtins→skills→agents→file-commands — the command surface TAIL-03 exercises), D-10..D-12 (fsnotify rescan — fixtures must be discovered)
- `.planning/phases/21-context-policy-parity-closures/21-CONTEXT.md` — D-01..D-04 (hook authority/merge/scope order — the plugin-hooks surface TAIL-03 exercises)
- `.planning/phases/22-background-execution-sandbox-reality/22-CONTEXT.md` — D-01 (background wake-turn machinery — one of the four modes)
- `.planning/phases/18-session-family/18-CONTEXT.md` — automation/cron turn discipline (the fourth mode's substrate)
- `.planning/phases/15-internal-runtime-carve-step-0/15-CONTEXT.md` — runtime carve boundaries the outcome-store seam plugs into

### External References
- `zed.dev/docs/ai/mcp` — Zed MCP ("Context Server") configuration; the D-01 worked example's canonical source
- `github.com/orgs/community/discussions/196910` — GH Actions scheduled-workflow cron drift; D-08's off-peak rationale
- occ ADR-001 nightly-parity pattern + cost-cascade feedback loop (v1.1 ROADMAP 2026-08-19 disposition, borrows #13/#14) — TAIL-01/TAIL-02's design prior art

### Code Anchors
- `internal/modelrouting/dispatch.go` — Scheduler (resolve→capability→breaker→cost→sem→fallback walk); D-06's feedback seams (Breaker, CostTracker, fallback ordering)
- `internal/modelrouting/breaker.go` + `safety` wiring (Plan 03-03) — the breaker state outcomes feed
- `internal/ecosys/` (loader.go, skills.go, hooks.go, precedence) — the three surfaces TAIL-03 exercises; read-only `.claude/` discipline fixtures must respect
- `internal/parity/` + `ass-guard profile check` (MIMC drift detector) — D-09's deterministic drift probe substrate
- `internal/evalsuite/suite.go` (`ASSGUARD_EVAL_GATE`) — the change-class gate D-09 leaves untouched
- `.mise.toml` tasks (ci, eval-gate) — what the nightly workflow invokes
- `docs/install.md` — D-02's sibling pattern (docs index shape)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- Scheduler seams (Breaker, CostTracker, fallback walk): D-06 attaches outcome feedback to already-designed joints — no new routing layer.
- `internal/parity/` + `profile check`: deterministic zcode-version/structure comparison already exists (EARLY-03's drift-warning slice) — the nightly job wraps it.
- `internal/ecosys/` loader + precedence tests: fixture-based discovery testing pattern to extend to functional outcomes.
- `.mise.toml` ci/eval-gate tasks: the workflow's entry points.

### Established Patterns
- Graceful degradation + loud notes — issue-on-drift (D-10), stats dump (D-07).
- Transcript-as-truth — outcome records are transcript-adjacent JSONL, replay-safe.
- Git-excluded `.ass-guard/` state (checkpoints precedent) — D-05's store location.
- Fixture-driven hermetic tests + one real-world spot-check (parity suite precedent) — D-14.

### Integration Points
- Dispatch call sites record outcomes (the Scheduler's provider.Send + fallback walk).
- `.github/workflows/` (new directory) — first workflow in repo.
- README docs index gains the LSP guide link.
- E2E mode harness builds on the existing evalharness scenario runner shape.

</code_context>

<specifics>
## Specific Ideas

Operator framing that shaped decisions:
- Zed is THE worked example for LSP ("there is an mcp inside zed providing lsp capabilities") — not Serena, not generic-only.
- All other choices endorsed as recommended.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 24-documentation-ops-tails*
*Context gathered: 2026-08-27*
