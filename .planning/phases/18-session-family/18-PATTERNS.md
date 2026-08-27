# Phase 18: Session Family - Pattern Map

**Mapped:** 2026-08-27
**Files analyzed:** 12 (new/modified)
**Analogs found:** 10 / 12

Note: Phase 16's TurnEmitter and Phase 17's ask-drain/gate machinery are contracts written but not yet in the tree (research A1). Where an analog depends on them, the analog named is the closest existing code plus the 16/17-CONTEXT contract.

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `internal/acp/handlers.go` (modify: add session/list, replace handleSessionLoad, add close/delete) | controller (RPC handler) | request-response | `internal/acp/handlers.go` itself (handleSessionNew/Prompt/Cancel/Logout) | exact |
| `internal/acp/server.go` (modify: capability struct / registration if needed) | controller (dispatch) | request-response | `internal/acp/server.go` registerHandlers/Serve | exact |
| `internal/session/list.go` (new: header scan, cursor, tombstone filter) | service (storage enumeration) | batch (directory scan) | `internal/coreexec/messaging.go` SessionReader.Sessions/read | exact |
| `internal/session/reconcile.go` (new: replay classification + synthetic closures + state seeding) | service (reconciliation engine) | batch (transcript pass → state seed) | `internal/session/reconstruction_test.go` + `session.Prompt`'s planMode/turnCounter state | role-match |
| `internal/session/tombstone.go` (new: marker write, artifact sweep, GC) | service (file lifecycle) | file-I/O | `internal/session/transcript.go` openTranscript (marker-file + perms discipline) | role-match |
| `internal/acpserve/options.go` (modify: resume-target selection) | config | request-response | `internal/acpserve/options.go` itself | exact |
| `cmd/ass-guard/acp_serve.go` (modify: --resume/--continue/-c flags) | CLI (cobra) | request-response | `cmd/ass-guard/acp_serve.go` newACPServeCmd flags | exact |
| `cmd/ass-guard/picker.go` (new: numbered list picker) | CLI utility | request-response (stdin/stderr) | no direct analog — see No Analog | none |
| `internal/session/list_test.go` (new) | test | — | `internal/session/manager_test.go` / `close_test.go` | role-match |
| `internal/session/reconcile_test.go` (new) | test (table-driven fixtures) | — | `internal/session/reconstruction_test.go` | exact |
| `internal/acp/session_family_test.go` (new) | test (handler round-trip) | — | `internal/acp/server_test.go` | exact |
| `internal/acpserve/kill9_test.go` (new) | test (integration harness) | — | `internal/acpserve/serve_test.go` | role-match |

## Pattern Assignments

### `internal/acp/handlers.go` — session/list, session/load, session/close, session/delete handlers

**Analog:** `internal/acp/handlers.go` (same file, existing handlers)

**Handler registration** (lines 23-31):
```go
func (s *Server) registerHandlers() {
	s.handlers[methodInitialize] = s.handleInitialize
	s.handlers["session/new"] = s.handleSessionNew
	// ... add "session/list", "session/close", "session/delete" here
}
```

**Params-parse + session lookup pattern** (lines 66-83, handleSessionNew):
```go
func (s *Server) handleSessionNew(ctx context.Context, params json.RawMessage) (any, error) {
	var p struct {
		Cwd        string `json:"cwd"`
		McpServers []any  `json:"mcpServers"` //nolint:tagliatelle // ACP wire field
	}
	if len(params) > 0 {
		_ = json.Unmarshal(params, &p)
	}
	id := newSessionID()
	st := &sessionState{id: id}
	s.mu.Lock()
	s.sessions[id] = st
	s.mu.Unlock()
	return sessionNewResult{SessionID: id}, nil
}
```
Copy this shape for list/close/delete: anonymous params struct with `//nolint:tagliatelle` on wire-cased fields, result struct with `json:"sessionId"` etc.

**The stub being replaced** (lines 184-192):
```go
func (s *Server) handleSessionLoad(ctx context.Context, params json.RawMessage) (any, error) {
	return nil, &RPCError{
		Code:    CodeMethodNotFound,
		Message: "session/load not supported (loadSession is false; replay is out of v1 scope — D-09)",
	}
}
```
Replace body with load; flip `"loadSession": false` at line 50 to `true` and add `sessionCapabilities: {list:{}, close:{}, delete:{}}` to `AgentCapabilities` (v1 gating — load stays top-level; see research Pitfall 8). **v1 LoadSessionResponse has NO sessionId field** — properties are `_meta`, `configOptions`, `modes` only.

**Cancel-and-drain precedent for close** (lines 153-182, handleSessionCancel):
```go
	s.mu.Lock()
	st, ok := s.sessions[p.SessionID]
	s.mu.Unlock()
	if !ok {
		return nil, nil //nolint:nilnil // notification path
	}
	st.cancelTurn()
	s.closeSessionIfPossible(p.SessionID)
```
session/close = same sequence (cancelTurn → closeSessionIfPossible → delete from map), but a REQUEST (has id → returns `{}` result); already-closed session returns success (D-12 idempotency — the `!ok` branch returns empty result, not error). Compare `handleLogout` (lines 195-213) which does delete-from-map + `closeSessionIfPossible` and returns `map[string]any{}`.

