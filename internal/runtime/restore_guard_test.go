package runtime //nolint:testpackage // internal package test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/checkpoint"
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
)

// --- 23-04 Task 1: Runner-level store + restore guard + session-start sweep ---

// TestRestoreGuardRefusalMatrix pins the SEEDG-02 guard over the full
// (state x store) matrix: active client turn / parked chain refuse with the
// blocking state NAMED; idle passes; nil store never changes the refusal
// semantics (the guard is state-based, the store is a separate nil-check the
// caller composes). The guard check itself never blocks on the turn mutex —
// it reads turnActive/chainCount only, asserted by calling it while a fake
// provider holds the turn.
func TestRestoreGuardRefusalMatrix(t *testing.T) { //nolint:funlen // matrix scenario
	t.Parallel()

	r, prov := newBlockingRunner(t,
		scriptedResp{
			finish: tracerToolUse,
			toolCalls: []provider.ToolCall{{
				ID: "c1", Name: "Read", Input: []byte(`{"file_path":"a.txt"}`),
			}},
		},
		scriptedResp{text: "done", finish: stopEndTurn},
	)

	const sid = "sess-guard"

	// Idle: passes.
	if err := r.restoreBlockers(sid); err != nil {
		t.Fatalf("idle guard refused: %v", err)
	}

	turnDone := make(chan struct{})

	go func() {
		defer close(turnDone)

		_, _ = r.Run(context.Background(), sid, &noopEmitter{},
			[]acp.ContentBlock{{Type: blockText, Text: "long turn"}})
	}()

	<-prov.entered // the turn holds the mutex + turnActive

	// The guard returns PROMPTLY while the turn blocks — never on the mutex.
	guardDone := make(chan error, 1)

	go func() { guardDone <- r.restoreBlockers(sid) }()

	var err error

	select {
	case err = <-guardDone:
	case <-time.After(2 * time.Second):
		t.Fatal("restoreBlockers blocked while the turn held the mutex (must read state only)")
	}

	if err == nil || !strings.Contains(err.Error(), "turn") {
		t.Fatalf("active-turn refusal = %v; want a refusal naming the turn", err)
	}

	// Parked chain: chainCount > 0 with NO mutex held (the trap — a mutex
	// ownership check would pass here and let the restore race the chain).
	close(prov.release)
	<-turnDone // the turn finished; turnActive false again

	r.chainEnter(sid)
	defer r.chainExit(sid)

	err = r.restoreBlockers(sid)
	if err == nil || !strings.Contains(err.Error(), "chain") {
		t.Fatalf("parked-chain refusal = %v; want a refusal naming the chain", err)
	}

	r.chainExit(sid)

	if err := r.restoreBlockers(sid); err != nil {
		t.Fatalf("idle-after-chain guard refused: %v", err)
	}
}

