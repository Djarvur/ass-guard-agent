# Phase 24: User Setup Required

**Generated:** 2026-09-10
**Phase:** 24-documentation-ops-tails (plan 24-04 — TAIL-02 nightly upstream-parity gate)
**Status:** Incomplete

Complete these items for the nightly-parity gate to run unattended. Claude automated everything possible (drift core, workflow file, mise task, dispatch attempts); these items require human access to the GitHub dashboard / operator infrastructure.

## Environment Variables

None — the drift job spends zero API tokens by design (D-09) and needs no secrets. `GITHUB_TOKEN` is injected by GitHub Actions itself.

## Dashboard Configuration

- [ ] **Register a self-hosted runner with the exact label set `[self-hosted, zcode]`**
  - Location: GitHub → repo `Djarvur/ass-guard-agent` → Settings → Actions → Runners → New self-hosted runner
  - Labels: `self-hosted`, `zcode` (the drift job's `runs-on` uses all-labels-must-match semantics — both labels must be present)
  - Machine requirements: the runner must live where the zcode CLI and its corpus exist (D-08: the corpus stays local), with `mise` and Go 1.26 installed
  - Current state (verified 2026-09-10 via `gh api repos/Djarvur/ass-guard-agent/actions/runners`): **zero runners registered** — until one carries these labels, the drift job sits `queued` on every run (the hosted build-test job is unaffected)

- [ ] **Confirm repo Workflow permissions allow GITHUB_TOKEN issue creation**
  - Location: GitHub → repo Settings → Actions → General → Workflow permissions
  - The drift job already declares `permissions: { contents: read, issues: write }` (least privilege, job-scoped — Pitfall 6), but the repo-level default may also need to permit read-write tokens for the issue-on-drift step to succeed (community discussion 124684)
  - Symptom if unset: first drift run uploads the report artifact but the issue step 403s (the artifact is the belt — D-10)

- [ ] **After the v1.2 milestone merges to master: verify the schedule activated**
  - Location: GitHub → repo Actions → nightly-parity → scheduled runs
  - Scheduled workflows fire ONLY when the workflow file exists on the default branch (`master`) — research Pitfall 2. Until the merge, `workflow_dispatch` is the manual backstop
  - Also run the deferred dispatch smoke (see 24-04-SUMMARY.md — the live dispatch was rejected pre-merge with HTTP 404 because the file is not yet on the default branch): Actions → nightly-parity → Run workflow, then confirm the build-test job goes green and the drift job's report artifact exists (or queues, per the runner state above)

## Verification

After completing setup:

```bash
# Runner online with the right labels
gh api repos/Djarvur/ass-guard-agent/actions/runners --jq '.runners[] | {name, status, labels: [.labels[].name]}'

# Manual smoke (post-merge, from any branch carrying the file)
gh workflow run nightly-parity.yml
gh run list --workflow nightly-parity.yml --limit 1
```

Expected results:
- Runner listed with labels containing both `self-hosted` and `zcode`, status `online`
- The dispatch run's build-test job concludes `success`; the drift job either runs (report artifact `nightly-report` present) or queues until the runner is online

---

**Once all items complete:** Mark status as "Complete" at top of file.
