// Package acpserve owns the ACP serve composition (D-06/D-07): the startup
// pipeline (bus → profile → provider factory → perm warnings → body store →
// audit mirror → runner construction + setup) and the server half (NewServer →
// schedule wiring → emitter injection → scheduler start → ctx-done reap →
// Serve), as ONE Run function in the pre-carve source's statement order.
package acpserve

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/audit"
	"github.com/Djarvur/ass-guard-agent/internal/checkpoint"
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/firstrun"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/providerfactory"
	"github.com/Djarvur/ass-guard-agent/internal/runtime"
	"github.com/Djarvur/ass-guard-agent/internal/sched"
	"github.com/Djarvur/ass-guard-agent/internal/session"
	"github.com/Djarvur/ass-guard-agent/internal/shaper"
)

// startAuditMirror (09-06, AUD-02/D-02): the per-session mirror, DEFAULT ON;
// the --audit-log override reroutes it ("-" → stderr; a path → single file).
// Any construction failure degrades loudly (stderr log, mirror disabled) —
// never a serve refusal.
func startAuditMirror(ctx context.Context, bus *event.Bus, opts *Options, stderr io.Writer) {
	//nolint:staticcheck // QF1002: the switch is intentional documentation
	switch {
	case opts.AuditLogPath == "":
		_ = audit.NewMirror(bus, filepath.Join(opts.WorkDir, ".ass-guard", "audit"), nil)
	case opts.AuditLogPath == "-":
		_ = audit.NewMirrorFile(bus, stderr, nil)
	default:
		mw, mclose, merr := audit.OpenFileSink(opts.AuditLogPath)
		if merr != nil {
			log.Printf("ass-guard: audit mirror disabled (%v): %v", opts.AuditLogPath, merr)

			return
		}

		_ = audit.NewMirrorFile(bus, mw, nil)

		go func() {
			<-ctx.Done()

			closeErr := mclose()
			if closeErr != nil {
				log.Printf("ass-guard: audit mirror sink close: %v", closeErr)
			}
		}()
	}
}

// ResolveWorkDir returns workDir, or the current working directory when the flag
// is empty. Eager resolution keeps the seed dir and the session dir consistent.
func ResolveWorkDir(workDir string) (string, error) {
	if workDir != "" {
		return workDir, nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve work dir: %w", err)
	}

	return wd, nil
}

// seedACPGuard runs the Phase-6 first-run seed (D-04) and logs the outcome to
// stderr. A failed seed is non-fatal — the agent stays runnable on defaults.
func seedACPGuard(workDir string) {
	seeded, err := firstrun.Ensure(workDir)
	if err != nil {
		log.Printf("ass-guard: first-run seeding failed (continuing): %v", err)

		return
	}

	if seeded {
		log.Printf("ass-guard: initialized %s", filepath.Join(workDir, ".ass-guard"))
	}
}

// ResolveProfilesDir honors an explicit --profiles-dir; otherwise it prefers the
// seeded <workDir>/.ass-guard/profiles when it exists (zero-config, DIST-03),
// falling back to the flag's default (the dev ./profiles) otherwise.
//
// De-cobra'd in plan 15-05 (one of the two sanctioned edits): the cobra
// Changed() read lives in the cmd shell, which passes the bool here.
func ResolveProfilesDir(profilesDir string, changed bool, workDir string) string {
	if changed {
		return profilesDir
	}

	seedProfiles := filepath.Join(workDir, ".ass-guard", "profiles")

	_, err := os.Stat(seedProfiles)
	if err != nil {
		return profilesDir
	}

	return seedProfiles
}

// SeedACPGuard is the exported entry for the cmd shell's first-run seed step —
// see seedACPGuard (kept unexported next to its only internal caller for
// source fidelity; the shell re-exports the call).
func SeedACPGuard(workDir string) { seedACPGuard(workDir) }

