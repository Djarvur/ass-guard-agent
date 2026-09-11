# Phase 21: Context & Policy Parity Closures - Pattern Map

**Mapped:** 2026-08-28
**Files analyzed:** 16 (8 modified, 4 new, 4 testdata/fixture groups)
**Analogs found:** 14 / 16 (2 genuinely new: the 17-locked gate join and the pure-Go image scaler)

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `internal/ecosys/hooks.go` (MOD) | middleware (policy) | request-response | itself — `parseHooksJSON`/`Fire`/`runOne` extended in place | exact (in-place) |
| `internal/ecosys/loader.go` (MOD) | config loader | file-I/O (batch) | `loadAll` tier merge (loader.go:66–114) + `readCapped`/`logPluginSkipf` (loader.go:580–596, 801–803) | exact |
| `internal/ecosys/memory.go` (NEW) | service (discovery) | file-I/O (walk) | `internal/ecosys/skills.go` listing shape + `loader.go` `loadTree` walk discipline | role-match |
| `internal/runtime/runtime.go` `sessionFor` (MOD) | service (context assembly) | transform | the skills/agent trailing-TextBlock merges at runtime.go:1046–1075 | exact (the vehicle) |
| `internal/provider/provider.go` (MOD) | model (wire types) | streaming | `StreamChunk` struct itself (provider.go:46–58) | exact (additive) |
| `internal/provider/streaming.go` (MOD) | streaming parser | streaming (SSE) | `drainSSE` tool-use lifecycle state machine (streaming.go:265–446) | exact |
| `internal/session/session.go` `streamAndEmit` (MOD) | controller (turn loop) | event-driven | the chunk-type switch itself (session.go:663–701) | exact (new case) |
| `internal/event/events.go` (MOD) | model (bus event) | pub-sub | `AgentMessageChunk` (events.go:46–53) | exact |
| `internal/runtime/runtime.go` `routeBusEvent` (MOD) | forwarder | pub-sub | `routeBusEvent` itself (runtime.go:651–660) | exact (new case) |
| `internal/session/projector.go` (MOD) | service (window builder) | transform | `accumulateMidTurn` + `splitAtResetBoundary` (projector.go:123–227) | exact |
| `internal/shaper/shaper.go` (MOD) | service (request shaping) | transform | `toMessageParams` assistant-batch rendering (shaper.go:165–226) | exact |
| `internal/session/transcript.go` (MOD) | model (storage schema) | CRUD (append-only) | `ContentBlock` (transcript.go:81–84) + Phase-16 additive-kind discipline (transcript.go:144–155) | exact |
| `internal/runtime/runtime.go` `expandUserBlocks` (MOD) | service (ingress) | transform | itself (runtime.go:395–449) + `ecosys.ParseInvocation` split (expand.go:24–33) | exact (the seam) |
| image scaler (NEW, likely `internal/runtime` or `internal/shaper`) | utility | transform | NO analog — new algorithm; `x/image/draw` per RESEARCH Pattern 5 | none |
| `internal/session/gate.go` hook-verdict head (NEW, POST-17) | middleware (policy gate) | request-response | NO analog in tree — 17-locked contract (`17-02-PLAN.md`); plans precondition-marked | none (contract-locked) |
| `internal/coreexec/register.go` `withHooks` (MOD, TRACER) | middleware seam | request-response | itself (register.go:72–88) | exact (leg disposal) |

---

## Pattern Assignments

### `internal/ecosys/hooks.go` — settings-scope hooks + JSON verdicts (PAR-03, Plans 1)

**Analog:** the file itself — extend in place; `parseHooksJSON` is the shape the settings parser must accept, `runOne`/`classifyHookRun` are the bounds already decided.

