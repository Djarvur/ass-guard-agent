# Stack Research — v1.1 New Features

**Domain:** Stack additions for the v1.1 milestone of ass-guard (Go AI coding agent with model-request mimicry)
**Researched:** 2026-08-14
**Confidence:** HIGH overall (every version verified against live sources in Aug 2026; OpenSpec surface captured from the installed v1.5.0 binary itself; per-area confidence in the tables)

> **Scope:** This document covers ONLY the five v1.1 features: (1) slash-command invocation + OpenSpec adapter reconciliation, (2) audit log on `acp serve`, (3) zcode parity re-capture, (4) Telegram peer with STT, (5) mimicry profile #2 (deepseek-harness). The v1.0 stack (ACP core, two-shape provider factory, scheduler, hook-DAG, MCP hosting, ecosys, goreleaser) is validated and NOT re-researched; it is referenced only where a new feature integrates with it.
>
> **Headline:** v1.1 needs exactly **two new Go dependencies** — `github.com/go-telegram/bot` (Telegram peer) and `github.com/klauspost/compress/zstd` (reading deepseek-harness session logs) — plus **zero new runtime-mandatory externals**. Features 2 and 3 need no stack at all (internal wiring). `gopkg.in/yaml.v3` is confirmed sufficient for command frontmatter; the real gaps are in `internal/ecosys` code (no recursion into subdirectories), not the library. Static-binary / no-cgo / goreleaser constraints survive everywhere (klauspost zstd is pure Go; whisper.cpp stays out-of-process).

---

## How To Read This Document

Each table row carries a **Confidence** level:

- **HIGH** — multiple independent current sources agree, or verified against a primary artifact (installed binary, module proxy, `go doc` of the pinned dependency); safe to build against.
- **MEDIUM** — solid primary source but one open verification item remains for our specific use; build against it, budget a small spike.
- **LOW** — plausible but genuinely uncertain; spike before committing.

---

## Recommended Stack

### New Core Technologies (v1.1 additions)

