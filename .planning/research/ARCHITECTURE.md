# Architecture Research

**Domain:** v1.1 feature integration onto the shipped ass-guard v1.0 architecture (24 Go packages, ~33.5k LOC) — slash-command kickoff, LOG-01 audit completion, zcode parity re-capture, Telegram peer, deepseek-harness profile #2
**Researched:** 2026-08-14
**Confidence:** HIGH for integration points (all verified against source + a live `openspec v1.5.0` probe); MEDIUM for Telegram/dsh internal details (library/target specifics verified at the API-surface level, internals to be validated during build)

> This document covers ONLY how the five v1.1 features integrate with the shipped architecture. The v1.0 architecture itself (event-bus spine, two-layer context, provider-shape isolation, unified engine, stdio discipline) is treated as given — it is embodied in the code and documented in MILESTONES.md. Every integration point below names the real package, type, and function it attaches to.

---

## System Overview

The five v1.1 features attach to the shipped architecture at four layers. Nothing rewrites a v1.0 component; every feature is either a new leaf package or new wiring inside existing seams.

```
┌──────────────────────────────────────────────────────────────────────────┐
│ FRONTENDS (process surfaces — stdout stays ACP-only in every mode)       │
│                                                                          │
│  ┌─────────────────────┐        ┌────────────────────────────────────┐   │
│  │ internal/acp        │        │ internal/telegram        [NEW]     │   │
│  │ JSON-RPC v1 stdio   │        │ go-telegram/bot long-poll goroutine│   │
│  │ session/prompt ─────┼──┐     │ per-chat session binding + voice   │   │
│  └─────────────────────┘  │     │ STT (Whisper default)              │   │
│                           │     └───────────────┬────────────────────┘   │
│  cmd/ass-guard: `acp serve`   `telegram` [NEW]  │ both share one core    │
└───────────────────────────┼─────────────────────┼────────────────────────┘
                            ▼                     ▼
┌──────────────────────────────────────────────────────────────────────────┐
│ TURN CORE (extracted from cmd/ass-guard sessionTurnRunner → internal/    │
│ runtime [NEW package]; owns sessionFor + runOneTurn + engine wiring)     │
│                                                                          │
│  ┌────────────────────────────────────────────────────────────────────┐  │
│  │ slash-command expander [MODIFIED: internal/ecosys + wiring]        │  │
│  │ "/opsx:explore args" → ecosys.AllCommands → markdown body          │  │
│  └────────────────────────────────────────────────────────────────────┘  │
│  ┌───────────────────────────┐  ┌────────────────────────────────────┐   │
│  │ internal/session (Core)   │  │ internal/engine (post-turn observer)│   │
│  │ Prompt loop, transcript,  │  │ Observe → Decide → continue/hook/   │   │
│  │ projector, boundaries     │  │ ask/wait; PatternTable from         │   │
│  └────────────┬──────────────┘  │ internal/openspec toml [MODIFIED]   │   │
│               │                 └────────────────────────────────────┘   │
│  ┌────────────▼──────────────────────────────────────────────────────┐   │
│  │ audit wiring [MODIFIED: RequestCapturer on serve path +            │   │
│  │ TranscriptWriter per session + optional --audit-log sink]          │   │
│  └───────────────────────────────────────────────────────────────────┘   │
├──────────────────────────────────────────────────────────────────────────┤
│ PROVIDER / PROFILE LAYER                                                 │
│  ┌────────────────────────────┐  ┌────────────────────────────────────┐   │
│  │ internal/profile [MODIFIED]│  │ internal/provider [MODIFIED]        │   │
│  │ loader + extractor ride    │  │ OpenAI shape gains system-block     │   │
│  │ unchanged; profile #2 =    │  │ mapping (today: silently dropped)   │   │
│  │ profiles/dsh bundle [NEW]  │  │ + dsh DeepSeek provider entry       │   │
│  └────────────────────────────┘  └────────────────────────────────────┘   │
└──────────────────────────────────────────────────────────────────────────┘
```

### Component Responsibilities (v1.1 delta view)

| Component | Status | Responsibility for v1.1 |
|-----------|--------|------------------------|
| `internal/ecosys` | MODIFIED | Gains namespaced command discovery (`commands/<ns>/<name>.md`), richer command frontmatter (allowed-tools/model), and a pure `Expand` function; gets its first non-test importer |
| `internal/openspec` | MODIFIED | `seeded.toml` `[commands]` reconciled to the real v1.5.0 binary surface; registered tools gain an Adapter-backed `Execute`; pattern table re-seeded from real handoff texts |
| `cmd/ass-guard` (`acp_serve.go`) | MODIFIED | Loads the ecosys registry at startup, expands `/ns:name` in `sessionTurnRunner.Run`, passes `--audit-log` into the serve path, attaches the RequestCapturer, wires the TranscriptWriter, gains the `telegram` subcommand |
| `internal/runtime` (or similar) | NEW | The extracted turn core (sessionTurnRunner + engine/hook/learning wiring) shared by ACP and Telegram frontends |
| `internal/telegram` | NEW | Bot long-poll loop, per-chat session binding, chat-message emit sink, /stop cancellation, voice download + STT hand-off |
| `internal/stt` (or `internal/telegram/stt`) | NEW | `Transcriber` interface; OpenAI Whisper default via `go-openai` (existing dep), Groq via base-URL swap, whisper.cpp via subprocess |
| `internal/audit` | MODIFIED (small) | Reused as the optional flat-file sink on the serve path; the primary audit artifact stays the transcript (D-20) |
| `internal/session` | MODIFIED (small) | `sessionFor` path gains TranscriptWriter construction; possibly a `CurrentTurnID()` accessor so captured requests carry a TurnID |
| `internal/provider` | MODIFIED | `openai.go buildRequest` maps `profile.System` → system message (today dropped — verified); dsh turns need it |
| `internal/profile` | MODIFIED (small) | Loader tolerates absent `thinking.json`/`tool_choice.json` (hard-errors today); `cmd/extract-profile` gains a dsh source+log extraction mode |
| `profiles/dsh/` + `internal/defaults/seed/profiles/dsh/` | NEW | Profile #2 artifact bundle, zero-config seeded |
| `internal/redact` | UNCHANGED | Both audit paths already redact through it — nothing new needed |

