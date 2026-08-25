# Feature Research — v1.2 Claude Code Parity

**Domain:** AI coding agent — ACP-native surfaces, chat commands, subagents, steering (v1.2 scope only)
**Researched:** 2026-08-26
**Confidence:** HIGH for ACP protocol shapes and Claude Code documented behavior (official docs + live schema); MEDIUM for compaction internals, Zed panel specifics, and steering-semantics comparisons (community/version-drift sources)

---

## Verified Landscape Facts (answers to the research questions)

These are the expected-behavior facts each v1.2 feature should match. Confidence tagged per block.

### (a) Claude Code built-in slash-commands — exact behavior [HIGH]

Source: official code.claude.com docs (`/commands`, `/costs`, `/model-config`, `/settings-reference`).

| Command | What it does | State changed | UI presented |
|---------|--------------|---------------|--------------|
| `/model` | Switches the main-loop model mid-session (`/model sonnet`). Direct-argument command — works even in non-TUI clients | Session-scope model routing | Argument-less: model selector; with arg: applies immediately |
| `/config` | Opens settings panel OR accepts direct `key=value` (e.g. `/config verbose true`) touching theme, verbose, autoCompactEnabled, output styles | Settings files (scoped) | Interactive arrow-key panel; Esc exits; enum items cycle on Enter |
| `/compact` | Summarizes conversation history to free context. Optional focus instructions (`/compact focus on the DB changes`) steer the summary | Replaces projected history with summary + kept-recent tail | After: shows summary size vs capacity |
| `/clear` (aliases `/reset`, `/new`) | Starts new empty-context conversation; optionally labels the previous session so it's findable in the `/resume` picker | New session id; resets usage counters too | Confirmation-free |
| `/cost` | Total cost, API duration, wall duration, lines added/removed, **per-model** breakdown with cache-read/cache-write tokens | None (read-only aggregation) | Text block |
| `/resume` | Opens interactive session picker; searchable, also by PR URL. **Cannot run remotely** (needs local terminal UI) | Loads selected session | Full-screen picker |
| `/memory` | Picker of memory-file locations (project `CLAUDE.md`, user `~/.claude/CLAUDE.md`, project-local) → opens chosen file in $EDITOR | File edits persist | Picker menu → editor |
| `/mcp` | Lists configured MCP servers with connection status, auth status, tools exposed; triggers OAuth flows for servers needing it | Can initiate auth | Server list w/ per-server detail |
| `/permissions` | Interactive editor for **allow / ask / deny** rule sets. Rule syntax `Tool(specifier)`: `Bash(npm run *)`, `Read(./.env)`, `WebFetch(domain:example.com)`, `mcp__server__*` | Writes rules to scoped settings files | Tabbed rule editor |
| `/doctor` | Full checkup: installation issues, unused skills, CLAUDE.md optimization opportunities. **Asks before changing anything** | Optional fixes after confirm | Findings report → per-finding confirmations |
| `/status` | Version, model, account/auth, context-window usage, related runtime info | None | Text block |
| `/help` | Lists all commands (built-in + discovered) | None | Text list |
| `/init` | Generates a starter `CLAUDE.md` (structure, build commands, architecture, conventions); can import config from other agents; every later session auto-loads it | Creates CLAUDE.md | Generation progress → review |

Key architectural fact for us: CC itself classifies these — **text-output commands** (`/compact`, `/clear`, `/usage`, `/exit`), **direct-argument commands** (`/model`, `/effort`, `/rename`), **MCP controls**, **configuration commands** work headlessly/remotely; commands needing a *local terminal interface* (`/plugin`, `/resume`) do not. In an ACP agent the TUI pickers become client-native equivalents (session list → `session/list`; memory picker → text list or elicitation form).

### (b) Permission modes & request_permission UX [HIGH]