// TestRestoreGuardCrossSessionMatrix pins the G-23-1 (CR-01) guard-level
// truth: the restore guard is WORKSPACE-scoped — every session of the Runner
// shares one workDir, one checkpoint store, and one worktree, so a restore
// must refuse while ANY OTHER session's client turn or engine chain is live,
// NAMING the busy session and its state — while the calling session's
// own-state refusals keep their byte-stable same-session wording and the
// consult stays prompt under a blocked turn (the walk reads state only, never
// a turn mutex).
func TestRestoreGuardCrossSessionMatrix(t *testing.T) { //nolint:funlen // matrix scenario
	t.Parallel()

	r, prov := newBlockingRunner(t, scriptedResp{text: "done", finish: stopEndTurn})

	const (
		sidA = "sess-a-live"
		sidB = "sess-b-idle"
	)

	// Fully idle runner: the guard for B is nil.
	if err := r.restoreBlockers(sidB); err != nil {
		t.Fatalf("idle cross-session guard refused: %v", err)
	}

	turnDone := make(chan struct{})

	go func() {
		defer close(turnDone)

		_, _ = r.Run(context.Background(), sidA, &noopEmitter{},
			[]acp.ContentBlock{{Type: blockText, Text: "session A long turn"}})
	}()

	<-prov.entered // A's turn holds A's turn mutex + A's turnActive

	// Cross-session turn refusal, PROMPTLY (A holds A's turn mutex — the
	// workspace walk must read state only, never wait on any turn mutex).
	guardDone := make(chan error, 1)

	go func() { guardDone <- r.restoreBlockers(sidB) }()

	var err error

	select {
	case err = <-guardDone:
	case <-time.After(2 * time.Second):
		t.Fatal("cross-session guard blocked while A's turn held A's mutex (must read state only)")
	}

	if err == nil || !strings.Contains(err.Error(), sidA) || !strings.Contains(err.Error(), "turn") {
		t.Fatalf("cross-session turn refusal = %v; want a refusal naming %s and the turn", err, sidA)
	}

	// Own-state wording stays byte-stable: A's own guard call still renders
	// the same-session message (no cross-session wording for own state).
	own := r.restoreBlockers(sidA)
	if own == nil || !strings.Contains(own.Error(), "turn") ||
		!strings.Contains(own.Error(), "for the session") ||
		strings.Contains(own.Error(), "every session of this process") {
		t.Fatalf("own-turn refusal = %v; want the byte-stable same-session wording", own)
	}

	// Idle-after-exit: A's turn ends, the cross-session refusal lifts.
	close(prov.release)
	<-turnDone

	if err := r.restoreBlockers(sidB); err != nil {
		t.Fatalf("idle-after-exit guard refused: %v", err)
	}

	// Cross-session parked-chain refusal (no mutex held anywhere — the trap a
	// mutex-ownership check would pass).
	r.chainEnter(sidA)
	defer r.chainExit(sidA)

	err = r.restoreBlockers(sidB)
	if err == nil || !strings.Contains(err.Error(), sidA) || !strings.Contains(err.Error(), "chain") {
		t.Fatalf("cross-session chain refusal = %v; want a refusal naming %s and the chain", err, sidA)
	}

	r.chainExit(sidA)

	if err := r.restoreBlockers(sidB); err != nil {
		t.Fatalf("idle-after-chain-exit guard refused: %v", err)
	}
}

// TestSessionStartSweep pins the D-08 session-start GC: merely CREATING a
// session sweeps the workspace store — an over-age fixture ref is evicted
// with no explicit GC call, and the sweep's bounds come from the runner's
// checkpointGCBounds seam (defaults 7d/50 until config read-back lands).
func TestSessionStartSweep(t *testing.T) {
	t.Parallel()

	r, prov := newBlockingRunner(t, scriptedResp{text: "ok", finish: stopEndTurn})
	close(prov.release)

	const sid = "sess-sweep"

	// Seed the workspace store with one fresh + one over-age ref (the 23-03
	// dated-snapshot discipline, through the store's own plumbing).
	work := r.workDir

	st, err := checkpoint.Open(work)
	if err != nil {
		t.Fatalf("checkpoint.Open: %v", err)
	}

	writeGuardFile(t, filepath.Join(work, "a.txt"), "content\n")

	snapDatedAt(t, work, "sess-old-turn-001", time.Now().UTC().Add(-30*24*time.Hour))
	snapDatedAt(t, work, "sess-new-turn-001", time.Now().UTC())

	// Merely creating the session swept the store.
	sess := r.sessionFor(context.Background(), sid)
	if sess == nil {
		t.Fatal("sessionFor returned nil")
	}

	entries, lerr := st.List()
	if lerr != nil {
		t.Fatalf("List: %v", lerr)
	}

	for _, e := range entries {
		if e.Ref == "refs/checkpoints/sess-old-turn-001" {
			t.Error("over-age ref survived the session-start sweep (30d old vs 7d bound)")
		}
	}

	sawNew := false

	for _, e := range entries {
		if e.Ref == "refs/checkpoints/sess-new-turn-001" {
			sawNew = true
		}
	}

	if !sawNew {
		t.Error("fresh ref evicted by the session-start sweep (age axis misconfigured?)")
	}
}