---

## Recommended Project Structure (v1.1 additions)

```
ass-guard-agent/
├── cmd/ass-guard/
│   ├── acp_serve.go          # MODIFIED: ecosys load, expansion call, audit flags, telegram sidecar flag
│   ├── telegram_cmd.go       # NEW: `telegram` cobra subcommand (telegram-only launch)
│   ├── provider_factory.go   # MODIFIED: generalize tracerProvider → shared captured-provider helper
│   └── profile_check.go      # MODIFIED: per-profile capture loader (dsh live path)
├── internal/
│   ├── runtime/              # NEW: extracted turn core (runner, engine wiring, sessionFor)
│   │   ├── runner.go         #   sessionTurnRunner moved here, frontend-agnostic RunTurn
│   │   └── engine_wiring.go  #   setupEngine, dispatchers, hook seams (moved from acp_serve.go)
│   ├── ecosys/
│   │   ├── loader.go         # MODIFIED: discoverCommands walks one subdirectory level (namespace)
│   │   ├── types.go          # MODIFIED: Command gains Namespace + frontmatter directives
│   │   └── expand.go         # NEW: pure "/ns:name args" parser + body expansion ($ARGUMENTS)
│   ├── telegram/             # NEW: frontend (bot loop, chat binding, voice download)
│   │   ├── frontend.go       #   Start(ctx) long-poll, handler registration, chat→session map
│   │   ├── sink.go           #   bus-chunk → Telegram message batching (rate-limit aware)
│   │   └── stt.go            #   Transcriber interface + whisper-openai/groq/whisper-cpp impls
│   ├── openspec/
│   │   ├── seeded.toml       # MODIFIED: real binary surface + real handoff patterns
│   │   └── register.go       # MODIFIED: register Adapter-backed Execute per command
│   └── profile/
│       └── loader.go         # MODIFIED: optional thinking/tool_choice files
├── profiles/dsh/             # NEW: profile #2 bundle (profile.yaml, system/, tools.json, …)
└── internal/defaults/seed/
    └── profiles/dsh/         # NEW: sanitized seed copy (sync.sh extended)
```

### Structure Rationale

- **`internal/runtime` extraction is the one structural refactor v1.1 needs.** Today the turn core (`sessionTurnRunner`, engine wiring, hook seams, MCP spawn) lives in `cmd/ass-guard/acp_serve.go` (package `main`). Telegram must drive the identical path (same sessions, same engine, same audit); a frontend-agnostic core package is the only way both surfaces share it without duplicating 400 lines of wiring. It is a move, not a rewrite — the ACP path calls the same functions afterward.
- **`internal/telegram` is a leaf** mirroring `internal/acp`'s shape: it owns a transport, a session-binding policy, and a chunk sink; it never imports provider/profile logic directly.
- **Expansion lives in `internal/ecosys`** because it is pure over the already-precedence-resolved `Registry`; no new package needed, and the read-only `.claude/` contract stays in one place.
- **Profile #2 is data, not code**: a directory bundle under the existing name-keyed loader (verified: `profile.NewLoader(root).Load(name)` is profile-agnostic — PROF-02 held).

---

## Architectural Patterns (feature-by-feature integration)

### Pattern 1: Slash-command expansion — hook at the TurnRunner, not the ACP handler

**What:** Intercept `/namespace:name args` in incoming prompt text, resolve via the ecosys precedence chain, expand the command's markdown body, and feed the expanded text as the turn's user message.

**Where it hooks (the decision):** `sessionTurnRunner.Run` in `cmd/ass-guard/acp_serve.go:405` (moving to `internal/runtime`), immediately before `runOneTurn` — NOT in `internal/acp.handleSessionPrompt` (`internal/acp/handlers.go:102`).

Rationale, verified against the code:

1. `internal/acp` is protocol-pure: it has no filesystem, no workDir, no ecosys dependency. Putting markdown-file resolution there drags ecosystem discovery into the JSON-RPC layer and breaks the package's clean test surface.
2. `sessionTurnRunner` already owns `workDir` (the discovery root), the profile, and the engine — everything expansion needs. It is also the single place the Telegram frontend will enter after the runtime extraction, so Telegram gets slash-commands for free (a hard requirement: "Telegram can drive an entire SDD scenario").
3. The expanded blocks must be what `engine.Observe` receives as `userPrompt` (`acp_serve.go:487`), so expansion must happen before `runOneTurn`, and the transcript's `user_message` line records the *expanded* body (the audit trail shows what the model actually saw).

