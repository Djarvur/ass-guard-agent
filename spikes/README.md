# ass-guard spikes (THROWAWAY)

> **THROWAWAY. These spikes are NOT the ass-guard codebase. Do NOT import from them in Phase 1.**
>
> Per CONTEXT.md decision **D-04** (Phase 0 produces throwaway spike code to prove
> empirical claims) and **D-05** (the `spikes/` directory has its own isolated
> `go.mod` as a distinct module `github.com/djarvur/ass-guard-spikes`; the repo root
> stays clean — no root `go.mod` until Phase 1). These programs exist only to produce
> dated evidence for `.planning/research/VERIFIED-FACTS.md` (Plan 00-05 is the single
> writer of that file). They are disposable. Phase 1 starts the real module at the
> repo root with deliberate dependency decisions uncoupled from whatever the spikes
> happened to pin here.

## Module layout

```
spikes/
├── README.md                  # this file — the canonical entrypoint doc
├── go.mod                     # module github.com/djarvur/ass-guard-spikes (isolated, D-05)
├── go.sum                     # GITIGNORED (D-05)
├── 01-jsonl-capture/          # item #1 — NOT a Go program; filesystem capture + notes
│   ├── README.md              # documents the corrected path + per-line schema discovery
│   └── FINDING.md             # D-02-shaped draft evidence for items #1 and #4
├── 02-openai-toolschema/      # item #2 — main.go: tool-call round-trip vs MiniMax M3 + Groq
│   └── main.go
├── 03-acp-handshake/          # item #3 — main.go: initialize → session/new → session/prompt + observe session/update
│   └── main.go
└── 05-stdout-collision/       # item #5 — main.go: go-telegram/bot goroutine + canned ACP frames on stdout; assert clean
    └── main.go
```

Each `0N-*/` subdirectory is its own `package main` (Go supports multiple `main`
packages in one module). Item #1 (`01-jsonl-capture/`) is a **filesystem capture** —
no `main.go`; `jq` / `python3 -m json.tool` suffice. Item #4 has **no spike
directory** (structurally moot per D-06 — see below).

## The five spikes and what each proves