| Technology | Version | Purpose | Confidence | Why Recommended |
|------------|---------|---------|------------|-----------------|
| `github.com/go-telegram/bot` | **v1.23.0** (Aug 3, 2026; verified against GitHub releases + Go module proxy) | Telegram peer frontend (text + voice) | **HIGH** | Zero-dependency, idiomatic `context.Context` throughout. `bot.Start(ctx)` long-polls and **blocks until ctx cancellation** — exactly the context-first drain the ACP-owned process lifecycle needs. File download for voice: `b.GetFile(ctx, &bot.GetFileParams{FileID})` → `b.FileDownloadLink(file)` (plain `https://api.telegram.org/file/bot<token>/<file_path>`) → stdlib `http.Get`. Covers Bot API 10.2 (July 14, 2026). v1.23.0's changes are additive/marshal-fixes (union `MarshalJSON` returns errors instead of panicking; no caller-value mutation; `ReplyParameters.message_id` optional) — no breakage to the handler/`Start`/`GetFile` API ass-guard uses. Phase 0 already verified stdout-silence (load-bearing: stdout belongs to ACP frames). **Pin v1.23.0.** |
| `github.com/klauspost/compress/zstd` | **v1.19.2** (latest on Go module proxy, verified 2026-08-14) | Decode deepseek-harness `session.jsonl.zstd` logs (profile #2 ground truth) | **HIGH** (library) / **MEDIUM** (against real dsh files — one spike) | Pure Go, no cgo — goreleaser CGO_ENABLED=0 cross-compile survives. The decoder handles **concatenated independent zstd frames by default** (multistream, like the reference CLI) and verifies XXH64 frame checksums — matching dsh's format exactly (one checksummed frame per append batch; see Feature 5). De-facto standard (used across the Go ecosystem). **Spike:** decode one real `session.jsonl.zstd` before building the harvest tool on it. Note: v1.18.1 was retracted upstream and early v1.19.x had an arm64 runtime bug fixed in later v1.19.x — pin v1.19.2, don't take "latest" blind. |
| STT backend abstraction | (internal interface in `internal/` — no library) | Voice → text as ordinary user input | **HIGH** | Three backends collapse into one Go interface `Transcribe(ctx, oggBytes) (text, error)`: OpenAI-compatible HTTP (default; covers OpenAI + Groq via base-URL swap, reusing the **existing** go-openai client) and whisper.cpp subprocess (config-gated, out-of-process). No new HTTP stacks, no audio-processing Go deps (Telegram voice arrives as OGG/Opus bytes; both cloud APIs accept it natively — only the whisper.cpp path needs transcoding, see below). |

### Existing Dependencies Confirmed for New Use (no change needed)

| Dependency | Pinned In go.mod | New v1.1 Use | Confidence | Verification |
|------------|------------------|--------------|------------|--------------|
| `sashabaranov/go-openai` | v1.42.0 | STT default backend: `client.CreateTranscription(ctx, openai.AudioRequest{Model, FilePath/Reader, Language, Format})` → `AudioResponse.Text` (verified via `go doc` against the pinned module) | **HIGH** | The transcription call POSTs multipart to `<BaseURL>/audio/transcriptions`. Works against OpenAI (`gpt-transcribe` — current recommended; `gpt-4o-transcribe`/`gpt-4o-mini-transcribe`; `whisper-1` legacy-but-supported; 25 MB file cap) and against Groq by pointing `cfg.BaseURL` at `https://api.groq.com/openai/v1` (models `whisper-large-v3` / `whisper-large-v3-turbo`, ~$0.03/hr — OpenAI-compatible by contract). Model slug is pure config — the existing `internal/provider` credential chain (flag > env > config) extends unchanged. |
| `gopkg.in/yaml.v3` | v3.0.1 | Command/skill frontmatter for `/namespace:name` expansion | **HIGH** (sufficient) | Real YAML is a strict superset of zcode's flat single-line frontmatter; yaml.v3 ignores unknown keys by default and parses YAML flow arrays (`tags: [a, b]` — which real OpenSpec opsx command files carry). **No new parser library.** Gaps are in our code (see Feature 1). yaml.v3 is maintenance-mode (v3.0.1, 2022) and its successor `go.yaml.in/yaml/v4` (rc) is already an indirect dep — do not churn; revisit only if v4 goes GA. |
| `internal/ecosys` (existing package) | — | Command discovery feeding session/ACP | — | **Load-bearing gap found:** `discoverCommands` scans `commands/*.md` FLAT and skips directories — so `commands/opsx/explore.md` (installed by `openspec init --tools claude`) is invisible today. Fix is code, not stack: recurse subdirectories, join names with `:` (zcode rule: `review/code.md` → `/review:code`). |
| `internal/openspec` Adapter | — | Reconcile to real v1.5.0 binary | — | The adapter's subprocess pattern (`Run(command, args...)`) is already right; only the command set and seeded.toml mutability table are wrong (see Feature 1). |

### External Binaries (optional, config-gated — NOT Go dependencies)

| Binary | Version | Purpose | When Required |
|--------|---------|---------|---------------|
| `openspec` | **v1.5.0** (installed at `/usr/local/bin/openspec`; surface captured from the binary — see Feature 1) | SDD toolkit binary (supporting tooling; the workflow itself is command-file-driven) | Already the operator's environment; adapter calls it via PATH (existing `ErrOpenSpecNotFound` handling). |
| `whisper-cli` (whisper.cpp) | v1.9.2 (Oct 15, 2025 — latest; maintenance-cadence project, stable) | Local STT backend (offline/privacy) | Only when `stt.backend = whisper-cpp-local`. Out-of-process subprocess — **never** cgo-bind (protects goreleaser cross-compile; unchanged from v1.0 decision). |
| `ffmpeg` | any recent distro build | Transcode Telegram OGG/Opus → 16 kHz mono 16-bit PCM WAV for whisper-cli | Only on the whisper.cpp path. whisper-cli reads 16-bit WAV (or an FFMPEG-enabled build; assume stock builds need WAV). `ffmpeg -i voice.oga -ar 16000 -ac 1 -c:a pcm_s16le voice.wav`. Cloud APIs ingest OGG/Opus natively — no ffmpeg on the default path. |

### Development Tools (dev-time only, not shipped)

| Tool | Purpose | Notes |
|------|---------|-------|
| Node 22.19+/24 + pnpm 11.7.0 (Corepack) | Inspect/build deepseek-harness from source (profile #2 extraction) | dsh pins pnpm 11.7.0; `pnpm install && pnpm run typecheck`. Build via tsc project references + tsdown; tests vitest. Only needed on the operator's capture machine. |
| Telegram @BotFather | Bot token issuance | Dev-time; token flows through the existing credential chain (flag > env `TELEGRAM_BOT_TOKEN` > config). |

## Installation

```bash
# New runtime deps (only two)
go get github.com/go-telegram/bot@v1.23.0
go get github.com/klauspost/compress@v1.19.2   # import _ "github.com/klauspost/compress/zstd"

# Nothing else. Explicitly NOT added:
#   - no STT SDK          (existing go-openai client covers OpenAI + Groq)
#   - no audio/cgo lib    (whisper.cpp stays a subprocess)
#   - no YAML/frontmatter lib (yaml.v3 already in go.mod, confirmed sufficient)
#   - no Telegram webhooks framework (long-poll via Start(ctx); no network port — constraint)
```

---

## Feature Deep Dives

### Feature 1 — Slash-command invocation + OpenSpec adapter reconciliation

**Stack verdict: zero new libraries.** All work is code in `internal/ecosys` + `internal/session`/`internal/acp` + `internal/openspec`.

#### 1a. Command discovery and frontmatter (internal/ecosys)

Ground truth on formats (three sources, all primary):

- **zcode command rules** (from the zcode-guide skill shipped with the mimicry target): command name = filename, must match `^[a-z0-9][a-z0-9_:-]{0,63}$`; **subdirectories join with `:`** (`review/code.md` → `/review:code`); frontmatter is a **flat parser** — only single-line top-level keys, indented lines and multi-line arrays dropped; recognized keys `description`, `argument-hint`, `allowed-tools`, `model`, `skills`, `disable-noninteractive` (hyphenated); unknown keys ignored but command still loads; **description or non-empty body required** (else dropped); missing description → first non-empty body line used; `$ARGUMENTS` full-string and `$1`/`$2` positional substitution; when args are supplied but no placeholder exists they're appended under a `User arguments:` heading; dynamic shell (`!`cmd``) rejected.
- **Claude Code command frontmatter** (2026 docs): `description`, `argument-hint`, `allowed-tools` (comma/space-separated string, e.g. `allowed-tools: Bash(gh *), Read, Grep`), `model`, `disable-model-invocation`, `context: fork`. All scalar/inline — YAML-parseable.
- **Real OpenSpec opsx files** (captured by running `openspec init --tools claude` in a temp dir — this is exactly what `/opsx:explore` comes from): installs `.claude/commands/opsx/{explore,propose,apply,archive,sync}.md` + `.claude/skills/openspec-{explore,propose,apply-change,archive-change,sync-specs}/SKILL.md`. Frontmatter includes `name: "OPSX: Explore"`, `description`, `category`, and **`tags: [workflow, explore, experimental, thinking]` — a YAML flow array**.

**yaml.v3 sufficiency verdict: SUFFICIENT, no library change.** Real YAML (yaml.v3) parses everything the zcode flat parser accepts (single-line scalars are valid YAML) plus flow arrays that zcode's flat parser would drop. yaml.v3 ignores unknown keys by default (`category`/`tags` won't break anything). Known-field tolerance matches Claude Code.

**Code gaps to close (in `internal/ecosys/loader.go`, none needing new deps):**

1. **Recursive command discovery** — `discoverCommands` currently skips directories; must walk subdirectories and colon-join names. Without this, `/opsx:*` commands are structurally invisible. (THE blocker.)
2. **Extend the command frontmatter struct** beyond `description`: add `argument-hint`, `allowed-tools`, `model` (normalize `allowed-tools` both as comma-string and as YAML list — Claude Code writes strings, YAML files may write lists).
3. **Argument substitution semantics** for expansion at session/ACP layer: `$ARGUMENTS` / `$1` / `$2` replacement + `User arguments:` append fallback (zcode parity).
4. **Description fallback** to first non-empty body line; drop commands with neither (zcode parity).
5. **Lenience fallback:** a file that fails strict `yaml.Unmarshal` (e.g. tab-indented frontmatter) is currently silently skipped; zcode's flat parser would still read its single-line keys. Add a flat line-parser fallback on unmarshal error to match.

Integration point: session/ACP consumes `Registry.AllCommands()`; `/namespace:name` expansion replaces the invocation text with the command Body (post-substitution) before the provider turn — no changes to `internal/shaper`/`internal/loop` needed (the expanded prompt is ordinary user text at that layer).

#### 1b. OpenSpec v1.5.0 real CLI surface (captured from the installed binary — ground truth, not docs)

Top-level commands (`openspec --help`, v1.5.0): `init, update, list, view, change, archive, spec, config, schema, store, doctor, context, workset, validate, show, status, instructions, templates, schemas, new, feedback, completion, help`.

**Correction to the adapter model:** v1.5.0 keeps `show` and `validate` (both top-level and as `change`/`spec` subcommands). What does NOT exist: `apply` and `implement` as commands — the seeded.toml's `[commands.apply]`/`[commands.implement]` entries are phantom; "apply" is an **artifact instructions surface** (`openspec instructions apply`). The workflow is command-file-driven (`opsx` markdown + skills); the binary is supporting tooling.

Agent-relevant surface with mutability classification (for the rebuilt `[commands]` table):

| Invocation | Mutability | Key flags | Notes |
|------------|-----------|-----------|-------|
| `list [--specs\|--changes] [--sort recent\|name] [--json]` | read-only | `--json` | Changes by default |
| `show [item] [--type change\|spec] [--json] [--deltas-only] [--requirements] [--no-scenarios] [-r <id>]` | read-only | `--json` | Several flags JSON-mode-only |
| `view` | read-only, **human-only** | — | Interactive dashboard; adapter must NOT call |
| `validate [item] [--all\|--changes\|--specs\|--archived] [--strict] [--json] [--concurrency n]` | read-only | `--json` | |
| `status [--change <id>] [--schema <name>] [--json]` | read-only | `--json` | Artifact completion |
| `instructions [artifact] [--change <id>] [--schema <name>] [--json]` | read-only | `--json` | `apply`/`archive` artifact surfaces |
| `templates [--schema <name>] [--json]` / `schemas [--json]` | read-only | `--json` | |
| `context [--store <id>] [--json] [--code-workspace <path> [--force]]` | read-only (one optional write: workspace file) | `--json` | The agent brief |
| `doctor [--store <id>] [--json]` | read-only | `--json` | Health findings exit 0 |
| `new change <name> [--description] [--goal] [--schema] [--store] [--json]` | **mutating** | `--json` | kebab-case names |
| `archive [change] [-y\|--yes] [--skip-specs] [--no-validate] [--json]` | **mutating** | `--yes` required non-TTY | Merges deltas, moves to `changes/archive/YYYY-MM-DD-<name>/` |
| `init [path] [--tools <list>] [--force] [--profile]` / `update [path] [--force]` | **mutating** | | `--tools claude` installs the opsx command files |
| `config <path\|list\|get <key>\|set <key> <val>\|unset\|reset\|edit\|profile>` | set/unset/reset/edit **mutating**; rest read-only | | |
| `schema <which\|validate\|fork\|init>` | fork/init **mutating**; which/validate read-only | `[experimental]` | |
| `store <setup\|register\|unregister\|remove\|list\|doctor>` | setup/register/unregister/remove **mutating**; list/doctor read-only | `--json` | Stores beta |
| `workset <create\|list\|open\|remove>` | create/remove **mutating**; list read-only; `open` human-only | `--json` (not `open`) | Purely local |
| `change <show\|list\|validate>` | read-only | `--json` | `change list` **DEPRECATED** (use `list`) |
| `spec <show\|list\|validate>` | read-only | | |

Global: `-V/--version`, `--no-color`, `-h/--help`. Exit codes 0/1. `--json` exists on: list, show, validate, status, instructions, templates, schemas, doctor, context, archive, new change, all store subcommands, all workset subcommands except open.

**Adapter reconciliation plan:** rebuild `seeded.toml` `[commands]` around this table (read-only: list/show/validate/status/instructions/context/doctor/spec */change show|validate; mutating: new change/archive/init/update/config set*/schema fork|init/store setup|register|unregister|remove/workset create|remove); never invoke `view`/`workset open`/`config edit`/`feedback`. Pattern table (`[[patterns]]`/`[[handoff_tools]]`) stays as-is — it matches assistant text, not the binary.

### Feature 2 — Audit log on the `acp serve` path (LOG-01)

**Stack verdict: nothing to add.** `internal/audit`, the tracer, and the redactor exist and work (proven on the main.go path). The work is wiring the same tracer construction into the `acp serve` command path. No libraries, no externals, no schema changes.

### Feature 3 — zcode parity re-capture (operator-gated)

**Stack verdict: nothing to add.** The capture pipeline (`extract-profile`, `internal/parity`, `~/.zcode/cli/rollout/model-io-sess_<id>.jsonl` harvest) is built and proven; the pinned session is simply absent on disk. The operator exports `ZAI_API_KEY`, runs a divergence-prone zcode session, and the existing flow re-captures. No new tooling.

### Feature 4 — Telegram peer (text + voice STT)

**Library: `go-telegram/bot` v1.23.0 (the only new frontend dep).**

Shape that fits the constraints:

- **Same process, one goroutine** — `bot.New(token, opts)` + handler registration (`bot.RegisterHandler`/`ProcessUpdate` callbacks) + `b.Start(ctx)` in a goroutine beside the ACP stdio loop; both frontends share the core engine (`internal/engine`, `internal/loop`). `Start` blocks until ctx cancel → context-first drain of the long-poll loop, mirroring the ACP shutdown path.
- **stdout discipline** — library verified stdout-silent in Phase 0; its logging hooks route to slog→stderr. All Telegram diagnostics via slog (same as every other package).
- **No network port** — long polling only, NO webhook mode (webhooks would violate the no-daemon/no-port constraint; `WithWebhookURL` simply unused).
- **Voice flow** — `upd.Message.Voice.FileID` → `b.GetFile(ctx, &bot.GetFileParams{FileID})` → `b.FileDownloadLink(file)` → `http.Get` → OGG/Opus bytes → `Transcribe(ctx, bytes)` → text enters the turn loop as ordinary user input.

**STT backends (one interface, three configs):**

| Backend | Config target | Mechanism | Notes |
|---------|--------------|-----------|-------|
| **OpenAI (default)** | provider `base_url=https://api.openai.com/v1`, model `gpt-transcribe` (recommended current) or `gpt-4o-transcribe`/`whisper-1` | Existing go-openai client, `CreateTranscription` + `AudioRequest{Model, Reader, Language}` | 25 MB cap (Telegram voice ≈1–2 min ≪ cap). OGG/Opus accepted natively. |
| **Groq** | `base_url=https://api.groq.com/openai/v1`, model `whisper-large-v3` or `whisper-large-v3-turbo` | Same client, base-URL swap only | Fastest; ~$0.03/hr (large-v3). OpenAI-compatible `/audio/transcriptions` verified by Groq docs. |
| **whisper.cpp local** | `stt.backend=whisper-cpp-local`, `bin=/path/to/whisper-cli`, `model=/path/to/ggml-*.bin` | Subprocess: `ffmpeg -i in.oga -ar 16000 -ac 1 -c:a pcm_s16le -out wav` → `whisper-cli -m <model> -f <wav>` (plus output-format flags from `whisper-cli -h`; the repo README documents `-m/-f/-t/-ml/--vad*`), parse stdout/transcript file | Offline/privacy. whisper.cpp v1.9.2 (Oct 2025, latest). Models via `sh ./models/download-ggml-model.sh <name>` (tiny→large-v3, large-v3-turbo; HF `ggerganov/whisper.cpp`; tiny 273 MB → large 3.9 GB). Stock whisper-cli reads 16-bit WAV only → ffmpeg transcode step (both externals config-gated). |

**Failure posture:** STT errors degrade to a stderr-logged transcript note and a user-visible "voice transcription failed" message; they must never kill the process or the ACP frontend (investigate-and-fix-ready logging constraint).

### Feature 5 — Mimicry profile #2: deepseek-ai/deepseek-harness ("dsh")

**Stack verdict: one new Go dep (klauspost/compress/zstd v1.19.2) for log harvest; dev-time Node/pnpm on the capture machine only.** The profile rides the existing N-profile architecture (`internal/profile`, `internal/shaper`) — the OpenAI-shape provider already speaks dsh's wire format.

#### Repo facts (verified against github.com/deepseek-ai/deepseek-harness, `master`)

- MIT, **developer preview** ("compatibility-breaking changes expected"), ~92k stars. TypeScript **pnpm monorepo**: pnpm 11.7.0 (Corepack), Node 22.19+/24+, tsc project references + **tsdown** bundling, **vitest**, Lefthook, Oxlint. Two isolated aggregate programs (host/client) — `pnpm run typecheck` is the smoke test.
- Layout: `packages/<group>/<pkg>` with ~49 packages: `core/*` (agent-loop, agent, **system-prompt**, agent-default-model…), `llm/llm` (abstract service), **`llm/llm-deepseek`** (DeepSeek adapter), `llm/llm-pi-ai` (multi-provider), `session/*` (**session-persistence-jsonl**, -sqlite, projections, titles), `context/*` (agent-instructions, time-context, tmux-context), model-facing tool packages (`shell`, `terminal`, `fs`, `web`, `todo`, `subagent`, `jobs`, `workflow`, `lsp`, `skill`, `code-runtime`), plus `acp` (dsh has its own ACP server) and `hooks` (a "shared Claude Code / Codex wire-protocol library").

#### Where the profile's four ingredients live

1. **Request shape** — `packages/llm/llm-deepseek/src/{adapter,serialize}.ts`. Verified wire facts (quote-level): POST `{baseURL}/chat/completions`, Bearer auth, SSE. Body: `model`; `stream: true` **always** + `stream_options: {include_usage: true}`; messages as `{role, content}` — system via `flattenText`, assistant carries `tool_calls[{id, type:'function', function:{name, arguments}}]` and `reasoning_content` (tool-call turns only, never null content), tool results `{role:'tool', tool_call_id, content}` with literal `'(no output)'` for empty outputs; tools `{type:'function', function:{name, description, parameters}}` **omitted entirely when empty**; **`tool_choice` is NEVER serialized**; `temperature`/`max_tokens`/`stop` only when defined (omit-don't-null); DeepSeek-specific `thinking: {type:'enabled'|'disabled'}` and `reasoning_effort: 'high'|'max'` (never 'off' on the wire). Text-only (images rejected). Defaults: context window 1M, max_tokens 256k, stream idle timeout 300s. Identity/telemetry headers: `x-deepseek-harness-user-id` (+ session/compaction flags); reads back `x-request-id`/`x-deepseek-request-id`, `retry-after`. **This maps 1:1 onto the existing OpenAI-shape provider — profile #2 is config + shaper data, not a new provider.**
2. **System prompt** — `packages/core/system-prompt` is a **runtime-composed registry, not a static blob**: ordered sections (`-100` harness identity "You are an AI agent powered by DeepSeek Harness.", `0` deployment persona, plugin sections), `{{variable}}` interpolation, an expert-waterfall event, and a separate durable user-role context snapshot. **Consequence (load-bearing): static source extraction CANNOT produce the final prompt.** Ground truth must come from a live capture (below) or by running the harness's own assembly.
3. **Tool catalog** — tools are plugin-registered across the model-facing packages (shell, fs, web, todo, subagent, …). The authoritative per-session catalog is the `tools` array on real requests → harvest from capture, use source as cross-check (same method as the zcode 103-tool catalog).
4. **Logs** — `packages/session/session-persistence-jsonl`: root is **deployment-configured (no default home like `~/.dsh`)**; layout `<root>/<normalized-cwd>/<encoded-session-id>/session.jsonl.zstd` (default) or `session.jsonl` (compression:'none'). Format: first line immutable session header; then `SessionEvent` JSON per line, with runs of ≥3 assistant streaming chunks packed into `text-chunks`/`reasoning-chunks`/`tool-call-chunks` rows (seq/time reconstructed from `seq0/time0 + dt`). Compression: **concatenation of independent checksummed zstd frames** (one per append batch) — hence klauspost/compress. **Load-bearing caveat: dsh session JSONL stores SESSION EVENTS (assistant chunks, tool calls), NOT the raw outgoing HTTP request** — unlike zcode's model-io JSONL, it does not persist the assembled system prompt + tools array. The honest capture path is the adapter's own configurability: `baseURL` comes from connection config, so a recording proxy (MITM/echo) at the configured baseURL captures the exact serialized requests — the same pattern as the zcode MITM capture, and it satisfies "profile content is log-extracted, not hand-written."

#### Extraction toolchain (dev-time)

1. Clone dsh; `corepack enable && pnpm install && pnpm run typecheck` (Node 22.19+/24, pnpm 11.7.0 pinned).
2. Configure a capture run: session root with `compression: 'none'` if harvesting events by line, and/or a baseURL-pointed recording proxy for the wire truth; `DEEPSEEK_API_KEY` from the operator's env.
3. Go side: harvest tool reuses the `extract-profile` pattern + zstd frame decoding (klauspost) for compressed roots; emits the profile bundle (system blocks, tool catalog, message shape, identity headers, defaults) into `profiles/`.
4. Verify with `ass-guard profile check` (existing drift detector) and the Phase-1 A/B method adapted to dsh sessions.

**Watch item:** dsh is developer-preview with announced breaking changes — pin the profile to a captured commit hash and record it in the profile manifest (drift detector re-capture is cheap once the toolchain exists).

---

## Alternatives Considered

| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|-------------------------|
| `go-telegram/bot` v1.23.0 | `PaulSonOfLars/gotgbot/v2` | If a dispatcher/updater shape is preferred; loses nothing critical. Active 2026 but go-telegram/bot's context-first API matches the ACP-owned lifecycle better (v1.0 decision unchanged; now version-pinned). |
| OpenAI/Groq STT via existing go-openai client | Dedicated STT SDKs | Never — the client already implements the multipart transcription endpoint; a second HTTP stack is pure surface area. |
| whisper.cpp as subprocess (+ffmpeg transcode) | cgo binding `ggml-org/whisper.cpp/bindings/go` | Never for ass-guard — breaks CGO_ENABLED=0 static cross-compile (constraint, unchanged from v1.0). |
| klauspost/compress/zstd | Shell out to system `zstd` binary | Only if the spike against real dsh files hits a decoder edge — unlikely (concatenated frames + XXH64 are the default decode path). Shelling out adds a runtime external; avoid. |
| yaml.v3 for frontmatter | `adrg/frontmatter` or goldmark-meta | Never — yet another parser dep for a format yaml.v3 already covers; the gaps are field coverage + recursion in our code, not parse capability. |
| Flat-parser fallback (on YAML error) | Strict YAML only | Strict-only silently drops commands zcode would load — the fallback is parity, not gold-plating. |
| Session-event harvest + baseURL recording proxy for dsh | Source-only profile | Source-only misses the composed system prompt (runtime-registry assembly) — violates the log-extracted-not-hand-written decision. |

## What NOT to Use

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| Telegram webhook mode | Opens a network port — violates no-daemon/no-port (editor owns lifecycle). | Long-poll `bot.Start(ctx)`. |
| Any cgo / audio-processing Go dependency | Breaks goreleaser CGO_ENABLED=0 macOS+Linux amd64+arm64 cross-compile. | Subprocess `whisper-cli` + `ffmpeg` (config-gated externals). |
| `openspec view` / `workset open` / `config edit` from the adapter | Interactive/human-only surfaces; hang a headless subprocess. | The `--json` command set (table in Feature 1). |
| Seeded `[commands]` entries `apply`/`implement` | Not real v1.5.0 commands (phantom from an older mental model). | `instructions apply` artifact surface / `new change`+`archive` lifecycle. |
| Replacing yaml.v3 with go.yaml.in/yaml/v4 now | v4 is rc; yaml.v3 is sufficient and already load-bearing across config parsing. | Keep yaml.v3; note v4 as the successor if/when GA. |
| Assuming dsh session JSONL contains model-io | It stores session events, not outgoing requests — a source-only or logs-only profile would be a guess. | baseURL recording-proxy capture + session-log harvest. |
| Hand-written dsh system prompt from README prose | The prompt is runtime-composed (sections, interpolation, persona) — prose ≠ wire truth. | Capture the assembled request. |

## Stack Patterns by Variant

**If `stt.backend = openai` (default):** voice bytes → `internal/provider` OpenAI client → `CreateTranscription`; model slug `gpt-transcribe` (or `whisper-1`) from provider config; credentials via existing flag>env>config chain. No ffmpeg, no externals.

**If `stt.backend = groq`:** identical code path; provider entry `base_url=https://api.groq.com/openai/v1`, model `whisper-large-v3-turbo`. Trade latency for a second vendor.

**If `stt.backend = whisper-cpp-local`:** config carries `bin`, `model`, optional `ffmpeg` path; pipeline = ffmpeg transcode → whisper-cli → parse. Fully offline; two optional externals; degrade gracefully when missing (typed structural error, stderr-visible).

**If a Telegram text message arrives:** handler → core engine turn — identical path to an ACP `session/prompt`, sharing scheduler, shaper, engine, learning.

**If `/namespace:name args...` arrives (ACP or Telegram):** session layer looks up `Registry.Commands[namespace:name]` (recursively discovered, colon-joined), substitutes `$ARGUMENTS`/`$1`/`$2` (or appends `User arguments:`), injects the command body as the user prompt for the turn.

**If dsh profile is active:** provider factory selects the OpenAI shape; shaper emits dsh wire specifics (always-stream + include_usage, omit-when-empty tools, no tool_choice, thinking/reasoning_effort mapping, `'(no output)'` tool-result substitution, identity headers); scheduler maps DeepSeek-model tiers to the dsh-profiled turns.

## Version Compatibility

| Package A | Compatible With | Notes |
|-----------|-----------------|-------|
| `go-telegram/bot` v1.23.0 | Bot API 10.2 (Jul 14, 2026) | Server-side Bot API is backward-compatible; pin v1.23.0. Marshal semantics tightened in v1.23.0 (error-not-panic; no caller mutation) — additive for our usage. |
| `klauspost/compress` v1.19.2 | Go 1.26 (go.mod) | Pure Go; avoid v1.18.1 (retracted) and early v1.19.x arm64 issue. Multistream concatenated frames + XXH64 checksum verification by default — matches dsh's per-batch frame layout. |
| `sashabaranov/go-openai` v1.42.0 (existing) | OpenAI + Groq transcription endpoints | `AudioRequest.Reader` for in-memory bytes; `AudioResponse.Text` for plain text. gpt-transcribe family: use plain text/json formats only (not verbose_json). |
| `gopkg.in/yaml.v3` v3.0.1 (existing) | zcode flat frontmatter ⊂ YAML; Claude Code command keys; opsx flow arrays | Unknown-key tolerance default; add flat-parser fallback for tab-malformed files. |
| `openspec` v1.5.0 binary | `internal/openspec` Adapter (subprocess) | Real-binary gate `ASSGUARD_OPENSPEC_BIN=1` runs against this exact surface; `--json` where offered; `archive` needs `--yes` non-TTY. |
| whisper.cpp v1.9.2 + ffmpeg (optional externals) | macOS/Linux amd64+arm64 | Both are plain subprocesses — no interaction with the static build. |
| dsh capture | pinned commit hash | Developer-preview upstream; record the hash in the profile manifest; re-capture on drift. |

## Sources

- https://github.com/go-telegram/bot + /releases — v1.23.0 (Aug 3, 2026), Bot API 10.2, release notes (marshal fixes); Go module proxy confirms v1.23.0 latest
- Context7 `/go-telegram/bot` — `GetFile`/`FileDownloadLink`/`Start(ctx)` usage (autodocs from repo)
- pkg.go.dev `github.com/klauspost/compress/zstd` + GitHub releases — v1.19.x current, multistream/checksum decode semantics; Go module proxy confirms v1.19.2 latest
- `go doc` against pinned `sashabaranov/go-openai` v1.42.0 — `AudioRequest`/`CreateTranscription`/`AudioResponse` signatures (ground truth)
- https://developers.openai.com/api/docs/guides/speech-to-text + API reference — `gpt-transcribe` recommended, `whisper-1` legacy-supported, 25 MB cap
- https://console.groq.com/docs/speech-to-text + /docs/model/whisper-large-v3-turbo — OpenAI-compatible `/openai/v1/audio/transcriptions`, model slugs, pricing
- https://github.com/ggml-org/whisper.cpp + releases — v1.9.2 (Oct 15, 2025), whisper-cli flags, `models/download-ggml-model.sh`, 16-bit WAV requirement, ffmpeg convert recipe
- https://github.com/deepseek-ai/deepseek-harness — repo layout (pnpm/tsdown/vitest, packages tree via GitHub API); README (Everything-is-a-Plugin, Cordis, developer preview)
- dsh sources (raw.githubusercontent.com, master): `docs/development.md` (toolchain), `packages/llm/llm-deepseek/src/adapter.ts` + `serialize.ts` (wire shape, headers, defaults), `packages/core/system-prompt/src/index.ts` (composed prompt), `packages/session/session-persistence-jsonl/README.md` (root config, layout, zstd framing, packed chunk rows), `docs/cookbook/adding-an-llm-adapter.md`
- Local zcode-guide skill (`diagnosing-commands/SKILL.md`) — zcode command discovery/name rules/flat frontmatter/substitution semantics (primary: shipped by the mimicry target)
- https://code.claude.com/docs/en/skills + community guides — Claude Code command frontmatter keys (`description`, `argument-hint`, `allowed-tools`, `model`, `disable-model-invocation`, `context`)
- **Installed `openspec` v1.5.0 binary** (`/usr/local/bin/openspec`) — full `--help` surface captured per command (ground truth); `openspec init --tools claude` run in a temp dir to enumerate installed `.claude/commands/opsx/*.md` + skills; https://github.com/Fission-AI/OpenSpec/releases (v1.5.0 "Stores Beta", Jun 28) and docs/cli.md for cross-checking

---
*Stack research for: ass-guard v1.1 — slash-command kickoff, audit log, parity re-capture, Telegram peer + STT, deepseek-harness profile #2*
*Researched: 2026-08-14*
