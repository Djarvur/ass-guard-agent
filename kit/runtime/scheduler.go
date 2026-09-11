package runtime

import (
	"time"

	"github.com/Djarvur/ass-guard-agent/kit/schedule"
)

// Scheduler is the cron port (25-05, D-16 — the Phase-15 D-13 amendment's
// deferred design landing, no second deferral): EXACTLY the four store calls
// the runner core makes (cron_wiring.go's due-walk, the exactly-once claim,
// and the catch-up pass). The behavioral store stays app-side (internal/sched
// per KIT-03) and satisfies the port STRUCTURALLY — the composition root
// hands the store to SetSchedule, no adapter, no translation layer, so a
// mismatched implementation cannot drop the claim/catch-up semantics (the
// port IS the store's method set). The value types in the signatures are
// kit-defined (kit/schedule — the promoted pure-data structs; no app type
// crosses the boundary).
//
// A nil schedule is the documented degraded state (test runners, a failed
// sched.Open): the scheduler goroutine never starts, due/catch-up walks
// no-op, and the interactive cron quartet surfaces its structured no-store
// errors (InteractiveConfig's nil arm).
type Scheduler interface {
	// Now returns the scheduler's clock (the store's injectable time source).
	Now() time.Time
	// Due returns the automations with at least one UNFIRED schedule slot at
	// or before now (the scheduler tick's walk); spent automations never
	// appear.
	Due(now time.Time) []schedule.Automation
	// ClaimForFire atomically claims one due firing (the exactly-once
	// re-check + advance under the store mutex); ok=false when another path
	// already claimed the window or the automation is spent.
	ClaimForFire(id string, now time.Time) (schedule.Automation, bool)
	// CatchUp computes the fire-once catch-up events for missed automations
	// (each event carries the missed-window note).
	CatchUp(now time.Time) []schedule.FireEvent
}
