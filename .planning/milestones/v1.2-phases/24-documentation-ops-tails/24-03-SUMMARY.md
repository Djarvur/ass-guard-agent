---
phase: 24-documentation-ops-tails
plan: "03"
subsystem: documentation
tags: [lsp, mcp, zed, ide-integration, docs]

requires:
  - phase: 24-documentation-ops-tails
    provides: research-pinned three-leg Zed surface (context_servers forwarding semantics, zed#52449 gap, ass-guard parse-but-ignore + .mcp.json loading seams)
provides:
  - docs/lsp-setup.md — IDE-side MCP configuration guide for LSP tools in ass-guard sessions (DOC-01, documentation only)
  - README docs-index entry linking the guide (D-02)
  - Recorded D-03 dry-run evidence (followable end-to-end, six mcp__gopls__* tools observed in a real session)
affects: [verify-work phase 24 UAT, docs index readers, future agent-side MCP-forwarding work (out of scope here)]

actuals:
  tokens: 4705    # chars/4 over the realized diff (docs/lsp-setup.md + README.md)
  tasks: 2        # auto tasks executed (the decision checkpoint was auto-resolved, not a task commit)
  commits: 2      # MEASURED: git rev-list --count 7ce8d905..HEAD
plan_head_before: 7ce8d905d4f5147c71d3da278ea92fd8b18820d8

tech-stack:
  added: []       # documentation-only — zero new dependencies (operator decision 2026-08-25)
  patterns:
    - "Honest-status doc pattern: three-leg capability table with a per-leg 'reaches ass-guard sessions?' column (drift absorber for Zed's moving surface)"
    - "Hermetic dry-run verification: scripted newline-delimited JSON-RPC ACP client + stub Anthropic-SSE provider against the REAL binary (the simulator_e2e_test.go discipline, externalized as a doc-embedded script)"

key-files:
  created:
    - docs/lsp-setup.md
  modified:
    - README.md

key-decisions:
  - "Auto-selected (auto_advance, gate=blocking): worked-example server = isaacphi/mcp-language-server + gopls (research default option 1); legitimacy verified at execution — 1,590 stars, BSD-3-Clause, active (pushed 2026-03-01), install via go install github.com/isaacphi/mcp-language-server@latest (outside the npm/pip/cargo gate scope; no curlpipe)"
  - "Doc states ass-guard's discard of ACP-forwarded mcpServers as today's behavior with the code anchor (handleSessionNew in internal/acp/handlers.go, field :338 today) — handler line numbers had shifted from research's :253-261; function-name citation used so the doc survives drift"
  - "Dry-run fix loop applied per plan: mcp.Start's failed-server skip is SILENT (doc comment claims slog logging that does not exist) — doc corrected to say no-stderr-note, PATH named as the top cause, observability gap logged to phase deferred-items.md"

patterns-established:
  - "Followable-dry-run-as-evidence (D-03): the doc's verification step is executable by any reader with Go — no editor, no model API spend"

requirements-completed: [DOC-01]

coverage:
  - id: D1
    description: "docs/lsp-setup.md — three-leg Zed LSP surface guide with honest status table, Zed-side + ass-guard-side setup, dry-run walk, known limitations, explicit agent-side-OUT scope statement"
    requirement: DOC-01
    verification:
      - kind: other
        ref: "grep gate: DOC-STRUCTURE-OK (context_servers/mcpServers/.mcp.json/diagnostics/52449/out-of-scope all present; re-run green after dry-run fixes)"
        status: pass
      - kind: other
        ref: "prohibition greps: no affirmative forwarded-mcpServers-consumption claim; context_servers is the only cited Zed settings key"
        status: pass
      - kind: manual_procedural
        ref: "D-03 dry-run executed top-to-bottom: catalog check script exit 0, six mcp__gopls__* tools observed from the real binary (final re-run against the fixed doc)"
        status: pass
    human_judgment: true
    rationale: "The doc's live editor leg (step 7: open Zed, prompt the agent to call the tools) is not executable in the executor environment — a GUI-editor confirmation remains for the operator/UAT; everything up to and including the scripted catalog proof ran green."
  - id: D2
    description: "README docs-index link 'lsp setup' -> docs/lsp-setup.md, adjacent to install, existing mid-sentence dot-separated style (D-02)"
    requirement: DOC-01
    verification:
      - kind: other
        ref: "grep -q 'lsp setup' README.md && grep -q 'docs/lsp-setup.md' README.md -> LINK-OK"
        status: pass
    human_judgment: false

duration: 26min
completed: 2026-09-10
status: complete
---

# Phase 24 Plan 03: DOC-01 LSP Setup Guide Summary

**IDE-side MCP configuration guide (docs/lsp-setup.md) proving LSP-symbol tools reach ass-guard sessions via the project `.mcp.json` leg — with a followable dry-run whose executed evidence shows six `mcp__gopls__*` tools in a real session catalog.**

## Performance

- **Duration:** 26 min
- **Started:** 2026-09-10T14:26:02Z
- **Completed:** 2026-09-10T14:52:31Z
- **Tasks:** 2 (plus 1 auto-resolved decision checkpoint)
- **Files modified:** 2

## Accomplishments

- `docs/lsp-setup.md` — the three-surface status table (Zed native `diagnostics` tool = Zed-Panel-only; `context_servers` forwarding = forwarded but parsed-and-ignored by ass-guard today; project `.mcp.json` = the leg that works), Zed-side and ass-guard-side setup sections, a fully executable dry-run, and known limitations — every external claim cited to zed.dev docs or an in-repo anchor, every Zed-shape claim re-verified live on 2026-09-10.
- README docs index extended with the `lsp setup` link in the existing mid-sentence style, adjacent to `install` (D-02).
- D-03 dry-run EXECUTED (not reviewed): the author followed the doc top-to-bottom and reached working LSP-symbol tools inside a real ass-guard session; evidence below.

## Decision Checkpoint (auto-resolved)

The plan's `checkpoint:decision` (gate="blocking") asked which LSP-capable MCP server anchors the worked example. Auto mode is active (`workflow.auto_advance: true`), and the gate is not `blocking-human`, so per the auto-mode protocol the planner-front-loaded first option was selected: **⚡ Auto-selected: mcp-language-server** (isaacphi/mcp-language-server + gopls). Legitimacy checked at execution as the option's cons required (see evidence below).

## D-03 Dry-Run Evidence (executed 2026-09-10)

**Install commands used (go install path; no npm/pip/cargo gate, no curlpipe):**

```sh
go install github.com/isaacphi/mcp-language-server@latest   # /home/nil/go/bin/mcp-language-server
go install golang.org/x/tools/gopls@latest                 # gopls v0.23.0
```

Legitimacy note: `github.com/isaacphi/mcp-language-server` — 1,590 stars, BSD-3-Clause, created 2024-12-30, last pushed 2026-03-01, not archived (GitHub API, checked 2026-09-10); exposes `definition`, `references`, `diagnostics`, `hover`, `rename_symbol`, `edit_file` per its README.

**Scratch project `.mcp.json`** (`/tmp/ass-guard-lsp-dryrun`, module `example.com/dryrun`, one `greet` function):

```json
{
  "mcpServers": {
    "gopls": {
      "command": "mcp-language-server",
      "args": ["--workspace", "/tmp/ass-guard-lsp-dryrun", "--lsp", "gopls"]
    }
  }
}
```

**Catalog check:** the doc's embedded script (`/tmp/lsp-dryrun-check.py`, extracted verbatim from the doc) starts a stub Anthropic-SSE provider on localhost, points the scratch `.ass-guard/config.yaml` at it, spawns `/tmp/ass-guard-dryrun acp serve --work-dir <scratch>` (real binary, built from this tree via `go build ./cmd/ass-guard/`), drives `initialize` → `session/new` → `session/prompt` over stdio newline-delimited JSON-RPC, and inspects the captured provider request. Observed (final run against the fixed doc, exit 0):

```text
provider requests captured: 1
mcp tools in session catalog:
  mcp__gopls__definition
  mcp__gopls__diagnostics
  mcp__gopls__edit_file
  mcp__gopls__hover
  mcp__gopls__references
  mcp__gopls__rename_symbol
```

(The machine's other user/plugin-level MCP servers also appear in the live catalog — `mcp__node_repl__*`, `mcp__plugin_*` — which motivated the doc's expected-output honesty fix.)

**Dry-run failure pass (the useful part):** the first run listed NO gopls tools. Root cause: `~/go/bin` was not on the spawning process's PATH, so ass-guard could not resolve `mcp-language-server` — and the skip was silent (see Deviations note on the doc fix). With the doc's own step-1 `PATH` export in the same shell (exactly what a top-to-bottom run does), the check passed. A standalone SDK probe using the repo's exact `go-sdk v1.7.0` additionally proved Connect+ListTools against this server pair succeed in 1.4s, isolating the cause to environment resolution, not protocol.

**zed#52449 re-check (2026-09-10):** still `state: open` ("Extension-installed MCP servers not available to local ACP agents", updated 2026-06-11) via the GitHub API — the doc's caveat line "open as of 2026-09-10" is current.

**Step 7 (live editor leg):** not executable in the executor environment (GUI editor); the doc keeps it as the human follow-through. This is the one leg left for operator/UAT confirmation.

## Task Commits

Each task was committed atomically:

1. **Task 1: write docs/lsp-setup.md** - `6e5c2a5` (docs)
2. **Task 2: README index link + executed D-03 dry-run + doc fixes** - `1388c28` (docs, amended once to drop an unintended README mode-change)

**Plan metadata:** (final commit below)

## Files Created/Modified

- `docs/lsp-setup.md` — the guide (created; +19-line dry-run-driven fix pass in Task 2)
- `README.md` — docs index entry (1 line)

## Decisions Made

- Auto-selected the research-default worked-example server (see checkpoint section).
- Cited the parse-but-ignore site by function name (`handleSessionNew` in `internal/acp/handlers.go`) rather than research's now-drifted line numbers.
- Named the silent MCP-skip reality in the doc (verified in code) instead of the code-comment's claimed slog logging; logged the observability gap to `deferred-items.md` (fix is agent-side code, out of scope by the 2026-08-25 operator decision).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] README.md mode change bundled into the Task 2 commit**
- **Found during:** Task 2 commit inspection
- **Issue:** The working-tree README.md carried a pre-existing 100755 mode (not mine); the commit would have recorded an unintended 100644→100755 flip alongside the 1-line index addition.
- **Fix:** `chmod 644` + `git commit --amend` — the amended commit `1388c28` contains only content changes.
- **Files modified:** README.md
- **Verification:** `git show --stat 1388c28` shows no mode change.
- **Committed in:** 1388c28 (amend)

The dry-run doc fixes (expected-output honesty, silent-skip correction, same-shell PATH note) are NOT deviations — the plan prescribes them ("Wherever the doc's steps diverge from reality, FIX THE DOC ... and re-run"); the affected step was re-executed green after the fix.

---

**Total deviations:** 1 auto-fixed (1 bug — unintended commit content).
**Impact on plan:** None — no scope creep; documentation-only invariant preserved (zero Go/config/workflow files touched).

## Issues Encountered

- Dry-run first pass failed with no gopls tools (see evidence section): environment PATH, not a doc bug — but the pass exposed that `mcp.Start` skips failed servers with zero stderr output despite its comment claiming slog logging. Doc corrected to describe reality; code gap logged to `deferred-items.md` (out of scope).

## User Setup Required

None - no external service configuration required.

## Known Stubs

None — documentation-only plan; no code, config, or placeholder surfaces introduced.

## Next Phase Readiness

- DOC-01 complete: guide shipped, linked, and dry-run-proven. Remaining plans 24-02 (TAIL-01 D-06/D-07 wiring), 24-04 (TAIL-02 nightly CI), 24-05 (TAIL-03 matrix) are unaffected by this plan (no shared files).
- For UAT: the doc's live editor leg (step 7) is the one human-confirmable surface.

---
*Phase: 24-documentation-ops-tails*
*Completed: 2026-09-10*

## Self-Check: PASSED
