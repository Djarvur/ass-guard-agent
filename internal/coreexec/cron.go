package coreexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Djarvur/ass-guard-agent/internal/sched"
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// The cron quartet (12-07, ACP-04 / D-02) over the real persisted
// ScheduleStore. FORM ROUTING: the 12-05 harvest caught NO successful cron
// calls in zcode 0.16.3 — every attempt was plan-mode-blocked, and the one
// CronList capture is zcode's OWN automation/list zod bug (reproduced across
// 3 sessions; the fixture note: "ass-guard's 12-07 cron executors do NOT
// mimic the bug"). Every result form below is therefore a DOCUMENTED
// corpus_absent default (the ACP-07 routing), shaped on the sibling captured
// ack forms (plain-string acks like TaskStop's, a JSON array listing like
// TodoRead's).

var (
	errNoScheduleStore = errors.New("coreexec: no schedule store configured")
	errCronInput       = errors.New("coreexec: cron input invalid")
	errCronNotFound    = errors.New("coreexec: cron automation not found")
	errCronFailed      = errors.New("coreexec: cron operation failed")
)

// cronCreateArgs is the CronCreate input subset the executor consumes.
type cronCreateArgs struct {
	Cron         string `json:"cron"`
	DelayMinutes *int   `json:"delayMinutes"` //nolint:tagliatelle // camelCase mirrors the captured schema
	Prompt       string `json:"prompt"`
	Title        string `json:"title"`
	Recurring    *bool  `json:"recurring"`
	MaxRuns      *int   `json:"maxRuns"` //nolint:tagliatelle // camelCase mirrors the captured schema
}

// CronCreateExecute validates the schema constraints (prompt + title
// required; cron XOR delayMinutes — the captured schema's rule that a
// relative delay NEVER becomes a fixed clock time), persists the automation,
// and returns the id-bearing ack form.
func CronCreateExecute(store *sched.ScheduleStore) toolcat.Stub {
	return func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
		if store == nil {
			return marshalStructured("croncreate: no schedule store configured", errNoScheduleStore)
		}

		var a cronCreateArgs

		err := json.Unmarshal(args, &a)
		if err != nil {
			return marshalStructured("croncreate: invalid input", errCronInput)
		}

		delay := 0
		if a.DelayMinutes != nil {
			delay = *a.DelayMinutes
		}

		recurring := true // the schema's documented default
		if a.Recurring != nil {
			recurring = *a.Recurring
		}

		maxRuns := 0
		if a.MaxRuns != nil {
			maxRuns = *a.MaxRuns
		}

		created, cerr := store.Create(&sched.Automation{
			Title: a.Title, Prompt: a.Prompt, Cron: a.Cron,
			DelayOnce: delay, Recurring: recurring, MaxRuns: maxRuns,
		})
		if cerr != nil {
			return marshalStructured("croncreate: "+cerr.Error(), errCronFailed)
		}

		ack := fmt.Sprintf(
			"Scheduled automation created with ID: %s. Title: %q. Schedule: %s. "+
				"It fires as a scheduled turn while an ass-guard session for this project is active; "+
				"missed fires catch up once.",
			created.ID, created.Title, created.ScheduleDisplay(),
		)

		return json.Marshal(ack)
	}
}

// cronListEntry is one CronList row (the documented corpus_absent default
// shape: a JSON array of entry objects carrying the fields the model needs
// to manage the automations — the TodoRead array precedent).
type cronListEntry struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Schedule  string `json:"schedule"`
	Prompt    string `json:"prompt"`
	NextDue   string `json:"nextDue,omitempty"`   //nolint:tagliatelle // camelCase form vocabulary
	LastFired string `json:"lastFired,omitempty"` //nolint:tagliatelle // camelCase form vocabulary
	Status    string `json:"status"`
}

// CronListExecute returns the listing (the captured zod_error is zcode's own
// validation defect, deliberately NOT mimicked — the fixture note).
func CronListExecute(store *sched.ScheduleStore) toolcat.Stub {
	return func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
		if store == nil {
			return marshalStructured("cronlist: no schedule store configured", errNoScheduleStore)
		}

		now := store.Now()
		list := store.List()
		entries := make([]cronListEntry, 0)

		for i := range list {
			a := &list[i]

			status := "scheduled"
			if a.Done {
				status = "finished"
			}

			entry := cronListEntry{
				ID: a.ID, Title: a.Title, Schedule: a.ScheduleDisplay(),
				Prompt: a.Prompt, Status: status,
			}

			if nd, ok := sched.NextDue(a, now); ok {
				//nolint:gosmopolitan // schema-mandated local rendering
				entry.NextDue = nd.Local().Format("2006-01-02 15:04")
			}

			if !a.LastFired.IsZero() {
				//nolint:gosmopolitan // schema-mandated local rendering
				entry.LastFired = a.LastFired.Local().Format("2006-01-02 15:04")
			}

			entries = append(entries, entry)
		}

		return json.Marshal(entries)
	}
}

