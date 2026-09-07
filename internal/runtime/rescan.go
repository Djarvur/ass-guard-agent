package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/Djarvur/ass-guard-agent/internal/ecosys"
)

// 20-05 (CMDS-04/ACP-04, D-10/D-11/D-12): live discovery. fsnotify directory
// watches on the discovery roots (debounced) drive Discover re-runs, atomic
// chain swaps, and available_commands_update re-fires; the invoke-time
// freshness backstop (commands.go) guarantees resolution never runs against a
// stale chain when the watch misses (editor quirks, races, D-12 degrade).
//
// Concurrency (RESEARCH Pitfall 1): the rescan body runs OFF the turn
// goroutines, builds a FRESH chain + registry, and swaps both wholesale
// through their accessors — nothing is ever mutated in place, and sessions
// resolve through the live chain (20-03), so a mid-session addition is
// dispatchable the moment the swap lands.

// rescanDebounce coalesces event bursts into one rescan (T-20-18; the locked
// D-10 discretion window is 100–500ms — 300ms is the documented midpoint).
const rescanDebounce = 300 * time.Millisecond

// discoveryLeaves are the per-root discovery subdirectories (watched when
// they exist; created-under-a-watched-parent picks them up dynamically).
var discoveryLeaves = []string{"skills", "commands", "agents"} //nolint:gochecknoglobals // immutable table

// watch-tree directory names (loader-constant mirrors — internal/runtime
// keeps no ecosys internals import beyond the public API).
const (
	claudeDirName   = ".claude"
	assguardDirName = ".ass-guard"
)

// watchRoots builds the watch set for workDir (literal paths only — NO
// EvalSymlinks resolution on the watch set, T-20-19): the stable PARENTS
// (project .claude/, user .claude/, project .ass-guard/) plus every EXISTING
// discovery leaf under them.
func watchRoots(workDir string) []string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}

	// .claude trees are watched as PARENTS (their discovery leaves ride under
	// them; a created leaf is armed dynamically). The project .ass-guard tree
	// is watched LEAF-ONLY: its PARENT also holds transcripts and session
	// state that change every turn — watching it wholesale would turn every
	// transcript append into a rescan event (T-20-18's storm, found live in
	// the simulator E2E). Leaf-less .ass-guard stays unwatched entirely.
	out := make([]string, 0, 8)

	parents := []string{filepath.Join(workDir, claudeDirName)}

	if home != "" {
		parents = append(parents, filepath.Join(home, claudeDirName))
	}

	// .claude trees: parent + every EXISTING leaf (fsnotify is
	// non-recursive — the parent alone would miss writes inside skills/,
	// commands/, agents/; created leaves arm dynamically from parent events).
	for _, parent := range parents {
		if dirExists(parent) {
			out = append(out, parent)
		}

		for _, leaf := range discoveryLeaves {
			dir := filepath.Join(parent, leaf)
			if dirExists(dir) {
				out = append(out, dir)
			}
		}
	}

	for _, root := range []string{filepath.Join(workDir, assguardDirName), filepath.Join(home, assguardDirName)} {
		for _, leaf := range discoveryLeaves {
			dir := filepath.Join(root, leaf)
			if dirExists(dir) {
				out = append(out, dir)
			}
		}
	}

	return slices.Compact(out)
}

// dirExists reports a real directory (stat, not symlink-following EvalSymlinks).
func dirExists(path string) bool {
	info, err := os.Stat(path)

	return err == nil && info.IsDir()
}

// rescanCoordinator owns the watcher lifecycle + the debounced rescan loop
// for one serve. Construction is infallible (a watcher-create failure
// degrades per D-12 inside Start).
type rescanCoordinator struct {
	r *Runner

	mu       sync.Mutex
	watchers []*fsnotify.Watcher
	done     chan struct{}
	stopped  bool
}

// newRescanCoordinator builds the coordinator (no I/O — Start does the work).
func newRescanCoordinator(r *Runner) *rescanCoordinator {
	return &rescanCoordinator{r: r, done: make(chan struct{})}
}