- **Modes:** `default` (ask per policy), `acceptEdits` (auto-accept file edits), `plan` (read-only planning), `bypassPermissions`. `disableBypassPermissionsMode: "disable"` enforces org policy against bypass. `defaultMode` in the settings `permissions` block sets the starting mode; CLI flag overrides per session.
- **Evaluation order (six steps, sequential):** Hooks → Deny rules → Ask rules → Permission mode → Allow rules → canUseTool callback. **Deny and ask beat allow and even beat bypassPermissions.** In non-interactive `dontAsk` mode, anything requiring confirmation is denied instead of prompted.
- **Rule syntax:** `Tool(specifier)` with prefix wildcards: `Bash(npm run test:*)`, `WebFetch(domain:...)`, `mcp__puppeteer__*`.
- **Persistence scopes (precedence):** enterprise managed → CLI args → `.claude/settings.local.json` → `.claude/settings.json` → `~/.claude/settings.json`. When a user picks "Always allow" at an interactive prompt, a rule is persisted at the chosen scope — that is the entire persistence story: **"always" = write a rule, not a session flag.**

### (c) ACP `session/request_permission` wire shape [HIGH — live schema]

- Request: `{sessionId, toolCall: ToolCallUpdate, options: PermissionOption[], _meta}`. Each option: `{optionId, name, kind}` where kind ∈ `allow_once | allow_always | reject_once | reject_always`.
- Response: `{outcome}` ∈ `selected{optionId}` | `cancelled`. **If the turn is cancelled while the ask is outstanding, the client MUST answer `cancelled`** — the agent must treat that identically to reject.
- Zed renders options verbatim as dialog buttons ("Allow once / Always allow / Reject"). Known early-ecosystem failure mode: agents that printed permission text instead of calling the RPC got raw-text prompts in the panel (Gemini CLI did this at first) — the RPC, not formatted text, is what buys native UX.
- **Session modes** are the other half of permissions UX: advertise `modes` in newSession/load responses (e.g. our tiers of ask/auto-edit/bypass), Zed shows a native dropdown in the panel bottom bar, `session/set_mode` switches, and `current_mode_update` pushes agent-side mode changes back into the dropdown. CC's four permission modes map onto this abstraction.

### (d) Compaction: microcompact vs auto-compact [MEDIUM — version-drifted internals]

- **Auto-compact triggers ≈92–95% context utilization** (threshold drifted across versions; warning banner appears near ~60%). Disable via `autoCompactEnabled:false` or `DISABLE_AUTOCOMPACT`.
- **What happens:** a structured summary is generated covering prior requests, actions taken, code changes, technical decisions, next steps. Old tool call results are dropped (represented only inside the summary). Preserved verbatim: system prompt, CLAUDE.md/memory injections, the most recent turns, the compact-boundary summary itself. Post-compact display shows summary size vs capacity.
- **Microcompact** is a distinct earlier housekeeping step: selectively clears older tool outputs (keep last N), placeholder-substituted. In v2.x it appears largely absorbed into uniform auto-compact handling. Not triggerable by `/compact`.
- Manual `/compact [instructions]` steers summary emphasis.
- Community consensus caveat: compaction loses file paths / early decisions; heavy users write key state to notes files before compaction, or prefer fresh sessions with hand-written handoffs. This validates SEED-004's "verify-first" stance: check what the zcode profile already captures about zcode's own auto-compact + cache_control placement before designing ours, because compaction placement interacts with prompt-cache economics.

### (e) Task/subagents: background, notifications, per-agent model [HIGH]

Official Agent SDK docs (the Task tool is exposed as `Agent`):

- **Discriminated result statuses:** `completed` (returns `agentId`, `agentType`, content blocks, **`resolvedModel`**, `modelsUsed[]`, token/tool/duration stats, optional worktree path/branch) | `async_launched` (returns `agentId`, `outputFile`, `canReadOutputFile` — fire-and-forget) | `remote_launched` (cloud session).
- **Background lifecycle events:** `TaskStartedMessage` carries `task_type` distinguishing local-Bash vs local-subagent vs remote; on finish, a **task_notification** system message carries `{task_id, status: completed|failed|stopped, output_file, summary, usage}`. Documented consumer guidance: **detect notifications by structured `origin.kind`, never by matching notice text** — directly relevant since our engine is an event-bus observer.
- **Retrieval:** `TaskOutput(task_id, block, timeout)` exists but is deprecated — the pattern is **Read the task's `output_file`**. `TaskStop` terminates; `SendMessage` steers a running background agent.
- **Per-agent model override:** subagent definitions (`.claude/agents/*.md` frontmatter or programmatic `AgentDefinition`) take `model:` = tier alias (`sonnet|haiku|opus`), `inherit`, or a full model ID; the Task call can also pass `model` directly. Result reports back the actually-`resolvedModel`. Skills support `context: fork` (isolated subagent whose prompt is the skill body; background-by-default unless `background: false`) — exactly the shape of our slash-invocable-skills feature.