// TestRunnerStorePromotionDegradation pins Pitfall 9: the Runner holds the
// store once per workspace, and a store that cannot open degrades LOUDLY —
// sessions run fully functional and the runner reports the nil store (never
// a silent /undo trap). The unopenable store is forced by making the store
// path a FILE (Open's MkdirAll fails).
func TestRunnerStorePromotionDegradation(t *testing.T) {
	t.Parallel()

	r, prov := newBlockingRunner(t, scriptedResp{text: "ok", finish: stopEndTurn})
	close(prov.release)

	// Poison the store path: .ass-guard/checkpoints as a regular file makes
	// every Open fail.
	poison := filepath.Join(r.workDir, ".ass-guard", "checkpoints")
	if err := os.MkdirAll(filepath.Dir(poison), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(poison, []byte("not a dir"), 0o600); err != nil {
		t.Fatalf("poison: %v", err)
	}

	const sid = "sess-degraded"

	// The session runs a FULL ordinary turn.
	stop, err := r.Run(context.Background(), sid, &noopEmitter{},
		[]acp.ContentBlock{{Type: blockText, Text: "still works"}})
	if err != nil || stop != stopEndTurn {
		t.Fatalf("degraded Run = (%q,%v); want (end_turn, nil)", stop, err)
	}

	// The Runner reports the nil store loudly (the accessor returns nil and
	// the open failure was logged once — the loud AUD-03 shape; /undo
	// composes this nil-check, never a silent trap).
	if st := r.checkpointStore(); st != nil {
		t.Error("checkpointStore returned a store over a poisoned path")
	}
}

// TestRunnerStoreHeldOnce pins the promotion itself: the SAME store instance
// serves every session of the workspace (constructed once, not per session).
func TestRunnerStoreHeldOnce(t *testing.T) {
	t.Parallel()

	r, prov := newBlockingRunner(t, scriptedResp{text: "ok", finish: stopEndTurn})
	close(prov.release)

	_ = r.sessionFor(context.Background(), "sess-a")
	_ = r.sessionFor(context.Background(), "sess-b")

	a, b := r.checkpointStore(), r.checkpointStore()
	if a == nil || b == nil {
		t.Fatal("checkpointStore nil on a healthy workspace")
	}

	if a != b {
		t.Error("checkpointStore returned different instances for one workspace")
	}
}

// errRestoreBlockedFor is the errors.As target assertion helper.
var _ = errors.Is // keep errors imported for the matrix's future typed checks

// writeGuardFile is the local file fixture helper.
func writeGuardFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// snapDatedAt backdates a checkpoint ref through RAW git against the
// workspace's shadow store (the store_test.go snapDated discipline,
// replicated for this package's fixtures — controlled committer dates drive
// the age axis; the store's internals are invisible cross-package, so the
// isolation wrapper is hand-rolled with the same shape).
func snapDatedAt(t *testing.T, workDir, id string, when time.Time) {
	t.Helper()

	gitDir := filepath.Join(workDir, ".ass-guard", "checkpoints", "shadow.git")

	env := append(os.Environ(),
		"GIT_INDEX_FILE="+filepath.Join(gitDir, "index"),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e.c",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e.c",
		"GIT_AUTHOR_DATE="+when.Format(time.RFC3339),
		"GIT_COMMITTER_DATE="+when.Format(time.RFC3339),
	)

	run := func(args ...string) string {
		t.Helper()

		full := append([]string{
			"--git-dir=" + gitDir, "--work-tree=" + workDir,
			"-c", "core.hooksPath=/dev/null",
		}, args...)

		cmd := exec.CommandContext(context.Background(), "git", full...)
		cmd.Env = env

		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("snapDatedAt git %v: %v\n%s", args, err, out)
		}

		return strings.TrimSpace(string(out))
	}

	run("add", "-A", "--", ".")
	tree := run("write-tree")
	parent := run("rev-parse", "refs/checkpoints/last")
	sha := run("commit-tree", tree, "-p", parent, "-m", id)
	run("update-ref", "refs/checkpoints/"+id, sha)
}