**Error convention:** handler errors either `*RPCError{Code: ..., Message: ...}` (specific JSON-RPC code) or plain `fmt.Errorf` — server.go:226-239 scrubs and wraps plain errors as -32603. `RPCError` and code constants live in `internal/acp/types.go:46,67-69`.

---

### `internal/session/list.go` — header scan + composite cursor + tombstone filter

**Analog:** `internal/coreexec/messaging.go` lines 173-257

**Session-id validation (reuse verbatim, apply to load/close/delete params too):**
```go
// [messaging.go:185-186]
var sessIDPattern = regexp.MustCompile(
	`^(sess_[A-Za-z0-9._-]+|[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})$`)
```
Copy this regex (or import/duplicate per package conventions) — traversal-safe by construction.

**Enumeration pattern** (messaging.go:241-257):
```go
func (r *SessionReader) Sessions() []string {
	entries, err := os.ReadDir(filepath.Join(r.dir, ".ass-guard"))
	if err != nil {
		return nil
	}
	var ids []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "transcript_") && strings.HasSuffix(name, ".jsonl") {
			ids = append(ids, strings.TrimSuffix(strings.TrimPrefix(name, "transcript_"), ".jsonl"))
		}
	}
	return ids
}
```
Extend: skip when a `<id>.deleted` sibling exists (check via `os.Stat` — filter without reading bytes, D-07); sort by mtime desc with (lastActivity, id) tie-break (D-05).

**Bounded line reading** (messaging.go:271-278 pattern — `bufio.Scanner` with `readerBufInit`/`readerBufMax`, skip non-conforming JSON lines):
```go
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, readerBufInit), readerBufMax)
	for sc.Scan() {
		var l transcriptLine
		jerr := json.Unmarshal(sc.Bytes(), &l)
```
For header scan read ONLY the first line (D-04 anti-pattern: never full reads for list).

---

### `internal/session/reconcile.go` — replay + synthetic closures + state seeding

**Analog:** `internal/session/session.go` (state seeding surfaces) + `manager.go` ReadAll (line source) + `reconstruction_test.go` (fixture discipline)

**Read pass** — `Manager.ReadAll` / `readTranscriptFile` (manager.go:287-384): already returns `[]Line` skipping non-conforming lines. Reconciliation classifies dangling pairs over this slice (tool_call ids without tool_result ids, ask_suspended without resolution, subagent_dispatch without result, etc. — the 10-class matrix in 18-RESEARCH.md).

**Synthetic closure emission** — follow the `Manager.Append*` discipline (manager.go:76-103 `appendLine`: marshal → redact → mutex-guarded write). Each synthetic closure needs a provenance marker (D-02) — reuse `Line.Cause` or add an additive field/kind (16-D-20 permits additive kinds):
```go
// [manager.go:195-197] — closest existing synthetic-terminal precedent:
func (m *Manager) AppendCanceled(turnID string, ts time.Time, reason string) error {
	return m.appendLine(&Line{Type: TypeCanceled, TurnID: turnID, Timestamp: ts, Text: reason})
}
```

**Turn-counter seeding** (session.go:127 `turnCounter atomic.Int64`; session.go:139-144):
```go
func (s *Session) nextTurnID() string {
	n := s.turnCounter.Add(1)
	return fmt.Sprintf("%s-turn-%03d", s.SessionID, n)
}
```
Resume: scan max `%03d` suffix of `<sessionID>-turn-NNN` TurnIDs and `s.turnCounter.Store(max)` before the first new turn (Pitfall 2).

**Plan-mode seeding** — `internal/session/planmode.go:67` (`TypePlanMode = "plan_mode"`); seed `planMode` from the LAST plan_mode line's Cause (`plan_mode_enter`/`plan_mode_exit`, see `AppendPlanMode` manager.go:257-262).

**Reconcile-then-accept gate:** the ACP handler constructs the Session, runs the pass, and only then inserts into `s.sessions` — the map insert at handlers.go:79-81 is the LAST step of load (mirror of handleSessionNew ordering, gated).

**Fixture discipline:** copy `internal/session/reconstruction_test.go`'s structure (drives a full Session with a fake `reconProvider` implementing `Send`/`Stream`/`ToolResultMessage`, line 174+); one hand-written transcript fixture per inventory class in `testdata/`, named by row number.

---

### `internal/session/tombstone.go` — marker write + artifact sweep + GC

**Analog:** `internal/session/transcript.go` openTranscript (lines 123-150) for file-perm and marker-file discipline