### (f) Hooks lifecycle incl. PreToolUse deny [HIGH]

- Event set includes: `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `SessionStart`, `Stop`, `PreCompact`, plus `Notification`, `SessionEnd`, `SubagentStop`.
- **PreToolUse decision control — three channels:**
  1. Exit code: `0` = no opinion, normal permission flow proceeds; `2` = **block** the tool call, stderr becomes Claude's visible feedback; other = non-blocking error.
  2. Structured JSON on stdout (with exit 0): `hookSpecificOutput.permissionDecision` ∈ `allow | deny | ask | defer` (+ `permissionDecisionReason`, `updatedInput` which *replaces* the whole input object, `additionalContext`).
  3. Conflict precedence across multiple hooks: **deny > defer > ask > allow**. `ask` forces a prompt even in auto mode and displays the rule origin (User/Project/Plugin/Local).
- This is a *pre-execution interceptor in the tool-dispatch path* — architecturally different from ass-guard's existing hook-DAG, which runs **post-turn**. The parity item is a new insertion point, not an extension of the existing DAG.

### (g) @-file mentions & image paste [HIGH]

- `@path` pulls file/dir contents into the prompt context; fuzzy matching (`@auth` matches auth.js…), trailing slash for dirs, Tab autocomplete. A `fileSuggestion` setting lets large monorepos swap in a custom shell command (stdin query → newline-separated paths on stdout, 5s timeout, ≤15 suggestions shown).
- Images: pasted or dragged (shift-drag to attach as files) into the prompt; carried as **image content blocks** alongside text. Editors do the same over ACP by sending image/resource content blocks in `session/prompt`.
- `#` shortcut appends text to a memory file (mid-session memory writes).
- For ass-guard the work is content-block plumbing end-to-end: accept image/resource_link blocks from the client → carry through profile shaping → provider payload; and resolve `@mentions` arriving inside prompt text against the workspace.

### (h) Zed ACP agent panel — native surfaces other agents use [MEDIUM]

Verified from agentclientprotocol.com (tool-calls page + schema) and Zed materials:

