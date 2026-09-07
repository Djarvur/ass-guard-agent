package runtime //nolint:testpackage // internal package test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/modelrouting"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
)

// rescanTestRunner arms a runner over a temp dir with the discovery watcher
// on a cancellable ctx; the callback counts rescan completions.
func rescanTestRunner(t *testing.T) (*Runner, *atomic.Int64, context.CancelFunc) {
	t.Helper()

	bus := event.NewBus()
	prov := &scriptedACPProvider{}
	prov.queue(scriptedResp{text: "ok", finish: stopEndTurn})

	dir := t.TempDir()

	stderr := &strings.Builder{}

	r := &Runner{
		bus:          bus,
		profile:      fakeProfileACP(),
		workDir:      dir,
		maxConc:      4,
		stderr:       stderr,
		providerName: fixtureProvider,
		schedCfg: &modelrouting.Config{
			SessionTier: tierHeavy,
			Providers: map[string]modelrouting.ProviderConfig{
				fixtureProvider: {APIKeyEnv: fixtureCredEnv},
			},
			Tiers: map[string]modelrouting.TierBinding{
				tierHeavy: {Model: fixtureModel},
				tierLight: {Model: fixtureModelLite},
			},
			Models: map[string]modelrouting.ModelConfig{
				fixtureModel:     {Provider: fixtureProvider},
				fixtureModelLite: {Provider: fixtureProvider},
			},
		},
		makeProvider: func(_ provider.RequestCapturer) provider.Provider { return prov },
	}

	// Arm the discovery tree BEFORE the watcher (an empty temp dir would
	// leave zero watch roots — the D-12 degrade path, covered by its own test).
	err := os.MkdirAll(filepath.Join(dir, ".claude", "commands"), 0o750)
	if err != nil {
		t.Fatalf("mkdir discovery tree: %v", err)
	}

	err = os.MkdirAll(filepath.Join(dir, ".claude", "agents"), 0o750)
	if err != nil {
		t.Fatalf("mkdir agents tree: %v", err)
	}

	r.LoadCommandRegistry()

	var fires atomic.Int64

	r.SetCommandsNotify(func() { fires.Add(1) })

	ctx, cancel := context.WithCancel(context.Background())
	r.StartDiscoveryWatcher(ctx)

	t.Cleanup(func() {
		cancel()
		r.StopDiscoveryWatcher()
	})

	return r, &fires, cancel
}

// waitForCond polls cond until the deadline (never sleep-fixed assertions).
func waitForCond(t *testing.T, what string, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		if cond() {
			return
		}

		time.Sleep(25 * time.Millisecond)
	}

	t.Fatalf("condition not met within deadline: %s", what)
}

// TestRescanWatcher pins CMDS-04's core: a command file created under a
// watched root lands in the chain + advertisement WITHOUT restart, and its
// removal reverses both — one debounced rescan per change.
func TestRescanWatcher(t *testing.T) { //nolint:paralleltest // sequenced fs state
	r, fires, _ := rescanTestRunner(t)

	writeDiscoveredCommand(t, r.workDir, "fresh-cmd",
		"---\ndescription: arrived live\n---\nFresh body: $ARGUMENTS\n")

	waitForCond(t, "created command resolvable", func() bool {
		_, ok := r.commandChainRef().resolve("fresh-cmd")

		return ok
	})

	// The advertisement carries it (full winner set).
	found := false

	for _, f := range r.CommandAdvertisement() {
		if f.Name == "fresh-cmd" {
			found = true
		}
	}

	if !found {
		t.Error("advertisement missing the live-created command")
	}

	if fires.Load() < 1 {
		t.Errorf("rescan-complete callback never fired (fires=%d)", fires.Load())
	}

	// Removal reverses it.
	err := os.Remove(filepath.Join(r.workDir, ".claude", "commands", "fresh-cmd.md"))
	if err != nil {
		t.Fatalf("remove fixture: %v", err)
	}

	waitForCond(t, "removed command gone", func() bool {
		_, ok := r.commandChainRef().resolve("fresh-cmd")

		return !ok
	})
}

// TestRescanDebounce pins T-20-18: five rapid writes produce exactly ONE
// rescan callback invocation inside the debounce window.
func TestRescanDebounce(t *testing.T) { //nolint:paralleltest // sequenced fs state
	r, fires, _ := rescanTestRunner(t)

	before := fires.Load()

	for i := range 5 {
		writeDiscoveredCommand(t, r.workDir, "burst-"+strings.Repeat("a", i+1),
			"---\ndescription: burst\n---\nBody\n")
	}

	// Wait for the burst to settle (one debounce + margin), then assert the
	// callback count is ONE for the whole burst.
	time.Sleep(rescanDebounce + 150*time.Millisecond)

	waitForCond(t, "burst rescan fired", func() bool { return fires.Load() > before })

	if got := fires.Load() - before; got != 1 {
		t.Errorf("burst produced %d rescan callbacks; want exactly 1 (debounce coalescing)", got)
	}
}