// --- 23-04 Task 3: integration battery (matrix axis, ordering, config-to-GC) ---

// TestRestoreGuardNilStoreAxis completes the refusal matrix's store axis: a
// NIL store never changes the guard's refusal semantics — the guard is
// state-based; the store nil-check is the CALLER's composition (the /undo
// output composes both, 23-05).
func TestRestoreGuardNilStoreAxis(t *testing.T) {
	t.Parallel()

	r, prov := newBlockingRunner(t, scriptedResp{text: "ok", finish: stopEndTurn})

	// Poison the store: every open fails -> the Runner's store is nil.
	poison := filepath.Join(r.workDir, ".ass-guard", "checkpoints")
	if err := os.MkdirAll(filepath.Dir(poison), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(poison, []byte("not a dir"), 0o600); err != nil {
		t.Fatalf("poison: %v", err)
	}

	const sid = "sess-nil-store"

	if st := r.checkpointStore(); st != nil {
		t.Fatal("checkpointStore must be nil over the poisoned path")
	}

	// Idle passes even with a nil store.
	if err := r.restoreBlockers(sid); err != nil {
		t.Errorf("nil-store idle guard refused: %v", err)
	}

	// The active states still refuse identically.
	close(prov.release)

	r.chainEnter(sid)

	if err := r.restoreBlockers(sid); err == nil || !strings.Contains(err.Error(), "chain") {
		t.Errorf("nil-store parked-chain refusal = %v; want the chain-naming refusal", err)
	}

	r.chainExit(sid)
}

// TestSessionStartOrdering pins the session-start sequence: the exclude
// append rides the store open (BEFORE any session exists), then the sweep,
// then the session is ready — and the exclude is check-ignore-live in the
// user repo.
func TestSessionStartOrdering(t *testing.T) {
	t.Parallel()

	r, prov := newBlockingRunner(t, scriptedResp{text: "ok", finish: stopEndTurn})
	close(prov.release)

	// A USER git repo workspace: the exclude lands at store open.
	gitInitUserRepo(t, r.workDir)

	const sid = "sess-order"

	sess := r.sessionFor(context.Background(), sid)
	if sess == nil {
		t.Fatal("sessionFor returned nil")
	}

	// The exclude is live: check-ignore fires for a store path.
	out := gitRunUser(t, r.workDir, "check-ignore", "-v", ".ass-guard/checkpoints/shadow.git")
	if !strings.Contains(out, ".ass-guard") {
		t.Errorf("check-ignore did not fire after session start: %q", out)
	}
}

// TestConfiguredBoundsReachSweep pins the config-to-GC wiring end-to-end:
// bounds injected through the SetCheckpointGCBounds seam (what the serve
// composition binds to the surface's read-back) parameterize the
// session-start sweep — perSession=10 evicts the 11th-oldest ref of a
// session at the NEXT session creation.
func TestConfiguredBoundsReachSweep(t *testing.T) {
	t.Parallel()

	r, prov := newBlockingRunner(t, scriptedResp{text: "ok", finish: stopEndTurn})
	close(prov.release)

	r.SetCheckpointGCBounds(func() (days int, perSession int) { return 7, 10 })

	// Seed 11 same-session checkpoint refs directly through the store.
	st, err := checkpoint.Open(r.workDir)
	if err != nil {
		t.Fatalf("checkpoint.Open: %v", err)
	}

	writeGuardFile(t, filepath.Join(r.workDir, "a.txt"), "seed\n")

	for i := range 11 {
		id := fmt.Sprintf("sess-cfg-turn-%03d", i+1) // 001..011
		if werr := st.Snapshot(context.Background(), "sess-cfg", id); werr != nil {
			t.Fatalf("Snapshot %s: %v", id, werr)
		}
	}

	// The next session START sweeps with the configured bound.
	_ = r.sessionFor(context.Background(), "sess-after")

	entries, lerr := st.List()
	if lerr != nil {
		t.Fatalf("List: %v", lerr)
	}

	if len(entries) != 10 {
		t.Fatalf("post-sweep refs = %d; want 10 (perSession bound from the seam)", len(entries))
	}

	for _, e := range entries {
		if e.Ref == "refs/checkpoints/sess-cfg-turn-001" {
			t.Error("the 11th-oldest ref survived the configured perSession=10 sweep")
		}
	}
}

// gitInitUserRepo initializes a user git repo in dir (one commit).
func gitInitUserRepo(t *testing.T, dir string) {
	t.Helper()

	gitRunUser(t, dir, "init", "--quiet")
	writeGuardFile(t, filepath.Join(dir, "seed.txt"), "seed\n")
	gitRunUser(t, dir, "add", "-A")
	gitRunUser(t, dir, "-c", "user.name=t", "-c", "user.email=t@e.c", "commit", "--quiet", "-m", "init")
}

// gitRunUser runs git in the user's repo, failing the test on error.
func gitRunUser(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}

	return string(out)
}

