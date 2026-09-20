# API Coverage — GitHub Actions / GitHub REST (TAIL-02 nightly workflow)

> Full coverage by default. Opt-outs are explicit, reasoned decisions.
> Detector fired on the phase scope (signals: `actions/github-script` `github.rest.issues.create`; MCP consumption in DOC-01 context). This phase integrates exactly ONE external service programmatically: GitHub's Actions/REST surface, via the nightly-parity workflow (plan 24-04). The Zed/MCP mention in DOC-01 is documentation-only (operator decision 2026-08-25 — no agent-side integration), declared separately below.

| capability | decision | reason |
|---|---|---|
| schedule trigger (cron) | INTEGRATE | D-08's vehicle — the unattended nightly |
| workflow_dispatch trigger | INTEGRATE | manual smoke/backstop (no local runner tooling exists); keeps the drift job triggerable before master merge |
| hosted-runner build/test job | INTEGRATE | D-09's build/test letter on ubuntu-latest |
| self-hosted runner label targeting | INTEGRATE | D-08 — zcode corpus stays local |
| run artifacts (upload-artifact) | INTEGRATE | D-10 — drift report uploaded on EVERY run |
| issues REST (create) via github-script | INTEGRATE | D-10 — auto-open issue on drift; object-arg body avoids shell injection |
| GITHUB_TOKEN permissions scoping | INTEGRATE | least privilege: contents: read; issues: write on the drift job only (Pitfall 6) |
| pull_request_target trigger | OPT-OUT | not needed — schedule + dispatch only; removes the untrusted-PR self-hosted execution surface (T-24-04-01) |
| releases / deployment / environments | OPT-OUT | not needed — the nightly ships nothing; goreleaser stays the release vehicle |
| PR comments / review summaries | OPT-OUT | not needed — the unattended signal is issue-based per D-10 |
| Actions secrets store | OPT-OUT | not needed — the nightly spends zero API tokens (D-09) and needs no credentials beyond GITHUB_TOKEN |
| Actions cache (go module/build cache) | OPT-OUT | not needed yet — nightly cadence makes cold builds acceptable; tracked as a workflow-speed follow-up if run time matters |
| workflow status badges / Pages | OPT-OUT | not needed — no public reporting surface in scope |

# No external API integration — Zed MCP surface (DOC-01)

No agent-side integration: plan 24-03 is documentation-only by operator decision 2026-08-25 — the guide DESCRIBES Zed's `context_servers` configuration and ass-guard's `.mcp.json` loading (existing in-tree behavior, no new code); the dry-run configures an existing MCP server, it does not integrate a new external API into the product.