// Start arms the watcher set and the debounced loop on ctx. The FIRST
// degrade posture (D-12): a watcher-construction failure or a per-root Add
// failure emits exactly ONE loud warning per root (deduped) and the loop
// continues on the remaining roots; total watcher loss leaves the invoke-time
// backstop as the only refresh path — a session never dies over discovery.
func (c *rescanCoordinator) Start(ctx context.Context) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		c.degradeOnce("watcher create", err)

		return
	}

	roots := watchRoots(c.r.workDirOrDefault())
	armed := 0

	warned := make(map[string]struct{})

	for _, root := range roots {
		aerr := w.Add(root)
		if aerr != nil {
			if _, seen := warned[root]; !seen {
				warned[root] = struct{}{}

				_, _ = fmt.Fprintf(c.r.stderrOrDefault(),
					"ass-guard: discovery watch root %s unavailable (%v) — falling back to invoke-time rescan for it\n",
					root, aerr)
			}

			continue
		}

		armed++
	}

	c.mu.Lock()
	c.watchers = []*fsnotify.Watcher{w}
	c.mu.Unlock()

	if armed == 0 {
		c.degradeOnce("no watch roots could be armed", nil)

		_ = w.Close()

		return
	}

	go c.loop(ctx, w)
}

// degradeOnce emits the ONE loud D-12 warning (whole-watcher class).
//
//nolint:funcorder // degrade seam beside Start's failure arms
func (c *rescanCoordinator) degradeOnce(what string, err error) {
	if err != nil {
		_, _ = fmt.Fprintf(c.r.stderrOrDefault(),
			"ass-guard: discovery watcher degraded (%s: %v) — invoke-time rescan is the only refresh path (D-12)\n",
			what, err)

		return
	}

	_, _ = fmt.Fprintf(c.r.stderrOrDefault(),
		"ass-guard: discovery watcher degraded (%s) — invoke-time rescan is the only refresh path (D-12)\n", what)
}

// loop consumes watch events until ctx ends: Chmod is ignored, Creates under
// a watched parent arm new leaves dynamically (Pitfall 2's first-install
// gap), and every burst coalesces through the debounce timer into ONE
// rescanAndSwap.
//
//nolint:funcorder // the loop belongs beside the lifecycle pair
func (c *rescanCoordinator) loop(ctx context.Context, w *fsnotify.Watcher) {
	defer func() {
		c.mu.Lock()
		c.stopped = true
		c.mu.Unlock()

		_ = w.Close()

		close(c.done)
	}()

	timer := time.NewTimer(0)
	if !timer.Stop() {
		<-timer.C
	}

	pending := false

	for {
		select {
		case ev, ok := <-w.Events:
			if !ok {
				return
			}

			if ev.Has(fsnotify.Chmod) {
				continue // README guidance: metadata-only
			}

			if ev.Has(fsnotify.Create) && dirExists(ev.Name) {
				// Dynamic leaf arming (Pitfall 2): a newly created directory
				// under a watched parent joins the set; failures degrade
				// quietly (the backstop still covers its contents).
				_ = w.Add(ev.Name)
			}

			if !pending {
				pending = true

				timer.Reset(rescanDebounce)
			}

		case err, ok := <-w.Errors:
			if !ok {
				return
			}

			c.degradeOnce("watch error stream", err)

		case <-timer.C:
			if pending {
				pending = false

				c.r.rescanAndSwap()
			}

		case <-ctx.Done():
			return
		}
	}
}

// Stop halts the loop and closes the watcher (idempotent; waits for exit).
func (c *rescanCoordinator) Stop() {
	c.mu.Lock()
	stopped := c.stopped
	c.mu.Unlock()

	if !stopped {
		// The loop exits on watcher close or its own defer; closing the
		// watcher unblocks the selects. (Serve ctx cancellation is the
		// primary stop path; Stop is the test/composition backstop.)
		for _, w := range c.watchers {
			_ = w.Close()
		}
	}

	select {
	case <-c.done:
	case <-time.After(time.Second):
	}
}

