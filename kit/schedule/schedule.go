// Package schedule holds the persisted schedule vocabulary shared by the
// kit core and the app-side store (25-05, D-05 per-case promotion): the
// Automation record and the FireEvent catch-up event — pure data, zero app
// dependencies. The json tags ARE the persisted
// .ass-guard/schedule/schedules.json shape, moved byte-identical from
// internal/sched (which type-aliases these structs so every existing caller
// and every persisted file keeps compiling and round-tripping unchanged).
// The behavioral store (cron parsing, due-walks, claims, persistence) stays
// app-side in internal/sched per KIT-03; the kit core reaches it only
// through the Scheduler port (kit/runtime, D-16).
package schedule

import (
	"fmt"
	"time"
)

// Automation is one persisted scheduled prompt (the store's record unit).
type Automation struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Prompt string `json:"prompt"`
	Cron   string `json:"cron,omitempty"` // 5-field spec; "" when delay-only
	// relative one-shot (minutes); camelCase mirrors the captured schema field.
	DelayOnce int `json:"delayMinutes,omitempty"` //nolint:tagliatelle // camelCase mirrors the captured schema field
	// Recurring mirrors the schema default (true repeats indefinitely; false
	// is finite — without MaxRuns it runs once).
	Recurring bool      `json:"recurring"`
	MaxRuns   int       `json:"maxRuns,omitempty"` //nolint:tagliatelle // camelCase mirrors the captured schema field
	LastFired time.Time `json:"lastFired"`         //nolint:tagliatelle // mirrors the captured schema field
	RunCount  int       `json:"runCount"`          //nolint:tagliatelle // camelCase mirrors the captured schema field
	CreatedAt time.Time `json:"createdAt"`         //nolint:tagliatelle // mirrors the captured schema field
	// Done marks a spent automation (one-shot fired, or finite run count
	// reached): it stays listed (history) but never fires again.
	Done bool `json:"done,omitempty"`
}

// ScheduleDisplay renders the human schedule phrase for result forms
// (corpus_absent documented default): the cron expression verbatim, or the
// relative "in N minutes" phrasing for one-shots.
func (a *Automation) ScheduleDisplay() string {
	if a.Cron != "" {
		return a.Cron
	}

	return fmt.Sprintf("in %d minutes", a.DelayOnce)
}

// FireEvent is one catch-up firing: the automation plus the missed-window
// note injected into its prompt context (D-02: each catch-up firing carries
// the note).
type FireEvent struct {
	Automation Automation
	Note       string
}