| Spike | STACK item | What it does | What it asserts | Evidence written to |
|-------|-----------|--------------|-----------------|---------------------|
| **01-jsonl-capture** (no Go) | #1 | Locates `~/.zcode/cli/rollout/model-io-sess_*.jsonl`, extracts one `model_io` line, redacts per D-03 | Path exists; schema has `request.body.{system,tools}`, `request.headers`, `response.toolCalls` | `01-jsonl-capture/FINDING.md` |
| **02-openai-toolschema** | #2 | Tool-call round-trip vs MiniMax M3 + Groq via `go-openai` | (a) `tools[]` serializes to OpenAI Chat Completions shape; (b) response `tool_calls[].function.arguments` is a JSON string; (c) follow-up `tool`-role turn is consumed | `02-openai-toolschema/RESULT.md` |
| **03-acp-handshake** | #3 | `initialize` → `session/new` → `session/prompt`; read `session/update` frames | (a) `initialize` returns `protocolVersion` + `agentCapabilities`; (b) `session/new` returns a `sessionId`; (c) ≥1 `session/update` frame before the prompt response | `03-acp-handshake/RESULT.md` |
| **05-stdout-collision** | #5 | `go-telegram/bot` goroutine + canned ACP frames on stdout; ctx cancel drains bot | (a) stdout byte-equals the canned ACP frames (zero extra bytes from the bot); (b) bot goroutine exits within ~2s of ctx cancel | `05-stdout-collision/RESULT.md` |
| *(#4 has no spike)* | #4 | — | STT is always external → zero cgo → goreleaser matrix unaffected (D-06) | `01-jsonl-capture/FINDING.md` §#4 |

## Environment variables each spike needs

Only the spikes that talk to the network need secrets. The filesystem capture
(#1) and the structural closure (#4) need nothing.

| Spike | Env vars |
|-------|----------|
| **01-jsonl-capture** | none (reads `~/.zcode/cli/rollout/*.jsonl` on disk) |
| **02-openai-toolschema** | `OPENAI_API_KEY` (or provider-specific: `MINIMAX_API_KEY`, `GROQ_API_KEY`); `OPENAI_BASE_URL` (swap per provider); `OPENAI_MODEL` (the model slug) |
| **03-acp-handshake** | `ACP_PEER_CMD` + `ACP_PEER_ARGS` (the ACP server to spawn and handshake with) |
| **05-stdout-collision** | `TELEGRAM_BOT_TOKEN` (a throwaway bot token; the spike may use a dummy token if it only exercises the goroutine lifecycle, not a real Telegram connection) |

## How to run a spike

```bash
# from the spikes/ directory (the isolated module)
cd spikes/02-openai-toolschema && go run .
cd spikes/03-acp-handshake      && go run .
cd spikes/05-stdout-collision   && go run .

# 01-jsonl-capture has no Go program — see its README.md for the jq/python capture
```

`go.sum` is gitignored (D-05); the first `go run` resolves and caches the deps. The
pinned versions (from `go.mod`) are the verified-as-of-2026-08-09 tags: `go-telegram/bot v1.23.0`, `sashabaranov/go-openai v1.42.0`. Never `@latest` (D-02's `Verified against` field demands a concrete version).

## Transport discipline (load-bearing)

**stdout is reserved exclusively for ACP JSON-RPC frames; all logging and diagnostics
go to stderr** (PROJECT.md "Transport discipline"; AGENTS.md "Constraints"). This is
the non-negotiable LSP-style discipline the real codebase enforces.

Every spike models this discipline from day one:

- **ALL diagnostic output → stderr** (`log.Printf`, `fmt.Fprintln(os.Stderr, ...)`,
  `slog` with a stderr handler). Never `fmt.Println` to stdout.
- **stdout** is reserved for the thing being tested: ACP frames in #3 and #5; nothing
  in #1 (no Go program) and #2 (the OpenAI client talks to the network, not stdout).
- Spike **#5's entire purpose** is to prove this discipline holds when `go-telegram/bot`
  and ACP coexist in one process: the bot's `WithErrorsHandler` routes to `slog`→stderr,
  the bot's `WithDebugHandler` is either unset or routed to stderr, and the assertion is
  that stdout is byte-equal to the canned ACP frames with zero extraneous bytes.

## Per-spike evidence files

Each spike subdirectory writes its own evidence file (`FINDING.md` for #1, `RESULT.md`
for #2/#3/#5). **Plan 00-05 (Wave 2/3) reads all four evidence files and folds them into
`.planning/research/VERIFIED-FACTS.md` in a single integration pass.** This
per-spike-file convention is what lets Plans 01–04 run in parallel with zero
`files_modified` overlap — no shared file is written by more than one Wave-1 plan.

Evidence files are committed (D-05: source + findings are committed; only `go.sum` and
built binaries are gitignored). All embedded samples are sanitized per **D-03** before
commit: prompt/system text, ids, token counts, tool arguments, and file paths are
redacted; JSON keys, HTTP header **names**, enum values, and array lengths are preserved
(the header-name set and structural shape ARE the mimicry target and must survive
redaction).

## Item #4 (whisper.cpp / STT cross-compile) — structurally moot, no spike

Per **D-06**, STT is **always** an external utility — ass-guard NEVER bundles STT via
cgo. This holds for every backend: OpenAI Whisper API = HTTPS call (zero cgo);
whisper.cpp local = out-of-process `whisper-cli` subprocess the user installs (never the
cgo binding); Groq STT = HTTPS call. Therefore ass-guard's goreleaser matrix (pure Go,
`CGO_ENABLED=0`, macOS+Linux amd64+arm64) is unaffected by STT choice, and STACK item
#4's premise (cgo cross-compile risk) does not arise. **Status: `STRUCTURALLY-MOOT`.**
The closure reasoning is written to `01-jsonl-capture/FINDING.md` §#4 (it shares that
file with the #1 finding because #1 is also a no-Go deliverable). No spike is produced;
the rule propagates to v2 STT work and is not relitigated. See
`00-RESEARCH.md` §6 and `00-CONTEXT.md` D-06.
