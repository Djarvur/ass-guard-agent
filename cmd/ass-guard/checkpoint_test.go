package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/checkpoint"
	"github.com/Djarvur/ass-guard-agent/internal/checkpointcmd"
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/providerfactory"
	"github.com/Djarvur/ass-guard-agent/internal/session"
	"github.com/Djarvur/ass-guard-agent/internal/shaper"
)

// seedCheckpointStore opens a store over a seeded temp workspace and takes
// one snapshot, returning the store.
func seedCheckpointStore(t *testing.T, work string) *checkpoint.Store {
	t.Helper()

	err := os.MkdirAll(filepath.Join(work, "src"), 0o755)
	if err != nil {
		t.Fatalf("mkdir src: %v", err)
	}

	err = os.WriteFile(filepath.Join(work, "src", "app.txt"), []byte("original\n"), 0o644)
	if err != nil {
		t.Fatalf("write src/app.txt: %v", err)
	}

	s, err := checkpoint.Open(work)
	if err != nil {
		t.Fatalf("checkpoint.Open: %v", err)
	}

	err = s.Snapshot(context.Background(), "sess-cli", "sess-cli-turn-001")
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	return s
}

// TestCheckpointListEmptyStore verifies an empty (absent) store prints
// "no checkpoints" to stderr and exits 0.
func TestCheckpointListEmptyStore(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	err := checkpointcmd.RunCheckpointList(&out, t.TempDir())
	if err != nil {
		t.Fatalf("RunCheckpointList on empty store: %v", err)
	}

	if !strings.Contains(out.String(), "no checkpoints") {
		t.Errorf("output = %q; want the no-checkpoints note", out.String())
	}
}

// TestCheckpointListShowsSnapshots verifies list output carries the turn
// ids, oldest first, on the provided (stderr) writer.
func TestCheckpointListShowsSnapshots(t *testing.T) {
	t.Parallel()

	work := t.TempDir()
	s := seedCheckpointStore(t, work)

	err := s.Snapshot(context.Background(), "sess-cli", "sess-cli-turn-002")
	if err != nil {
		t.Fatalf("Snapshot 2: %v", err)
	}

	var out bytes.Buffer

	err = checkpointcmd.RunCheckpointList(&out, work)
	if err != nil {
		t.Fatalf("RunCheckpointList: %v", err)
	}

	got := out.String()
	first := strings.Index(got, "sess-cli-turn-001")
	second := strings.Index(got, "sess-cli-turn-002")

	if first < 0 || second < 0 {
		t.Fatalf("output = %q; want both turn ids", got)
	}

	if first > second {
		t.Errorf("output = %q; turn 001 must list before turn 002", got)
	}
}

// TestCheckpointRestoreRoundtripCLI verifies the restore subcommand's body:
// after a mutating turn, runCheckpointRestore returns the workspace to the
// snapshot's bytes and reports the restored ref on stderr.
func TestCheckpointRestoreRoundtripCLI(t *testing.T) {
	t.Parallel()

	work := t.TempDir()
	seedCheckpointStore(t, work)

	err := os.WriteFile(filepath.Join(work, "src", "app.txt"), []byte("mutated by a bad turn\n"), 0o644)
	if err != nil {
		t.Fatalf("mutate src/app.txt: %v", err)
	}

	var out bytes.Buffer

	err = checkpointcmd.RunCheckpointRestore(context.Background(), &out, work, "sess-cli-turn-001")
	if err != nil {
		t.Fatalf("RunCheckpointRestore: %v", err)
	}

	if !strings.Contains(out.String(), "sess-cli-turn-001") ||
		!strings.Contains(out.String(), "workspace restored to pre-turn state") {
		t.Errorf("output = %q; want the restored ref + the restored note", out.String())
	}

	data, rerr := os.ReadFile(filepath.Join(work, "src", "app.txt"))
	if rerr != nil {
		t.Fatalf("read src/app.txt: %v", rerr)
	}

	if string(data) != "original\n" {
		t.Errorf("src/app.txt = %q; want the pre-turn bytes", string(data))
	}
}

// TestCheckpointRestoreRejectsBadID verifies a malformed id is a structured
// error (never a silent success, never a shell-injection surface).
func TestCheckpointRestoreRejectsBadID(t *testing.T) {
	t.Parallel()

	work := t.TempDir()
	seedCheckpointStore(t, work)

	var out bytes.Buffer

	err := checkpointcmd.RunCheckpointRestore(context.Background(), &out, work, "HEAD")
	if err == nil {
		t.Fatal("restore HEAD must fail (malformed checkpoint id)")
	}

	if !strings.Contains(err.Error(), "invalid checkpoint id") {
		t.Errorf("err = %v; want the invalid-id structured error", err)
	}
}

// --- Task 3: serve wiring + the gated live rollback demonstration ---