**Load-bearing gap found (verified live):** `internal/ecosys.discoverCommands` (`internal/ecosys/loader.go:164`) scans `commands/*.md` flat and explicitly skips directories (`if e.IsDir() … continue`). But `openspec init --tools claude` (probed against the real v1.5.0 binary) installs commands as a **subdirectory**: `.claude/commands/opsx/{explore,apply,propose,sync,archive}.md` plus five skills (`.claude/skills/openspec-*/SKILL.md`). With today's loader, zero opsx commands are discoverable. The loader needs one-level-deep discovery mapping `commands/<ns>/<name>.md` → key `ns:name` (matching the `/opsx:explore` invocation surface). Top-level `commands/<name>.md` stays the un-namespaced form.

**Expansion semantics:**

- `ecosys.Command` (types.go) gains the frontmatter directives the body may carry: `allowed-tools`, `model`, plus keep `description`. The current `parseCommand` decodes only `description`; opsx v1.5.0 files use `name/description/category/tags` (verified) — unknown keys are ignored by YAML decode, so this is additive.
- `$ARGUMENTS`: the ecosys doc already promises the placeholder. Verified: opsx v1.5.0 bodies do NOT use `$ARGUMENTS` (they reference "the argument after `/opsx:explore`" in prose). Expansion rule (Claude-Code convention): substitute `$ARGUMENTS` where present; otherwise append the args as a trailing line (`Arguments: <args>`). Both paths needed.
- Frontmatter directives: v1 `allowed-tools` can restrict the session's projected tool set for that turn (a natural extension of the per-session profile copy already built in `sessionFor`, `acp_serve.go:534-538`); `model` can influence the tier/model override through the existing scheduler config. Ship allowed-tools as a follow-on within the phase; do not let it block the kickoff UAT.
- Unknown `/command` (no registry match): pass the text through unchanged + a stderr log. This matches the project's structural-safety posture (no match ⇒ nothing happens); an ACP error response would break Telegram parity where the same rule applies.
- Exposure: ACP v1 has no commands-list method, so "expose loaded slash-commands" = stderr log at startup + a small `ass-guard commands` listing (optional). The user types `/opsx:explore` as ordinary prompt text; Zed forwards it verbatim.

**How OpenSpec stages then drive (the full kickoff chain):**

```
user types "/opsx:explore add-auth" in Zed
  → ACP session/prompt (verbatim text)
  → runtime.RunTurn: ecosys.Expand → opsx:explore markdown body (+args)
  → Session.Prompt: transcript user_message = expanded body; provider turn
  → model follows the command body: runs `openspec list --json`, `openspec show …`
    (via Bash tool calls — the binary as supporting tooling) and/or the
    namespaced openspec:* catalog tools
  → turn ends (end_turn)
  → engine.Observe → Decide(TurnOutput, OpenSpecPatternTable)
      text handoff ("proposal ready…") OR tool-call handoff → ActionContinue
  → NextStagePrompt injects the next stage as a REAL turn (D-12)
  → hooks fire post-implement/post-phase via acpDispatcher.Hook → hookdag
```

**Adapter reconciliation — verified against the real binary (v1.5.0 `--help` probed):** the real surface is `init, update, list, view, change, archive, spec, config, schema, store, doctor, context, workset, validate, show, status, instructions, templates, feedback, completion`. The seeded `[commands]` table (`internal/openspec/seeded.toml`) declares `list/show/validate/apply/implement` — `apply` and `implement` do not exist as binary subcommands (they are agent-workflow stages driven by the command files). Reconcile: read-only `list, show, view, validate, status, instructions, context, doctor, spec(list/view)`; mutating `archive, change, config, schema, store`. Note the UAT report's claim that show/validate "don't exist" was half-right: they DO exist on v1.5.0; the true mismatch is `apply`/`implement`.

**Should tools be re-derived from `openspec/config.yaml`? No.** Probed the file produced by `openspec init`: it holds `schema:` (e.g. `spec-driven`), optional project `context:` and per-artifact `rules:` — it is project configuration for artifact generation, NOT a command-surface declaration. Deriving the tool table from it is not viable. The `[commands]` table should be reconciled against the binary surface (a dev-time probe; the table stays hand-maintained TOML), and mutability stays the single source of truth for boundaries via `toolcat.IsBoundary` (verified: `openspec.RegisterTools` → `toolcat.Tool.Mutability` → existing boundary floor, no boundary-engine change).

**Second load-bearing gap (verified):** `openspec.RegisterTools` (`internal/openspec/register.go:28`) registers catalog entries with **no `Execute`** — the comment says the engine invokes them via the Adapter, but the actual 04-05 wiring gives the session `toolexec.RealExecutor{Catalog}`, whose nil-Execute path returns `{"error":"tool openspec:* has no implementation yet"}` (`internal/toolexec/real.go:67-71`). The Adapter (`internal/openspec/adapter.go`) is constructed nowhere outside tests. The fix is small: in `RegisterTools`, set `Execute` to a closure over an `Adapter` that parses `{command, args}` input and calls `Run` — then model-invoked `openspec:list` style calls actually execute the binary. (The command-file bodies mostly drive the binary through `Bash`; the namespaced tools are the structured alternative and must work when the model prefers them.)

**When to use / trade-offs:** expansion-at-runner keeps ACP pure and shares across frontends, at the cost of one indirection layer (runtime package). Doing it in the ACP handler instead would be less code today and wrong tomorrow (Telegram + the "same core" constraint).

### Pattern 2: LOG-01 audit wiring on `acp serve` — transcript is the artifact, file sink is optional

**What:** Make `--audit-log` (and, more importantly, `request_shaped` capture) real on the serve path.

**Verified current state:**