// --- 23-06 (G-23-1/CR-01): the cross-session /undo refusal battery ---

// undoSentinel is the post-snapshot sentinel file: created AFTER the seeded
// checkpoint (and after any turn-start entry snapshot), a successful restore's
// checkout --no-overlay + clean -fd DELETES it — so its survival is the direct
// "no restore ran" witness on the shared worktree.
const undoSentinel = "undo-sentinel.txt"

// undoSentinelExists reports the sentinel's survival.
func undoSentinelExists(t *testing.T, workDir string) bool {
	t.Helper()

	_, err := os.Stat(filepath.Join(workDir, undoSentinel))

	return err == nil
}

// runUndoPrompt drives ONE /undo invocation through r.Run (the real classifier
// + intercept path) in a goroutine with the battery's 2s behavioral timeout —
// a regression that makes the undo WAIT on any turn mutex fails the timeout
// instead of hanging the suite — and returns the captured frames.
func runUndoPrompt(t *testing.T, r *Runner, sessionID string) []commandFrame {
	t.Helper()

	emit := &tracerEmitter{}

	type undoResult struct {
		stop string
		err  error
	}

	undoDone := make(chan undoResult, 1)

	go func() {
		stop, err := r.Run(context.Background(), sessionID, emit,
			[]acp.ContentBlock{{Type: blockText, Text: "/undo"}})
		undoDone <- undoResult{stop, err}
	}()

	var res undoResult

	select {
	case res = <-undoDone:
	case <-time.After(2 * time.Second):
		t.Fatal("the /undo Run did not return within 2s " +
			"(must refuse without waiting on any turn mutex)")
	}

	if res.err != nil || res.stop != stopEndTurn {
		t.Fatalf("/undo Run = (%q,%v); want (end_turn, nil)", res.stop, res.err)
	}

	return emit.snapshot()
}

// blockedTurnResult carries a driven blocked turn's outcome.
type blockedTurnResult struct {
	stop string
	err  error
}

