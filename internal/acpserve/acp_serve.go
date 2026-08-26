// Package acpserve owns the ACP serve composition (D-06/D-07): the startup
// pipeline (bus → profile → provider factory → perm warnings → body store →
// audit mirror) and the server half (NewServer → schedule wiring → emitter
// injection → scheduler start → ctx-done reap → Serve).
//
// TRANSITIONAL SHAPE (plan 15-05, dissolved by plan 15-06): the final form is
// a single Run(ctx, in, out, stderr, opts). Until the runner family leaves
// package main (15-06), the runner literal cannot live here — package main
// cannot be imported. The pipeline therefore ships as two composition halves
// around the still-in-cmd runner construction:
//
//	prep, err := acpserve.PrepareServe(ctx, stderr, opts)   // pre-runner statements
//	runner := &sessionTurnRunner{...from prep...}            // stays in cmd (15-06)
//	runner.loadCommandRegistry() / runner.setupEngine()      // stays in cmd (15-06)
//	return acpserve.FinishServe(ctx, in, out, stderr, opts.Opts, runner, hooks)
//
// FinishServe's runner interactions cross the boundary exclusively through
// the FinishHooks callback seam (never field access), invoked at the exact
// statement positions of the pre-carve source. When 15-06 lands
// runtime.NewRunner, the hooks dissolve and Run reunifies.
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
	"github.com/Djarvur/ass-guard-agent/internal/modelrouting"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/providerfactory"
	"github.com/Djarvur/ass-guard-agent/internal/sched"
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

// PreparedServe carries the pre-runner pipeline state between PrepareServe and
// the cmd-side runner construction (transitional 15-05 shape).
type PreparedServe struct {
	Opts         Options
	Bus          *event.Bus
	Profile      profile.Profile
	SchedCfg     *modelrouting.Config
	Factory      *modelrouting.ProviderFactory
	ProviderName string
	BodyStore    *audit.BodyStore
}

// PrepareServe runs the serve pipeline's pre-runner statements in their original
// order (source :321-360): event bus → profile load → provider factory +
// scheduling config → loose-perm warnings → body store → audit mirror. The
// caller constructs the runner from the returned state, then hands control to
// FinishServe.
func PrepareServe(ctx context.Context, stderr io.Writer, opts *Options) (*PreparedServe, error) {
	bus := event.NewBus()

	prof, err := profile.NewLoader(opts.ProfilesDir).Load(opts.Profile)
	if err != nil {
		return nil, fmt.Errorf("call: %w", err)
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
		return nil, fmt.Errorf("setup provider factory: %w", ferr)
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

	return &PreparedServe{
		Opts:         *opts,
		Bus:          bus,
		Profile:      prof,
		SchedCfg:     schedCfg,
		Factory:      factory,
		ProviderName: providerName,
		BodyStore:    bodyStore,
	}, nil
}

// FinishHooks carries FinishServe's runner-touching operations as callbacks —
// the ONLY way the package-boundary half reaches unexported runner members
// (package main cannot be imported; the closures stay in cmd until 15-06).
// Each hook fires at the exact statement position of the pre-carve source.
type FinishHooks struct {
	// AssignSchedule replaces the source's `runner.schedule = scheduleStore`
	// statement (the sched.Open success arm only).
	AssignSchedule func(store *sched.ScheduleStore)
	// InjectEmitter replaces `runner.emitFor = srv.Emitter` (WINDOWS #3:
	// strictly between NewServer and startScheduler).
	InjectEmitter func(emit func(sessionID string) acp.ChunkEmitter)
	// StartScheduler replaces `runner.startScheduler(ctx)`.
	StartScheduler func(ctx context.Context)
	// CloseSessions replaces the ctx-done goroutine's
	// `runner.closeAllSessions()` reap.
	CloseSessions func()
}

// FinishServe runs the serve pipeline's post-engine statements in their original
// order (source :398-425): acp.NewServer → sched.Open (degrade-loudly) →
// schedule assignment → emitter injection (WINDOWS #3) → scheduler start →
// ctx-done session reap → Serve. The runner arrives as the acp.TurnRunner
// interface; its unexported operations arrive as hooks.
func FinishServe(
	ctx context.Context, in io.Reader, out, stderr io.Writer,
	opts *Options, runner acp.TurnRunner, hooks FinishHooks,
) error {
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
		hooks.AssignSchedule(scheduleStore)
	}

	hooks.InjectEmitter(srv.Emitter) // WINDOWS #3: server-driven turns reach the client
	hooks.StartScheduler(ctx)

	// Phase 5 (Plan 05-02 T4): when the server-level ctx is cancelled
	// (SIGINT/SIGTERM), close every live session's MCP host so no subprocess
	// outlives the ass-guard process. Serve returns after ctx cancellation.
	go func() {
		<-ctx.Done()

		hooks.CloseSessions()
	}()

	return srv.Serve(ctx) //nolint:wrapcheck // direct delegation
}