- `--audit-log` is a root persistent flag (`cmd/ass-guard/main.go:74`); cobra propagates it to `acp serve`, but `runACPServeCmd` never reads it — the value dies unparsed-to-use. The tracer path (`runTrace`) owns the only audit wiring: `openAuditSink` + `audit.NewAuditLogger(bus, sink)` + `tracerProvider(...)` which reconstructs the Anthropic adapter with `WithAnthropicRequestCapture` (`cmd/ass-guard/provider_factory.go:90-109`).
- On the serve path, `makeProvider` (`acp_serve.go:272`) calls `factory.Build(providerName, shaper.New())` with **no capturer** — so `event.RequestShaped` is never published, and neither an AuditLogger nor a TranscriptWriter could see anything.
- **Second verified gap:** `session.NewTranscriptWriter` has zero non-test callers. The serve path never runs it, so streaming audit lines (`request_shaped`, `agent_message_chunk`, `usage`) never reach the per-session transcript — only the session-structure lines written synchronously by `Session.Prompt`. D-20 ("audit log = transcript, one artifact") is only half-realized on the serve path.

**Recommended wiring (reuse, not new mechanisms):**

1. **Attach the capturer:** generalize `tracerProvider` into one shared helper (e.g. `capturedProvider(factory, name, capturer)`) handling BOTH shapes — the OpenAI adapter already has `WithOpenAIRequestCapture` (`internal/provider/openai.go:53`, verified). `serveOptions` gains `AuditLogPath`; `runACPServe` passes a capturer closure publishing `event.RequestShaped{Profile: prof.Name, Timestamp: now}` to the shared bus.
2. **Primary artifact = transcript (D-20):** construct and run one `TranscriptWriter` per session inside the `sessionFor` path (`go tw.Run(ctx)`), so `AppendRequestShaped`/chunks/usage land in `.ass-guard/transcript_<sessionID>.jsonl`. This is the LOG-01 completion in the D-20 sense and needs no new code in `internal/audit`.
3. **Optional flat file:** when `--audit-log` is set, also `audit.NewAuditLogger(bus, sink)` for a process-wide cross-session JSONL (reuse verbatim; it already redacts and already panics on a stdout sink — keep that guard).
4. **TurnID on captured requests:** the `RequestCapturer` signature (`func(body []byte, headers map[string]string)`) carries no TurnID; the tracer publishes with it empty. Cheapest correct fix: add an atomic `CurrentTurnID()` accessor on `Session` (it already keeps `turnCounter`) and let the per-session capturer closure read it; acceptable fallback for v1.1 is empty TurnID (the line still lands, ordering preserved by append).

**Redaction already exists — nothing to build:** `internal/redact` redacts secret-carrier VALUES preserving field names (JSON-tree walker: authorization/api_key/bearer/token/x-api-key/…) with a Bearer/sk- regex fallback for non-JSON, plus `ScrubError`. Both consumers already route through it: `Manager.appendLine` redacts every transcript line before write (LOG-03), and `audit.AuditLogger.handle` redacts before sink. Verified importers: acp server/handlers (error scrubbing), session manager, audit, provider errors.

**When to use / trade-offs:** the alternative — a separate session-level event log under `.ass-guard/` — would create a second artifact and violate D-20's one-writer property (the TranscriptWriter doc comment explicitly warns about two-writer drift). Keep the flat `--audit-log` file strictly optional/operator-facing.

### Pattern 3: Telegram frontend — a second driver over the same turn core

**What:** A Telegram bot as a full peer surface: text and voice in, the same engine/sessions/audit out; runs either as a goroutine beside ACP stdio or as its own launch mode.

**Integration shape:**

1. **Shared core (the prerequisite):** extract `sessionTurnRunner` + `setupEngine` + the hook/learning/MCP wiring from `cmd/ass-guard/acp_serve.go` into `internal/runtime`. The ACP server then consumes it via the existing `acp.TurnRunner` interface (unchanged seam — `internal/acp/server.go:31`), and Telegram consumes the same runner through a frontend-agnostic entry (the chunk-forwarding goroutine in `Run` is the only ACP-specific part; it becomes one sink implementation among two). `runOneTurn`/`eng.Observe`/`sessionFor` are already frontend-agnostic — verified: everything below `Run` talks in `session.ContentBlock` and bus events.
2. **Long-poll loop:** `github.com/go-telegram/bot` (new dep — NOT in go.mod today, verified), chosen in v1.0 research for zero-dependency + idiomatic `context.Context` throughout. `bot.Start(ctx)` runs the GetUpdates loop; ctx cancellation drains it. Handler-based registration maps message updates to the frontend.
3. **Per-chat session binding:** `map[chatID]*session.Session` over the same `sessionFor` construction, with **stable session IDs** (`tg-<chatID>`) so the durable transcript (`transcript_tg-<chatID>.jsonl`) survives process restarts — unlike ACP's per-editor-session UUIDs. One chat = one continuing session = one engine state.
4. **Chunk streaming to chat:** subscribe `AgentMessageChunk` on the shared bus exactly as `sessionTurnRunner.Run` does, batch into Telegram messages (rate limits: ~1 msg/sec per chat, edit-message throttling — v1 recommendation: stream coarse chunk batches, fall back to final-message-only if editing proves noisy). Final `stopReason` mirrors the ACP contract.
5. **Voice flow:** `message.Voice` (ogg/opus) → `getFile` → download from `https://api.telegram.org/file/bot<token>/<file_path>` (Bot API mechanics; the library exposes GetFile + the URL helper) → `Transcriber.Transcribe(ctx, io.Reader) (string, error)` → the transcript text becomes an ordinary user prompt through the same `RunTurn` (with a transcript note that it originated as voice). Backends behind one interface: **OpenAI Whisper API default via `sashabaranov/go-openai` transcription (already a dependency — zero new deps for the default)**; Groq = base-URL swap on the same client; whisper.cpp = out-of-process `whisper-cli` subprocess (never cgo — goreleaser/static-binary constraint). Config in `.ass-guard/telegram.yaml` (seeded by firstrun; mirrors scheduling.yaml conventions), bot token via the established credential precedence (flag > `$TELEGRAM_BOT_TOKEN` env > config literal).
6. **Launch modes:** new cobra subcommand `telegram` (telegram-only: runner + bot, no ACP server, stdin unused) AND an `acp serve --telegram` flag to run the bot goroutine beside the ACP server sharing one runner (the PROJECT.md requirement "goroutine in the same process as ACP stdio"). One GetUpdates loop per token — never two (409 conflict), a pitfall if both modes run against the same bot.
7. **Shutdown:** derive one ctx from `signal.NotifyContext` (the `acp serve` RunE already does this — `acp_serve.go:141`); on cancel: bot long-poll returns (`Start(ctx)`), in-flight Telegram turns abort via per-chat cancel funcs mirroring `sessionState.cancelTurn` (a `/stop` chat command maps to the same cancel), then `closeAllSessions()` reaps MCP subprocesses (existing helper, `acp_serve.go:610`).
8. **stdout discipline:** the bot library's logger must be redirected to stderr (`bot.WithLogger` / slog bridge). In telegram-only mode stdout stays byte-clean even though unused — enforce via the same review/lint gate as ACP.