// TestSessionFor_WiresCheckpointer (Test 13, EARLY-01): sessionFor opens the
// checkpoint store at <workDir>/.ass-guard/checkpoints/shadow.git, the
// Session's Checkpointer is non-nil, and one Prompt through the wired
// session leaves a refs/checkpoints/ entry (default ON).
func TestSessionFor_WiresCheckpointer(t *testing.T) {
	t.Parallel()

	r, _ := newExpansionRunner(t, false, scriptedResp{text: "wired", finish: stopEndTurn})

	const sessionID = "sess-ckpt-w"

	sess := r.sessionFor(context.Background(), sessionID)

	// The store exists at the workspace-pinned path.
	_, serr := os.Stat(filepath.Join(r.workDir, ".ass-guard", "checkpoints", "shadow.git"))
	if serr != nil {
		t.Fatalf("checkpoint store missing after sessionFor: %v", serr)
	}

	// The Session's Checkpointer is wired (nil would mean silently disabled).
	if sess.Checkpointer == nil {
		t.Fatal("sessionFor must wire the Session.Checkpointer (default ON)")
	}

	// One Prompt through the wired session leaves a turn checkpoint.
	_, perr := sess.Prompt(context.Background(),
		[]session.ContentBlock{{Type: blockText, Text: "leave a checkpoint"}})
	if perr != nil {
		t.Fatalf("Prompt: %v", perr)
	}

	store, oerr := checkpoint.Open(r.workDir)
	if oerr != nil {
		t.Fatalf("checkpoint.Open: %v", oerr)
	}

	entries, lerr := store.List()
	if lerr != nil {
		t.Fatalf("List: %v", lerr)
	}

	if len(entries) != 1 {
		t.Fatalf("List = %+v; want exactly the turn's checkpoint", entries)
	}

	if entries[0].Ref != "refs/checkpoints/"+sessionID+"-turn-001" {
		t.Errorf("checkpoint ref = %q; want the turn-001 ref for %s", entries[0].Ref, sessionID)
	}
}

// checkpointLiveGates gates the live rollback demonstration (loud skip
// naming BOTH env vars).
func checkpointLiveGates(t *testing.T) {
	t.Helper()

	if os.Getenv("ASSGUARD_CHECKPOINT_E2E") != "1" || os.Getenv("ZAI_API_KEY") == "" {
		t.Skipf("set ASSGUARD_CHECKPOINT_E2E=1 and ZAI_API_KEY=<GLM Coding Plan key> " +
			"to run the live checkpoint rollback demonstration (real model mutating a scratch repo)")
	}
}

// ckptLiveTree fingerprints every regular file under dir, skipping the
// .ass-guard root (the store + transcripts live there).
func ckptLiveTree(t *testing.T, dir string) map[string]string {
	t.Helper()

	out := map[string]string{}

	werr := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", path, err)
		}

		if d.IsDir() {
			if d.Name() == ".ass-guard" {
				return filepath.SkipDir
			}

			return nil
		}

		if !d.Type().IsRegular() {
			return nil
		}

		rel, rerr := filepath.Rel(dir, path)
		if rerr != nil {
			return fmt.Errorf("rel %s: %w", path, rerr)
		}

		data, derr := os.ReadFile(path)
		if derr != nil {
			return fmt.Errorf("read %s: %w", path, derr)
		}

		sum := sha256.Sum256(data)
		out[rel] = hex.EncodeToString(sum[:])

		return nil
	})
	if werr != nil {
		t.Fatalf("ckptLiveTree(%s): %v", dir, werr)
	}

	return out
}

// ckptLiveGitState captures the USER repo's observable git facets (HEAD
// bytes, index bytes hash, porcelain status). Byte captures precede the
// status run (a status may legitimately refresh the index stat-cache).
type ckptLiveGitState struct {
	headRef   string
	indexHash string
	status    string
}

func ckptLiveCaptureGit(t *testing.T, work string) ckptLiveGitState {
	t.Helper()

	headBytes, err := os.ReadFile(filepath.Join(work, ".git", "HEAD"))
	if err != nil {
		t.Fatalf("read user .git/HEAD: %v", err)
	}

	indexBytes, err := os.ReadFile(filepath.Join(work, ".git", "index"))
	if err != nil {
		t.Fatalf("read user .git/index: %v", err)
	}

	sum := sha256.Sum256(indexBytes)

	status := exec.CommandContext(context.Background(), "git", "status", "--porcelain")
	status.Dir = work

	stOut, serr := status.CombinedOutput()
	if serr != nil {
		t.Fatalf("git status: %v\n%s", serr, stOut)
	}

	return ckptLiveGitState{
		headRef:   string(headBytes),
		indexHash: hex.EncodeToString(sum[:]),
		status:    string(stOut),
	}
}

