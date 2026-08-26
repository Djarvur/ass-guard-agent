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

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/audit"
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/firstrun"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/providerfactory"
	"github.com/Djarvur/ass-guard-agent/internal/runtime"
	"github.com/Djarvur/ass-guard-agent/internal/sched"
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
func Run(ctx context.Context, in io.Reader, out, stderr io.Writer, opts *Options) error {
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

	runner := runtime.NewRunner(runtime.RunnerConfig{
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

	srv := acp.NewServer(in, out, stderr, acp.WithTurnRunner(runner))

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
	go func() {
		<-ctx.Done()

		runner.CloseAllSessions()
	}()

	return srv.Serve(ctx) //nolint:wrapcheck // direct delegation
}