**When to use / trade-offs:** running Telegram in-process (not a separate daemon) honors "single static binary, no daemon"; the cost is that a Telegram-driven heavy turn competes with ACP turns for the process-wide provider semaphore (existing `provider.NewSemaphore(maxConc)`) — acceptable and arguably correct (PARA-04 bounds outbound concurrency across parent + subagents already; Telegram turns are just more parents).

### Pattern 4: Profile #2 (deepseek-harness) — data bundle plus two targeted code fixes

**What:** Mimic `deepseek-ai/deepseek-harness` ("dsh" — DeepSeek's MIT-licensed, plugin-based open-source coding agent; TypeScript, "traceable sessions") for DeepSeek-model turns, riding the N-profile architecture with no zcode-specific paths.

**The artifact (identical bundle shape, verified loader contract):** `profiles/dsh/` containing `profile.yaml` (name/model/max_tokens), `system/block-*.txt` (byte-faithful, lexical order), `tools.json` (name+description+input_schema), `identity.yaml` (header NAMES byte-faithful, value templates), `thinking.json`, `tool_choice.json`, plus `coverage.yaml` (tiered manifest with `TargetCaptureRef` provenance). `internal/profile.Loader.Load(name)` is directory-keyed and profile-agnostic — zero changes to load dsh. Selection is already a flag: `ass-guard acp serve --profile dsh`.

**Two code adaptations required (both verified gaps):**

1. **OpenAI-shape system mapping:** `internal/provider/openai.go buildRequest` (verified, lines 108-155) never reads `profile.System` — system blocks are silently dropped on the OpenAI shape. DeepSeek's API is OpenAI-shape, so dsh turns would carry NO system prompt. The adapter must prepend the system content as message(s) — exact form (one joined system message vs multiple vs developer-role) is a ground-truth extraction question: mimic however dsh itself sends it.
2. **Optional profile fields:** `Loader.Load` hard-errors when `thinking.json`/`tool_choice.json` are missing (`os.ReadFile` error returned). Anthropic-isms may be absent in dsh (Chat Completions has no thinking param). Either convention: ship empty files, or (cleaner) make the loader tolerate absence when `profile.yaml` declares `shape: openai`. Keep the Shaper untouched (Anthropic path unchanged).

**Extraction (ground-truth, per the "log-extracted, not hand-written" Key Decision):** dsh is open source, so capture is two-sourced: (a) its **TypeScript source** — system prompts, tool registry schemas, identity/request assembly are in-repo code; (b) **its own session logs** (dsh advertises traceable sessions — on-disk format is a build-time discovery item). This differs from zcode (closed binary, logs-only) but the profile contract is the same: verbatim system text (TIER-1), tool name+schema set (TIER-1), header names (TIER-2), model/max_tokens (TIER-3). `cmd/extract-profile` + `internal/profile/extract.go` gain a dsh mode (the current `ModelIO` rollout schema is zcode-specific); `CoverageEntry.Source` already records which artifact each field came from. Where logs and source disagree, logs win (source shows what it *would* send; logs show what it *did*).

**Drift-check adaptation:** `drift.Detect` + `CoverageManifest.Validate` are profile-agnostic (verified — they consume a manifest + a captured map). The only zcode-ism is `profile_check.go`'s live path hardcoding `~/.zcode/cli/rollout`. Fix: a per-profile capture loader (dsh's log location resolved from its config/layout), with `--capture-file` continuing to work for fixtures. `ass-guard profile check dsh` then runs the identical tiered diff.