// Run constructs the ACP server and runs it until ctx is cancelled or
// stdin reaches EOF. It wires the real Session Core as the TurnRunner (Plan
// 02-05): each session/prompt drives a session.Session whose Provider.Stream
// streams chunks to the event bus; the runtime Runner forwards bus chunks to
// the ACP adapter as session/update notifications.
func Run( //nolint:funlen // :320-425
	ctx context.Context, in io.Reader, out, stderr io.Writer, opts *Options,
) error {
	bus := event.NewBus()

	prof, err := profile.NewLoader(opts.ProfilesDir).Load(opts.Profile)
	if err != nil {
		return fmt.Errorf("call: %w", err)
	}

	// Phase 7 (D-08): build the provider factory ONCE at startup from the
	// operator's layered config.yaml (global ~/.config/ass-guard-agent/,
	// then project .ass-guard/ — project wins) overlaid on the embedded
	// default. The heavy-tier provider is resolved through the validated
	// scheduler resolver; the session's provider is the factory-built
	// credentialed instance (T-07-07 — resolution is deterministic +
	// validated).
	// 14-05 (EARLY-05): setupModelRouting is setupProviderFactory's
	// cfg-retaining twin — the serve path keeps the loaded scheduling config
	// + the resolved session provider for the light-tier subagent routing at
	// sessionFor (ONE load, no second config read, identical semantics).
	schedCfg, factory, providerName, ferr := providerfactory.SetupModelRouting(opts.WorkDir, stderr)
	if ferr != nil {
		return fmt.Errorf("setup provider factory: %w", ferr)
	}

	// SC3 (Phase 7, extended by 260817-11v): warn once at startup when an
	// operator config.yaml (global or project layer) is looser than 0600 —
	// either may carry a literal api_key (credential-on-disk hygiene,
	// T-07-05). Advisory only; the seed itself stays 0644 (D-06).
	globalPath, gerr := providerfactory.GlobalConfigPath()
	if gerr == nil {
		providerfactory.WarnLooseConfigPerm(globalPath, stderr)
	}

	providerfactory.WarnLooseConfigPerm(providerfactory.ProjectConfigPath(opts.WorkDir), stderr)

	// 09-05: one capped body store per serve process (construction is lazy —
	// Put reports errors; a broken store degrades audit, never the serve).
	bodyStore := audit.NewBodyStore(filepath.Join(opts.WorkDir, ".ass-guard", "audit", "bodies"), 0)

	startAuditMirror(ctx, bus, opts, stderr)

	runner := runtime.NewRunner(&runtime.RunnerConfig{
		Bus:          bus,
		BodyStore:    bodyStore,
		Profile:      prof,
		WorkDir:      opts.WorkDir,
		MaxConc:      opts.MaxConcurrent,
		ConfigAdded:  opts.ConfigAddedBoundaries,
		AskTimeout:   opts.AskTimeout,
		ServeCtx:     ctx,
		SchedCfg:     schedCfg,
		ProviderName: providerName,
		Stderr:       stderr,
		MakeProvider: func(capturer provider.RequestCapturer) provider.Provider {
			// 09-01: the SINGLE factory seam — the same construction the
			// tracer uses (the divergent copy is gone; Pitfall 8).
			// Construction errors keep the existing degradation semantics
			// (ignore, the no-provider path surfaces at first use).
			p, _ := factory.BuildWithCapturer(providerName, shaper.New(), capturer)

			return p
		},
	})
	// Slash-command registry (08-04): load ONCE at startup, engine-independent
	// (the engine-off path expands too). A failed load degrades — turns run on
	// plain text (see LoadCommandRegistry).
	runner.LoadCommandRegistry()

	if opts.EngineEnabled {
		err := runner.SetupEngine()
		if err != nil {
			// A bad config degrades to defaults, never a server crash (the
			// engine is an observer — D-04 graceful degradation at startup).
			log.Printf("ass-guard: engine setup failed (continuing without engine): %v", err)
		}
	}

	// 16-01 (composition root, D-01..D-02): the TurnEmitter is armed WITH the
	// server — one ordered drain owns every session/update notification (fg
	// preempts at the head, bg FIFO, bounded lanes block-never-drop), and the
	// emitter-backed handles flow to both the prompt-turn path (srv.Emitter,
	// below) and the runner's server-driven-turn seam (SetEmitter).
	//
	// 16-05 (ACP-08): the ConfigSurface rides the same construction — menu +
	// effective values over the operator's two layer files (globalPath from
	// the startup resolution above; "" degrades to project-only), injected
	// via WithConfigSurface — then the composition binds its live-apply hook
	// to the runner and its out-of-band notification to the server.
	surface := NewConfigSurface(
		globalPath, providerfactory.ProjectConfigPath(opts.WorkDir), providerName, stderr)

	// 18-04 (D-09): the grace-expired tombstone GC — ONE sweep at startup,
	// after WorkDir resolution and before Serve (the chosen trigger point
	// within the 30d contract). Physically purges ONLY tombstoned
	// transcript+marker pairs past the configured grace; the audit subtree
	// is never enumerated (D-20 — audit survives unconditionally). A sweep
	// failure degrades loudly, never a serve refusal.
	swept, sweepErr := session.SweepTombstones(opts.WorkDir, surface.EffectiveTombstoneGrace(), time.Now())
	if sweepErr != nil {
		log.Printf("ass-guard: tombstone sweep failed (continuing to serve): %v", sweepErr)
	}

	if len(swept) > 0 {
		log.Printf("ass-guard: tombstone sweep purged %d grace-expired session(s): %v", len(swept), swept)
	}

	// 18-04 (D-08): session/delete's checkpoint sweep rides the same
	// workspace store the runner opens per-session (Open is idempotent — one
	// store per workspace). An open failure degrades loudly; delete then
	// skips the checkpoint step (best-effort).
	var ckptDeleter acp.CheckpointStore

	//nolint:contextcheck // plan-pinned signature: Store.Open carries no ctx
	st, coerr := checkpoint.Open(opts.WorkDir)
	if coerr == nil {
		ckptDeleter = st
	} else {
		log.Printf("ass-guard: checkpoint store unavailable for session/delete (%v) — "+
			"deletes skip checkpoint removal", coerr)
	}

	srv := acp.NewServer(in, out, stderr,
		acp.WithTurnRunner(runner),
		acp.WithTurnEmitter(acp.TurnEmitterConfig{}),
		acp.WithConfigSurface(surface),
		// 18-01 (ACP-06/T-18-02): the load handler stats tombstones and reads
		// transcripts against the SERVER's workspace — never the client's
		// cwd. opts.WorkDir is the eagerly-resolved --work-dir (runACPServeCmd
		// resolves before Run); "" degrades to the process cwd inside the
		// server (the same resolution ResolveWorkDir applies).
		acp.WithWorkDir(opts.WorkDir),
		// 18-04 (ACP-05/ACP-07): session/list + session/delete ride the
		// session-backed storage seam (the 18-03 engine + the tombstone
		// writer — session_store.go; acp stays session-free per 25-D-13).
		acp.WithSessionStore(sessionStoreAdapter{}),
		// 18-04 (D-08): the checkpoint-removal seam behind session/delete.
		acp.WithCheckpointStore(ckptDeleter))

	surface.SetNotify(func(sessionID string, opts []acp.ConfigOptionFrame) {
		nerr := srv.NotifyConfigOptions(sessionID, opts)
		if nerr != nil {
			log.Printf("ass-guard: config_option_update enqueue failed (continuing): %v", nerr)
		}
	})
	surface.SetApplyHook(runner.ApplyTurnModel)

	// 16-REVIEW CR-02: the initialize _meta blob channel must reach the WIRE,
	// not just the chip. The hook stamps the runner's pre-editor-stamp default
	// whenever a blob fill moved tier/model, so defaultTurnModel can never
	// diverge from the advertisement on the blob path (16-09 gap 4b pinned the
	// layer path only; the unstamped fill was the operator-observed turn-001
	// chip-shows-X/wire-sends-Y divergence class).
	surface.SetBlobDefaultHook(runner.SetDefaultTurnModel)

	// 17-02 (ACP-01): the permission-ask surface rides the 16-03 registry —
	// its fire callback is injected runner-side so internal/session never
	// imports internal/acp (the onSurface-callback precedent). Injected
	// BEFORE the scheduler starts so the first automation firing already
	// asks through the wire.
	permAsker := NewPermissionAsk(ctx, srv.Registry(), stderr)
	runner.SetPermissionAskFire(permAsker.Fire)

	// 17-04 (ACP-02/D-09): the elicitation-ask surface rides the same
	// registry + capability machinery — the sticky elicitation-form
	// capability (16-D-13/D-18) gates the elicitation/create dispatch and
	// D-08's conservative boolean-property gate reads the connection's
	// boolean advertisement; the plain-text fallback publishes through the
	// runner's subscriber-backed chunk seam (today's path verbatim). The
	// question-family enqueue and the engine-ask conversion both fire
	// through this one dispatcher (the ONE firing path is the ask queue).
	elicitAsker := NewElicitationAsk(ElicitationAskConfig{
		Ctx:      ctx,
		Registry: srv.Registry(),
		Stderr:   stderr,
		CapOK:    func() bool { return srv.Capability(acp.CapElicitationForm) == acp.CapabilityOK },
		BoolAdvertised: func() bool {
			return srv.Capability(acp.CapBooleanConfigOption) == acp.CapabilityOK
		},
		Fallback: runner.PublishAskChunk,
	})
	runner.SetAskFire(elicitAsker.Fire)

	// 17-02 Task 3: the permissions.mode live seams — the advertisement reads
	// the runner's accessor (chip==wire with the gate), the apply hook flips
	// it after every successful persist, and the boot seeds the accessor from
	// the layer resolution so a hand-edited `permissions: {mode: gated}`
	// gates after a restart exactly as advertised.
	surface.SetPermModeRead(runner.PermMode)
	surface.SetPermModeHook(func(mode string) error {
		runner.SetPermMode(mode)

		return nil
	})
	runner.SetPermMode(surface.EffectivePermMode())

	// 12-07 (ACP-04/D-02): the per-project schedule store + the scheduler
	// goroutine on the serve-lifetime ctx (no daemon, no port — Close/ctx
	// owns its lifecycle). A failed open degrades to a serve WITHOUT
	// scheduled firings (the store's own quarantine handles corruption).
	scheduleStore, schedErr := sched.Open(opts.WorkDir)
	if schedErr != nil {
		_, _ = fmt.Fprintf(stderr,
			"ass-guard: schedule store disabled (%v) — cron tools report no-store errors\n", schedErr)
	} else {
		runner.SetSchedule(scheduleStore)
	}

	runner.SetEmitter(srv.Emitter) // WINDOWS #3: server-driven turns reach the client
	runner.StartScheduler(ctx)

	// Phase 5 (Plan 05-02 T4): when the server-level ctx is cancelled
	// (SIGINT/SIGTERM), close every live session's MCP host so no subprocess
	// outlives the ass-guard process. Serve returns after ctx cancellation.
	// 17-03 (D-13/T-17-09): the ask-queue drain runs FIRST — the third
	// teardown path (with the session/cancel notification and logout): every
	// OPEN permission dialog resolves cancelled through the registry cascade
	// and every queued ask drains cancelled-normal while the writer is still
	// open (no orphaned dialogs, no zombie asks, no writes to a closed writer).
	go func() {
		<-ctx.Done()

		runner.DrainAllAsks()

		runner.CloseAllSessions() //nolint:contextcheck // the reap is ctx-driven by design
	}()

	return srv.Serve(ctx) //nolint:wrapcheck // direct delegation
}