// startBlockedTurn launches a client turn for sessionID whose provider call
// blocks until the entered channel fires (the caller waits on it) — the
// turnDone channel carries the turn's eventual stop reason.
func startBlockedTurn(t *testing.T, r *Runner, sessionID, text string, entered <-chan struct{}) <-chan blockedTurnResult {
	t.Helper()

	turnDone := make(chan blockedTurnResult, 1)

	go func() {
		stop, err := r.Run(context.Background(), sessionID, &noopEmitter{},
			[]acp.ContentBlock{{Type: blockText, Text: text}})
		if err != nil {
			t.Errorf("%s turn Run err: %v", sessionID, err)
		}

		turnDone <- blockedTurnResult{stop, err}
	}()

	<-entered

	return turnDone
}

// twoBlockProvider blocks its first TWO Stream calls, each on its own
// entered/release pair — the two-concurrent-blocked-turns fixture the
// own-turn outranking row needs (blockingProvider's once covers only one).
type twoBlockProvider struct {
	scriptedACPProvider
	entered []chan struct{}
	release []chan struct{}
	mu      sync.Mutex
	next    int
}

func (p *twoBlockProvider) Stream(
	ctx context.Context, prof *profile.Profile, msgs []provider.Message,
) (<-chan provider.StreamChunk, error) {
	p.mu.Lock()
	i := p.next
	p.next++
	p.mu.Unlock()

	if i < len(p.entered) {
		if p.entered[i] != nil {
			p.entered[i] <- struct{}{}
		}

		select {
		case <-p.release[i]:
		case <-ctx.Done():
		}
	}

	return p.scriptedACPProvider.Stream(ctx, prof, msgs)
}

// newTwoBlockRunner builds an engine-OFF runner over twoBlockProvider (the
// newBlockingRunner construction, two blocking slots).
func newTwoBlockRunner(t *testing.T, script ...scriptedResp) (*Runner, *twoBlockProvider) {
	t.Helper()

	prov := &twoBlockProvider{
		entered: []chan struct{}{make(chan struct{}, 1), make(chan struct{}, 1)},
		release: []chan struct{}{make(chan struct{}), make(chan struct{})},
	}
	prov.queue(script...)

	r := &Runner{
		bus:          event.NewBus(),
		profile:      fakeProfileACP(),
		workDir:      t.TempDir(),
		maxConc:      4,
		makeProvider: func(_ provider.RequestCapturer) provider.Provider { return prov },
	}

	r.LoadCommandRegistry()

	return r, prov
}