- **Tool calls:** `session/update` with `sessionUpdate:"tool_call"` then `"tool_call_update"`. Fields: `toolCallId`+`title` (required); `kind` ∈ `read|edit|delete|move|search|execute|think|fetch|other` (drives icons); `status` ∈ `pending|in_progress|completed|failed` (older schema revisions used `error` — current docs say `failed`); `locations[{path,line}]` enable click-to-jump; `rawInput/rawOutput` for inspection; `content[]` blocks include text/image/**`diff{path, oldText /* null = new file */, newText}`**/**`terminal{terminalId}`** — diffs render inline in the panel. Updates are sparse: only changed fields beyond `toolCallId`.
- **Plan:** `plan` update carrying `PlanEntry[]{content, priority: high|medium|low, status: pending|in_progress|completed}`; rendered as a native checklist. (Spec reordered the priority enum between revisions — treat values as stable, ordering as presentation.)
- **Commands autocomplete:** `available_commands_update` with `AvailableCommand{name, description, input?: {hint}}` powers the `/` popup in the prompt editor. The `hint` string is what shows before args are typed — good place for e.g. `model name or tier`.
- **Elicitation:** `elicitation/create` is a union — `form{requestedSchema}` | `url{elicitationId, url}` | `other{mode}`; response action `accept` (optional `content` payload) / `decline` / `cancel`; follow-up `elicitation/complete{elicitationId}` notification for async completion (url mode).
- **Sessions:** `session/list` (capability `list`; `cursor` + optional `cwd` filter → `sessions: SessionInfo[]`, `nextCursor`), `session/load{sessionId, cwd, mcpServers, additionalDirectories}`, `session/close` and `session/delete` (capabilities `close`/`delete`). Capabilities are declared at initialize — the client only offers the UX the agent declares.
- **Editor-driven config:** newSession/load responses may carry `modes` + `configOptions`; `session/set_config_option` is a union (`boolean` value | `value_id` reference) keyed by `configId`, and its response returns **the full updated configOptions array** (authoritative round-trip). `config_options_update` pushes changes to the client.
- **Thought/message chunks:** `agent_thought_chunk` / `agent_message_chunk` stream natively (thinking blocks show styled in Zed).

### (i) Steering queues mid-turn [MEDIUM]

Two reference implementations converge on the same insight: the only safe injection points are boundaries between LLM calls.

- **pi (badlogic/pi-mono):** two user-selectable modes (Tab toggles; default via settings `steeringMode`):
  - *Steering:* queued message is injected into the conversation **as soon as the current tool call finishes, before the next API call** — the model sees it mid-turn and can change course without losing work done so far.
  - *Follow-up:* message waits until the whole turn completes, then enters as a new message.
  - Esc always means abort-now. Rationale (author's words): you want to be able to steer at any point with full control over the loop.
- **Strands Agents (Python):** shipped "queue input during running turn" (issue #855 → PR #913). Formal interrupt machinery: interruptible tools raise through an `InterruptState`; agent returns `stop_reason="pause_turn"`; caller re-invokes the agent with collected input to resume. Pausing requires a session manager (persistence).
- Implication for ass-guard: a **bounded queue drained at tool-call boundaries** (steering) with an opt-in "wait for turn end" mode covers both references; abort remains `session/cancel` (already shipped).

---

## Feature Landscape — v1.2 Areas

### Area 1 — ACP completeness (priority 1)

#### Table Stakes (Users Expect These)

| Feature | Why Expected | Complexity | Notes / Dependency on existing |
|---------|--------------|------------|-------------------------------|
| `session/request_permission` clickable asks | Every serious ACP agent does this; plain-text gates feel broken in Zed | MEDIUM | Wire shape simple; hard part is the pending-ask state machine during a running turn + mandatory `cancelled` handling. Replaces today's plain-text tool-gate prompts. Depends on: tool-execution gate seam (exists), cancel-drain (exists) |
| `tool_call` + `tool_call_update` streaming | Panel cards are how users watch progress; absent = black box | MEDIUM | Map catalog tools → `kind` (static per tool) + lifecycle statuses; extract `diff` blocks from Edit/Write inputs; `locations` from Read/Grep targets. Depends on: tool catalog (103 tools, exists), tracer/event bus (exists) |
| `available_commands_update` | `/` autocomplete is the discoverability surface for everything else in this milestone | LOW | We already discover commands + (soon) skills; this just advertises them. Emit on discovery + after skill/config changes. Depends on: ECOS discovery (shipped v1.1) |
| `plan` update streaming | Users expect a visible checklist during multi-step SDD runs | LOW–MEDIUM | Hook-DAG stages map naturally to plan entries; emit on stage transitions. Depends on: unified engine stage events (shipped Phase 4) |
| Session list/resume/close/delete | Operator must-have (D-09 reversal); every ACP client expects `loadSession` | MEDIUM | Replay-on-load exists (Phase 2). Add: durable session metadata store, `SessionInfo` projection, capability declaration `list/close/delete`, tombstoning on delete. Depends on: two-layer context + replay (shipped) |
| Editor-driven configuration (configOptions / set_config_option) | Native switcher beats editing YAML; Zed renders it free | MEDIUM | Advertise e.g. tier default + active model as configIds; handle boolean/value_id variants; respond with full authoritative array. **API keys stay env/file by explicit project decision** |
| `elicitation/create` (form mode) | Structured asks (learning-mode questions fit perfectly) surfaced natively | LOW once permission path exists | Same JSON-RPC pattern as request_permission; `requestedSchema` → our ask-once-remember learning prompts. URL mode deferred |

#### Differentiators (Competitive Advantage)

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Learning-store questions via elicitation forms | Ask-once-remember becomes a clickable form instead of a text question — unique among ACP agents | LOW | Rides elicitation/form + existing learning store |
| Audit-explaining tool cards | `rawInput`/`rawOutput` + redaction discipline gives users inspectable tool calls (ties to investigate-and-fix-ready constraint) | LOW | Mostly a projection of existing audit/tracer data |

#### Anti-Features

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| Credentials via editor settings | Convenience | Explicitly rejected in PROJECT.md (secrets leak into plaintext editor config/sync) | Env/file credential resolution (shipped Phase 7) |
| `elicitation/url` mode at launch | Protocol completeness | Needs browser-roundtrip + elicitationId lifecycle; zero current callers | Form mode now; url mode when a concrete need appears |

### Area 2 — Built-in chat commands (priority 2)

#### Table Stakes

| Command | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| `/help` | Zero-cost discoverability baseline | LOW | List built-ins + discovered commands; rides available_commands_update data |
| `/status` | First diagnostic users reach for | LOW | Version (ldflags exists), active model/tier, session id, context-window % (projector knows) |
| `/model` | Mid-session routing switch; CC treats it as direct-argument | LOW–MEDIUM | Maps to session-scope scheduler override; precedence must slot under existing D-02 chain. Depends on: scheduler resolver (shipped Phase 3) |
| `/clear` | Context reset without restarting the editor session | LOW–MEDIUM | "New session same thread": new sessionId, preserved ACP connection; label previous for resume picker. Depends on: session core |
| `/compact` | THE context-management command; advertised as prerequisite in PROJECT.md | HIGH (inherits compaction cost) | Blocked on compaction existing. Optional focus instructions → summary prompt param |
| `/cost` | Usage transparency; subscription users expect per-model splits | LOW–MEDIUM | Aggregate from durable transcript records (they exist); per-model split needs usage captured per turn — verify capture exists, else add accounting |
| `/resume` | Pairs with session list family | MEDIUM | In ACP the picker is client-side; agent-side `/resume` = text list + selection or defer to client's native loader. Do not rebuild a TUI picker |
| `/memory` | Memory-file visibility/editing | LOW–MEDIUM | Text-list + contents dump (text-output class); editor opening is client territory. `#` append shortcut is CC-TUI-specific — skip |
| `/mcp` | Server health is the top debugging need | LOW | Aggregate connection state of hosted MCP subprocesses (reaper/process manager exists from Phase 5) |
| `/permissions` | Visibility into the gate rules | MEDIUM | Read-only rule dump first (what would run ungated vs ask); interactive editing deferred to editor-config/rules phase |
| `/config` | Settings visibility | MEDIUM | Rides editor-config configOptions for what's switchable; dumps static config otherwise |
| `/init` | Onboarding convention | MEDIUM | Generate AGENTS.md/CLAUDE.md starter from repo scan; mirrors CC behavior including import-from-other-agents offer |
| `/doctor` | Trust-building diagnostics | MEDIUM | Checks: config parse, credential resolution (lazy factory gives typed errors), discovery scan, provider reachability, scheduling validate. Asks before fixing |