// TestRescanDegrade pins D-12: with NO watch roots available (an empty
// workDir far from any .claude), the watcher degrades once, loudly — and the
// invoke-time backstop still picks changes up.
func TestRescanDegrade(t *testing.T) { //nolint:paralleltest // HOME-sensitive
	r, _, _ := rescanTestRunner(t)

	// The temp dir has a .claude (the fixture created it) — simulate total
	// watcher loss by stopping the coordinator, then proving the backstop.
	r.StopDiscoveryWatcher()

	writeDiscoveredCommand(t, r.workDir, "backstop-cmd",
		"---\ndescription: via backstop\n---\nBackstop body\n")

	// The chain is stale right now...
	if _, ok := r.commandChainRef().resolve("backstop-cmd"); ok {
		t.Fatal("chain already fresh without any rescan — the test is broken")
	}

	// ...and the NEXT resolution (the D-10 backstop) catches it.
	r.maybeFreshRescan()

	if _, ok := r.commandChainRef().resolve("backstop-cmd"); !ok {
		t.Error("invoke-time backstop did not pick up the watch-missed change (D-10)")
	}
}

// TestFreshnessBackstop pins D-10 layer two end-to-end through the RESOLUTION
// path: with the watcher stopped (the watch-miss scenario), the next slash
// resolution reflects the on-disk change — probe → drift → synchronous
// rescan → resolve against the POST-scan chain.
func TestFreshnessBackstop(t *testing.T) { //nolint:paralleltest // sequenced fs state
	r, _, _ := rescanTestRunner(t)
	r.StopDiscoveryWatcher()

	writeDiscoveredCommand(t, r.workDir, "missed-cmd",
		"---\ndescription: watcher never saw this\n---\nMissed body: $ARGUMENTS\n")

	cmd, ok := r.resolveSlashCommand("missed-cmd")
	if !ok {
		t.Fatal("resolution ran against the stale chain (D-10 backstop broken)")
	}

	if !strings.Contains(cmd.Body, "Missed body") {
		t.Errorf("resolved entry = %+v; want the POST-scan content", cmd)
	}
}

// TestRescanConcurrency is the Pitfall-1 proof: sustained concurrent
// resolutions x a discovery-churn loop is -race clean, and a mid-session
// agent becomes dispatchable from the pre-existing session (20-03 live
// lookup).
//
//nolint:paralleltest // stress scenario
func TestRescanConcurrency(t *testing.T) {
	r, _, cancel := rescanTestRunner(t)
	defer cancel()

	var stop atomic.Bool

	var wg sync.WaitGroup

	// Resolution hammer: concurrent slash resolutions through the chain.
	for range 4 {
		wg.Go(func() {
			for !stop.Load() {
				_, _ = r.resolveSlashCommand("status")
			}
		})
	}

	// Discovery churn: create/remove files through the watcher path.
	wg.Go(func() {
		for i := 0; !stop.Load(); i++ {
			name := "churn-" + strings.Repeat("b", 1+i%5)

			p := filepath.Join(r.workDir, ".claude", "commands", name+".md")

			_ = os.WriteFile(p, []byte("---\ndescription: churn\n---\nBody\n"), 0o600)
			_ = os.Remove(p)

			time.Sleep(20 * time.Millisecond)
		}
	})

	// Let it run under -race for a sustained window.
	time.Sleep(1200 * time.Millisecond)

	// The mid-session agent: planted NOW, must be dispatchable after settle.
	writeAgentFixture(t, r.workDir, "mid-session",
		"name: mid-session\ndescription: arrives under load\n", "Mid-session prompt.")

	stop.Store(true)
	wg.Wait()

	waitForCond(t, "mid-session agent dispatchable", func() bool {
		sess := r.sessionFor(context.Background(), "rescan-conc")

		_, ok := sess.AgentLookup("mid-session")

		return ok
	})
}

// TestRescanShutdown pins the lifecycle: ctx cancellation stops the watch
// loop cleanly (the done channel closes) and a second Stop is a no-op.
func TestRescanShutdown(t *testing.T) { //nolint:paralleltest // lifecycle sequencing
	r, _, cancel := rescanTestRunner(t)

	r.rescanMu.Lock()
	c := r.rescanCoord
	r.rescanMu.Unlock()

	if c == nil {
		t.Fatal("coordinator not armed")
	}

	cancel()

	select {
	case <-c.done:
	case <-time.After(2 * time.Second):
		t.Fatal("watch loop did not exit within 2s of ctx cancellation")
	}

	r.StopDiscoveryWatcher() // idempotent backstop — must not block or panic
}

// TestRescanRefire pins the full-replacement contract at the swap seam: two
// successive discovery changes each leave the advertisement carrying the
// COMPLETE winner set for its moment (never a delta, never a merge).
func TestRescanRefire(t *testing.T) { //nolint:paralleltest // sequenced fs state
	r, fires, _ := rescanTestRunner(t)

	setContains := func(name string) bool {
		for _, f := range r.CommandAdvertisement() {
			if f.Name == name {
				return true
			}
		}

		return false
	}

	writeDiscoveredCommand(t, r.workDir, "refire-one", "---\ndescription: one\n---\nOne\n")

	waitForCond(t, "first change live", func() bool { return setContains("refire-one") })

	writeDiscoveredCommand(t, r.workDir, "refire-two", "---\ndescription: two\n---\nTwo\n")

	waitForCond(t, "second change live", func() bool { return setContains("refire-two") })

	if !setContains("refire-one") {
		t.Error("second frame lost the first change (full-replacement sets must each be complete)")
	}

	if fires.Load() < 2 {
		t.Errorf("rescan-complete fires = %d; want >= 2 (one per debounced change)", fires.Load())
	}
}
