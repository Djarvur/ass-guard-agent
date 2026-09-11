package acpserve //nolint:testpackage // internal package test

// 25-08 subject-split move (kit/runtime -> internal/acpserve): the gated
// live-rollback demonstration drives the REAL app composition
// (providerfactory's live model mutating a scratch git repo through the full
// serve path) — the composition root is the subject, so per the Phase-15
// D-02 rule the test lives at its subject's home. Construction retargets:
// the unexported Runner literal + the engine/catalog twins -> NewRunner +
// loadEngineSetup/loadCommandCatalog; acpRun -> r.Run with session blocks.
// Recorded retargets: the sessionFor pre-warm is dropped (Run constructs the
// session; the tree walk skips .ass-guard regardless, so the pre-turn
// capture is unchanged) and the closing transcript log rides the on-disk
// path convention. Every assertion is byte-identical.

import (
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

	"github.com/Djarvur/ass-guard-agent/internal/providerfactory"
	"github.com/Djarvur/ass-guard-agent/kit/checkpoint"
	"github.com/Djarvur/ass-guard-agent/kit/event"
	"github.com/Djarvur/ass-guard-agent/kit/profile"
	"github.com/Djarvur/ass-guard-agent/kit/provider"
	"github.com/Djarvur/ass-guard-agent/kit/runtime"
	"github.com/Djarvur/ass-guard-agent/kit/session"
	"github.com/Djarvur/ass-guard-agent/kit/shaper"
)

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

	repo := wireFindRepoRoot(t)

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

	r := runtime.NewRunner(&runtime.RunnerConfig{
		Bus:     event.NewBus(),
		Profile: prof,
		WorkDir: scratch,
		MaxConc: 6,
		MakeProvider: func(_ provider.RequestCapturer) provider.Provider {
			p, _ := factory.Build(providerName, shaper.New())

			return p
		},
	})

	setup, serr := loadEngineSetup(scratch)
	if serr != nil {
		t.Fatalf("engine setup: %v", serr)
	}

	if err := r.SetupEngine(setup); err != nil {
		t.Fatalf("SetupEngine: %v", err)
	}

	r.SetCatalog(loadCommandCatalog(scratch))

	const sessionID = "sess-ckpt-live"

	preTree := ckptLiveTree(t, scratch)
	preGit := ckptLiveCaptureGit(t, scratch)

	// ONE real mutating model turn.
	prompt := "Use the Write tool to create the file " +
		filepath.Join(scratch, "notes-from-agent.md") +
		" with exactly this content: checkpoint live demo. Then reply done."

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	_, rerr := r.Run(ctx, sessionID, opsxNoEmitter{},
		[]session.ContentBlock{{Type: wireBlockText, Text: prompt}})
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

	transcriptPath := filepath.Join(scratch, ".ass-guard", "transcript_"+sessionID+".jsonl")

	t.Logf("checkpoint live evidence: transcript=%s restoredRef=%s entries=%d",
		transcriptPath, target.Ref, len(entries))
}