#### Anti-Features

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| TUI-style interactive pickers inside commands (`/resume` fullscreen picker etc.) | CC parity reflex | We have no terminal UI (explicit out-of-scope); ACP clients own list UX | Expose data via `session/list` + text output; let Zed render |
| Theme/vim-mode/output-style config keys | CC parity checklist | Meaningless without a TUI; config bloat | Support only agent-meaningful configOptions |

### Area 3 — Slash-invocable skills (priority 3)

#### Table Stakes

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| `invocationFor` resolves skill keys | CC types `/skill-name args` and the SKILL.md body becomes the prompt; users expect identical muscle memory | LOW–MEDIUM | Discovery already walks skills; add key→invoker resolution + body-expansion-with-appended-args at the command-expansion seam |
| AGENTS addressable as slash commands | BMad-style installer layouts put agents in `.claude/agents/`; typing `/bmad-agent-x` should just work | LOW | Same discovery walk; register agent names into the command table |
| Per-agent `model:` wired into dispatch | Frontmatter parsed-but-ignored today; CC resolves tier alias / `inherit` / full ID and reports resolvedModel back | MEDIUM | Resolution order: agent frontmatter → dispatch-time param → session default → tier. Depends on: scheduler resolver; report resolvedModel in result metadata |

#### Differentiators

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| `context: fork` skills as background subagents | Long skills stop blocking the main thread; matches CC's newest skills behavior | MEDIUM–HIGH | Needs background-agent infra from parity area first — sequence after it |
| Skill-invocation telemetry in `/cost` | Per-skill spend attribution nobody else shows | LOW | Rides usage accounting |