**The hooks-file shape the settings.json parser reuses** (lines 119–182 — same `hooks → event → matcher-group → {type,command,timeout}` shape per RESEARCH Pattern 1):
```go
func parseHooksJSON(path, pluginRoot string) []HookConfig {
	data, ok := readCapped(path, pluginArtifactMaxBytes)
	if !ok {
		return nil
	}

	var raw struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
				Timeout int    `json:"timeout"`
			} `json:"hooks"`
		} `json:"hooks"`
	}

	if json.Unmarshal(data, &raw) != nil || len(raw.Hooks) == 0 {
		logPluginSkipf("malformed or empty hooks file %s (skipped)", path)

		return nil
	}
	// ... deterministic sorted event order; per-entry skip-with-warning ...
```

**Malformed-config degradation discipline (D-02/Pitfall 8 must copy this verbatim)** (lines 141–146 + loader.go:580–596, 801–803):
```go
const pluginArtifactMaxBytes = 1 << 20

func readCapped(path string, maxBytes int64) ([]byte, bool) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() > maxBytes {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return data, true
}

func logPluginSkipf(format string, args ...any) {
	shadowWarnLogger.Warn("plugin skip: " + fmt.Sprintf(format, args...))
}
```
A malformed settings.json NEVER returns an error up through `Load`/`sessionFor` — warn to stderr + skip.

**Runner bounds (extend, do not re-decide)** (lines 91–97):
```go
const (
	hookDefaultTimeoutSec = 60
	hookOutputCap         = 30000
	hookRefusalExitCode   = 2
)
```

**The Fire loop the verdict resolver wraps** (lines 190–228): iterate `matchingHooks(event, toolName)` in slice order; per-hook `runOne`; refusal flips `Proceed`. PAR-03 keeps this shape but the per-hook outcome becomes four-valued {deny, allow, ask, NO-DECISION} (Pitfall 4: exit-0-silent is NO-DECISION, never allow) and a separate pure `resolveVerdict([]scopedResult)` applies D-03 deny-wins. **Keep the resolver a standalone pure function** so the 17-join is a one-line delegation.

