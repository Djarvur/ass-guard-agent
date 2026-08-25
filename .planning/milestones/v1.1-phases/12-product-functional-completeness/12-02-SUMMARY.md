---
phase: 12-product-functional-completeness
plan: 02
subsystem: api
tags: [claude-code-plugins, installed-plugins-json, discovery, hooks, subagents, mcp, go]

# Dependency graph
requires:
  - phase: 08-slash-command-kickoff
    provides: the ecosys discovery chain (skills/commands/plugins), SkillListing dynamic merge, coreexec RegisterCore chokepoint, shadow-warning stderr seam
provides:
  - Installed-plugin discovery (installed_plugins.json v1+v2 registry, cache layout, .claude-plugin manifests) from PROJECT .claude/plugins/ + USER ~/.claude/plugins/ as the two lowest precedence tiers
  - All five plugin contribution kinds merged natively: skills/, commands/, agents/ (spawnable subagent types), hooks/hooks.json (lifecycle runner), .mcp.json (lowest MCP layer through the existing host)
  - ecosys.HookRunner — bounded, sanitized, exit-2-refusing Claude-Code hook semantics at UserPromptSubmit/PreToolUse/PostToolUse/Stop/SubagentStop/SessionStart/SessionEnd seams
  - The runtime write-boundary proof extended to the plugin pass (content + mtimes pinned)
affects: [14-adoption-readiness, 13-openspec-workflow-completion, v1.2 plugin lifecycle PLUG-03]

# Actuals (#2632) — pairs with the plan's estimate to calibrate future estimates.
# Same estimateTokens scale (chars/4 over the realized diff), never a harness token count.
actuals:
  tokens: 33300    # 133,349 diff chars / 4 over the 28 files actually changed
  tasks: 4
  commits: 9

# Tech tracking
tech-stack:
  added: []   # zero new dependencies (v1.1 discipline) — stdlib JSON/YAML only
  patterns:
    - "plugins-as-native-consumer: installed-cache layout parsed beside the existing simple manifest shape, same skill/command readers"
    - "hook context injects as a trailing System block on the session profile (the skills-listing vehicle) — never into the user message"
    - "out-param accumulators for slice-typed registry fields (Hooks) beside by-value map mutation"
    - "presence-gated live probe: real roots read read-only, t.Skip without them"

key-files:
  created:
    - internal/ecosys/hooks.go
    - internal/ecosys/hooks_test.go
    - internal/ecosys/testdata/plugins-installed/ (registry + cache fixture: manifest, skill, command, agent, hooks.json, .mcp.json)
    - internal/session/subagent_types_test.go
    - internal/session/hooks_seam_test.go
    - internal/coreexec/hooks_wiring_test.go
  modified:
    - internal/ecosys/loader.go
    - internal/ecosys/types.go
    - internal/ecosys/skills.go
    - internal/ecosys/doc.go
    - internal/session/session.go
    - internal/session/subagent.go
    - internal/coreexec/register.go
    - cmd/ass-guard/acp_serve.go

key-decisions:
  - "Both installed_plugins.json shapes parsed (v1 array + the v2 object the operator's machine actually carries); project-scoped v2 entries gated by projectPath so other projects' installs never leak"
  - "Plugin contributions merge as the two LOWEST tiers (project plugins over user plugins); the locked D-06 direction of the existing chain is untouched — plan-truth's 'ass-guard root >' phrasing was internally inconsistent with D-06 and the code; documented as implemented"
  - "Exit-2 PreToolUse refusal is the ONE blocking hook semantic (operator policy channel, not a confirmation tier); exit 2 on other events has no seam and warns+proceeds"
  - "Unmapped events (Notification, PreCompact) parse + warn + observe-only"
  - "SessionStart fires lazily at the first Prompt (the session's first activity is its turn); SessionEnd in Close"
  - "User+project .claude/skills|commands|agents/ stay first-class; agents/ is a new chain entry in both trees"