// TestUndoCrossSessionRefusal pins the G-23-1 idle full path: an idle session
// B's /undo while session A's client turn is live over the SHARED worktree
// REFUSES — the D-05 output naming A, a durable local_command record with
// outcome "refused: workspace busy", zero provider calls, nothing snapshotted
// or restored (the sentinel survives, A's turn stays alive) — and the refusal
// is TRANSIENT: once A's turn ends, the same /undo restores normally.
func TestUndoCrossSessionRefusal(t *testing.T) { //nolint:funlen // end-to-end refusal + transience scenario
	r, prov := newBlockingRunner(t, scriptedResp{text: "done", finish: stopEndTurn})

	const (
		sidA = "sess-a-live"
		sidB = "sess-b-idle"
	)

	// A's turn goes live over the shared worktree and blocks in the provider.
	turnDoneA := startBlockedTurn(t, r, sidA, "session A long turn", prov.entered)

	// B's checkpoint, then the post-snapshot sentinel.
	seedUndoSnap(t, r.workDir, sidB, sidB+"-turn-001", "state-A\n")
	writeGuardFile(t, filepath.Join(r.workDir, undoSentinel), "post-snapshot\n")

	// Idle B's /undo through the REAL path (r.Run -> classifier -> class-B
	// intercept): must return promptly with the refusal naming A.
	frames := runUndoPrompt(t, r, sidB)

	out := undoOutputText(frames)
	if !strings.Contains(out, "undo refused") || !strings.Contains(out, sidA) {
		t.Fatalf("output missing the cross-session refusal naming %s:\n%s", sidA, out)
	}

	// No checkout/clean ran under A's live turn: the sentinel survived.
	if !undoSentinelExists(t, r.workDir) {
		t.Fatal("the sentinel was deleted under A's live turn (a restore ran)")
	}

	// A's turn is still alive — nothing was cancelled anywhere.
	select {
	case <-turnDoneA:
		t.Fatal("A's turn ended on B's refused /undo")
	default:
	}

	// The durable record: local_command with the refusal outcome.
	lines, rerr := r.sessions[sidB].Manager.ReadAll()
	if rerr != nil {
		t.Fatalf("ReadAll: %v", rerr)
	}

	rec := localCommandLine(t, lines)
	if rec.Name != "undo" || rec.Expansion != "refused: workspace busy" {
		t.Fatalf("local_command = {name:%q outcome:%q}; want {undo refused: workspace busy}",
			rec.Name, rec.Expansion)
	}

	// Transience: after A's turn finishes, the SAME /undo restores normally.
	close(prov.release)

	select {
	case <-turnDoneA:
	case <-time.After(5 * time.Second):
		t.Fatal("A's turn did not finish after release")
	}

	frames2, lines2 := undoRun(t, r, sidB, "/undo")

	if out2 := undoOutputText(frames2); !strings.Contains(out2, "undo complete") {
		t.Fatalf("the retry after A ended did not restore:\n%s", out2)
	}

	if undoSentinelExists(t, r.workDir) {
		t.Fatal("the sentinel survived a completed restore (the clean never ran)")
	}

	rec2 := localCommandLine(t, lines2)
	if rec2.Name != "undo" || rec2.Expansion != "ok" {
		t.Fatalf("retry local_command = {name:%q outcome:%q}; want {undo ok}", rec2.Name, rec2.Expansion)
	}

	// Zero provider calls from either /undo — A's turn is the only one.
	if got := prov.callCount(); got != 1 {
		t.Fatalf("provider Stream calls = %d; want 1 (A's turn only)", got)
	}
}