### Area 4 — CC parity audit: 10 divergences (priority 4)

| Divergence | Expected behavior (CC) | Complexity | Notes |
|-----------|------------------------|------------|-------|
| Compaction on context overflow | Auto-summary ≈92–95%, preserve system+memory+recent tail, drop old tool results | HIGH | Verify-first (SEED-004): inspect zcode profile for existing auto-compact + cache_control evidence before designing. Summary generation = light-tier call. Interacts with two-layer projector — natural boundary |
| Session resume | Covered in Area 1 | — | Rides ACP session family; no separate design |
| Permissions UX | Covered in Area 1 | — | Rides request_permission + modes |
| Full subagents (Task + background + notifications) | Discriminated completed/async_launched results; structured task_notification events; output-file retrieval; SendMessage steering; TaskStop | HIGH | Goroutine turn-loops exist; add: task registry, output files, notification events on the bus, per-task ids threaded through ACP updates. Detect-by-kind not text-match (official guidance) |
| Slash autocomplete in editor | Covered in Area 1 | — | Rides available_commands_update |
| Hooks PreToolUse deny | Exit-2 block w/ stderr feedback; JSON permissionDecision allow/deny/ask/defer; deny>defer>ask>allow precedence; updatedInput replacement | MEDIUM | NEW insertion point pre-tool-dispatch — distinct from post-stage hook-DAG. Keep DAG untouched; add a pre-execution interceptor consulting the same hook configs where applicable |
| AGENTS.md/CLAUDE.md auto-injection | Startup context loads global user prefs + project memory automatically | LOW | Discovery already reads these trees; inject into system context at session start (respecting precedence) |
| Streamed thinking blocks | `agent_thought_chunk` streams CoT; Zed styles it | LOW–MEDIUM | Provider adapters must surface thinking deltas as a distinct channel instead of swallowing them |
| Rich prompt content (@-mentions, images) | Image/resource blocks carried in prompt; @-paths resolved to content | MEDIUM | Content-block plumbing through Profile Shaper → provider payload; @-mention resolution against workspace; Zed sends the blocks — no autocomplete needed agent-side beyond what files ship |
| Persistent shell / background Bash | Commands can move to background (`run_in_background`), output retrievable, completion notifies; shell state persists across Bash calls | MEDIUM–HIGH | Process-group spawn/shutdown exists (MCP hosting precedent). Add: PTY or persistent process pool, output buffers, bg-task registry shared with subagent notifications |

### Area 5 — SEED-004 gap fixes (priority 5)