// cronUpdateArgs is the CronUpdate input subset (id + title required — the
// schema makes the synchronized title structural).
type cronUpdateArgs struct {
	ID        string          `json:"id"`
	Cron      string          `json:"cron"`
	Delay     *int            `json:"delayMinutes"` //nolint:tagliatelle // camelCase mirrors the captured schema
	Prompt    string          `json:"prompt"`
	Title     string          `json:"title"`
	Recurring *bool           `json:"recurring"`
	MaxRuns   *int            `json:"maxRuns"` //nolint:tagliatelle // camelCase mirrors the captured schema
	Raw       map[string]bool `json:"-"`       // which optional fields were present
}

// CronUpdateExecute patches the automation (omit = preserve). The
// synchronized-title check: the schema REQUIRES a title on every update;
// when the schedule changes but the supplied title is byte-identical to the
// old one, the ack carries the structured guidance note — advice, not a hard
// block (the no-confirmation-tier model).
func CronUpdateExecute(store *sched.ScheduleStore) toolcat.Stub {
	return func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
		if store == nil {
			return marshalStructured("cronupdate: no schedule store configured", errNoScheduleStore)
		}

		var a cronUpdateArgs

		err := json.Unmarshal(args, &a)
		if err != nil {
			return marshalStructured("cronupdate: invalid input", errCronInput)
		}

		present := map[string]bool{}

		var raw map[string]json.RawMessage

		uerr := json.Unmarshal(args, &raw)
		if uerr == nil {
			for k := range raw {
				present[k] = true
			}
		}

		if a.ID == "" {
			return marshalStructured("cronupdate: id required", errCronInput)
		}

		if a.Title == "" {
			return marshalStructured("cronupdate: title required (synchronized title)", errCronInput)
		}

		cur, gerr := store.Get(a.ID)
		if gerr != nil {
			return marshalStructured("cronupdate: "+gerr.Error(), errCronNotFound)
		}

		oldCron, oldDelay, oldTitle := cur.Cron, cur.DelayOnce, cur.Title
		applyCronUpdate(&cur, &a, present)

		scheduleChanged := cur.Cron != oldCron || cur.DelayOnce != oldDelay
		titleGuidance := scheduleChanged && a.Title == oldTitle

		serr := store.Save(&cur)
		if serr != nil {
			return marshalStructured("cronupdate: "+serr.Error(), errCronFailed)
		}

		ack := fmt.Sprintf("Scheduled automation %s updated. Title: %q. Schedule: %s.",
			cur.ID, cur.Title, cur.ScheduleDisplay())

		if titleGuidance {
			ack += " Note: the schedule changed while the title stayed the same — " +
				"CronUpdate requires a title synchronized with the schedule's meaning; " +
				"update the title if its natural-language phrase is now stale."
		}

		return json.Marshal(ack)
	}
}

// applyCronUpdate folds the patch into cur (omit = preserve).
func applyCronUpdate(cur *sched.Automation, a *cronUpdateArgs, present map[string]bool) {
	if present["cron"] && a.Cron != "" {
		cur.Cron = a.Cron
		cur.DelayOnce = 0
	}

	if present["delayMinutes"] && a.Delay != nil {
		cur.DelayOnce = *a.Delay
		cur.Cron = ""
	}

	if a.Prompt != "" {
		cur.Prompt = a.Prompt
	}

	cur.Title = a.Title

	if a.Recurring != nil {
		cur.Recurring = *a.Recurring
		if *a.Recurring {
			cur.MaxRuns = 0
		}
	}

	if a.MaxRuns != nil {
		cur.MaxRuns = *a.MaxRuns
	}
}

// CronDeleteExecute removes the automation and acks.
func CronDeleteExecute(store *sched.ScheduleStore) toolcat.Stub {
	return func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
		if store == nil {
			return marshalStructured("crondelete: no schedule store configured", errNoScheduleStore)
		}

		var a struct {
			ID string `json:"id"`
		}

		err := json.Unmarshal(args, &a)
		if err != nil || a.ID == "" {
			return marshalStructured("crondelete: id required", errCronInput)
		}

		derr := store.Delete(a.ID)
		if derr != nil {
			return marshalStructured("crondelete: "+derr.Error(), errCronNotFound)
		}

		return json.Marshal("Scheduled automation " + a.ID + " deleted.")
	}
}