patterns-established:
  - "toolsList frontmatter tolerance: YAML list, space-separated scalar (Claude Code allowed-tools), and comma scalar all decode — strict []string dropped 40 real skills (live-proven)"
  - "installPath containment: symlink-resolved path must stay under the symlink-resolved root; 1 MiB artifact read caps"

requirements-completed: [ACP-10]

# Coverage metadata (#1602) — one entry per shipped deliverable.
coverage:
  - id: D1
    description: "installed_plugins.json + cache-layout discovery end-to-end (skills/commands, provenance, graceful degradation, old shape regression-free)"
    requirement: ACP-10
    verification:
      - kind: unit
        ref: "internal/ecosys/loader_test.go#TestInstalledPluginsDiscoveryE2E"
        status: pass
      - kind: unit
        ref: "internal/ecosys/loader_test.go#TestInstalledPluginAbsentSkipped"
        status: pass
      - kind: unit
        ref: "internal/ecosys/loader_test.go#TestInstalledPluginMalformedManifest"
        status: pass
      - kind: unit
        ref: "internal/ecosys/loader_test.go#TestInstalledPluginOldShapeStillWorks"
        status: pass
    human_judgment: false
  - id: D2
    description: "Both roots read + the five-tier precedence matrix with shadow warnings + the write-boundary proof + the documented chain"
    requirement: ACP-10
    verification:
      - kind: unit
        ref: "internal/ecosys/precedence_test.go#TestPrecedenceMatrixPlugins"
        status: pass
      - kind: unit
        ref: "internal/ecosys/readonly_test.go#TestDiscoverWritesNothing"
        status: pass
      - kind: unit
        ref: "internal/ecosys/precedence_test.go#TestDocumentedPrecedenceChain"
        status: pass
    human_judgment: false
  - id: D3
    description: "Live evidence: the operator's real ~/.claude/plugins/ discovered read-only (8 plugins, their bundled skills, cache-internal paths)"
    requirement: ACP-10
    verification:
      - kind: unit
        ref: "internal/ecosys/precedence_test.go#TestLiveInstalledPluginsProbe"
        status: pass
    human_judgment: false
  - id: D4
    description: "Agents (plugin + first-class .claude/agents/) as spawnable subagent types; plugin .mcp.json as the lowest MCP layer"
    requirement: ACP-10
    verification:
      - kind: unit
        ref: "internal/ecosys/loader_test.go#TestInstalledPluginAgentsDiscovered"
        status: pass
      - kind: unit
        ref: "internal/session/subagent_types_test.go#TestSubagentTypeUsesAgentDef"
        status: pass
      - kind: unit
        ref: "internal/ecosys/loader_test.go#TestPluginMCPJSONMergesLowest"
        status: pass
      - kind: unit
        ref: "internal/ecosys/loader_test.go#TestAgentAndMCPTolerance"
        status: pass
    human_judgment: false
  - id: D5
    description: "hooks/hooks.json: parse, match, bounded fire with documented stdin/stdout/exit semantics, exit-2 refusal through the tool-result channel, unmapped events observe-only"
    requirement: ACP-10
    verification:
      - kind: unit
        ref: "internal/ecosys/hooks_test.go#TestHooksParseFromFixture"
        status: pass
      - kind: unit
        ref: "internal/ecosys/hooks_test.go#TestPreToolUseStdinAndExitRouting"
        status: pass
      - kind: unit
        ref: "internal/ecosys/hooks_test.go#TestHookBoundaries"
        status: pass
      - kind: unit
        ref: "internal/coreexec/hooks_wiring_test.go#TestRegisterCoreHookRefusal"
        status: pass
      - kind: unit
        ref: "internal/session/hooks_seam_test.go#TestHookSeamsFireAtLifecycle"
        status: pass
    human_judgment: false
  - id: D6
    description: "mise ci gate green across the whole repo (vet + lint + build + race tests)"
    verification:
      - kind: other
        ref: "command:mise ci"
        status: pass
    human_judgment: false

# Metrics
duration: 40min
completed: 2026-08-18
status: complete
---

# Phase 12 Plan 02: Plugin-install discovery — installed_plugins.json + cache layout, full contributions Summary

