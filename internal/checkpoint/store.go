// Package checkpoint implements the EARLY-01 shadow-git checkpoint store:
// the no-confirmation-tier agent's reversibility backstop. Every PARENT turn
// snapshots the workspace's file state into ONE shadow git repository under
// <workDir>/.ass-guard/checkpoints/shadow.git, addressed by turn
// (refs/checkpoints/<sessionID>-turn-<NNN>), and `ass-guard checkpoint
// list|restore` operates on it from the terminal. One store per workspace —
// `checkpoint list` therefore shows the workspace's full recovery history.
//
// Core invariants (all test-pinned):
//
//   - The USER's repository git state (index, HEAD, remotes, config, hooks)
//     is NEVER touched: every git invocation runs with an explicit
//     --git-dir/--work-tree pair, the store's OWN index file, neutralized
//     global/system config, and a disabled hooks path.
//   - A snapshot failure is loud but NEVER fatal to the turn (the session
//     seam degrades per the AUD-03 discipline).
//   - A checkpoint ref appears only AFTER its commit object exists
//     (commit-then-update-ref ordering); an interrupted snapshot never
//     exposes a partial ref.
//   - A turn with zero workspace changes still commits (--allow-empty), so
//     every turn boundary stays restorable.
//
// Snapshots are NOT redacted: the store must restore byte-identically, so
// workspace secrets (e.g. .env) are copied into the store verbatim. The
// mitigation is physical: the store directories are mode 0700, they live
// under the ass-guard root, and no network path ever touches them
// (T-14-02). Git ignore semantics apply: the user repo's own .gitignore
// excludes files from snapshots (and therefore from restores), and the
// worktree-root .git directory is never ingested.
package checkpoint

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultKeep is the retention bound: after each snapshot the store prunes
// its oldest turn refs beyond this many (T-14-03, unbounded-growth DoS).
const DefaultKeep = 50

const (
	// gitBinary is a pre-existing platform dependency (darwin/linux), NOT a
	// module dependency — the zero-dep invariant holds (go.mod untouched).
	gitBinary = "git"

	// storeRootDir is the ass-guard root segment under the workspace.
	storeRootDir = ".ass-guard"
	// storeSubDir hosts the shadow git dir under <workDir>/.ass-guard/.
	storeSubDir = "checkpoints"
	// shadowGitDirName is the shadow git repository's directory name.
	shadowGitDirName = "shadow.git"

	// refPrefix is the turn-addressed ref namespace — the ONLY namespace
	// Restore will ever name (refs confined here, T-14-01).
	refPrefix = "refs/checkpoints/"
	// lastRef is the store's ANCHOR commit: the shadow repo's HEAD points
	// here and every snapshot commits as a DIRECT child of it (23-03's
	// de-chain — see commitSnapshot). Nothing advances it past the init
	// root, so a deleted checkpoint ref's objects become unreachable and
	// gc reclaims them. It is not a turn checkpoint and is excluded from
	// List/prune/Sweep.
	lastRef = "refs/checkpoints/last"

	// lockFileName is the whole-store lock file (create-with-O_EXCL under
	// shadow.git; released by removal; stolen stale after lockStaleAfter).
	lockFileName = "ass-guard.lock"

	// infoExcludeCarried is the exclude entry keeping the store from
	// snapshotting itself (the transcripts under .ass-guard/ ride along).
	infoExcludeCarried = ".ass-guard/\n"

	// initCommitMsg anchors the empty-tree root commit made on a fresh store
	// so every snapshot commit has a parent-able HEAD.
	initCommitMsg = "ass-guard checkpoint store init"

	// hooksPathOff disables the shadow repo's hooks (-c core.hooksPath).
	hooksPathOff = "/dev/null"
	// neutralConfig stands in for the operator's gitconfig files.
	neutralConfig = "/dev/null"

	// dirPermOwnerOnly and lockPermOwnerOnly pin the store's physical
	// secrecy (T-14-02): owner-only traversal and lock writes.
	dirPermOwnerOnly  = 0o700
	filePermOwnerOnly = 0o600
	lockPermOwnerOnly = 0o600

	// shadowIdentity isolates the shadow repo's commit identity from any
	// user gitconfig (which is neutralized anyway).
	shadowIdentityName  = "ass-guard"
	shadowIdentityEmail = "ass-guard@localhost"

	// refLineFieldCount is the field count of listRefs' for-each-ref format
	// ("<refname> <unix-ts>").
	refLineFieldCount = 2

	// bareFalse is the core.bare value flipped onto the bare-layout store.
	bareFalse = "false"
)