| Gap | Expected behavior | Complexity | Notes |
|-----|-------------------|------------|-------|
| Checkpoints/undo via shadow-git | Workspace snapshot at turn boundaries; rollback surface (`ass-guard checkpoint` CLI + optional ACP command) | MEDIUM | Shadow repo beside workspace (not user's git); auto-commit per turn; restore = checkout. Watch nested-repo and large-binary hygiene |
| Compaction verify-first | Research gate before building compaction | LOW | Deliverable = finding, feeds Area 4 compaction design |
| Sandbox flag made real | macOS Seatbelt / Linux bwrap+seccomp wrapping of tool subprocesses | HIGH | Parity-driven: implement what zcode tool semantics imply; platform-split profiles; escape-hatch policy needed or legit workflows break |
| Steering/input queue | Queue drained at tool-call boundaries (pi steering) or turn-end (follow-up); abort stays cancel | MEDIUM | Queue lives in turn loop; drain points already structurally present (between LLM calls). Telegram prerequisite — design for shared core now, Telegram later |

### Area 6 — SEED-001 kit extraction (priority 6, last)

| Item | Why | Complexity | Notes |
|------|-----|------------|-------|
| Extract agent kit library (profile mechanism, providers, session/turn loop, projector, engine+DAG, tool catalog, scheduling, redaction) | Reuse + clean composition; ass-guard becomes reference app | HIGH | Pure refactor gated on everything above being stable — doing it earlier guarantees churn. Rides internal/runtime extraction |

---

## Feature Dependencies

```
[available_commands_update]
    └──requires──> [command+skill discovery (shipped)]
    └──enhances──> [all built-in commands] ──enhances──> [slash-invocable skills]

[request_permission] ──requires──> [tool-gate seam (shipped)] + [cancel-drain (shipped)]
    └──same-pattern-enables──> [elicitation/create] ──enhances──> [learning store asks]

[session modes advertisement] ──pairs-with──> [request_permission]   (dropdown + dialogs = full permissions UX)

[/compact] ──requires──> [compaction] ──requires──> [verify-first spike (SEED-004)]

[session list/resume/close/delete]
    └──requires──> [replay (shipped)] + [durable session metadata]
    └──enables──> [/resume]

[per-agent model:] ──requires──> [scheduler resolver (shipped)]
    └──required-by──> [full subagents]   (background agents inherit model overrides)

[background Bash + full subagents] ──share──> [task registry + notification events]
    └──feeds──> [/cost] (usage accounting)

[steering queue] ──requires──> [turn-loop injection points (structural)] 
    └──prerequisite-for──> [Telegram (v1.3)]

[kit extraction] ──must-be-last──> [everything above stabilized]
```

### Dependency Notes

- **/compact requires compaction requires verify-first:** the SEED-004 spike is cheap and de-risks the single most complex feature in the milestone — schedule it early even though compaction itself sits in parity-priority 4.
- **available_commands_update is the highest-leverage LOW-cost item:** it makes every later command/skill discoverable; shipping it first makes subsequent command work immediately visible.
- **request_permission and elicitation share one JSON-RPC interaction pattern** (ask → outcome → continue); build the pending-interjection state machine once, reuse for both.
- **Subagents and background Bash converge on one task-notification subsystem** — design it once (structured kinds, output files) rather than twice.
- **Per-agent model must precede full subagents**, or background agents launch with the wrong routing and the resolvedModel reporting is retrofitted.
- **Kit extraction conflicts with everything:** any phase ordered before it invalidates extracted interfaces. Strictly last.

## MVP Definition

### Launch With (v1.2 core — operator priority order)

- [ ] ACP completeness bundle: request_permission + tool_call/plan streaming + available_commands_update — the "feels native in the editor" bar
- [ ] Session list/resume/close/delete — operator must-have (D-09)
- [ ] Editor-driven configuration — native tier/model switching
- [ ] Cheap built-in commands riding the expansion seam: /help /status /model /clear /cost /mcp /memory
- [ ] Slash-invocable skills + AGENTS-as-commands + per-agent model wiring
- [ ] Compaction verify-first spike (early, cheap, unblocks the expensive item)

### Add After Validation (v1.2 continued)

- [ ] elicitation/create form mode (after permission state machine proves out)
- [ ] /compact (after compaction lands) + /resume /config /permissions /init /doctor
- [ ] Full subagents + background Bash on a shared task-notification subsystem
- [ ] Hooks PreToolUse deny path; thinking-block streaming; @-mentions/images
- [ ] Steering queue; checkpoints/undo; sandbox real

### Future Consideration (v1.3+)

- [ ] Kit extraction stabilization as public-ish library surface (extraction itself is v1.2 priority 6)
- [ ] Telegram peer consuming the steering queue + shared turn core
- [ ] Elicitation url mode; permission-rule interactive editing; remote-launched (cloud) agents

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| available_commands_update | HIGH | LOW | P1 |
| request_permission + session modes | HIGH | MEDIUM | P1 |
| tool_call + plan streaming | HIGH | MEDIUM | P1 |
| session list/resume/close/delete | HIGH | MEDIUM | P1 |
| Editor-driven config | MEDIUM | MEDIUM | P1 |
| Cheap built-in commands (/help /status /model /clear /cost /mcp) | HIGH | LOW | P1 |
| Skill-key invocation + AGENTS commands | HIGH | LOW–MEDIUM | P2 |
| Per-agent model dispatch | MEDIUM | MEDIUM | P2 |
| Compaction (verify-first → build) | HIGH | HIGH | P2 |
| /compact /resume /memory /config /permissions | MEDIUM | LOW–MEDIUM | P2 |
| elicitation/create (form) | MEDIUM | LOW | P2 |
| Full subagents + background Bash | HIGH | HIGH | P3 |
| PreToolUse deny; thinking blocks; @-mentions/images | MEDIUM | LOW–MEDIUM | P3 |
| Steering queue | MEDIUM (HIGH for Telegram) | MEDIUM | P3 |
| Checkpoints/undo | MEDIUM | MEDIUM | P3 |
| Sandbox real | MEDIUM | HIGH | P3 |
| /init /doctor | LOW–MEDIUM | MEDIUM | P3 |
| Kit extraction | HIGH (strategic) | HIGH | P4 (strictly last) |

**Priority key:** P1 must-have for the native-editor bar · P2 should-have this milestone · P3 close-out/parity tail · P4 strategic refactor

## Competitor Feature Analysis

| Feature | Claude Code | Gemini CLI (ACP) | pi (badlogic) | Our Approach |
|---------|-------------|------------------|---------------|--------------|
| Permission asks | TUI dialogs + rule persistence, 4 modes | ACP request_permission (had raw-text bug early) | minimal/none | ACP RPC + session modes; "always" = persisted rule in .ass-guard scope |
| Compaction | auto ≈95% + manual + (absorbed) microcompact | error-on-overflow historically, start-new-session | manual /compact only | verify zcode-profile evidence first; then auto+manual with focus args |
| Subagents | Task/Agent, background, notifications, per-agent model | none native | none | goroutine loops + shared task-notification subsystem |
| Steering | queued messages post-turn (no true mid-turn steer) | limited | true steering at tool-call boundaries + follow-up mode | pi-style boundary drain; cancel unchanged |
| Commands | 15+ built-ins + discovered | basic | few, file-based | built-ins via expansion seam + discovered commands/skills/AGENTS advertised over ACP |
| Config | /config panel + settings hierarchy | editor settings partial | settings.json | ACP configOptions for editor-native switching; env/file credentials |

## Sources

- Official Claude Code docs via Context7 (`/websites/code_claude` — commands, model-config, costs, settings-reference, hooks, hooks-guide, agent-sdk/typescript+python, skills, claude-directory, communications-kit, context-window, best-practices, whats-new) [HIGH]
- Agent Client Protocol: agentclientprotocol.com/protocol/schema + /protocol/tool-calls (live fetches) [HIGH]
- Zed ACP UX: zed.dev blog/docs, zed-industries/zed issues/PRs, agentclientprotocol.com [MEDIUM]
- Compaction internals: Anthropic docs + GitHub issues anthropics/claude-code #2497/#4251, r/ClaudeAI reports [MEDIUM]
- pi steering: github.com/badlogic/pi-mono packages/coding-agent/docs/customization.md, mariozechner.at [MEDIUM]
- Strands interrupts/queueing: strandsagents.com docs, GitHub issue #855 / PR #913 [MEDIUM]

---
*Feature research for: ass-guard-agent v1.2 Claude Code Parity*
*Researched: 2026-08-26*
