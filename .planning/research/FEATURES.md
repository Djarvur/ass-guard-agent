# Feature Research

**Domain:** v1.1 feature areas for ass-guard (slash-command invocation; audit log on `acp serve`; zcode parity re-capture; Telegram peer; deepseek-harness mimicry profile #2)
**Researched:** 2026-08-14
**Confidence:** HIGH for slash-command semantics (Claude Code official docs + zcode local ground truth + OpenSpec main-branch docs), HIGH for audit-log norms (Claude Code official OTel docs), MEDIUM for Telegram frontends (comparables are community projects; official Claude Code Channels covered via multiple secondary sources), MEDIUM for deepseek-harness (developer preview; docs inspected but turn-level wire protocol not fully enumerated — flagged as open verification items)

> Scope note: this replaces the 2026-08-09 v1.0 landscape doc (v1.0 shipped; its landscape conclusions are absorbed into PROJECT.md). This doc covers ONLY the five NEW v1.1 feature areas. Complexity is implementation complexity for ass-guard specifically (Go, single static binary, ACP-stdio + Telegram-in-process, existing v1.0 capabilities listed in PROJECT.md Validated). Volatile surfaces web-verified August 2026.

---

## Verified Landscape Facts (grounding for the five areas)

### (a) Slash-command semantics — Claude Code 2026, zcode, and OpenSpec

**Claude Code (official docs, current 2026):**

- **Commands have been merged into skills.** A file at `.claude/commands/deploy.md` and a skill at `.claude/skills/deploy/SKILL.md` both create `/deploy` and function identically. Existing `commands/` files keep working; skills are the recommended form (supporting-file dirs, invocation-control frontmatter, auto-load by relevance).
- **Layout / naming:** command name = filename minus extension. Namespacing comes from directories: plugin skills (`<plugin>/skills/<name>/SKILL.md`) → `/plugin:name`; clashing nested dirs get directory-qualified names (`/apps/web:deploy`). Search paths: enterprise/managed > personal (`~/.claude/`) > project (`.claude/`) > bundled; a skill beats a same-named command file.
- **Frontmatter fields (all optional):** `name`, `description`, `when_to_use`, `argument-hint` (autocomplete hint), `arguments` (named positional args), `disable-model-invocation` (user-only), `user-invocable: false` (hide from `/` menu), `allowed-tools` / `disallowed-tools` (per-turn grants/removals — grants clear on the next message), `model`, `effort`, `context: fork` (run in a subagent; with `agent`, `background`), `hooks`, `paths` (glob activation limits), `shell`, `metadata`, `license`, `compatibility`.
- **Substitution:** `$ARGUMENTS` = full argument string (if unused, arguments are appended as `ARGUMENTS: <value>`); `$ARGUMENTS[N]` / `$N` 0-based positional (missing index stays literal); named `$name` args via the `arguments` field; env vars `${CLAUDE_SESSION_ID}`, `${CLAUDE_SKILL_DIR}`, `${CLAUDE_PROJECT_DIR}`, `${CLAUDE_EFFORT}`, `${CLAUDE_PLUGIN_ROOT}`, `${CLAUDE_PLUGIN_DATA}`; `\$` escapes a literal dollar; multi-word args need quoting.
- **Composition with skills:** command stacking — `/write-tests /fix-issue 123` loads the first skill plus up to five more; a non-expanding token ends the run and becomes argument text. `context: fork` runs the command as a background subagent with the file content as prompt. Dynamic injection via `` !`command` `` (or ` ```! ` blocks) runs pre-invocation shell in Claude Code (can be disabled); failures abort the invocation.

**zcode (mimicry target — local ground truth from the shipped `zcode-guide` diagnostics skill):**

- Discovery order (first match wins, normalized name = path with `:` separators, lowercased): explicitly configured roots > `~/.zcode/commands` > `~/.agents/commands` > workspace `.zcode/commands` (every level from cwd up to repo root) > workspace `.agents/commands` > enabled plugin roots. Within a level `.zcode` scans before `.agents`. Local files always beat plugins.
- Name must match `^[a-z0-9][a-z0-9_:-]{0,63}$` — violations are dropped. Nested dirs join with `:` (`review/code.md` → `/review:code`).
- **Flat single-line frontmatter parser** (indented/multi-line values are silently dropped). Recognized keys: `description`, `argument-hint`, `allowed-tools`, `model`, `skills`, `disable-noninteractive`. Unknown keys are ignored but the command still loads. A description OR a non-empty body is required (both empty → dropped; absent description → first body line used).
- `$ARGUMENTS` = full argument string; `$1`/`$2` positional, out-of-range → empty; supplied args with no placeholder → appended under a "User arguments:" heading. `${ARGUMENTS}` brace form is NOT recognized. **Inline dynamic shell (`` !`cmd` ``) is REJECTED** in zcode (unlike Claude Code) — this is a real semantic divergence from Claude Code that ass-guard's expansion must follow (mimicry target wins over Claude Code here).
- `skills:` frontmatter auto-mounts skills for the command's turn. Commands whose names collide with built-ins (or `compress`) are filtered from the interactive `/` menu only. Config can disable a command by absolute file path.

**OpenSpec (repo verified: `Fission-AI/OpenSpec` — NOT "Fission-A/AI-OpsSpec"; no rename found; npm `@fission-ai/openspec`, current 1.9.0 (published 2026-08-13), MIT, ~329k weekly downloads, 64.9k stars):**

- `openspec init` (interactive tool picker) generates per-tool command/skill files; `openspec update` refreshes them so the latest commands are active. For Claude Code the generated form is `.claude/commands/opsx/<id>.md` → `/opsx:<id>`. Other tools get different spellings: `/opsx-propose` (Cursor/Copilot), `@opsx-propose` (Amazon Q), `$openspec-propose` (Codex), or skills-only forms (Kimi Code `/skill:...`, shared `.agents/`).
- **Core profile (default), stage-by-stage SDD workflow:**
  - `/opsx:explore` — "no-stakes thinking partner": reads codebase, compares options, diagrams; writes NO artifacts
  - `/opsx:propose <name>` — creates `openspec/changes/<name>/` with `proposal.md`, `specs/`, `design.md`, `tasks.md`; stops "ready for `/opsx:apply`"
  - `/opsx:apply` — reads `tasks.md`, implements incomplete tasks one by one, checks off `[x]`, resumable
  - `/opsx:update` — revises planning artifacts only ("never edits code"), reconciles ripple effects; pulls state via `openspec status --change <name> --json` and reads the output
  - `/opsx:sync` — merges delta specs (ADDED/MODIFIED/REMOVED/RENAMED) into `openspec/specs/`; mostly optional (archive prompts to sync)
  - `/opsx:archive` — moves the change to `openspec/changes/archive/YYYY-MM-DD-<name>/`, "preserving everything for audit trail"
- **Expanded profile** (opt-in via `openspec config profile`): `/opsx:new` (scaffold + `.openspec.yaml`), `/opsx:continue` (exactly one next artifact from the dependency graph), `/opsx:ff` (all artifacts in dependency order), `/opsx:verify` (completeness/correctness/coherence, CRITICAL/WARNING/SUGGESTION), `/opsx:bulk-archive`, `/opsx:onboard` (interactive 11-phase tutorial).
- **Division of labor (confirms the corrected hosting model):** the agent is the executor (reads artifacts, writes code/specs, resolves conflicts agentically); the CLI binary is supporting tooling — `init`, `update`, `list`, `status --change <name> [--json]`, `schemas`, `schema init`, `config profile`.
- **Version-drift warning for the adapter:** PROJECT.md reconciled the adapter against openspec **v1.5.0** (`list/view/change/spec/archive/doctor/context`); main-branch docs at **1.9.0** show a different binary surface (`status --change` rather than `view`, etc.). The adapter must pin to the *installed* binary's `--help`, not the docs of a moving main branch.

### (b) Agent Telegram frontends — what comparables offer

Two representative 2026 surfaces:

**`RichardAtCT/claude-code-telegram`** (Python, ~2.8k stars, MIT, v1.3.0, long-lived service):
- **Session binding:** per-user, per-project-directory sessions with automatic persistence and auto-resume on return; `/repo <name>` switching; optional Project Threads mode routing each project to a Telegram forum topic via a `projects.yaml` registry (+ `/sync_threads` reconciliation)
- **UX:** agentic mode (natural conversation; `/start`, `/new`, `/status`, `/verbose 0|1|2` streaming-detail control, persistent typing indicator) and a classic mode with 13 terminal-style commands + inline keyboards/quick actions
- **Media:** file uploads with archive extraction; image/screenshot analysis; **voice messages with pluggable transcription (Mistral Voxtral / OpenAI Whisper / local whisper.cpp)**
- **Ops/security:** chat whitelist, cost caps per user, token-bucket rate limiting, audit logging, SQLite persistence with migrations, usage/cost tracking; sandboxed to an `APPROVED_DIRECTORY`; GitHub webhook server (HMAC) and cron/proactive notifications as opt-in extras

**Claude Code official "Channels" plugin** (shipped 2026; Telegram is a channel):
- **Voice flow:** hold mic → transcription before the message reaches Claude; provider fallback chain OpenAI Whisper → Groq → Deepgram → local whisper-cli; optional TTS replies (ElevenLabs); "handles format conversion, chunking, and transcription automatically"
- **Threading:** forum topics fully supported, each isolated; reply-chain tracking (~3 levels) in groups
- **Media/format:** stickers/GIFs → multi-frame collages; PDF/DOCX/CSV up to 10 MB; long replies via Telegraph Instant View articles; MarkdownV2 auto-escaping
- **Lifecycle:** pairing by DMing the bot a 6-character code, then `/telegram:access pair <code>`; allowlist policy mode; daemon supervisor with exponential-backoff restarts, a context watchdog restarting at 70% usage, single-instance lock; launchd/systemd for 24/7; forwarded-message batching (debounce ~5 s, summarize, one reply)

**Long-poll vs webhook in a non-daemon process:** every comparable effectively polls (python-telegram-bot polling loop; Channels' daemon). Webhook mode requires a public HTTPS endpoint and a listening server — incompatible with ass-guard's no-daemon/no-network-port constraint (the editor owns process lifecycle). Long-polling (`getUpdates`) works behind NAT, needs no inbound port, and drains cleanly via `context.Context` cancellation — matching the shipped design decision (go-telegram/bot, Phase-0-verified stdout-silent).

### (c) deepseek-harness ("dsh") — user-visible surface

- **Repo:** `deepseek-ai/deepseek-harness`, MIT, **developer preview v0.1** with an explicit "THERE WILL BE COMPATIBILITY-BREAKING CHANGES" warning; TypeScript/Node (pnpm), 92.2k stars, very high commit velocity; announced alongside DeepSeek V4-Pro (VentureBeat positions it as an open-source Claude Code rival). It self-describes as an agent *runtime framework*, not a finished terminal product.
- **Architecture:** Cordis kernel (plugin mounting/unmounting/dependencies); **everything is a plugin** — models, tools, skills, sessions, sandboxes, storage, loops, scheduling, even the UI. Extension seams: `ctx.llm`, `ctx.tools`, `ctx.shell`, `ctx.fs`, `ctx.sandbox`; swapping a sandbox provider moves Bash/PTY/LSP with it. Community plugins discoverable via the `dsh-plugin` GitHub topic.
- **Entrypoints/modes:** `npx @deepseek-ai/dsh web` (Web UI at 127.0.0.1:3080) and `dsh headless` are the two shipped profile templates. Preset profiles: **Standard** (full toolset: file editing, shell, file/web search, skills, planning, goals, subagents, workflows), **Code** (programmatic tool calling — the model writes one TypeScript program orchestrating multi-step tool calls via the Code Mode SDK), **Minimal** (exactly two tools — persistent bash + `str_replace_editor` — for benchmarking), **Creator** (author/inspect presets, in-memory plugin testing).
- **Providers/config:** `$DSH_HOME/settings.yaml`, e.g. `llm-pi-ai.providers.<id>` with `apiKeyEnv`, `api: openai-completions`, `baseURL`, per-model modality lists (`input: [text, image]`). Catalog providers (Anthropic, OpenAI, Bedrock, Vertex, Azure, Codex) get endpoint + protocol + model list from the installed catalog; custom providers cover gateways/self-hosted (model discovery via OpenAI-compatible `GET /models`). Credentials live in `$DSH_HOME/.credentials.yaml` (write-only in the UI; only a redacted descriptor is ever shown). DeepSeek's own built-in route is chat-completions and **text-only**.
- **Skills/instructions:** SKILL.md-based skills (slash-invocable and auto-triggered — consistent with the Agent Skills ecosystem); repo carries root `AGENTS.md` + `CLAUDE.md` + `docs/AGENTS.md` (contributor-facing; no evidence of a distinctive dsh-specific project-instruction convention beyond the agents.md standard).
- **Traceability (directly relevant to area (d) too):** append-only session log — "**model-visible means logged**": system prompts, reasoning, tool calls/results, subagent scheduling, context injections. A Trajectory view inspects by source; resume, fork, search, and replay all operate on the same event stream.
- **Implications for profile #2:** (1) ground truth is easier than zcode — the harness is open source (shape from source) *plus* its own session logs (`$DSH_HOME`) give log-extracted capture, satisfying the "log-extracted, not hand-written" decision from both directions; (2) the outgoing wire shape is provider-dependent (`openai-completions` custom providers; catalog-supplied protocols for Anthropic et al.) — which protocol a *DeepSeek-model turn* uses (chat-completions vs Responses vs Anthropic-shape) is an **open verification item** before writing the shaper; (3) developer-preview churn means the profile will need version pinning + the existing drift detector, not one-shot capture.

### (d) Audit-log feature shape in comparable agents

**Claude Code (official OTel telemetry docs — the most-documented 2026 norm):**

- **Events** (all `claude_code.` prefixed): `user_prompt`, `assistant_response`, `tool_result`, `tool_decision` (any accept/reject permission decision, with `decision`, `source` — config/hook/user_permanent/user_temporary/user_abort/user_reject — and `tool_source` provenance), `api_request`, `api_error`, `api_refusal`, `permission_mode_changed`, `auth`, `mcp_server_connection`, `internal_error`, `plugin_installed`/`plugin_loaded`; raw `api_request_body`/`api_response_body` only behind an opt-in env flag.
- **Correlation IDs:** `prompt.id` (UUID tying all events from one prompt), `tool_use_id` (joins tool spans + result/decision events), `message.uuid`, `request_id`, `client_request_id`. Correlation keys are the difference between a log and an audit trail.
- **Metrics:** session.count (with `start_type` fresh/resume/continue), token.usage, cost.usage, lines_of_code, commit/pull_request counts, active_time.
- **Redaction norms (opt-in content, always-redact secrets):** prompts → `<REDACTED>` unless explicitly enabled; assistant text redacted unless enabled; tool params/Bash commands/MCP+skill names omitted unless `OTEL_LOG_TOOL_DETAILS=1` (then values > 512 chars truncated, ~4 KB bound); custom agent/plugin/MCP names → `"custom"`/`"third-party"`; internal errors → class name + errno only; auth errors → category only; extended thinking always redacted. **Always included:** session.id, anonymous user.id, org id, token counts, durations, costs, HTTP status, built-in tool names. Content cap 60 KB by default; raw-body-to-file mode writes full JSON to disk with a `body_ref` pointer in the event.
- Platform-side, Anthropic's Audit Logs API records 35 event types with 6-year compliance retention — that's the SaaS tier; the local-agent norm is the above.

**dsh:** the purest statement of the norm — append-only session log where everything model-visible is recorded (prompts, reasoning, tool calls/results, subagent scheduling, context injections), with the log doubling as the substrate for resume/fork/replay. **claude-code-telegram:** audit logging + usage/cost tracking + rate-limit events into SQLite. **ass-guard v1.0** already has the right bones (tracer-wired redacted request logging + redactor; transcript-as-diagnostic-surface per D-03/D-20) — the v1.1 gap is only that `--audit-log` isn't written on the `acp serve` path.

---

## Feature Landscape

### Area 1 — Slash-command invocation (`/namespace:name` expansion + OpenSpec adapter)

#### Table Stakes (Users Expect These)

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| `/namespace:name` expansion from discovered command files (`.claude/commands/opsx/explore.md` → `/opsx:explore`) | Both Claude Code and zcode namespace via directories→colons; OpenSpec's Claude-Code form depends on it; without it the toolkit cannot be kicked off at all (the v1.0 UAT gap) | LOW | ecosys discovery already loads commands (ECOS-01..03) — the work is expansion into the prompt pre-turn: body → user message, frontmatter consumed. Nested dirs join with `:` (NOT `/`) — zcode pitfall #11 |
| `$ARGUMENTS` + `$1..$N` positional substitution; args-without-placeholder appended under a "User arguments:" heading | Universal substitution contract (Claude Code and zcode agree on `$ARGUMENTS`/positional + append-when-unused; zcode additionally rejects the `${ARGUMENTS}` brace form) | LOW | Mimic zcode exactly (target wins over Claude Code): out-of-range positional → empty; append heading when no placeholder |
| Frontmatter: `description`, `argument-hint`, `allowed-tools`, `model`, `skills`, `disable-noninteractive` (flat single-line parser) | The six keys zcode actually parses; `skills:` auto-mounts skills for the turn; `allowed-tools` grants are per-turn and clear next message | LOW | zcode's parser is flat/single-line (multi-line arrays silently dropped) — replicate leniently: unknown keys ignored, command still loads; empty command (no description + empty body) dropped |
| Discovery precedence: user > workspace > plugins, first-match-wins dedup | zcode ground truth: configured roots > `~/.zcode/commands` > `~/.agents/commands` > workspace `.zcode` > `.agents` > plugins; local always beats plugin | LOW | ass-guard's ecosys already implements `.claude/` + `.ass-guard/` precedence read-only — reconcile with zcode's `.zcode`/`.agents` roots for profile fidelity |
| Command execution as a context boundary (lean window reset) | OpenSpec stages are long; v1.0 two-layer context resets at command boundaries by design; each `/opsx:*` stage must start clean but replay from the durable transcript | LOW | Already built (Phase 2) — just ensure expanded commands mark the boundary |
| OpenSpec core profile driveable end-to-end: explore → propose → apply → (update/sync) → archive | The toolkit's documented workflow; `/opsx:propose` creates the change folder + artifacts, `/opsx:apply` implements `tasks.md` checking off `[x]`, `/opsx:archive` moves to `openspec/changes/archive/YYYY-MM-DD-<name>/` | MEDIUM | Mostly engine work: handoffs between stages are the autocontinue engine's dual-signal (text-pattern OR tool-call) matches — `/opsx:propose`'s "ready for `/opsx:apply`" stop-phrase is a textbook handoff signal |
| Adapter pinned to the real openspec binary (`list`, `status --change --json`, `schemas`, `config`) called as a tool by the agent | OpenSpec's own division of labor: agent executes command files; binary is supporting tooling; `/opsx:update` literally reads `openspec status --change <name> --json` output | MEDIUM | WARNING: binary surface moved 1.5.0 → 1.9.0 (PROJECT.md reconciled against 1.5.0; main docs show `status` not `view`). Pin the adapter to the installed binary's `--help`; keep `ASSGUARD_OPENSPEC_BIN=1` as the phase gate |

#### Differentiators (Competitive Advantage)

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Zero-continue stage chaining (engine auto-invokes the next `/opsx:*` command) | Claude Code/zcode users tap each stage manually; ass-guard's reason-to-exist is removing exactly that — the `/opsx:propose`→`apply`→`archive` chain driven by the unified engine with hooks between stages | MEDIUM | Depends on Phase-4 engine + seeded hook table; this is where v1.1's headline value lands |
| Cross-tool spelling tolerance (`/opsx:explore` AND `/opsx-explore`) | OpenSpec generates different spellings per tool (Cursor `/opsx-propose`, Codex `$openspec-propose`); tolerating both costs nothing and survives toolkit re-runs | LOW | Prefix-match on the opsx namespace; don't over-generalize to every tool's grammar |
| Command-args surfaced into ACP `session/update` | IDE users see what was expanded and with which args (transparency the raw toolkits don't give) | LOW | Rides existing session/update stream |

#### Anti-Features (Commonly Requested, Often Problematic)

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| Dynamic shell injection (`` !`cmd` `` pre-invocation) | Claude Code supports it; command authors want computed context | zcode REJECTS it (mimicry divergence — target wins); runs arbitrary shell pre-prompt in a no-confirmation-tier agent | Reject with zcode's error semantics; static text + `$ARGUMENTS` only |
| Full Claude-Code skill-merge semantics (commands≡skills, `context: fork`, stacking up to 6) | Claude Code's 2026 direction is commands-as-skills | Scope creep for a kickoff milestone; zcode keeps commands and skills separate (with `skills:` mounting); fork-subagent semantics change the turn shape the profile pins | Implement zcode's `skills:` auto-mount only; revisit under ECOS-04 (post-v1.1, "working unchanged") |
| Expanded-profile commands (`/opsx:new/continue/ff/verify/bulk-archive/onboard`) in the gate | They're documented and tempting | Core profile is the default install; expanded is opt-in via `openspec config profile` — testing it doubles UAT surface for near-zero first-customer value | Verify core profile E2E; assert expanded files *load* (expansion works) without E2E-driving them |

### Area 2 — Audit log on the `acp serve` path (LOG-01 completion)

#### Table Stakes

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Audit log written on every path, `acp serve` included | An audit log that silently doesn't record the primary production path is worse than none (v1.0 known gap #3) | LOW | The tracer + redactor already exist (Phase 1/2) — this is wiring: construct the tracer in the ACP serve path, not only via main.go CLI flags |
| Correlation IDs per prompt/turn/tool | Claude Code's `prompt.id` + `tool_use_id` are what make events an *audit trail* rather than a log; dsh's whole trajectory view is built on the event stream being joinable | LOW | ass-guard already has transcript ids and tool_use ids from the session core — thread them into audit records |
| Per-event record of: user input, model request (redacted), tool call + result, engine decisions (continue/hook/wait), errors with recoverable/non-recoverable classification | Claude Code records user_prompt/tool_result/tool_decision/api_error/internal_error; ass-guard's investigate-and-fix-ready constraint (PROJECT.md) demands strictly more context than "an error occurred" | LOW–MEDIUM | The engine-decision events are the ass-guard-specific addition (no comparable records autocontinue decisions — that's a differentiator below) |
| Secrets always redacted (API keys, auth headers, credential env vars) | Universal norm: Claude Code strips auth headers/env always; content is the only thing ever gated by flags | LOW | Redactor exists; assert coverage in tests |
| Append-only, machine-parseable output (JSONL) with size caps/truncation on big payloads | Claude Code caps content at 60 KB default with truncation markers, 512-char tool-value truncation under details mode; unbounded logs rot disks and leak more | LOW | Matches existing transcript discipline |

#### Differentiators

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Engine-decision audit events (why the agent continued / ran a hook / waited) | No comparable agent records its autocontinue reasoning; for a hands-off agent this IS the trust surface — reviewers can see why the machine drove the next stage | LOW | Engine already emits internal events; serialize them into the audit stream |
| Audit log doubling as parity evidence (request hashes/shape fingerprints per turn) | Ties audit to the mimicry north star — the audit trail can prove "structurally indistinguishable" continuously, not just at capture time | LOW | Drift detector already computes shape fingerprints; appending them per-turn makes drift visible in production |

#### Anti-Features

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| Raw request/response bodies logged by default | "Full fidelity" debugging appeal | Inverts the redaction norm (Claude Code gates raw bodies behind an explicit env flag precisely because they carry file contents/secrets in tool results); compliance exposure | Opt-in flag mirroring `OTEL_LOG_RAW_API_BODIES=file:<dir>`: full bodies to a separate file, `body_ref` pointer in the audit record |
| OTLP/SIEM export in v1.1 | Enterprise asks | Claude Code ships no SIEM integration either (events go to whatever OTLP endpoint you configure); exporter plumbing, mTLS, header helpers = a milestone of its own | Local JSONL now; an OTLP exporter is a natural post-v1.1 add-on once the event schema is stable |
| Untruncated tool outputs in audit | "Nothing missed" | 60 KB caps + 512-char value truncation are the documented norm for good reasons (PII blast radius, disk); transcript already holds the full record | Audit = index + redacted summary; transcript remains the full-fidelity artifact (D-03/D-20) |

### Area 3 — zcode parity re-capture (operator-gated)

#### Table Stakes

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Fresh capture session pinned as the new ground truth (`ZAI_API_KEY` + a divergence-prone session) | The pinned session (`eea3dc48`) is absent on disk — the Phase-1 within-session stability test cannot run until a replacement exists; profile-content-is-log-extracted is a Key Decision | LOW (code) / operator-dependent | No new code expected; a capture-runbook + pinned artifact hash. The A/B parity thesis is proven; this closes the stability leg |
| Re-run drift detector + stability test against the new capture | `ass-guard profile check` and the coverage manifest exist and are useless without a live capture | LOW | Gate: stability test green in CI against the pinned session |

#### Differentiators

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Capture provenance recorded in the profile (session id, date, zcode version, tool-count manifest) | Makes the profile artifact auditable and re-capturable; turns "re-capture" from a chore into a repeatable procedure — groundwork for profile #2 where the same procedure runs against dsh | LOW | Extends the existing coverage manifest |

#### Anti-Features

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| Automated continuous re-capture (nightly zcode runs) | "Always fresh" mimicry | Burns operator API spend unattended; zcode updates are event-driven, not nightly-diff-driven; the operator gate exists deliberately | Manual operator-gated capture + drift detector alerting when the pinned zcode version changes |
| Re-capture blocking the milestone | Sequencing instinct | It's priority 3 precisely because it unblocks a *test*, not a feature — other areas don't depend on it | Run it adjacent to Area 2 in the same phase (both small); don't serialize Area 4/5 behind it |

### Area 4 — Telegram peer (text + voice STT)

#### Table Stakes

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Chat → project/session binding with persistence + auto-resume | Every comparable does per-chat/per-project binding (claude-code-telegram: per-user per-directory sessions, `/repo` switching; Channels: forum-topic isolation). Without it, Telegram turns can't address a working tree | MEDIUM | ass-guard's twist: a Telegram chat binds to an ass-guard *session* (the same session core ACP uses), not to a spawned CLI process — no daemon, no child Claude, one engine |
| Text in → streamed/edited reply out, 4096-char chunking, typing indicator, MarkdownV2 escaping | Telegram physics; both comparables do chunked delivery + MarkdownV2 auto-escape + persistent "working" indicator | LOW–MEDIUM | Map ACP `session/update` stream → message edits; edit-rate throttle per Telegram limits |
| Voice messages → STT → ordinary text input (ogg/opus download → transcribe → inject) | Both comparables treat voice as first-class (claude-code-telegram: Voxtral/Whisper/whisper.cpp; Channels: Whisper→Groq→Deepgram→whisper-cli chain). For ass-guard this is the explicit v1.1 requirement: configurable STT backend, OpenAI Whisper API default, whisper.cpp subprocess + Groq via config | MEDIUM | Download via Telegram file API (ogg/opus), transcribe via the OpenAI-shape client (already in the provider factory — audio endpoint is a small add), inject as plain user text. NO extra dependency for the default |
| Access control: allowlist/pairing | Both comparables gate access (whitelist; Channels' 6-char pairing code + allowlist policy). An agent with shell tools reachable from an open bot is a remote-code hole | LOW | Config allowlist of chat/user ids at minimum; pairing flow is a nice-to-have differentiator |
| Long-polling (not webhook), draining on shutdown | ass-guard runs editor-owned, no network port, no daemon; webhooks need public HTTPS + a listener (constraint violation); every comparable effectively polls | LOW | go-telegram/bot long-poll goroutine (Phase-0 verified stdout-silent); context cancellation drains the loop AND in-flight Telegram-driven turns |
| Long-reply handling (documents/Telegraph-style) | Both comparables solve >4096-char replies (files, Instant View articles) | LOW | Send as `.md` document attachment or split; keep it boring |

#### Differentiators

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| **In-process peer beside ACP stdio** — same core engine, same sessions, no daemon | No comparable does this: claude-code-telegram and Channels both run a *separate* daemon supervising CLI processes; ass-guard's Telegram is a goroutine in the single static binary the editor already owns. Mobile drive-by of the SAME session the IDE has open is unique | HIGH (lifecycle, not features) | The load-bearing risks: stdout discipline (Telegram must never write stdout — enforced by review/lint), shared turn-loop serialization (one turn per session at a time), and context-first shutdown draining both frontends |
| Full SDD scenario drivable from Telegram (incl. `/opsx:*` commands) | The milestone's bar is "full peer", not a notification relay (which is what Channels is for Claude Code) | MEDIUM | Slash-command expansion (Area 1) must be surface-agnostic — expansion happens in the session layer, so Telegram gets it for free |
| STT backend fallback chain (Whisper → Groq → whisper.cpp) | Channels normalized the fallback chain; matching it in config (not code) fits the configurable-backend pattern from v1.0 | LOW | Scheduler's typed ProviderError classification already models fallback chains — reuse the semantics |
| Forum-topic-per-project threading | claude-code-telegram's Project Threads mode is its most-loved feature for multi-repo users | MEDIUM | Add after validation (v1.1.x): a `projects.yaml`-style registry mapping topics → working dirs |

#### Anti-Features

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| Approval/permission buttons in Telegram | Every comparable offers inline confirm buttons; it feels like the safety answer | Directly violates the shipped safety model (no confirmation tier; pattern/hook table + manual cancellation only). Bolted-on approval on ONE surface also breaks the mimicry invariant that behavior is surface-independent | Manual cancellation (stop button → cancel session turn) only; surface the same cancellation ACP has |
| Webhook mode | "More production-ready" framing | Requires public HTTPS endpoint + listener → violates no-daemon/no-port constraint and complicates editor-owned lifecycle | Long-poll always; document NAT-friendliness as a feature |
| TTS voice replies, sticker/GIF collages, calendar commands | Channels-feature parity envy | Novelty surface area; TTS adds a synth provider + audio upload path for little SDD value; sticker collages are a party trick | Post-v1.1 backlog; voice IN is the workflow need (walk-and-talk spec'ing), voice OUT is not |
| Classic-mode terminal commands (`/cd`, `/ls`, `/git` … 13 commands) | claude-code-telegram's mode B | Duplicates the model's own tools with a worse UX; encourages shell-as-IRC instead of agentic turns | Agentic natural-language only; `/new`, `/status`, `/stop` as the small command set |

### Area 5 — Mimicry profile #2: deepseek-harness (dsh) for DeepSeek-model turns

#### Table Stakes

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Profile artifact for dsh: system blocks, tool catalog, message shape, identity — captured, not hand-written | The Key Decision (log-extracted, not hand-written) applies to every profile; dsh ground truth is *easier*: the repo is open source (TypeScript — shape readable from source) AND `$DSH_HOME` session logs ("model-visible means logged": prompts, reasoning, tool calls/results, subagent scheduling, context injections) provide log-side capture | MEDIUM | Capture tooling must handle a second log format + a source-reading pass. Rides the N-profile architecture (explicitly built for this — "no zcode-specific paths") |
| Wire-shape support for dsh's outgoing protocol | dsh's provider layer speaks `openai-completions` for custom providers, catalog-supplied protocols for Anthropic/OpenAI et al.; DeepSeek's own built-in route is chat-completions (text-only). A DeepSeek-model turn is therefore most likely OpenAI-chat-shape — which the two-shape ProviderFactory already speaks | MEDIUM | **Open verification item (phase gate):** confirm what a real dsh DeepSeek turn sends (chat-completions vs Responses vs other) by reading `dsh-llm-deepseek` source + a captured session log BEFORE writing the shaper. If it's Responses-API shape, that's a third provider shape and a scope decision |
| Profile selection config (per-project/per-turn override: zcode for GLM turns, dsh for DeepSeek turns) | The operator's stated setup (opencode subscription serving MiniMax + DeepSeek) means both profiles coexist; PROJECT.md's opencode-exclusion decision routes DeepSeek turns through dsh shape | LOW | Scheduler already resolves provider+model per turn; profile becomes a dimension of that resolution (model→profile mapping in config) |
| Drift detection against pinned dsh version | dsh is developer preview with a bolded compatibility-breaking-changes warning — profile rot is near-certain without pinning | LOW | Reuse `ass-guard profile check`; pin the profile to a dsh commit/version and record it in provenance (Area 3's differentiator) |

#### Differentiators

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| **Cross-harness mimicry as a product capability** (2 profiles, one shaper) | Nobody in the 2026 market does structural request mimicry at all (v1.0 research conclusion); proving it generalizes beyond zcode converts "a zcode trick" into "the platform thesis validated twice" | LOW (if N-profile architecture holds) | The main risk is hidden zcode-isms in the shaper — the v1.1 requirement "rides the existing N-profile architecture (no zcode-specific paths)" is the acceptance test |
| Source+log dual-grounded profile | zcode was log-only (closed-ish runtime); dsh is open-source — shape-from-source cross-checked-against-logs is a *stronger* grounding method than profile #1 had | MEDIUM | Capture pipeline gains a "read the source" half; worth doing once generically (it also serves future open-source targets) |

#### Anti-Features

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| Tracking dsh HEAD | Newest-shape fidelity | Developer preview with announced breaking changes — a moving profile target invalidates captures weekly | Pin a dsh version; re-capture operator-gated on meaningful dsh releases (same procedure as Area 3) |
| Mimicking dsh's Web UI / plugin runtime behavior | "Full harness parity" framing | Mimicry is about *outgoing model requests*, not UI; dsh's UI/plugins never reach the model provider beyond the request shape | Scope the profile strictly to request shape (system blocks, tools, message shape, identity) |
| Assuming Anthropic-shape for DeepSeek turns (reusing zcode's shaper shape) | Reuse temptation — one shaper, two configs | dsh's DeepSeek route is chat-completions (text-only); zcode's shape is Anthropic. Wrong shape = model behavior diverges = north star violated | Verify first (see table stakes), shaper-shape follows the profile, never the reverse |

---

## Feature Dependencies

```
[Slash-command kickoff (Area 1)]
    ├──requires──> [ecosys command/skill discovery]        (v1.0 ECOS-01..03, EXISTS — unwired)
    ├──requires──> [session/ACP layer prompt path]          (v1.0 Phase 2, EXISTS)
    ├──requires──> [two-layer context boundary reset]       (v1.0 Phase 2, EXISTS — commands = boundaries)
    ├──requires──> [unified engine dual-signal handoffs]    (v1.0 Phase 4, EXISTS — propose→apply chaining)
    └──requires──> [OpenSpec adapter reconciled to installed binary]  (v1.0 exists; v1.1 reconcile + ASSGUARD_OPENSPEC_BIN gate)

[Audit log on acp serve (Area 2)]
    ├──requires──> [tracer + redactor]                      (v1.0 Phase 1/2, EXISTS — wiring gap only)
    └──enhances──> [parity evidence]  <──enhances── [Area 3 re-capture]   (shape fingerprints per turn)

[zcode parity re-capture (Area 3)]
    └──requires──> [operator action: ZAI_API_KEY + capture session]      (external; unblocks Phase-1 stability test)

[Telegram peer (Area 4)]
    ├──requires──> [session core turn loop]                 (v1.0 Phase 2, EXISTS — shared, one turn per session)
    ├──requires──> [unified engine]                          (v1.0 Phase 4, EXISTS — same decisions both surfaces)
    ├──requires──> [scheduler + provider factory]            (v1.0 Phases 3/7, EXISTS — STT rides OpenAI-shape client)
    ├──requires──> [stdout discipline enforcement]           (v1.0 constraint — Telegram goroutine must stay stderr-only)
    └──enhances──> [Area 1]  (slash-commands must work from Telegram — expansion lives in session layer, surface-agnostic)

[dsh profile #2 (Area 5)]
    ├──requires──> [N-profile architecture + shaper]         (v1.0 Phase 1, EXISTS — acceptance test: no zcode-specific paths)
    ├──requires──> [provider factory wire shape]             (openai-completions EXISTS; Responses/other = VERIFY then maybe build)
    ├──requires──> [drift detector + provenance]             (v1.0 EXISTS; provenance extension comes from Area 3)
    └──requires──> [dsh ground truth: source + $DSH_HOME logs]  (capture runbook; dsh pinned version)

Conflicts / tensions:
[Telegram approval buttons] ──conflicts──> [no-confirmation-tier safety model]     (resolved: cancellation only)
[dsh HEAD tracking]         ──conflicts──> [profile stability / drift detector]    (resolved: pin version)
[Claude-Code !`cmd` expansion] ──conflicts──> [zcode mimicry (target rejects it)]  (resolved: follow zcode)
[Telegram webhook mode]     ──conflicts──> [no-daemon / no-network-port constraint] (resolved: long-poll only)
```

### Dependency Notes

- **Area 1 requires ecosys wiring first** — everything else in the milestone's priority order (1→4→3→5→2 per PROJECT.md; adjacent small items may merge) sits behind kickoff working, because the UAT-closed OpenSpec loop is the product's proof.
- **Area 4 enhances Area 1:** if slash expansion is implemented in the session layer (not the ACP handler), Telegram inherits `/opsx:*` for free; if implemented in the ACP layer, Area 4 must re-implement it — build it surface-agnostic.
- **Area 3 enhances Area 5:** the capture-provenance extension and runbook written for zcode re-capture should be written generically (parameterized by target) so dsh capture is a config, not a second tool.
- **Area 2 is independent** (pure wiring) — mergeable with Area 3 into one small phase per PROJECT.md's "adjacent may merge."

## MVP Definition

### Launch With (v1.1)

- [ ] `/namespace:name` expansion with zcode semantics ($ARGUMENTS/$1..$N, "User arguments:" append, flat frontmatter, `:` namespacing, zcode discovery precedence, dynamic-shell rejected) — the kickoff gap closer
- [ ] OpenSpec core profile E2E: explore → propose → apply → archive chained by the engine with zero manual continues; adapter pinned to the installed binary's surface; `ASSGUARD_OPENSPEC_BIN=1` gate green; 11 deferred UAT checks closed
- [ ] Audit log written on `acp serve`: correlation IDs, secrets-redacted, append-only JSONL, truncation caps, engine-decision events
- [ ] zcode re-capture: new pinned session, drift detector + Phase-1 within-session stability test green
- [ ] Telegram: text + voice (Whisper default), chat↔session binding with resume, allowlist, long-poll drain on shutdown, 4096 chunking, MarkdownV2 escaping, `/opsx:*` drivable
- [ ] dsh profile #2: wire-shape verified (gate), profile captured from source+logs at a pinned dsh version, model→profile mapping in scheduler config, drift check extended

### Add After Validation (v1.1.x)

- [ ] Forum-topic-per-project threading — trigger: multi-project Telegram users
- [ ] STT fallback chain (Whisper → Groq → whisper.cpp subprocess) — trigger: default-backend latency/cost complaints
- [ ] Audit OTLP exporter — trigger: enterprise/team ask; keep event schema stable first
- [ ] Raw-body opt-in audit mode (`file:<dir>` + body_ref) — trigger: debugging needs
- [ ] Expanded OpenSpec profile commands E2E — trigger: first user opting into `openspec config profile`

### Future Consideration (v2+)

- [ ] dsh Responses-API (or other) wire shape — only if verification shows DeepSeek turns use it and users need it (third provider shape is architecture scope)
- [ ] Telegram TTS replies / rich-media parity with Channels — novelty, not workflow
- [ ] Claude-Code command≡skill merge semantics (`context: fork`, stacking) — under ECOS-04 "working unchanged in every interaction mode"
- [ ] Channels-style pairing codes — if allowlist proves clunky for teams

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| Slash-command expansion + OpenSpec core E2E (Area 1) | HIGH (product proof; closes UAT gap) | MEDIUM | P1 |
| Audit log on acp serve (Area 2) | HIGH (trust; known gap) | LOW | P1–P2 (mergeable) |
| zcode parity re-capture (Area 3) | HIGH internally (unblocks stability test) | LOW code / operator-gated | P2 |
| Telegram text+voice peer (Area 4) | HIGH (new surface; mobile/async SDD) | HIGH (lifecycle + binding + STT) | P2 (milestone priority 4) |
| dsh profile #2 (Area 5) | MEDIUM–HIGH (thesis generalization; operator's DeepSeek turns) | MEDIUM (verification-gated) | P3 (milestone priority 5) |
| Telegram forum topics | MEDIUM | MEDIUM | P3 |
| Audit OTLP export | MEDIUM (enterprise) | MEDIUM | P3 |
| Expanded OpenSpec profile E2E | LOW (opt-in) | MEDIUM | P3 |

**Priority key:**
- P1: Must have for launch (v1.1 cannot ship without)
- P2: Should have, add when possible
- P3: Nice to have, future consideration

## Competitor Feature Analysis

| Feature | Claude Code (official surface) | zcode (mimicry target) | claude-code-telegram / Channels | dsh | Our Approach |
|---------|-------------------------------|------------------------|----------------------------------|-----|--------------|
| Slash commands | Merged into skills; rich frontmatter (fork/context/stacking); `!`cmd`` dynamic injection allowed | Separate commands; flat 6-key frontmatter; `skills:` auto-mount; dynamic shell REJECTED; strict name regex; first-match-wins precedence | Channels adds `/telegram:access pair`-style commands | SKILL.md skills, slash-invocable + auto-trigger | Implement **zcode semantics** (target wins); surface-agnostic expansion (ACP + Telegram) |
| SDD workflow driving | Manual stage taps; OpenSpec command files work as-is | Same manual pattern | n/a (adjunct surface) | n/a | **Zero-continue chaining** of `/opsx:*` stages via the unified engine — the differentiator nobody has |
| Audit/telemetry | OTel events + metrics + correlation IDs; opt-in content; secrets always redacted; 60 KB caps | (not public) | claude-code-telegram: SQLite audit + cost tracking | Append-only session log, "model-visible means logged", trajectory view, resume/fork/replay on the stream | Same norms (correlation IDs, redaction, caps) + engine-decision events + parity fingerprints — audit as trust surface |
| Telegram frontend | Channels plugin = messaging adjunct into a running session; daemon supervisor; voice via Whisper-chain; forum topics; allowlist/pairing | none | Full bots; per-project session binding; `/verbose`; whitelist; cost caps; polling daemons | none | **In-process peer goroutine** beside ACP stdio in one static binary; same sessions/engine; long-poll + context drain; cancellation-only safety (no approval tier) |
| Voice STT | Whisper→Groq→Deepgram→whisper-cli fallback chain (Channels) | none | Voxtral/Whisper/whisper.cpp options | n/a | Configurable backend, OpenAI Whisper default via existing OpenAI-shape client; fallback chain via scheduler error classification |
| Mimicry of other harnesses | none | none | none | none | **Category of one**: zcode profile (proven) + dsh profile #2 (open-source + log dual grounding, version-pinned) |
| Model providers | Anthropic + catalog | Anthropic-shape (Z.ai) | delegates to Claude Code | Plugin providers: openai-completions custom, catalog protocols, `$DSH_HOME` settings.yaml + .credentials.yaml | Two-shape factory (existing); dsh-profile turns ride openai-completions shape pending verification |

## Sources

- [Claude Code — Slash Commands docs](https://code.claude.com/docs/en/slash-commands) (commands≡skills merge, frontmatter, substitution, stacking, precedence)
- [Claude Code — Skills docs](https://code.claude.com/docs/en/skills)
- [zcode `diagnosing-commands` skill](file:///Users/nil/.zcode/cli/plugins/cache/zcode-plugins-official/zcode-guide/0.1.0/skills/diagnosing-commands/SKILL.md) — local ground truth for the mimicry target's command semantics
- [Fission-AI/OpenSpec repo](https://github.com/Fission-AI/OpenSpec) + [docs/commands.md](https://github.com/Fission-AI/OpenSpec/blob/main/docs/commands.md) + [docs/opsx.md](https://github.com/Fission-AI/OpenSpec/blob/main/docs/opsx.md) (repo verified Fission-AI, not "Fission-A/AI-OpsSpec")
- [npm @fission-ai/openspec](https://www.npmjs.com/package/@fission-ai/openspec) — v1.9.0 current (2026-08-13)
- [Claude Code — Monitoring Usage (OTel) docs](https://code.claude.com/docs/en/monitoring-usage) (event names, correlation IDs, redaction rules, content caps)
- [Anthropic Audit Logs API for compliance](https://amitkoth.com/log-claude-api-calls-compliance-siem/) (35 event types, 6-year retention — platform-side norm)
- [DeepSeek Harness developer preview page](https://deepseek.com/harness/en/) + [deepseek-ai/deepseek-harness repo](https://github.com/deepseek-ai/deepseek-harness) + [docs/user/guide/providers.md](https://github.com/deepseek-ai/deepseek-harness/blob/master/docs/user/guide/providers.md) + [x-cmd package page](https://www.x-cmd.com/install/deepseek-harness/) (v0.1 preview, Cordis, plugins, providers/protocols, session-log traceability)
- [VentureBeat — DeepSeek Harness launch](https://venturebeat.com/technology/deepseek-harness-launches-as-open-source-rival-to-claude-code-alongside-v4-pro-on-api-with-higher-prices)
- [DeepSeek API docs — Codex integration](https://api-docs.deepseek.com/quick_start/agent_integrations/codex/) (Responses API + SKILL.md skills context)
- [RichardAtCT/claude-code-telegram](https://github.com/RichardAtCT/claude-code-telegram) (session binding, media, voice transcription options, whitelist, audit logging, polling service)
- [Claude Code + Telegram voice & threading (dev.to)](https://dev.to/timmothybuilder/claude-code-telegram-how-to-supercharge-your-ai-assistant-with-voice-threading-more-1b69) (Channels plugin: STT fallback chain, forum topics, pairing, daemon supervisor)
- [implicator.ai — adding voice to Claude Code Channels](https://www.implicator.ai/claude-code-channels-has-no-voice-support-you-can-add-it-in-20-minutes-2/)
- [General Analysis — Claude Code OTel observability](https://generalanalysis.com/guides/claude-code-control-observability-opentelemetry)

---
*Feature research for: ass-guard v1.1 (slash-command kickoff, audit log, parity re-capture, Telegram peer, dsh profile #2)*
*Researched: 2026-08-14*