**Claude-Code plugin installs consumed as native: v1+v2 installed_plugins.json + cache layout from both `.claude/plugins/` roots, all five contribution kinds (skills/commands/agents/hooks/.mcp.json) merged under a tested five-tier precedence, with a bounded hook runner at the lifecycle seams — read-only everywhere, proven against the operator's real plugin cache.**

## Performance

- **Duration:** ~40 min (start 2026-08-18T22:01Z → complete 2026-08-18T22:41Z, wall time incl. lint conformance)
- **Tasks:** 4/4 (every task TDD: RED commit → GREEN commit; tracer gate re-verified end-to-end before expansion)
- **Files modified:** 28 (15 created, 13 modified; 2,837 insertions)
- **Commits:** 9 (4×RED, 4×GREEN, 1×lint-conformance refactor)
- **mise ci:** green (exit 0)

## Accomplishments

- **Installed-plugin discovery (Task 1, tracer):** `parseInstalledPlugins` reads BOTH registry shapes — the v1 array (the plan's ground truth) and the v2 object the operator's machine actually carries (`{"version":2,"plugins":{"name@marketplace":[scoped installs]}}`) — with projectPath gating so installs scoped to OTHER projects never leak. `discoverInstalledPlugins` resolves installPaths (relative→root, absolute→as-is) under a symlink-resolved containment check, caps artifact reads at 1 MiB, reads `.claude-plugin/plugin.json`, and reuses the SAME skill/command readers as the `.claude/` trees. Absent/malformed artifacts skip with a stderr warning naming the path; the registry keeps loading.
- **Precedence + write boundary + live probe (Task 2):** the five-tier matrix is pinned by overlapping fixtures (existing chain beats both plugin roots; project plugins over user plugins; plugin-only keys resolve; `.claude/skills|commands` first-class; exactly-one shadow warning naming both files). `TestDiscoverWritesNothing` proves content AND mtimes untouched under every probed root. doc.go documents the full chain incl. the dropped `~/.zcode/cli/plugins/` root with the operator rationale.
- **Agents + plugin MCP (Task 3):** `agents/*.md` (frontmatter + body prompt) discovered from plugin bundles AND the new first-class `.claude/agents/` chain entry; `AgentListing` renders the type list in the captured Agent-tool entry shape; a typed `subagent_type` dispatch applies the definition's Tools as the restricted set and its Prompt as a per-dispatch system block (profile COPY — the shared profile never mutated). Plugin `.mcp.json` servers parse into the neutral ServerConfig and merge as the lowest MCP layer (user `~/.claude.json` over plugin; `spawnMCP` then merges the whole set below the project `.mcp.json`).
- **Hooks (Task 4):** the live-measured hooks.json object shape parses into `Registry.Hooks` (mapped + unmapped events); `ecosys.HookRunner` fires at UserPromptSubmit (turn entry, post-expansion), PreToolUse/PostToolUse (the RegisterCore chokepoint via the `coreexec.ToolHooks` seam), Stop/SubagentStop (turn ends), SessionStart (first Prompt)/SessionEnd (Close). Documented stdin JSON (session_id, transcript_path, cwd, hook_event_name, tool_name/tool_input/tool_response/prompt), exit-0 stdout captured as context (injected as a System block on the session profile), exit-2 on PreToolUse REFUSES the call with the hook's stderr as the structured tool result, other exits warn+skip. Bounded: per-hook timeout (default 60s), sanitized env (PATH/HOME + CLAUDE_PLUGIN_ROOT only), session-workdir cwd, 30000-char output caps. No hook failure can kill a turn.

## Live Evidence (the plan's Task 2.5 leg — operator's real `~/.claude/plugins/`, read-only)

`TestLiveInstalledPluginsProbe` (presence-gated, passes on this machine; log: 8 plugins discovered):

| live plugin | marketplace | version | scope | bundled skills discovered |
|---|---|---|---|---|
| caveman | caveman | 18e45320a0b1 | user | cavecrew, caveman, caveman-commit, caveman-compress, caveman-help, caveman-review, caveman-stats |
| cc-skills-golang | samber | 1.5.0 | user | 42 skills (golang-benchmark … golang-uber-fx) |
| cc-websearch | djarvur-plugin-marketplace | 0.1.0 | user | webfetch, websearch |
| context7 | claude-plugins-official | unknown | user | — (empty install dir) |
| gopilot | gopilot | 1.0.35 | user | go-error-hygiene, go-tdd-baby-steps, gopilot |
| modern-go-guidelines | goland-claude-marketplace | 1.0.0 | user | use-modern-go |
| sequential-thinking | ai-mktpl | 0.1.11 | user | sequential-thinking (+ hooks/hooks.json + .mcp.json parsed) |
| shared-lib | ai-mktpl | 1.0.5 | user | — (+ hooks/hooks.json parsed) |

Skipped with warnings, by design: clangd-lsp, gopls-lsp, session-report (no `.claude-plugin/plugin.json` at their install paths — the plan's missing-artifact skip), and every project-scoped registry entry belonging to other projects. **Bonus live proof:** during the full `mise ci` run the operator's REAL shared-lib SessionStart hook fired inside the cmd test suite and degraded exactly as designed (`hook [SessionStart] exited 1 (skipped): … CLAUDE_PLUGIN_DATA not set`) — the live wiring executes real plugin hooks and never fatals.

## Task Commits

Each task committed atomically, TDD:

1. **Task 1 (tracer): installed-plugins discovery** — `a9d8a47` (RED) → `b1ce826` (GREEN)
2. **Task 2: precedence matrix + write boundary + live probe + doc chain** — `137a7b6` (RED) → `70b7909` (GREEN)
3. **Task 3: agents + plugin .mcp.json** — `6679b1c` (RED) → `dcc9101` (GREEN)
4. **Task 4: hooks parse/match/fire/boundary** — `3a12c92` (RED) → `96b4051` (GREEN)
5. **Lint conformance + inject-vehicle correction** — `32f5a50` (refactor)

## Files Created/Modified

- `internal/ecosys/loader.go` — installed-plugin machinery (parse/resolve/containment/walk/decompose), shared dir-scoped readers, agents discovery, hooks + MCP wiring, five-tier merge
- `internal/ecosys/types.go` — Plugin provenance (Source/Version/Scope/InstallPath), Agent, Registry.Agents/Hooks
- `internal/ecosys/hooks.go` — the HookRunner (parse, match, payload, bounded exec, exit classification, seams)
- `internal/ecosys/skills.go` — AgentListing (the captured type-entry shape)
- `internal/ecosys/doc.go` — the documented chain + dropped-root rationale
- `internal/session/session.go` / `subagent.go` — SubagentTypes resolution + per-dispatch profile copy; the five hook seams; context inject
- `internal/coreexec/register.go` — the ToolHooks seam + withHooks refusal wrap
- `cmd/ass-guard/acp_serve.go` — sessionFor wiring (agent listing, SubagentTypes, HookRunner into both seams), MCP merge below project config
- `internal/ecosys/testdata/plugins-installed/` — committed offline fixture tree (registry + cache: manifest, skill, command, agent, hooks.json, .mcp.json, one disk-absent entry)

## Decisions Made

- Both registry shapes (v1 + live v2) parse; v2 project-scoped installs apply only to their own project.
- The existing D-06 chain direction is untouched; plugin tiers sit below it. The plan-truth's chain phrasing ("ass-guard root > project `.claude/`") conflicted with the locked D-06 (`.claude/` over `.ass-guard/`); implemented + documented as D-06-unchanged with plugins below both — "existing chain entries override plugin contributions" holds either way.
- Exit-2 blocking exists ONLY at PreToolUse (the plan's named policy channel). All other failures/exit-2s warn + proceed (AUD-03).
- Hook context injects as a System block (see deviation 3).
- SessionStart fires lazily at the first Prompt (the session-create equivalent inside internal/session; sessionFor has no turn ctx).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing critical] Tolerant frontmatter tools parsing**
- **Found during:** Task 2 (live probe)
- **Issue:** Real-world SKILL.md frontmatter uses Claude Code's space-separated `allowed-tools:` scalar; the strict `[]string` unmarshal failed the WHOLE file — the live probe proved cc-skills-golang 1.5.0 losing all 42 skills silently.
- **Fix:** `toolsList` (list / space-scalar / comma-scalar all decode). Commit `70b7909`.

**2. [Rule 2 - Missing critical] v2 installed_plugins.json support**
- **Found during:** Task 1 implementation
- **Issue:** The plan's ground-truth block described the v1 array only; the operator's real registry is the v2 object with per-plugin scoped-install lists. A v1-only parser would have discovered NOTHING on this machine — the live-evidence leg would have been vacuous.
- **Fix:** Both shapes parse; first applicable install per key; projectPath gating. Commits `b1ce826`/`70b7909`.

**3. [Rule 1 - Bug] Hook context-inject vehicle corrected**
- **Found during:** the full-gate run (Task 4 close-out)
- **Issue:** The first vehicle appended hook stdout as a content block on the USER message — the operator's real sequential-thinking SessionStart hook fired in the cmd suite and broke `TestExpansion_UnknownCommandFallsThrough` (the recorded user turn must stay byte-pure; Claude Code injects as context, not user text).
- **Fix:** Context injects as a trailing System block on the session profile (the skills-listing vehicle); the user_message line stays pure; the seam test re-pinned. Commit `32f5a50`.

**4. [Rule 1 - Bug] By-value slice loss for Registry.Hooks**
- **Found during:** Task 4 GREEN
- **Issue:** `reg.Hooks = append(reg.Hooks, …)` inside the by-value `discoverInstalledPlugins` was invisible to callers (maps mutate through the receiver; slices do not) — parsed hooks never reached the registry.
- **Fix:** `hooksOut *[]HookConfig` accumulator. Commit `96b4051`.

**5. [Rule 3 - Blocking] manifest-version fallback lost in decomposition**
- **Found during:** lint-refactor decomposition
- **Issue:** The extracted `resolveInstalledPlugin` mutated a value copy's `entry.Version`; the manifest version fallback would silently drop.
- **Fix:** The effective version returns explicitly (regression-covered by the fixture's registry-carried version + live v2 `unknown` values). Commit `32f5a50`.

---

**Total deviations:** 5 auto-fixed (2 missing-critical, 2 bugs, 1 blocking)
**Impact on plan:** All necessary for real-world correctness — two were proven against the operator's LIVE machine. No scope creep; zero new dependencies; redlines intact (writes nowhere — mtime-proven).

## Issues Encountered

None beyond the deviations above. The plan's file lists were non-exhaustive as expected: `cmd/ass-guard/acp_serve.go` (sessionFor/spawnMCP/loadCommandRegistry) and `internal/session/subagent.go` carry the wiring the plan's actions named by symbol.

## Known Stubs

None. Every contribution kind is wired end-to-end; no placeholder paths.

## Threat Flags

None beyond the plan's threat model — all seven register rows were mitigated as specified (containment, caps, precedence, write-boundary proof, sanitized hook env, timeouts, MCP through the existing host). The one surface addition within the plan's own register: hook execution fired inside the test suite (live hooks) — the exact T-12-02-06 posture documented (operator-installed config, same trust tier as the hook-DAG).

## User Setup Required

None — no external service configuration.

## Next Phase Readiness

- ACP-10 delivered per the 2026-08-19 operator revision (both `.claude/plugins/` roots, all five contribution kinds, zcode root dropped, first-class `.claude/` entries, read-only).
- Ready for the Phase 14 dispatch order recorded in STATE.md (`/gsd:plan-phase 14` next).
- Corpus-absent forms flagged for the re-capture: the agent-listing header text, the hook-context System block, and the hooks stdin payload live entirely outside any captured session.

## Self-Check: PASSED

All 6 created key-files exist on disk; all 9 task commits exist in history; the live probe evidence was captured from the real roots this session.

---
*Phase: 12-product-functional-completeness*
*Completed: 2026-08-18*