// TestCheckpointLiveRollback_Gated (Test 14 — the EARLY-01 live leg, gated):
// ONE real model turn mutates a scratch git repo through the FULL serve
// path; the turn's checkpoint restores the workspace byte-identically while
// the user repo's HEAD/index/status stay untouched. Evidence (transcript +
// ref list) is logged for the SUMMARY.
func TestCheckpointLiveRollback_Gated(t *testing.T) { //nolint:paralleltest,funlen,cyclop // live model leg
	checkpointLiveGates(t)

	repo := findRepoRoot(t)

	factory, providerName, ferr := providerfactory.SetupProviderFactory(repo, os.Stderr)
	if ferr != nil {
		t.Fatalf("BLOCKER: provider factory: %v", ferr)
	}

	if _, _, ok := factory.Endpoint(providerName); !ok {
		t.Fatalf("BLOCKER: provider %q has no credentialed endpoint — the demo needs the real model", providerName)
	}

	scratch := t.TempDir()

	// The scratch workspace IS a user git repo (the untouched-repo invariant
	// is asserted against a real .git, not a bare dir).
	for _, args := range [][]string{
		{"init", "--quiet"},
		{"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "--quiet", "--allow-empty", "-m", "root"},
	} {
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = scratch

		out, gerr := cmd.CombinedOutput()
		if gerr != nil {
			t.Fatalf("git %v in scratch: %v\n%s", args, gerr, out)
		}
	}

	seedFile := filepath.Join(scratch, "seed.txt")

	werr := os.WriteFile(seedFile, []byte("pre-turn content\n"), 0o644)
	if werr != nil {
		t.Fatalf("seed scratch: %v", werr)
	}

	prof, perr := profile.NewLoader(filepath.Join(repo, "profiles")).Load(profileZcode)
	if perr != nil {
		t.Fatalf("load real zcode profile: %v", perr)
	}

	bus := event.NewBus()

	r := &sessionTurnRunner{
		bus:     bus,
		profile: prof,
		workDir: scratch,
		maxConc: 6,
		makeProvider: func(_ provider.RequestCapturer) provider.Provider {
			p, _ := factory.Build(providerName, shaper.New())

			return p
		},
	}

	serr := r.setupEngine()
	if serr != nil {
		t.Fatalf("setupEngine: %v", serr)
	}

	r.loadCommandRegistry()

	const sessionID = "sess-ckpt-live"

	// sessionFor creates the store (default ON) BEFORE any turn: the
	// pre-turn capture therefore sees the .ass-guard root as part of the
	// stable landscape (the tree walk skips it).
	r.sessionFor(context.Background(), sessionID)

	preTree := ckptLiveTree(t, scratch)
	preGit := ckptLiveCaptureGit(t, scratch)

	// ONE real mutating model turn.
	prompt := "Use the Write tool to create the file " +
		filepath.Join(scratch, "notes-from-agent.md") +
		" with exactly this content: checkpoint live demo. Then reply done."

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	_, rerr := r.Run(ctx, sessionID, &noopEmitter{},
		[]acp.ContentBlock{{Type: blockText, Text: prompt}})
	if rerr != nil {
		t.Fatalf("live turn: %v", rerr)
	}

	_, serr2 := os.Stat(filepath.Join(scratch, "notes-from-agent.md"))
	if serr2 != nil {
		t.Fatalf("the live turn did not mutate the workspace (no notes-from-agent.md): %v", serr2)
	}

	// Restore the turn's checkpoint through the SAME store the serve path
	// wrote (the public CLI surface's engine).
	store, oerr := checkpoint.Open(scratch)
	if oerr != nil {
		t.Fatalf("checkpoint.Open: %v", oerr)
	}

	entries, lerr := store.List()
	if lerr != nil {
		t.Fatalf("List: %v", lerr)
	}

	if len(entries) == 0 {
		t.Fatal("no checkpoints after the live turn — serve wiring failed")
	}

	target := entries[len(entries)-1]

	resErr := store.Restore(context.Background(), strings.TrimPrefix(target.Ref, "refs/checkpoints/"))
	if resErr != nil {
		t.Fatalf("Restore(%s): %v", target.Ref, resErr)
	}

	// Byte-identical workspace (full recursive path set + per-file bytes).
	if post := ckptLiveTree(t, scratch); !reflect.DeepEqual(post, preTree) {
		t.Error("live restore is NOT byte-identical with the pre-turn workspace tree")
	}

	// The user repo's git state is untouched.
	postGit := ckptLiveCaptureGit(t, scratch)

	if postGit.headRef != preGit.headRef {
		t.Errorf("user .git/HEAD changed across the live turn + restore")
	}

	if postGit.indexHash != preGit.indexHash {
		t.Error("user .git/index bytes changed across the live turn + restore")
	}

	if postGit.status != preGit.status {
		t.Errorf("user git status changed:\nbefore:\n%s\nafter:\n%s", preGit.status, postGit.status)
	}

	t.Logf("checkpoint live evidence: transcript=%s restoredRef=%s entries=%d",
		r.sessions[sessionID].Manager.Path(), target.Ref, len(entries))
}