**Scheduling:** a DeepSeek provider entry (openai shape, operator's opencode-subscription endpoint or DeepSeek API) in `scheduling.yaml` is pure config — the two-shape `ProviderFactory.Build` already constructs it. Open design note: today profile is chosen per-launch (`--profile`), while the scheduler picks provider per tier. Coupling tier→profile (so DeepSeek-model turns automatically use the dsh profile) is the natural v1.2 extension; v1.1 keeps profile selection explicit at launch — do not silently introduce per-turn profile switching under this feature.

### Pattern 5 (cross-cutting): operator-gated verification as phase gates

The kickoff phase's gate is the operator-gated real-binary run (`ASSGUARD_OPENSPEC_BIN=1`) — the stub-binary E2E is exactly what hid the model mismatch last time (UAT root cause). The parity re-capture (`ZAI_API_KEY` + a divergence-prone capture session, pinned in `profiles/zcode/coverage.yaml` `TargetCaptureRef`) unblocks `internal/profile/stability_test.go`, which fails today because session `eea3dc48` is absent on disk. Both are operator actions riding existing machinery — the code work around them is small, which is why they pair naturally in one phase.

---

## Data Flow

### Request Flow (v1.1 — slash-command kickoff, the headline flow)

```
[User types /opsx:explore add-auth in Zed]
    ↓ (ACP session/prompt — verbatim text block)
internal/acp.handleSessionPrompt → turnRunner.Run(ctx, sessionID, emit, prompt)
    ↓
internal/runtime.RunTurn
    ├─ ecosys registry (loaded once at startup via Discover(workDir))     [NEW wiring]
    ├─ ecosys.Expand("/opsx:explore add-auth")                            [NEW]
    │    → Command{Namespace:"opsx", Name:"explore", Body, AllowedTools, Model}
    │    → expanded text (body with $ARGUMENTS→args, or body + "Arguments: …")
    ↓
runOneTurn → engine.Observe(engineTurnRunnerAdapter, patternTable, expandedBlocks)
    ↓
session.Session.Prompt  (transcript user_message = EXPANDED body; provider turn;
    ↓                    model drives openspec binary via Bash / openspec:* tools)
engine.Decide(TurnOutput, OpenSpecPatternTable)  → continue / hook / ask / nothing
    ↓ (ActionContinue)
NextStagePrompt → real turn → … until non-continue decision or cancel
```

### Audit Flow (LOG-01 completion)

```
Session.streamAndEmit → Provider.Stream(shape(profile, messages))
    ├─ adapter invokes RequestCapturer(body, headers)                      [NEW on serve path]
    ↓ capturer publishes event.RequestShaped{TurnID?, VerbatimRequest, Profile}
event.Bus
    ├─ session.TranscriptWriter (per-session, Run(ctx))                    [NEW wiring]
    │    → Manager.AppendRequestShaped → redact.Redact → transcript_<id>.jsonl
    └─ audit.AuditLogger (optional --audit-log file)                       [reuse]
         → redact.Redact → <audit-log> JSONL
```

### Telegram Flow (text + voice)

```
Telegram update → internal/telegram handler
    ├─ text:  prompt → runtime.RunTurn(chatSession, blocks, telegramSink)
    ├─ voice: getFile → download ogg/opus → Transcriber.Transcribe
    │         → text block → same RunTurn (voice origin noted in transcript)
    └─ /stop: chat-level cancel func → drains in-flight turn (mirror session/cancel)
runOneTurn (identical to ACP: same engine, same hooks, same audit)
    ↓ AgentMessageChunk events on shared bus
telegramSink batches chunks → chat messages (rate-limit aware)
```

### Key Data Flows

1. **Command expansion is pre-transcript:** the expanded body is recorded as the user_message — the transcript remains the faithful record of what the model saw (investigate-and-fix-ready contract).
2. **One bus, two sinks:** ACP `session/update` notifications and Telegram messages are both subscriptions on the same `AgentMessageChunk` stream; neither sink is in the turn's critical path.
3. **Voice becomes text before the core:** STT is a frontend concern; the turn core never knows a prompt came from audio (preserves the mimicry contract — outgoing requests are identical regardless of surface).
4. **Profile #2 is load-time data:** dsh changes what the Shaper emits by changing the bundle, not the shaping code (one exception: the OpenAI system-mapping fix, which is generic, not dsh-specific).

## Scaling Considerations

Not a user-scale system; the real "scaling" axes and what breaks first:

| Axis | What breaks first | Mitigation already in place / needed |
|------|-------------------|--------------------------------------|
| Transcript volume (request_shaped lines are ~80 KB each) | Transcript files grow fast once LOG-01 lands on serve | Acceptable (operator artifact, self-gitignored `.ass-guard/`); optional rotation is post-v1.1 |
| Concurrent turns (ACP + Telegram chats) | Outbound provider concurrency | Existing process-wide `provider.NewSemaphore` (PARA-04) already bounds parent + subagents; Telegram turns are additional parents under the same cap |
| Telegram message rate | Edit/send throttling per chat (~1/s) | Batch chunks in the sink; final-message fallback |
| Ecosystem discovery cost | Re-scanning `.claude/` per prompt | Load once at startup (per `acp serve` process); reload-on-change is post-v1.1 |

### Scaling Priorities

1. **First bottleneck:** audit line volume vs transcript size — measure during the kickoff phase E2E; keep `--audit-log` file opt-in.
2. **Second bottleneck:** Telegram turn concurrency from multiple chats — bounded by the semaphore; serialize per-chat (one in-flight turn per chat, queue or reject with a "busy" reply) to keep transcripts deterministic.

## Anti-Patterns

### Anti-Pattern 1: Expanding slash-commands inside internal/acp

**What people do:** Patch `handleSessionPrompt` to detect `/…` and read command files.
**Why it's wrong:** Puts filesystem/ecosys knowledge into the protocol layer; Telegram (and any future frontend) then needs a second implementation; the acp package's clean test surface (in-memory reader/writer) dies.
**Do this instead:** Expand in the shared turn core (runtime.RunTurn) before `runOneTurn`; internal/acp stays transport-pure.

### Anti-Pattern 2: Treating the openspec binary as the stage driver

**What people do:** Model the workflow as `openspec apply` / `openspec implement` subprocess stages (exactly what seeded.toml did).
**Why it's wrong:** Those subcommands don't exist (verified against v1.5.0). The toolkit's workflow is driven by agent-executed command files (`/opsx:*` markdown); the binary is supporting tooling (list/show/validate/status/instructions/context/doctor/archive/change…).
**Do this instead:** ecosys discovery + expansion drives stages; the adapter's `[commands]` table mirrors the real binary surface; the pattern table matches real handoff texts.

### Anti-Pattern 3: A second audit artifact under `.ass-guard/`

**What people do:** Add a parallel `events.jsonl` beside the transcript for LOG-01.
**Why it's wrong:** Violates D-20 (audit = transcript, one writer, one artifact); two writers drift; redaction must be maintained twice.
**Do this instead:** Wire the existing TranscriptWriter per session; keep the flat `--audit-log` strictly as an optional operator-facing mirror reusing `internal/audit`.

### Anti-Pattern 4: cgo-binding whisper.cpp, daemonizing Telegram, or porting the bot to HTTP

**What people do:** Link whisper via cgo for "integrated" STT; run Telegram as a separate daemon process or open a port.
**Why it's wrong:** Breaks the single static binary / no-daemon / no-port constraints (goreleaser CGO_ENABLED=0 cross-compile; editor-owned lifecycle).
**Do this instead:** whisper.cpp as an out-of-process `whisper-cli` subprocess if ever needed (Whisper API default needs no new dep); Telegram as a goroutine inside the one process; stdout stays ACP-only in every mode.

### Anti-Pattern 5: Hand-writing the dsh profile from its source "because it's open source"

**What people do:** Copy system prompts/tool schemas out of the TypeScript repo and call the profile done.
**Why it's wrong:** The Key Decision is log-extracted content — source shows intent, logs show the wire. Source-only extraction repeats the hand-written-profile guess the project explicitly rejected.
**Do this instead:** Extract from dsh's own session logs where available (traceable sessions), use source to fill fields logs don't carry, and record provenance per field in `coverage.yaml` (`CoverageEntry.Source`).

## Integration Points

### External Services

| Service | Integration Pattern | Notes |
|---------|---------------------|-------|
| `openspec` v1.5.0 binary | subprocess via existing `internal/openspec.Adapter`; registered as catalog `Execute` closures | Read-only surface (`list/show/view/validate/status/instructions/context/doctor`) + mutating (`archive/change/config/schema/store`); `apply`/`implement` do NOT exist — stages are command-file driven |
| Telegram Bot API | `github.com/go-telegram/bot` long-poll (`Start(ctx)`); `getFile` + `https://api.telegram.org/file/bot<token>/<file_path>` for voice | New dep (verified absent from go.mod); one GetUpdates loop per token (409 on two); redirect library logs to stderr |
| OpenAI Whisper API (STT default) | `sashabaranov/go-openai` transcription client (existing dep) | Groq = base-URL swap; whisper.cpp = subprocess, never cgo |
| DeepSeek / opencode-subscription endpoint | OpenAI-shape provider entry in `scheduling.yaml` via existing `ProviderFactory` | Requires the OpenAI adapter system-mapping fix or dsh sends no system prompt |
| Zed (ACP client) | unchanged stdio v1 server | Slash text arrives as ordinary prompt content; `session/load` stays no-op (D-09) |

### Internal Boundaries

| Boundary | Communication | v1.1 consideration |
|----------|---------------|--------------------|
| acp ↔ runtime | `acp.TurnRunner` interface (unchanged) | Runner moves into `internal/runtime`; ACP wiring is a constructor change only |
| telegram ↔ runtime | new frontend-agnostic `RunTurn(ctx, sessionID, blocks, sink)` | Same sessions/engine/audit; per-chat stable session IDs |
| runtime ↔ ecosys | startup `Discover(workDir)` + per-turn pure `Expand` | First non-test importer of ecosys (today zero — verified); ecosys stays read-only on `.claude/` |
| runtime ↔ engine/openspec | `engine.Observe` + `PatternTable` (unchanged) | Only the seeded config content changes; `triggerFromSignal` may need real stage names (post-explore/post-propose/post-apply/post-archive) beyond today's post-implement/post-phase |
| audit ↔ session | both consume `RequestShaped` on the shared bus | TranscriptWriter per session (primary), AuditLogger optional; both already redact via `internal/redact` |
| profile ↔ provider | `Profile` struct consumed by Shaper (anthropic) / buildRequest (openai) | OpenAI path must start honoring `System` blocks; loader must tolerate absent Anthropic-ism files |

## Suggested Build Order

Operator's strict priority chain (PROJECT.md): **kickoff gap first; LOG-01 + zcode re-capture may share a phase; Telegram before dsh profile.** Dependency analysis agrees, with one argument for sequencing the audit phase before Telegram (below).

| Phase | Contents | New / Modified | Gate |
|-------|----------|----------------|------|
| **1. Slash-command kickoff** | (a) ecosys namespaced discovery + `Expand`; runtime wiring at `Run`-before-`runOneTurn`; `ecosys.Command` frontmatter fields; openspec `seeded.toml` reconciliation (real binary surface, real handoff patterns, `apply`/`implement` removed); `RegisterTools` gains Adapter-backed `Execute`; extend `triggerFromSignal` stage vocabulary | M: `internal/ecosys`, `internal/openspec`, `cmd/ass-guard/acp_serve.go` (or the runtime extraction if done here); N: `ecosys/expand.go` | `mise ci`; **operator-gated `ASSGUARD_OPENSPEC_BIN=1` run against real openspec v1.5.0**; the 11 deferred Phase-4 UAT checks; run `openspec init --tools claude` in a scratch project and drive `/opsx:explore → propose → apply → archive` end-to-end |
| **2. Operational gaps (merged — adjacent per operator)** | (b) LOG-01 on serve: shared captured-provider helper (both shapes), `serveOptions.AuditLogPath`, per-session TranscriptWriter, optional AuditLogger sink, `CurrentTurnID()` accessor; (c-parity) zcode re-capture: operator runs the gated capture (`ZAI_API_KEY` + divergence-prone session), re-pin `coverage.yaml` `TargetCaptureRef`, `profile check zcode` green, stability test unblocked | M: `cmd/ass-guard/provider_factory.go`, `acp_serve.go`, `internal/session` (accessor); operator action for the capture | `mise ci`; `--audit-log` file verified redacted on a live serve; stability test passes on the new pinned session |
| **3. Telegram peer** | Runtime extraction to `internal/runtime` (prerequisite, mechanical move); `internal/telegram` frontend + per-chat binding + chunk sink; `internal/stt` Transcriber (Whisper default); `telegram` cobra subcommand + `acp serve --telegram` sidecar; context-first shutdown; `telegram.yaml` seed | N: `internal/telegram`, `internal/stt` (or nested), `cmd/ass-guard/telegram_cmd.go`, dep `go-telegram/bot`; M: `acp_serve.go` → runtime move, `internal/defaults` seed | `mise ci`; a full SDD scenario driven from a Telegram chat (text); voice message transcribed and drove a turn; SIGTERM drains long-poll + in-flight turns; stdout byte-clean in both modes |
| **4. deepseek-harness profile #2** | dsh ground-truth capture (source + its logs); `profiles/dsh` bundle + sanitized seed; OpenAI adapter system mapping; loader optional-field tolerance; extract-profile dsh mode; profile_check per-profile capture loader; scheduling.yaml DeepSeek provider entry | N: `profiles/dsh`, seed copy; M: `internal/provider/openai.go`, `internal/profile/loader.go`, `cmd/extract-profile`, `cmd/ass-guard/profile_check.go` | `mise ci`; `--profile dsh` A/B parity harness run (existing `internal/parity`) against a DeepSeek endpoint; `profile check dsh` green |

**Why 2 before 3 (the one judgment call):** the operator chain allows LOG-01+re-capture to share a phase and requires only kickoff-first and Telegram-before-dsh. Doing audit wiring in Phase 2 means the shared captured-provider helper lands in `cmd/ass-guard` *before* the Phase-3 runtime extraction moves it — the capturer wiring is written once, in its final home, instead of being re-wired during the move. (PROJECT.md's compressed chain notation `1→4→3→5→2` reads as Telegram immediately after kickoff; the three operator constraints are satisfied by both groupings — flagging this so the roadmapper phases it deliberately. The Phase-2→3 order is recommended for the write-once reason above; if the operator prefers 1→4 first, Phase 2's audit work must be re-homed in Phase 3's extraction with zero functional change.)

Cross-phase invariants (every phase): `mise ci` clean (vet + golangci-lint v2 all-linters + CGO_ENABLED=0 build + `go test -race`); stdout = ACP frames only; no daemon, no port; static binary; `.claude/` strictly read-only.

## Sources

- Codebase (primary, all read this session): `internal/{acp,session,engine,openspec,ecosys,audit,redact,profile,provider,shaper,scheduler,toolcat,toolexec,hookdag,learning,mcp,event,defaults,firstrun,drift,parity}/*.go`, `cmd/ass-guard/{main,acp_serve,provider_factory,profile_check}.go`, `go.mod`, `profiles/zcode/`, `internal/defaults/seed/`
- `.planning/PROJECT.md` (v1.1 milestone scope, priority chain, constraints) and `.planning/milestones/v1.0-phases/04-…/04-UAT.md` (kickoff gap root cause)
- Live probe (2026-08-14): `openspec v1.5.0` (`/usr/local/bin/openspec`) — `--help` surface; `openspec init --tools claude` in a scratch dir → `.claude/commands/opsx/{explore,apply,propose,sync,archive}.md` + 5 skills; `openspec/config.yaml` contents (schema/context/rules — no command surface)
- v1.0 research carried in repo: `.planning/research/{STACK,FEATURES,VERIFIED-FACTS}.md` (go-telegram/bot selection, JSONL/rollout formats, provider shapes)
- External grounding: Telegram Bot API file handling (`getFile` → `https://api.telegram.org/file/bot<token>/<file_path>`, core.telegram.org/bots/api#getfile); DeepSeek Harness (github.com/deepseek-ai/deepseek-harness — MIT, plugin-based, TypeScript, traceable sessions; deepseek.com/harness/en)

---
*Architecture research for: v1.1 feature integration (kickoff / audit / parity / Telegram / dsh profile)*
*Researched: 2026-08-14*