var (
	// idPattern is the strict checkpoint-id grammar (T-14-01): a checkpoint
	// id is ALWAYS <sessionID>-<family>-<zero-padded number> where family is
	// "turn" (turn-boundary snapshots) or "pre" (pre-restore snapshots,
	// 23-03/D-09) — the only shapes ever validated into a ref name. No user
	// string reaches git as a refspec unvalidated. All three enforcement
	// sites (this pattern, validateTurnID, parseRefLine) move TOGETHER
	// (Pitfall 6): a partial update mints snapshot ids that fail validation
	// and a restore proceeds unsnapshotted — the exact worst case.
	idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+-(turn|pre)-(\d{3,})$`)

	// lockStaleAfter bounds how long a crashed holder's lock survives before
	// the next operation steals it (with a warning — T-14-06).
	lockStaleAfter = 30 * time.Second //nolint:gochecknoglobals // immutable policy constant

	// lockPollInterval is the file-lock retry cadence while another process
	// holds the store lock.
	lockPollInterval = 50 * time.Millisecond //nolint:gochecknoglobals // immutable policy constant

	// errEmptyWorkDir guards Open against the empty workDir; errEmptySessionID
	// guards Snapshot's session argument.
	errEmptyWorkDir   = errors.New("checkpoint: Open requires a non-empty workDir")
	errEmptySessionID = errors.New("checkpoint: empty session id")
)

// gitRun is the subprocess seam: every git invocation funnels through this
// package var so tests can count/inject invocations (the ref-validation gate
// asserts zero spawns for malformed ids).
var gitRun = func( //nolint:gochecknoglobals // injectable test seam
	ctx context.Context, dir string, env []string, name string, args ...string,
) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = env

	var out bytes.Buffer

	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()

	return out.Bytes(), err
}

// Entry is one restorable turn checkpoint, as surfaced by List.
type Entry struct {
	SessionID   string
	TurnNum     int
	Ref         string
	CommittedAt time.Time
	// Kind is the id family: "turn" (turn-boundary snapshot) or "pre"
	// (pre-restore snapshot, 23-03/D-09). Field-additive — entries parsed by
	// pre-23-03 readers carried the turn family implicitly.
	Kind string
}

// Store is the per-workspace shadow-git checkpoint store. All Snapshot and
// Restore operations serialize on a whole-store lock (in-process mutex +
// cross-process O_EXCL lock file), so a serve-loop snapshot and a terminal
// restore racing on one store never interleave their git index writes.
type Store struct {
	gitDir  string
	workDir string
	mu      sync.Mutex
}

// Open resolves (and lazily initializes) the shadow store under
// <workDir>/.ass-guard/checkpoints/shadow.git. Init is idempotent; the store
// directories are pinned to mode 0700 (T-14-02). A fresh store immediately
// carries an empty-tree root commit so every snapshot commit chains off a
// parent-able HEAD (refs/checkpoints/last).
func Open(workDir string) (*Store, error) {
	if workDir == "" {
		return nil, errEmptyWorkDir
	}

	abs, err := filepath.Abs(workDir)
	if err != nil {
		return nil, fmt.Errorf("checkpoint: resolve work dir: %w", err)
	}

	storeDir := filepath.Join(abs, storeRootDir, storeSubDir)
	gitDir := filepath.Join(storeDir, shadowGitDirName)

	err = os.MkdirAll(storeDir, dirPermOwnerOnly)
	if err != nil {
		return nil, fmt.Errorf("checkpoint: create store dir: %w", err)
	}

	err = os.Chmod(storeDir, dirPermOwnerOnly)
	if err != nil {
		return nil, fmt.Errorf("checkpoint: chmod store dir: %w", err)
	}

	s := &Store{gitDir: gitDir, workDir: abs}

	_, serr := os.Stat(filepath.Join(gitDir, "objects"))
	if serr != nil {
		err = s.initStore(context.Background())
		if err != nil {
			return nil, err
		}
	}

	err = os.Chmod(gitDir, dirPermOwnerOnly)
	if err != nil {
		return nil, fmt.Errorf("checkpoint: chmod shadow git dir: %w", err)
	}

	return s, nil
}

// Snapshot records the workspace's current file state under the turn's
// checkpoint ref. Re-snapshotting the same turn id updates the SAME ref
// (retry-safe, no duplicates). Under the whole-store lock: stage, commit
// (empty allowed), point the turn ref at the commit, prune retention.
func (s *Store) Snapshot(ctx context.Context, sessionID, turnID string) error {
	err := validateTurnID(sessionID, turnID)
	if err != nil {
		return err
	}

	return s.withLock(ctx, func() error {
		sha, err := s.commitSnapshot(ctx, turnID)
		if err != nil {
			return err
		}

		err = s.updateRef(ctx, turnID, sha)
		if err != nil {
			return err
		}

		return s.prune(ctx)
	})
}

// SnapshotPreRestore mints a PRE-RESTORE snapshot (23-03, D-09): the
// pre-restore safety net a restore composes BEFORE overwriting the workspace
// (the caller aborts the restore when this fails — fail-closed). The minted
// id is <sessionID>-pre-<NNN> in the SAME grammar, store, and GC lifecycle as
// turn checkpoints (restorable via the same machinery — undo-of-undo). The
// counter is deterministic across process restarts: seeded by scanning the
// session's existing -pre- refs, next = max+1.
func (s *Store) SnapshotPreRestore(ctx context.Context, sessionID string) (string, error) {
	if sessionID == "" {
		return "", errEmptySessionID
	}

	var minted string

	err := s.withLock(ctx, func() error {
		entries, err := s.listRefs(ctx)
		if err != nil {
			return err
		}

		maxPre := 0

		for _, e := range entries {
			if e.SessionID == sessionID && e.Kind == idFamilyPre && e.TurnNum > maxPre {
				maxPre = e.TurnNum
			}
		}

		id := fmt.Sprintf("%s-%s-%03d", sessionID, idFamilyPre, maxPre+1)

		// The minted id is internally constructed, but the centralized
		// validate-before-refspec gate (T-14-01) still runs — the grammar
		// contract is enforced at ONE site, inherited by every family.
		if err := validateTurnID(sessionID, id); err != nil {
			return err
		}

		sha, err := s.commitSnapshot(ctx, id)
		if err != nil {
			return err
		}

		err = s.updateRef(ctx, id, sha)
		if err != nil {
			return err
		}

		minted = id

		return s.prune(ctx)
	})
	if err != nil {
		return "", err
	}

	return minted, nil
}

// List returns the turn checkpoints ascending by (sessionID, turn number).
// The convenience tip (refs/checkpoints/last) is not a turn checkpoint and
// never appears.
func (s *Store) List() ([]Entry, error) {
	return s.listRefs(context.Background())
}

// DeleteSession removes EVERY checkpoint ref belonging to the session (18-04,
// D-08: the session/delete artifact sweep — the transcript is tombstoned, not
// removed, but the deleted session's checkpoint objects go now). Under the
// whole-store lock; only refs returned by the store's own grammar-validated
// listing are ever deleted, so the caller's session id never reaches git as
// a refspec. A session with no refs is a no-op (idempotent).
func (s *Store) DeleteSession(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return errEmptySessionID
	}

	return s.withLock(ctx, func() error {
		entries, err := s.listRefs(ctx)
		if err != nil {
			return err
		}

		var errs []error

		for _, e := range entries {
			if e.SessionID != sessionID {
				continue
			}

			_, derr := s.git(ctx, "update-ref", "-d", e.Ref)
			if derr != nil {
				errs = append(errs, fmt.Errorf("checkpoint: delete %s: %w", e.Ref, derr))
			}
		}

		return errors.Join(errs...)
	})
}

// Sweep is the D-08 retention authority (23-03; wired at session start by
// 23-04): refs violating EITHER axis are deleted — committed-older-than
// maxAge (axis 1; maxAge <= 0 disables the axis) or beyond the per-session
// newest-count (axis 2; perSession <= 0 normalizes to DefaultKeep) — and the
// SAME withLock critical section then runs reflog expire --expire=now --all
// + gc --prune=now so the victims' objects are actually reclaimed (Pitfall
// 5: splitting ref deletion from object expiry leaves the store growing
// forever). Pre-restore entries participate identically (D-09: one store,
// one lifecycle). Relationship to the per-snapshot keep-50 prune: the prune
// stays (a same-second burst inside ONE session can exceed perSession
// between sweeps without double-deleting — both delete only refs that exist,
// idempotently); Sweep remains the cross-axis authority at session start.
func (s *Store) Sweep(ctx context.Context, maxAge time.Duration, perSession int) error {
	if perSession <= 0 {
		perSession = DefaultKeep
	}

	return s.withLock(ctx, func() error {
		entries, err := s.listRefs(ctx)
		if err != nil {
			return err
		}

		now := time.Now().UTC()

		ageVictims := expireByAge(entries, now, maxAge)

		ageSurvivors := make([]Entry, 0, len(entries))

		victimRefs := make(map[string]bool, len(ageVictims))
		for _, v := range ageVictims {
			victimRefs[v.Ref] = true
		}

		for _, e := range entries {
			if !victimRefs[e.Ref] {
				ageSurvivors = append(ageSurvivors, e)
			}
		}

		victims := append(ageVictims, expireByCount(ageSurvivors, perSession)...)

		for _, v := range victims {
			_, derr := s.git(ctx, "update-ref", "-d", v.Ref)
			if derr != nil {
				return fmt.Errorf("checkpoint: sweep %s: %w", v.Ref, derr)
			}
		}

		// Object expiry in the SAME critical section as the deletions —
		// the whole point of the sweep (D-08: the store stops growing).
		_, err = s.git(ctx, "reflog", "expire", "--expire=now", "--all")
		if err != nil {
			return fmt.Errorf("checkpoint: sweep reflog expire: %w", err)
		}

		_, err = s.git(ctx, "gc", "--prune=now", "--quiet")
		if err != nil {
			return fmt.Errorf("checkpoint: sweep gc: %w", err)
		}

		return nil
	})
}

// expireByAge is the pure D-08 axis-1 victim selection: entries committed
// STRICTLY BEFORE now-maxAge (the exactly-maxAge boundary survives).
// maxAge <= 0 selects nothing (axis disabled).
func expireByAge(entries []Entry, now time.Time, maxAge time.Duration) []Entry {
	if maxAge <= 0 {
		return nil
	}

	cutoff := now.Add(-maxAge)

	var victims []Entry

	for _, e := range entries {
		if e.CommittedAt.Before(cutoff) {
			victims = append(victims, e)
		}
	}

	return victims
}

// expireByCount is the pure D-08 axis-2 victim selection: per session, the
// NEWEST perSession entries survive (recency = CommittedAt, ties broken by
// TurnNum then Ref for determinism); the overflow is evicted. Accounting is
// strictly per-session — a chatty session's overflow never evicts another
// session's refs.
func expireByCount(entries []Entry, perSession int) []Entry {
	if perSession <= 0 {
		return nil
	}

	kept := make(map[string]int, 4)

	sorted := append([]Entry(nil), entries...)
	slices.SortFunc(sorted, func(a, b Entry) int {
		if !a.CommittedAt.Equal(b.CommittedAt) {
			return b.CommittedAt.Compare(a.CommittedAt) // newest first
		}

		if a.TurnNum != b.TurnNum {
			return b.TurnNum - a.TurnNum
		}

		return strings.Compare(b.Ref, a.Ref)
	})

	var victims []Entry

	for _, e := range sorted {
		kept[e.SessionID]++

		if kept[e.SessionID] > perSession {
			victims = append(victims, e)
		}
	}

	return victims
}

// The user-repo exclude append's typed skips (23-03, SEEDG-02): neither is a
// failure — the CALLER logs a structured stderr note (the AUD-03 loud,
// never-fatal discipline).
var (
	// ErrExcludeSkippedNotRepo: the workdir is not a git repository — there
	// is no .git/info/exclude to carry the store-root rule.
	ErrExcludeSkippedNotRepo = errors.New(
		"checkpoint: user-repo exclude skipped: workdir is not a git repository")

	// ErrExcludeSkippedWorktree: .git is a FILE (a linked worktree /
	// submodule checkout) — its info/exclude lives in the gitdir it points
	// at, which is outside the sanctioned one-write surface (the user repo's
	// own .git/info/exclude under the workdir).
	ErrExcludeSkippedWorktree = errors.New(
		"checkpoint: user-repo exclude skipped: .git is a file (worktree/submodule)")
)

// EnsureUserRepoExclude appends the store-root ignore rule (".ass-guard/")
// to the USER repo's .git/info/exclude — the one sanctioned write surface
// into the operator's repo config (SEEDG-02): git status in the user's
// worktree never shows the store. Append-only and idempotent: existing lines
// are preserved byte-for-byte, the rule lands exactly once, and the file is
// never truncated (MkdirAll info at 0700, O_APPEND|O_CREATE at 0600). A
// workdir whose .git is absent returns ErrExcludeSkippedNotRepo and one
// whose .git is a FILE returns ErrExcludeSkippedWorktree — typed skips, not
// failures.
func (s *Store) EnsureUserRepoExclude() error {
	gitPath := filepath.Join(s.workDir, ".git")

	fi, err := os.Stat(gitPath)
	switch {
	case os.IsNotExist(err):
		return ErrExcludeSkippedNotRepo
	case err != nil:
		return fmt.Errorf("checkpoint: stat user .git: %w", err)
	case !fi.IsDir():
		return ErrExcludeSkippedWorktree
	}

	infoDir := filepath.Join(gitPath, "info")

	err = os.MkdirAll(infoDir, dirPermOwnerOnly)
	if err != nil {
		return fmt.Errorf("checkpoint: create user info dir: %w", err)
	}

	excludePath := filepath.Join(infoDir, "exclude")
	rule := storeRootDir + "/"

	existing, err := os.ReadFile(excludePath)
	if err == nil {
		for line := range strings.SplitSeq(string(existing), "\n") {
			if line == rule {
				return nil // already carried — idempotent
			}
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("checkpoint: read user exclude: %w", err)
	}

	f, err := os.OpenFile(excludePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, filePermOwnerOnly)
	if err != nil {
		return fmt.Errorf("checkpoint: open user exclude: %w", err)
	}

	defer func() { _ = f.Close() }()

	if _, err := f.WriteString(rule + "\n"); err != nil {
		return fmt.Errorf("checkpoint: append user exclude rule: %w", err)
	}

	return nil
}

// Restore returns the workspace to the checkpoint's recorded pre-turn state:
// a force checkout of the ref's tree over the whole workspace, then a clean
// that removes files created after the snapshot (never .ass-guard/, the
// store itself). The id is validated against the strict grammar BEFORE any
// git subprocess — no user string reaches git as a refspec unvalidated
// (T-14-01). The user's repository git state is never touched (T-14-04).
func (s *Store) Restore(ctx context.Context, id string) error {
	if !idPattern.MatchString(id) {
		return fmt.Errorf( //nolint:err113 // dynamic validation error
			"checkpoint: invalid checkpoint id %q: want <sessionID>-turn-<NNN> or <sessionID>-pre-<NNN>", id)
	}

	return s.withLock(ctx, func() error {
		// --no-overlay (CR-01): plain pathspec checkout is an OVERLAY — it
		// writes paths present in the target tree but never deletes index
		// entries absent from it. Files added after the snapshot were staged
		// into the shadow index by the NEXT snapshot's add -A, so they stay
		// TRACKED (clean -fd only removes untracked files) and survived
		// restores of any non-latest checkpoint. --no-overlay makes checkout
		// delete index+worktree entries the target tree lacks; clean -fd then
		// only has to sweep genuinely untracked files.
		_, err := s.git(ctx, "checkout", "--no-overlay", "-f", refPrefix+id, "--", ".")
		if err != nil {
			return fmt.Errorf("checkpoint: restore %q (unknown checkpoint id?): %w", id, err)
		}

		_, err = s.git(ctx, "clean", "-fd", "-e", storeRootDir+"/")
		if err != nil {
			return fmt.Errorf("checkpoint: clean restored workspace: %w", err)
		}

		return nil
	})
}

// NestedRepoError is the typed restore refusal (23-03, SEEDG-02): the
// workspace contains nested git repositories, whose contents a restore can
// neither snapshot nor destroy correctly — `git add -A` records them as
// gitlinks (mode 160000, contents NOT snapshotted) and `git clean -fd` never
// descends into them (verified failure shape: restoring over a nested repo
// silently leaves its contents unprotected while the restore reports
// success). Refusal is outright; the composition layer (23-04/23-05) decides
// nothing here — the store only reports and refuses.
type NestedRepoError struct {
	// Paths are the workspace-relative roots of the detected nested
	// repositories (directories containing a .git entry — dir OR file).
	Paths []string
}

func (e *NestedRepoError) Error() string {
	return "checkpoint: refusing restore: nested git repositories present " +
		"(gitlink contents are silently unprotected by restore): " + strings.Join(e.Paths, ", ")
}

// RestoreGuard runs nested-repo detection over workDir and refuses when any
// is found (23-03, SEEDG-02/D-12: no auto path — never restore over, never
// descend). A flat workspace passes with nil.
func (s *Store) RestoreGuard(workDir string) error {
	if nested := findNestedRepos(workDir); len(nested) > 0 {
		return &NestedRepoError{Paths: nested}
	}

	return nil
}

// findNestedRepos walks the workspace for NESTED git repositories — .git
// entries below the top level, as directories (plain nested clones) AND as
// files (worktrees/submodules) — skipping the store root directory itself
// and the user's own top-level repository. Symlinks are never followed
// (filepath.WalkDir uses lstat semantics: no cycles, no escaping the
// workspace, bounded cost). Unreadable entries are skipped — detection is
// best-effort over the walkable surface, and the refusal stays conservative
// (anything found refuses).
func findNestedRepos(workDir string) []string {
	storeRoot := filepath.Join(workDir, storeRootDir)

	var found []string

	_ = filepath.WalkDir(workDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries; the walk continues
		}

		if d.IsDir() {
			if path == storeRoot {
				return filepath.SkipDir // the store never counts as nested
			}

			if d.Name() == ".git" {
				if rel, rerr := filepath.Rel(workDir, filepath.Dir(path)); rerr == nil && rel != "." {
					found = append(found, rel)
				}

				return filepath.SkipDir // never descend into the nested repo's metadata
			}

			return nil
		}

		// A non-directory .git entry is the worktree/submodule variant —
		// gitlink contents are exactly as unprotected. The TOP-LEVEL .git
		// (the user's own repo, or a worktree workspace) is not nested.
		if d.Name() == ".git" {
			if rel, rerr := filepath.Rel(workDir, filepath.Dir(path)); rerr == nil && rel != "." {
				found = append(found, rel)
			}
		}

		return nil
	})

	return found
}

// initStore creates the shadow repository: a bare-LAYOUT git dir at
// shadow.git (a plain `git init <dir>.git` nests .git inside on current
// git), immediately flipped to core.bare=false — with the explicit
// --git-dir/--work-tree pair on every later invocation this is the classic
// shadow-repo recipe (bare=true refuses worktree operations). HEAD points
// at the turn-addressed namespace's convenience tip (never refs/heads/*,
// never the user's repo), info/exclude carries .ass-guard/ (self-exclusion),
// and the empty-tree root commit makes every snapshot commit parent-able.
func (s *Store) initStore(ctx context.Context) error {
	_, err := gitRun(ctx, s.workDir, s.gitEnv(), gitBinary, "init", "--quiet", "--bare", s.gitDir)
	if err != nil {
		return fmt.Errorf("checkpoint: init shadow store: %w", err)
	}

	_, err = s.git(ctx, "config", "core.bare", bareFalse)
	if err != nil {
		return fmt.Errorf("checkpoint: unset bare on shadow store: %w", err)
	}

	_, err = s.git(ctx, "symbolic-ref", "HEAD", lastRef)
	if err != nil {
		return fmt.Errorf("checkpoint: point HEAD at %s: %w", lastRef, err)
	}

	infoDir := filepath.Join(s.gitDir, "info")

	err = os.MkdirAll(infoDir, dirPermOwnerOnly)
	if err != nil {
		return fmt.Errorf("checkpoint: create info dir: %w", err)
	}

	excludePath := filepath.Join(infoDir, "exclude")

	err = os.WriteFile(excludePath, []byte(infoExcludeCarried), filePermOwnerOnly)
	if err != nil {
		return fmt.Errorf("checkpoint: write info/exclude: %w", err)
	}

	_, err = s.git(ctx, "commit", "--allow-empty", "-m", initCommitMsg)
	if err != nil {
		return fmt.Errorf("checkpoint: root commit: %w", err)
	}

	return nil
}

// gitEnv builds the isolation environment (T-14-01/T-14-04): the store's OWN
// index, neutralized global+system gitconfig, no terminal prompting, and the
// shadow identity. The operator's gitconfig, hooks, and index can never
// influence — nor be influenced by — the shadow store.
func (s *Store) gitEnv() []string {
	return append(os.Environ(),
		"GIT_INDEX_FILE="+filepath.Join(s.gitDir, "index"),
		"GIT_CONFIG_GLOBAL="+neutralConfig,
		"GIT_CONFIG_SYSTEM="+neutralConfig,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_AUTHOR_NAME="+shadowIdentityName,
		"GIT_AUTHOR_EMAIL="+shadowIdentityEmail,
		"GIT_COMMITTER_NAME="+shadowIdentityName,
		"GIT_COMMITTER_EMAIL="+shadowIdentityEmail,
	)
}

// git runs one isolated git invocation against the shadow store, wrapping
// any failure with the subcommand and its captured output.
func (s *Store) git(ctx context.Context, args ...string) ([]byte, error) {
	full := append([]string{
		"--git-dir=" + s.gitDir,
		"--work-tree=" + s.workDir,
		"-c", "core.hooksPath=" + hooksPathOff,
	}, args...)

	out, err := gitRun(ctx, s.workDir, s.gitEnv(), gitBinary, full...)
	if err != nil {
		return out, fmt.Errorf("checkpoint: git %s: %w: %s", args[0], err, bytes.TrimSpace(out))
	}

	return out, nil
}

// The id families (23-03): "turn" is the turn-boundary family; "pre" is the
// pre-restore family (D-09). Both are first-class — neither grammar site may
// accept anything the other rejects.
const (
	idFamilyTurn = "turn"
	idFamilyPre  = "pre"
)

// validateTurnID enforces the strict id grammar and the session ownership
// BEFORE any git subprocess: the checkpoint id is the only user input that
// ever names a ref (T-14-01). Family-aware (23-03): both -turn- and -pre-
// ids validate, each against its own session-prefix shape.
func validateTurnID(sessionID, turnID string) error {
	if sessionID == "" {
		return errEmptySessionID
	}

	m := idPattern.FindStringSubmatch(turnID)
	if m == nil {
		return fmt.Errorf( //nolint:err113 // dynamic validation error
			"checkpoint: invalid turn id %q: want <sessionID>-turn-<NNN> or <sessionID>-pre-<NNN>", turnID)
	}

	if !strings.HasPrefix(turnID, sessionID+"-"+m[1]+"-") {
		return fmt.Errorf( //nolint:err113 // dynamic validation error
			"checkpoint: turn id %q does not belong to session %q", turnID, sessionID)
	}

	return nil
}

// commitSnapshot stages the whole workspace and commits it under the id's
// message, returning the commit sha. NO ref update happens here — the split
// IS the partial-ref guarantee (T-14-06): a checkpoint ref appears only
// after its commit object exists. Tests drive this step directly to pin
// that ordering.
//
// 23-03 (D-08): the commit is plumbing — write-tree + commit-tree parented
// DIRECTLY on the store's init anchor (lastRef, which nothing advances past
// the root). The previous shape ran `git commit` on HEAD→lastRef, chaining
// every snapshot on the previous one and anchoring ALL history at last —
// deleting a checkpoint ref (prune/Sweep/DeleteSession) could never make
// its objects unreachable, so gc reclaimed nothing and the store grew
// forever (empirically verified: 315 loose objects became 315 packed, zero
// pruned). Direct children of the anchor keep every snapshot independently
// reachable only through its own ref, so Sweep's ref deletion + reflog
// expire + gc --prune=now actually reclaims the victims. A zero-change
// workspace still commits (commit-tree always creates the object); a
// re-snapshot of the same id updates the same ref (idempotent).
func (s *Store) commitSnapshot(ctx context.Context, turnID string) (string, error) {
	_, err := s.git(ctx, "add", "-A", "--", ".")
	if err != nil {
		return "", fmt.Errorf("checkpoint: stage workspace: %w", err)
	}

	out, err := s.git(ctx, "write-tree")
	if err != nil {
		return "", fmt.Errorf("checkpoint: write tree: %w", err)
	}

	parent, err := s.git(ctx, "rev-parse", lastRef)
	if err != nil {
		return "", fmt.Errorf("checkpoint: resolve anchor commit: %w", err)
	}

	out, err = s.git(ctx, "commit-tree",
		strings.TrimSpace(string(out)), "-p", strings.TrimSpace(string(parent)), "-m", turnID)
	if err != nil {
		return "", fmt.Errorf("checkpoint: commit snapshot: %w", err)
	}

	sha := strings.TrimSpace(string(out))
	if sha == "" {
		return "", errors.New("checkpoint: commit-tree returned no sha") //nolint:err113 // static guard error
	}

	return sha, nil
}

// updateRef points the turn-addressed ref at the snapshot commit (the
// ref-mutation step, always AFTER commitSnapshot).
func (s *Store) updateRef(ctx context.Context, turnID, sha string) error {
	_, err := s.git(ctx, "update-ref", refPrefix+turnID, sha)
	if err != nil {
		return fmt.Errorf("checkpoint: update %s%s: %w", refPrefix, turnID, err)
	}

	return nil
}

// listRefs parses refs/checkpoints/ into sorted Entries.
func (s *Store) listRefs(ctx context.Context) ([]Entry, error) {
	out, err := s.git(ctx, "for-each-ref", "--format=%(refname) %(committerdate:unix)", refPrefix)
	if err != nil {
		return nil, fmt.Errorf("checkpoint: list refs: %w", err)
	}

	entries := make([]Entry, 0, DefaultKeep)

	for line := range strings.SplitSeq(string(out), "\n") {
		e, ok := parseRefLine(line)
		if !ok {
			continue
		}

		entries = append(entries, e)
	}

	slices.SortFunc(entries, func(a, b Entry) int {
		if a.SessionID != b.SessionID {
			return strings.Compare(a.SessionID, b.SessionID)
		}

		if a.TurnNum != b.TurnNum {
			return a.TurnNum - b.TurnNum
		}

		// Same sequence number across families: deterministic order (the
		// pre-restore sibling of a turn sorts beside it — 23-05's stack walk
		// consumes this ordering).
		return strings.Compare(a.Kind, b.Kind)
	})

	return entries, nil
}

// parseRefLine parses one for-each-ref output line
// ("<refname> <unix-ts>") into an Entry, skipping the convenience tip and
// any ref outside the strict id grammar.
func parseRefLine(line string) (Entry, bool) {
	fields := strings.Fields(line)
	if len(fields) != refLineFieldCount {
		return Entry{}, false
	}

	refName, ts := fields[0], fields[1]
	if refName == lastRef {
		return Entry{}, false
	}

	id := strings.TrimPrefix(refName, refPrefix)

	m := idPattern.FindStringSubmatch(id)
	if m == nil {
		return Entry{}, false
	}

	family, seq := m[1], m[2]

	turnNum, err := strconv.Atoi(seq)
	if err != nil {
		return Entry{}, false
	}

	unixSec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return Entry{}, false
	}

	return Entry{
		SessionID:   strings.TrimSuffix(id, "-"+family+"-"+seq),
		TurnNum:     turnNum,
		Ref:         refName,
		CommittedAt: time.Unix(unixSec, 0).UTC(),
		Kind:        family,
	}, true
}

// prune enforces the per-snapshot GLOBAL retention backstop (T-14-03): after
// a snapshot, the oldest refs beyond DefaultKeep are deleted. The session-
// start Sweep (23-04 wires it) is the D-08 retention AUTHORITY — age+count
// dual axis with object expiry; this prune stays as the between-sweeps
// backstop (no double-delete risk: both remove only refs that exist, and
// only the Sweep expires objects). Ties on commit timestamp (same-second
// snapshots are the common case) break by (sessionID, turn number), which
// tracks snapshot sequence deterministically.
func (s *Store) prune(ctx context.Context) error {
	entries, err := s.listRefs(ctx)
	if err != nil {
		return err
	}

	for range max(len(entries)-DefaultKeep, 0) {
		victim := entries[0]
		entries = entries[1:]

		_, derr := s.git(ctx, "update-ref", "-d", victim.Ref)
		if derr != nil {
			return fmt.Errorf("checkpoint: prune %s: %w", victim.Ref, derr)
		}
	}

	return nil
}

// withLock serializes mutating store operations: an in-process mutex (the
// serve loop's snapshots) plus a cross-process O_EXCL lock file under the
// shadow git dir (a terminal restore racing a serve snapshot). A lock older
// than lockStaleAfter is stolen with a warning (a crashed holder never
// wedges the store — T-14-06); ctx cancellation aborts the wait.
func (s *Store) withLock(ctx context.Context, fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	release, err := s.acquireLock(ctx)
	if err != nil {
		return err
	}

	defer release()

	return fn()
}

// acquireLock takes the whole-store file lock, returning the release func.
// The lock file is created with O_EXCL (cross-process mutual exclusion); a
// lock whose mtime exceeds lockStaleAfter is removed and retried once a
// poll interval passes — a crashed holder never wedges the store.
func (s *Store) acquireLock(ctx context.Context) (func(), error) {
	lockPath := filepath.Join(s.gitDir, lockFileName)

	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, lockPermOwnerOnly)
		if err == nil {
			_ = f.Close()

			return func() { _ = os.Remove(lockPath) }, nil
		}

		fi, serr := os.Stat(lockPath)
		if serr == nil && time.Since(fi.ModTime()) >= lockStaleAfter {
			age := time.Since(fi.ModTime())
			slog.Warn("checkpoint: stealing stale store lock", "lock", lockPath, "age", age.String())

			_ = os.Remove(lockPath)

			continue
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("checkpoint: acquire store lock: %w", ctx.Err())
		case <-time.After(lockPollInterval):
		}
	}
}