```go
const filePermOwner = 0o600   // transcript.go:11
const dirPerm = 0o750        // transcript.go:12
// transcript.go:131-135 — the existing marker-file precedent (self-gitignore):
giPath := filepath.Join(storeDir, ".gitignore")
_, err = os.Stat(giPath)
if os.IsNotExist(err) {
	err = os.WriteFile(giPath, []byte(selfGitignoreContent), filePermDefault)
}
```
Tombstone = `os.WriteFile(path, nil, filePermOwner)` beside the transcript; stat-existence filter on list; GC purges only tombstoned files past grace with loud `slog` logging (established pattern — see session.go:727 `slog.Warn("session: tool result could not be recorded...", ...)` for structured loud-failure style). Never `os.Remove` on a transcript outside the grace-expired sweep (Pitfall 4).

---

### `cmd/ass-guard/acp_serve.go` + `internal/acpserve/options.go` — resume flags + target injection

**Analog:** `cmd/ass-guard/acp_serve.go` newACPServeCmd (lines 27-77)

```go
// [acp_serve.go:50-75] — the flag-registration pattern to extend:
	c := &cobra.Command{
		Use:   "serve",
		...
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			auditPath, _ := cmd.Flags().GetString("audit-log")
			...
		},
		SilenceUsage: true,
	}
	c.Flags().StringVar(&profileName, "profile", profileZcode, "...")
	c.Flags().IntVar(&maxConcurrent, "max-concurrent", mnd6, "...")
	c.Flags().StringVar(&workDir, "work-dir", "", "...")
```
Add `--resume` (string, optional value), `--continue`/`-c` (bool) on this command; resolve the target session BEFORE `acpserve.Run` and carry it through a new `Options` field (options.go:12-25 — plain exported struct fields, doc comment per field; add e.g. `ResumeSessionID string`). `--continue` resolves cwd-scoped most-recent (research Pitfall 6).

### `cmd/ass-guard/picker.go` — numbered list picker

No direct analog (see below). Nearest interaction precedent: stderr-only diagnostics discipline from `acp_serve.go` ("log output to stderr — transport discipline") and `internal/acp/server.go:114` (`log.New(stderrSink, "ass-guard/acp: ", ...)`). Numbered rows on stderr, read numbers line-wise from stdin, EOF/invalid → clean error. No raw-mode/termios.

## Shared Patterns

### Session-id validation (path-traversal guard)
**Source:** `internal/coreexec/messaging.go:185-186` (`sessIDPattern`)
**Apply to:** session/load, session/close, session/delete params and every client-supplied sessionId; reject BEFORE any file open (V5 input validation).

### RPC error convention
**Source:** `internal/acp/types.go:46-69`; wrapping in `internal/acp/server.go:226-240`
**Apply to:** all new handlers. `*RPCError{Code: CodeMethodNotFound, ...}` for unsupported/gated; plain errors auto-scrub to -32603. Wire-cased struct fields carry `//nolint:tagliatelle // ACP wire field`.

### Mutex-guarded session map access
**Source:** `internal/acp/handlers.go:114-121`
```go
	s.mu.Lock()
	st, ok := s.sessions[p.SessionID]
	s.mu.Unlock()
```
**Apply to:** list/load/close/delete handler lookups; load's insert is gated on replay completion (D-03).

### Append-only transcript writes
**Source:** `internal/session/manager.go:76-103` (`appendLine`: marshal → redact → mutex → single Write to O_APPEND fd)
**Apply to:** synthetic closures, resume markers, tombstone bookkeeping — never rewrite or truncate transcript bytes.

### File permissions / store-dir discipline
**Source:** `internal/session/transcript.go:11-12, 123-150`
**Apply to:** tombstone files (0600), sweep constrained to `.ass-guard/`.

### Loud structured degradation
**Source:** `session.appendToolResultLoud` (session.go:721-738) `slog.Warn(...)` with key/value context
**Apply to:** GC sweep logging, delete best-effort failures.

## No Analog Found

| File | Role | Data Flow | Reason |
|------|------|-----------|--------|
| `cmd/ass-guard/picker.go` | CLI utility | request-response | No existing interactive stdin/stderr prompt in the codebase; use D-11 spec + stderr discipline from acp_serve.go |
| Phase-16 TurnEmitter replay surface | (dependency, not a file we write) | streaming | Not yet in tree; 16-CONTEXT D-02 contract governs — replay frames ride the same ordered emitter as live turns |
| configOptions menu entries (D-09 grace config) | config surface | — | `configOptions` does not exist in the tree yet (grep: zero non-test hits) — Phase 16 dependency; if unbuilt, land the grace key behind a plain config field and join the menu when 16 lands |

## Metadata

**Analog search scope:** internal/acp, internal/session, internal/coreexec, internal/acpserve, cmd/ass-guard
**Files scanned:** ~15 (handlers.go, server.go, types.go refs, transcript.go, manager.go, session.go, planmode.go (via session.go/manager.go), messaging.go, options.go, acp_serve.go, reconstruction_test.go, serve/server tests via listing)
**Pattern extraction date:** 2026-08-27