// TestUndoActivePathCrossSessionRefusal pins the G-23-1 ACTIVE-path branch:
// with session A live, session B's mid-turn /undo REFUSES instead of running
// the D-12 auto-cancel-then-restore — for BOTH of B's own-state variants
// (truth 2's outranking claim): B's parked engine chain, and B's OWN live
// client turn. Nothing of B's is cancelled, nothing is restored, A stays
// blocked, and the refusal is the named, durable, D-05 shape.
func TestUndoActivePathCrossSessionRefusal(t *testing.T) { //nolint:funlen // two-variant scenario
	const (
		sidA = "sess-a-live"
		sidB = "sess-b-idle"
	)

	// Variant 1: B's own PARKED CHAIN is outranked — the refusal fires
	// instead of the auto-cancel; B's chain is NOT cancelled.
	t.Run("own parked chain outranked by live session A", func(t *testing.T) {
		r, prov := newBlockingRunner(t, scriptedResp{text: "done", finish: stopEndTurn})

		turnDoneA := startBlockedTurn(t, r, sidA, "session A long turn", prov.entered)

		seedUndoSnap(t, r.workDir, sidB, sidB+"-turn-001", "state-A\n")
		writeGuardFile(t, filepath.Join(r.workDir, undoSentinel), "post-snapshot\n")

		// B's own engine chain parked (chainCount > 0, no mutex held).
		r.chainEnter(sidB)
		defer r.chainExit(sidB)

		// B's mid-turn /undo through r.Run — the classifier sees the chain
		// and routes it to the active path.
		frames := runUndoPrompt(t, r, sidB)

		out := undoOutputText(frames)
		if !strings.Contains(out, "undo refused") || !strings.Contains(out, sidA) {
			t.Fatalf("output missing the cross-session refusal naming %s:\n%s", sidA, out)
		}

		// The refusal outranks the auto-cancel: B's chain was NOT cancelled.
		if got := r.chainCount(sidB); got != 1 {
			t.Fatalf("B's parked chain count = %d after the refusal; want 1 (nothing cancelled)", got)
		}

		if !undoSentinelExists(t, r.workDir) {
			t.Fatal("the sentinel was deleted under A's live turn (a restore ran)")
		}

		select {
		case <-turnDoneA:
			t.Fatal("A's turn ended on B's refused /undo")
		default:
		}

		lines, rerr := r.sessions[sidB].Manager.ReadAll()
		if rerr != nil {
			t.Fatalf("ReadAll: %v", rerr)
		}

		rec := localCommandLine(t, lines)
		if rec.Name != "undo" || rec.Expansion != "refused: workspace busy" {
			t.Fatalf("local_command = {name:%q outcome:%q}; want {undo refused: workspace busy}",
				rec.Name, rec.Expansion)
		}

		// Cleanup: release A's turn.
		close(prov.release)

		select {
		case <-turnDoneA:
		case <-time.After(5 * time.Second):
			t.Fatal("A's turn did not finish after release")
		}
	})

	// Variant 2: B's OWN CLIENT TURN is outranked too — the refusal fires
	// instead of auto-cancelling B's turn, exactly as it outranks its parked
	// chain. B's turn stays alive (still blocked in its provider call).
	t.Run("own client turn outranked by live session A", func(t *testing.T) {
		r, prov := newTwoBlockRunner(t,
			scriptedResp{text: "a done", finish: stopEndTurn},
			scriptedResp{text: "b done", finish: stopEndTurn},
		)

		turnDoneA := startBlockedTurn(t, r, sidA, "session A long turn", prov.entered[0])

		seedUndoSnap(t, r.workDir, sidB, sidB+"-turn-001", "state-B\n")

		// B's OWN client turn goes live (blocks in its own provider call).
		turnDoneB := startBlockedTurn(t, r, sidB, "session B own turn", prov.entered[1])

		// The sentinel lands AFTER both turn-start entry snapshots: no
		// candidate checkpoint contains it, so its survival cleanly
		// witnesses "no restore ran".
		writeGuardFile(t, filepath.Join(r.workDir, undoSentinel), "post-snapshot\n")

		// B's mid-turn /undo — a SECOND r.Run on B; the classifier's
		// own-turn leg (not timing) routes it to the active path.
		frames := runUndoPrompt(t, r, sidB)

		out := undoOutputText(frames)
		if !strings.Contains(out, "undo refused") || !strings.Contains(out, sidA) {
			t.Fatalf("output missing the cross-session refusal naming %s:\n%s", sidA, out)
		}

		// The refusal outranks auto-cancelling B's own turn: it is STILL
		// alive (blocked in its provider call).
		select {
		case <-turnDoneB:
			t.Fatal("B's own turn was cancelled by the refusal path (the refusal must outrank the auto-cancel)")
		default:
		}

		if !undoSentinelExists(t, r.workDir) {
			t.Fatal("the sentinel was deleted under A's live turn (a restore ran)")
		}

		select {
		case <-turnDoneA:
			t.Fatal("A's turn ended on B's refused /undo")
		default:
		}

		lines, rerr := r.sessions[sidB].Manager.ReadAll()
		if rerr != nil {
			t.Fatalf("ReadAll: %v", rerr)
		}

		rec := localCommandLine(t, lines)
		if rec.Name != "undo" || rec.Expansion != "refused: workspace busy" {
			t.Fatalf("local_command = {name:%q outcome:%q}; want {undo refused: workspace busy}",
				rec.Name, rec.Expansion)
		}

		// Cleanup: release B's turn, then A's; both end normally.
		close(prov.release[1])

		select {
		case <-turnDoneB:
		case <-time.After(5 * time.Second):
			t.Fatal("B's turn did not finish after release")
		}

		close(prov.release[0])

		select {
		case <-turnDoneA:
		case <-time.After(5 * time.Second):
			t.Fatal("A's turn did not finish after release")
		}
	})
}
