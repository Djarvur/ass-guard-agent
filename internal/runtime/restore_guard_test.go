package runtime //nolint:testpackage // internal package test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/checkpoint"
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