**Legacy exit-2 classification (D-02's second channel — already exact)** (lines 353–377):
```go
func classifyHookRun(event string, runErr error, stdout, stderr string) hookExecResult {
	if runErr == nil {
		return hookExecResult{stdout: capHookOutput(stdout)}
	}
	var exitErr *exec.ExitError
	if !errors.As(runErr, &exitErr) { ...skip... }
	if exitErr.ExitCode() == hookRefusalExitCode {
		msg := capHookOutput(stderr)
		if msg == "" { msg = capHookOutput(stdout) }
		return hookExecResult{refused: true, message: msg}
	}
	...
}
```
Note the CC rule to mirror: exit 2 blocks regardless of any JSON verdict on stdout.

**Sanitized env + stdin payload (unchanged contract)** (lines 382–414): payload = `{session_id, transcript_path, cwd, hook_event_name}` + fields (`tool_name`, `tool_input`); env = `PATH`, `HOME`, `CLAUDE_PLUGIN_ROOT` only.

**Matcher dialect (Pitfall 3)**: current code compiles every matcher as a Go regex (lines 276–284). Settings-scope hooks get CC's two-path dialect (exact-string/alternatives when chars ∈ `[alnum/_/-/space/,/|]`, else unanchored regex); plugin hooks keep legacy regex behavior ("existing plugin hooks unchanged", D-02). Document the divergence in the doc comment.

---

### `internal/ecosys/loader.go` — scope loading order (PAR-03)

**Analog:** `loadAll`'s tier merge (lines 66–114). D-03's firing order (project settings → user settings → plugin) is resolved at runner construction, NOT by reordering this merge (Pitfall 1 — other consumers depend on overlay precedence):
```go
merged := mergeRegistries(mergeRegistries(userPlug, projPlug), core)
hooks := make([]HookConfig, 0, len(pluginHooks)+len(merged.Hooks))
hooks = append(hooks, pluginHooks...)
merged.Hooks = append(hooks, merged.Hooks...)
```
The flat slice preserves MERGE order; tag each new settings hook with `Scope` at parse time and partition/sort by scope when the runner is built.

---

### `internal/ecosys/memory.go` (NEW) — memory walker + mtime cache (PAR-04, Plan 2)

**Analog 1 — rendering shape:** `internal/ecosys/skills.go:30–57` — header + blank line + one entry per line, empty → `""` (caller skips the merge):
```go
func SkillListing(reg Registry) string {
	skills := reg.AllSkills()
	if len(skills) == 0 {
		return ""
	}
	entries := make([]string, 0, len(skills))
	for _, s := range skills {
		entries = append(entries, "- "+s.Name+": "+truncateSkillDesc(s.Description)+" (file: "+s.Path+")")
	}
	return skillListingHeader + "\n\n" + strings.Join(entries, "\n")
}
```
The memory injection block copies this render-then-skip-on-empty contract plus the truncation idiom (`truncateSkillDesc`, skills.go:61–68 — rune-safe hard cut + ellipsis; D-08's per-file cap applies the same shape at 16–32 KB).

**Analog 2 — walk discipline:** `loader.go` `loadTree` (lines 141–178) — every read via `os.ReadFile` (read-only), missing root = empty contribution (non-fatal), malformed entries skip with warning. The memory walker adds: cwd → git root (stop at `.git`), per level `CLAUDE.md` wins over `AGENTS.md` (D-05, skipped-with-note), then user globals with `~/.ass-guard` precedence over `~/.claude/CLAUDE.md` (D-07). Reuse `readCapped` for per-file reads. The mtime cache is a package-level `map[path]{mtime,size,content}` — a pure optimization; miss re-reads.

---

### `internal/runtime/runtime.go` `sessionFor` — the fourth trailing-TextBlock merge (PAR-04)

**Analog:** the file itself, lines 1046–1075 — the exact vehicle, copy the double-append copy discipline verbatim:
```go
// The System slice is COPIED first (a struct copy alone would share the
// backing array — the shared r.profile must never be mutated ...)
prof.System = append([]profile.TextBlock(nil), prof.System...)
shaper.ComposeRuntimeWorkDir(&prof, dir)

// ... merge the skills listing into the profile COPY ... Empty registry → no merge.
if listing := ecosys.SkillListing(r.reg); listing != "" {
	prof.System = append(append([]profile.TextBlock(nil), prof.System...),
		profile.TextBlock{Type: blockText, Text: listing})
}
// ... same shape for AgentListing at 1072-1075 ...
```
The memory merge is a fourth block of exactly this shape at this exact site (after AgentListing): `ecosys.MemoryInjection(...)` returns `""` on no-files → skip; non-empty → one trailing `profile.TextBlock` with framing + per-level bodies + truncation notes (D-08).

---

### `internal/provider/streaming.go` — thinking in the SSE state machine (PAR-05, Plan 3, hop 1)

**Analog:** the tool-use lifecycle tracker in `drainSSE` (lines 275–286 state, 348–382 transitions, 405–446 flush):
```go
// Tool-use lifecycle state: content_block_start → content_block_delta
// (input_json_delta fragments) → content_block_stop. We accumulate the
// input JSON across deltas and emit the complete chunk on block stop.
var (
	tuName, tuID string
	tuInput      strings.Builder
	inToolUse    bool
	emittedTools = map[string]struct{}{}
)
```
```go
case "content_block_start":
	cb, _ := ev["content_block"].(map[string]any)
	if cb != nil {
		if t, _ := cb[keyType].(string); t == blockToolUse {
			flushToolUse(...)
			tuName, _ = cb["name"].(string)
			tuID, _ = cb["id"].(string)
			tuInput.Reset()
			inToolUse = true
			continue // don't emit yet — wait for deltas
		}
	}
case "content_block_delta":
	delta, _ := ev["delta"].(map[string]any)
	if delta != nil && inToolUse {
		if dt, _ := delta[keyType].(string); dt == "input_json_delta" { ... }
	}
case "content_block_stop":
	if inToolUse { flushToolUse(...); continue }
```
The thinking accumulator is a SIBLING state: `content_block_start` with `content_block.type == "thinking"` opens it; `thinking_delta` (`delta.thinking`) and `signature_delta` (`delta.signature`) accumulate; `content_block_stop` emits `StreamChunk{Type: "thinking", Raw: <assembled block JSON>}`. `redacted_thinking` arrives complete in `content_block_start` (`data` field) — emit immediately. The assembled JSON must be built so the block's bytes equal the provider's field values (D-12: assemble from the raw delta strings, never through a typed round-trip). Flush thinking on EOF/error/[DONE] exactly as `flushToolUse` does (lines 300–330 call sites).

### `internal/provider/provider.go` — StreamChunk grows a thinking type (hop 2)

**Analog:** the struct itself (lines 38–58) — additive discriminated union; `Raw json.RawMessage` ALREADY exists on the struct (the done chunk uses it):
```go
type StreamChunk struct {
	Type         string
	Text         string
	ToolCall     *ToolCall
	ToolCallID   string
	Usage        *Usage
	FinishReason string
	Raw          json.RawMessage
	Error        error
}
```
Add `Type: "thinking"` (string const per A6/goconst convention, sibling of `blockText`/`blockToolUse`) + a `Raw`-carrying payload. No existing case changes.

### `internal/session/session.go` `streamAndEmit` — the thinking case (hop 3)

**Analog:** the switch itself (lines 663–701) — the `blockText` case is the mirror to copy:
```go
switch chunk.Type {
case blockText:
	sb.WriteString(chunk.Text)
	if s.Bus != nil && chunk.Text != "" {
		s.Bus.Publish(event.AgentMessageChunk{
			TurnID: turnID, MessageID: turnID, Content: chunk.Text,
		})
	}
case blockToolUse:
	...
case "usage":
	...
case stopDone:
	...
case chunkErrorType:
	...
// PAR-05 adds: case "thinking" → s.Manager.AppendRawThinking(turnID, model, chunk.Raw)
//             + s.Bus.Publish(event.AgentThoughtChunk{TurnID, MessageID, Content-block})
}
```

**Transcript append (endpoint ALREADY BUILT — call it, do not re-implement):** `manager.go:344–354`:
```go
func (m *Manager) AppendRawThinking(turnID, model string, payload json.RawMessage) error {
	return m.appendLineUnredacted(&Line{
		Type: TypeRawThinking, TurnID: turnID, Timestamp: now(),
		Model: model, Content: payload,
	})
}
```
`appendLineUnredacted` (manager.go:106–134) is the redactor-exempt path — the doc comment FORBIDS extracting a shared marshal helper; Phase 21 must not touch it.

### `internal/event/events.go` — AgentThoughtChunk (hop 3's event)

**Analog:** `AgentMessageChunk` (lines 43–53):
```go
type AgentMessageChunk struct {
	TurnID    string
	MessageID string
	Content   string
}

// Kind returns the event discriminator.
func (AgentMessageChunk) Kind() string { return "AgentMessageChunk" }
```
`AgentThoughtChunk` mirrors this (content likely `json.RawMessage` or the fragment text) + a `BufAgentThoughtChunk` buffer const beside `BufAgentMessageChunk = 128` (events.go:12). Wire the kind into both Subscribe sites (the runtime forwarder channels) per the `startChunkForwarder` channel pattern (runtime.go:591–645).

### `internal/runtime/runtime.go` `routeBusEvent` — forward the thought chunk (hop 4)

**Analog:** the switch itself (lines 651–660):
```go
func routeBusEvent(e event.Event, emit acp.ChunkEmitter, toolEmit acp.ActivityEmitter) {
	switch c := e.(type) {
	case event.AgentMessageChunk:
		_ = emit.AgentMessageChunk(c.MessageID, c.Content)
	case event.ToolCall:
		forwardToolCall(toolEmit, e)
	case event.ToolCallUpdate:
		forwardToolCallUpdate(toolEmit, e)
	}
}
```
New case → `emit.ThoughtChunk(c.MessageID, content)` via the `ActivityEmitter` interface (emitter.go:417–424 — `ThoughtChunk(messageID string, content ContentBlock) error` is already in the interface; plain `ChunkEmitter` fakes skip it via the existing type-assert idiom at runtime.go:599).

**Emitter endpoint (NO CHANGE NEEDED — exists):** `emitter.go:516–525`:
```go
func (h *EmitterHandle) ThoughtChunk(messageID string, content ContentBlock) error {
	return h.enqueueUpdate(map[string]any{
		keySessionUpdate: updKindThoughtChunk,
		"messageId":      messageID,
		"content":        content,
	})
}
```

### `internal/session/projector.go` — raw_thinking into the mid-turn window (hop 5a)

**Analog:** `accumulateMidTurn` (lines 170–227) — the turn-scoped fold with batch flush; raw_thinking lines join the ASSISTANT accumulation keyed by turnID (Pitfall 5: thinking must live and die with its assistant message, never cut separately):
```go
for i := anchor; i < len(lines); i++ {
	l := &lines[i]
	if l.TurnID != turnID {
		continue // only the CURRENT turn's lines fold
	}
	switch l.Type {
	case TypeToolCall:
		pending = append(pending, provider.ToolCall{ID: l.ToolCallID, Name: l.Name, Input: l.Input})
		names[l.ToolCallID] = l.Name
	case TypeToolResult:
		flushBatch()
		...
	case TypeAssistantMessage:
		flushBatch()
		out = append(out, provider.Message{Role: roleAssistant, Content: l.Text})
	}
}
```
Add `case TypeRawThinking:` → stash the block onto the in-progress assistant accumulation (pending-thinking list flushed INTO the assistant message at `flushBatch`); a thinking line whose assistant batch never forms must be dropped WITH its turn unit (never think-without-its-turn). `splitAtResetBoundary` (lines 123–157) stays untouched — boundary cuts take the whole turn unit by construction. Field extraction from the RawMessage happens ONLY here (values pass through untouched to the shaper).

### `internal/shaper/shaper.go` — thinking + image blocks into SDK params (hop 5b / PAR-06)

**Analog:** `toMessageParams`' assistant-batch branch (lines 196–218) — the place content-block unions are assembled:
```go
blocks := make([]anthropic.ContentBlockParamUnion, 0, 1+len(m.ToolCalls))
if len(m.ToolCalls) == 0 {
	blocks = append(blocks, anthropic.NewTextBlock(m.Content))
} else {
	if m.Content != "" {
		blocks = append(blocks, anthropic.NewTextBlock(m.Content))
	}
	for _, tc := range m.ToolCalls {
		input, perr := parseToolCallInput(tc.Input)
		if perr != nil { return nil, fmt.Errorf("tool %q input: %w", tc.Name, perr) }
		blocks = append(blocks, anthropic.NewToolUseBlock(tc.ID, input, tc.Name))
	}
}
```
Thinking blocks ride here as first-class blocks: `anthropic.ThinkingBlockParam{Signature, Thinking}` / `anthropic.RedactedThinkingBlockParam{Data}` (SDK v1.63.0 message.go:6509–6521, 5192–5200 — quoted in RESEARCH Pattern 4). MUST handle BOTH types (filtering `type=="thinking"` only drops redacted blocks → provider 400). Images map via `anthropic.ImageBlockParam` + `Base64ImageSourceParam` (message.go:3319–3345, 120–124). The `Message` struct (shaper.go:35–44) grows additive fields (e.g. `Blocks []Block` or thinking/image slices) — text-only rendering stays byte-identical.

### `internal/session/transcript.go` — ContentBlock image variant (PAR-06)

**Analog:** the struct + the additive-kind discipline (lines 79–84, 144–155):
```go
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}
```
```go
// Phase-16 additive kinds (D-20..D-23). ... Readers must tolerate these
// kinds AND unknown future kinds/fields.
```
Add image fields (`Data`, `MediaType`, dims, resize-provenance, `Ref`) — omit-empty so existing text blocks serialize byte-identically. Follow the 09-05 metadata-in-line/body-by-Ref discipline for the payload (Open Question 3 recommendation: transcript line carries dims/media-type/provenance + Ref; full bytes on disk).

### `internal/runtime/runtime.go` `expandUserBlocks` — @-mentions + image ingress (PAR-06)

**Analog 1:** the seam itself (lines 385–449) — parse-from-first-text-block, copy-on-write, degrade-to-stderr:
```go
func (r *Runner) expandUserBlocks(
	sess *session.Session, blocks []session.ContentBlock,
) []session.ContentBlock {
	idx := firstTextBlockIndex(blocks)
	if idx < 0 { return blocks }
	key, args, ok := ecosys.ParseInvocation(blocks[idx].Text)
	if !ok { return blocks }
	cmd, found := r.reg.Commands[key]
	if !found { return blocks }
	out := append([]session.ContentBlock(nil), blocks...)
	out[idx] = session.ContentBlock{Type: blockText, Text: cmd.Expand(args)}
	...
	err := sess.Manager.AppendCommandProvenance("", key, cmd.Path, args)
	if err != nil {
		log.Printf("ass-guard: command provenance write failed (continuing): %v", err)
	}
	return out
}
```
@-mention expansion rides this SAME seam (invoked at runtime.go:540 before the turn): `@file` → content after the Read-rule consult (injected evaluator; implicit allow pre-17), `@dir` → one-level listing, unresolvable → loud note + proceed (D-10). Provenance lines mirror `AppendCommandProvenance`.

**Analog 2 — parse/registry-miss split:** `ecosys/expand.go:24–33` — `ParseInvocation` reports parse-only; the registry-miss decision belongs to the CALLER ("/foo parses fine here"). Copy this split: a `ParseMentions(text)` in `internal/ecosys` (or runtime) returns parsed tokens; resolution/gating stays in runtime.

**Image ingress (no in-file analog):** decode dims via `image.DecodeConfig` FIRST (pixel-bomb guard, Pitfall 6), validate vs provider limits, downscale only when over-limit via `golang.org/x/image/draw` (D-09: `dst := image.NewRGBA(...); draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)`), store ORIGINAL bytes on disk, ship scaled base64. Provider-shape validation: Anthropic adapter accepts images; the OpenAI-shape adapter (`internal/provider/openai.go` — text-only today) triggers D-11 drop + ONE loud note naming the provider.

### `internal/coreexec/register.go` `withHooks` — PreToolUse leg disposal (TRACER plan)

**Analog:** the wrap itself (lines 72–88):
```go
return func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	if proceed, message := hooks.PreToolUse(ctx, name, args); !proceed {
		return marshalStructured("hook: "+message, errHookRefused)
	}
	out, err := exec(ctx, args)
	hooks.PostToolUse(ctx, name, args, out)
	return out, err
}
```
Post-join (17's `gateCall` head owns verdicts), the PreToolUse consultation here is removed or reduced to pass-through (Pitfall 2 double-fire); PostToolUse observation stays. Wired at runtime.go:1106–1110 (`Hooks: hookRunner`).

---

## Shared Patterns

### Graceful degradation with loud stderr notes
**Source:** `internal/ecosys/loader.go:801–803` (`logPluginSkipf`) + `internal/runtime/runtime.go:426–434` (`log.Printf` on transcript-write failure, "continuing")
**Apply to:** ALL four legs — malformed settings.json (Pitfall 8), ignored project-scope allow (D-01), unresolvable @path (D-10), provider-without-image-support drop (D-11), memory truncation (D-08). Warn + proceed, NEVER error up through `Load`/`sessionFor`/turn entry. All output to stderr (stdout is ACP's — transport discipline).

### Profile-copy append without mutating the shared profile
**Source:** `internal/runtime/runtime.go:1046–1058`
**Apply to:** the memory injection merge (and any future system-context merge)
```go
prof.System = append(append([]profile.TextBlock(nil), prof.System...),
	profile.TextBlock{Type: blockText, Text: listing})
```
The double-append makes a fresh slice; `r.profile` is shared across sessions.

### Additive-only schema evolution
**Source:** `internal/session/transcript.go:144–155` (Phase-16 kinds), `internal/provider/provider.go:46–58` (StreamChunk), `internal/event/events.go` (Kind() discriminators)
**Apply to:** ContentBlock image fields, StreamChunk "thinking" type, AgentThoughtChunk event, transcript kinds. New types/kinds are ADDITIVE; existing readers tolerate unknown kinds; existing serialization stays byte-identical.

### json.RawMessage verbatim passthrough (the D-12 invariant)
**Source:** `internal/session/manager.go:106–134` (appendLineUnredacted — deliberately unshared code path), `internal/provider/streaming.go:449–456` (`tuInputBytes` — accumulated-string→RawMessage), `internal/shaper/shaper.go:306–333` (schemaExtras ExtraFields verbatim)
**Apply to:** the entire thinking chain. RawMessage in → RawMessage stored → field VALUES extracted only at the projector → SDK param re-serialization. Never unmarshal+remarshal thinking bytes for storage.

### Parse/skip-with-warning for operator config
**Source:** `internal/ecosys/hooks.go:141–146`, `internal/ecosys/loader.go:584–596` (readCapped)
**Apply to:** settings.json scope parsing (both project and user), memory file reads. Missing = empty contribution; malformed = stderr warning + skip; oversized = cap or skip.

### Deterministic ordering
**Source:** `internal/ecosys/hooks.go:147–158` (sorted event keys), `internal/ecosys/skills.go` sortedSkillNames
**Apply to:** D-03 scope firing order (project → user → plugin, resolved at runner construction — see Pitfall 1 in RESEARCH.md; do NOT reorder the loader merge), memory level accumulation order (deepest-first or cwd-first — pick one, sort deterministically).

---

## No Analog Found

| File | Role | Data Flow | Reason |
|------|------|-----------|--------|
| `internal/session/gate.go` hook-verdict head (POST-17) | middleware (policy gate) | request-response | Phase 17 is PLANNED NOT EXECUTED — `internal/perm/` and `internal/session/gate.go` do not exist in the tree (verified). Code against the LOCKED 17-01/17-02 contract verbatim: `gateCall(ctx, turnID, callID, tool, input) gateVerdict` with `gateExecute/gateDeny/gateSuspend/gateDeclineAutomation`; hook mapping deny→gateDeny, ask→gateSuspend (D-04), allow (user scope only)→gateExecute. The TRACER plan carries `precondition:` blocks from 17-02 and REPLACES the implicit-allow seam comment — never a second gate (17-D-05). |
| Image scaler (pure-Go downscale) | utility | transform | No image processing exists anywhere in the tree. RESEARCH Pattern 5 gives the algorithm (`image.DecodeConfig` first → validate → `draw.CatmullRom.Scale` → re-encode); dependency `golang.org/x/image@v0.45.0` is the phase's only new `go get` and must survive `CGO_ENABLED=0` (`mise [tasks.build]`). |

---

## Metadata

**Analog search scope:** `internal/ecosys`, `internal/runtime`, `internal/session`, `internal/provider`, `internal/shaper`, `internal/acp`, `internal/event`, `internal/coreexec`, `internal/profile`
**Files read in full or targeted:** 18 source files (hooks.go, types.go, skills.go, expand.go, loader.go [regions], provider.go, streaming.go [drainSSE region], session.go [streamAndEmit], manager.go [append paths], transcript.go, projector.go, shaper.go, runtime.go [4 seams], emitter.go [ThoughtChunk region], events.go, register.go [withHooks])
**Verification notes:** `AppendRawThinking` (manager.go:349) and `EmitterHandle.ThoughtChunk` (emitter.go:519) confirmed present with zero production callers in the read paths — Phase 21 fills the middle exactly as RESEARCH states. `routeBusEvent`, `streamAndEmit`, `expandUserBlocks`, `sessionFor` merges all confirmed at the line ranges RESEARCH cites.
**Pattern extraction date:** 2026-08-28