// StartDiscoveryWatcher arms live discovery on ctx (the acpserve serve
// lifecycle call; teardown rides ctx or StopDiscoveryWatcher).
func (r *Runner) StartDiscoveryWatcher(ctx context.Context) {
	r.rescanMu.Lock()
	defer r.rescanMu.Unlock()

	if r.rescanCoord != nil {
		return // already armed (one coordinator per serve)
	}

	r.rescanCoord = newRescanCoordinator(r)
	r.rescanCoord.Start(ctx)
}

// StopDiscoveryWatcher tears the coordinator down (tests + composition).
func (r *Runner) StopDiscoveryWatcher() {
	r.rescanMu.Lock()
	c := r.rescanCoord
	r.rescanCoord = nil
	r.rescanMu.Unlock()

	if c != nil {
		c.Stop()
	}
}

// rescanAndSwap is the ONE rescan body (startup + watch-triggered + the
// invoke-time backstop share it): Discover off the turn goroutines → install
// the fresh registry + chain wholesale (never in-place mutation, Pitfall 1)
// → fire the rescan-complete seam AFTER the swap (the re-advertised set is
// the chain that is already live).
func (r *Runner) rescanAndSwap() {
	reg, servers, err := ecosys.Discover(r.workDirOrDefault())
	if err != nil {
		_, _ = fmt.Fprintf(r.stderrOrDefault(),
			"ass-guard: discovery rescan failed (keeping the current chain): %v\n", err)

		return
	}

	r.installRegistry(reg, servers)
}

// watchSetSignature is the invoke-time probe's fingerprint: the discovery
// root set's existence + ModTime (stat-level, O(watch set) — never a walk,
// never a full Discover per keystroke).
func watchSetSignature(roots []string) string {
	out := make([]string, 0, len(roots))

	for _, root := range roots {
		info, err := os.Stat(root)
		if err != nil {
			out = append(out, root+":missing")

			continue
		}

		out = append(out, fmt.Sprintf("%s:%d", root, info.ModTime().UnixNano()))
	}

	slices.Sort(out)

	return strconv.FormatUint(hashString(joinedSignature(out)), 16)
}

// joinedSignature joins for hashing (kept tiny for the probe's cost bound).
func joinedSignature(parts []string) string {
	var sb strings.Builder

	for _, p := range parts {
		sb.WriteString(p)
		sb.WriteString(";")
	}

	return sb.String()
}

// hashString is the probe's FNV-1a (std-shape, no dep).
func hashString(s string) uint64 {
	var h uint64 = 14695981039346656037

	for i := range len(s) {
		h ^= uint64(s[i])
		h *= 1099511628211
	}

	return h
}

// discoveryFresh reports whether the discovery roots still match the
// signature the current chain was built under (the D-10 backstop's check;
// stat errors degrade to "fresh" — resolution proceeds on the current chain
// with at most one throttled warning, never blocks).
func (r *Runner) discoveryFresh() bool {
	r.chainSigMu.Lock()
	built := r.chainSignature
	r.chainSigMu.Unlock()

	if built == "" {
		return true // never built a signature (bare runners) — nothing to compare
	}

	if watchSetSignature(watchRoots(r.workDirOrDefault())) == built {
		return true
	}

	return false
}

// noteChainSignature stamps the signature the just-installed chain was built
// under (called by installRegistry).
func (r *Runner) noteChainSignature() {
	r.chainSigMu.Lock()
	r.chainSignature = watchSetSignature(watchRoots(r.workDirOrDefault()))
	r.chainSigMu.Unlock()
}

// maybeFreshRescan is the invoke-time backstop (D-10 layer two): on drift,
// ONE synchronous rescanAndSwap BEFORE the invocation resolves. Bounded by
// the stat probe; a rescan failure keeps the current chain (logged).
func (r *Runner) maybeFreshRescan() {
	if r.discoveryFresh() {
		return
	}

	r.rescanAndSwap()
}
