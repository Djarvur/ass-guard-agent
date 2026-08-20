package sched_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/sched"
)

// fixedNow pins the injected clock: 2026-08-20 09:15 local-equivalent (UTC
// here — the parser honors the injected location's wall clock).
func fixedNow() time.Time { return time.Date(2026, 8, 20, 9, 15, 0, 0, time.UTC) }

func openStore(t *testing.T) *sched.ScheduleStore {
	t.Helper()

	s, err := sched.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	return s
}

// TestParser_NextDue pins the 5-field parser against the schema's own
// examples, in LOCAL wall-clock time (the "Do not convert to UTC" rule), with
// an invalid expression returning a structured error (never a panic).
func TestParser_NextDue(t *testing.T) {
	t.Parallel()

	now := fixedNow()

	cases := []struct {
		expr string
		want time.Time
	}{
		{"*/20 * * * *", time.Date(2026, 8, 20, 9, 20, 0, 0, time.UTC)},
		{"0 * * * *", time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)},
		{"0 9 * * 1-5", time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC)}, // 08-20 is a Thursday: next weekday slot is Friday
		{"30 9 21 8 *", time.Date(2026, 8, 21, 9, 30, 0, 0, time.UTC)},
	}

	for _, tc := range cases {
		spec, err := sched.ParseCron(tc.expr)
		if err != nil {
			t.Errorf("ParseCron(%q): %v", tc.expr, err)

			continue
		}

		if got := spec.Next(now, time.UTC); !got.Equal(tc.want) {
			t.Errorf("Next(%q) = %s, want %s", tc.expr, got, tc.want)
		}
	}

	if _, err := sched.ParseCron("not a cron"); err == nil {
		t.Error("ParseCron(invalid) = nil error, want structured error")
	}

	if _, err := sched.ParseCron("61 * * * *"); err == nil {
		t.Error("ParseCron(61 minutes) = nil error, want bounds error")
	}
}

// TestDelayOnce_NextDue: delayMinutes computes a ONE-SHOT due from the
// creation instant (the schema's relative-delay rule — never a fixed clock).
func TestDelayOnce_NextDue(t *testing.T) {
	t.Parallel()

	a := sched.Automation{DelayOnce: 5, CreatedAt: fixedNow(), Recurring: true}
	due, ok := sched.NextDue(&a, fixedNow())
	if !ok || !due.Equal(fixedNow().Add(5*time.Minute)) {
		t.Fatalf("NextDue(delay) = %s ok=%v, want +5m", due, ok)
	}

	// After it fired once, a delay automation is DONE (never re-fires).
	a.Done = true
	if _, ok := sched.NextDue(&a, fixedNow().Add(time.Hour)); ok {
		t.Error("NextDue(fired one-shot) = due, want done")
	}
}

// TestStore_CRUDRoundTrip: create → list → update → delete with the store
// file on disk reflecting each step (0600 perms asserted).
func TestStore_CRUDRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s, err := sched.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	created, err := s.Create(sched.Automation{Title: "drink water", Prompt: "remind me", DelayOnce: 5})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if created.ID == "" || created.CreatedAt.IsZero() {
		t.Fatalf("Create returned %+v — id/createdAt must be assigned", created)
	}

	path := filepath.Join(dir, ".ass-guard", "schedule", "schedules.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("store file: %v", err)
	}

	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("store perms = %o, want 0600", perm)
	}

	if got := s.List(); len(got) != 1 || got[0].ID != created.ID {
		t.Fatalf("List = %+v, want the created automation", got)
	}

	a := created
	a.Cron = "*/10 * * * *"
	a.DelayOnce = 0
	a.Title = "every 10m water"

	if err := s.Save(a); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if got := s.List(); got[0].Cron != "*/10 * * * *" || got[0].Title != "every 10m water" {
		t.Errorf("after Save: %+v", got[0])
	}

	if err := s.Delete(a.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if got := s.List(); len(got) != 0 {
		t.Errorf("after Delete: %+v, want empty", got)
	}
}

// TestStore_RoundTripPersistence: a close/reopen round-trip preserves
// automations + lastFired exactly (the catch-up dedup depends on it).
func TestStore_RoundTripPersistence(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s1, err := sched.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	created, err := s1.Create(sched.Automation{Title: "t", Prompt: "p", Cron: "0 * * * *"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	firedAt := fixedNow().Add(-2 * time.Hour)
	if err := s1.MarkFired(created.ID, firedAt); err != nil {
		t.Fatalf("MarkFired: %v", err)
	}

	s2, err := sched.Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}

	got := s2.List()
	if len(got) != 1 || !got[0].LastFired.Equal(firedAt) || got[0].RunCount != 1 {
		t.Fatalf("round-trip = %+v, want lastFired+runCount preserved", got)
	}
}

// TestStore_CorruptFileQuarantined: a corrupt store file never crashes or
// blocks — it is quarantined and a fresh store opens (graceful degradation).
func TestStore_CorruptFileQuarantined(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".ass-guard", "schedule", "schedules.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := sched.Open(dir)
	if err != nil {
		t.Fatalf("Open(corrupt): %v", err)
	}

	if got := s.List(); len(got) != 0 {
		t.Errorf("List(corrupt) = %+v, want fresh empty", got)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("fresh store not written after quarantine: %v", err)
	}
}

// TestStore_DueAndCatchUp: the due walker + the fire-once catch-up with the
// missed-window note — and lastFired persistence means a SECOND catch-up pass
// does not fire again (D-02 exactly-once).
func TestStore_DueAndCatchUp(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	now := fixedNow()

	// Overdue by an hour (hourly cron, never fired).
	a, err := s.Create(sched.Automation{Title: "hourly", Prompt: "check builds", Cron: "0 * * * *"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// The scheduler tick path: Due at now (the 09:00 slot passed at 09:15).
	due := s.Due(now)
	if len(due) != 1 || due[0].ID != a.ID {
		t.Fatalf("Due = %+v, want the overdue automation", due)
	}

	if err := s.MarkFired(a.ID, now); err != nil {
		t.Fatal(err)
	}

	if got := s.Due(now); len(got) != 0 {
		t.Errorf("Due after fire = %+v, want empty until next slot", got)
	}

	// Catch-up semantics (fire-once + note + lastFired dedup) are pinned by
	// TestCatchUp_FireOnceWithNote over the reopened-store restart path.
}

// TestCatchUp_FireOnceWithNote drives the catch-up semantics over a reopened
// store (the restart path): each missed automation fires once, carries the
// missed-window note, and a second restart does not fire it again.
func TestCatchUp_FireOnceWithNote(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s1, err := sched.Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	now := fixedNow()

	b, err := s1.Create(sched.Automation{Title: "daily", Prompt: "report", Cron: "0 9 * * *"})
	if err != nil {
		t.Fatal(err)
	}

	_ = s1.SetNow(func() time.Time { return now })

	// Simulate the last fire 72h ago: slots at 09:00 on 08-18/19/20 all missed.
	if err := s1.MarkFired(b.ID, now.Add(-72*time.Hour)); err != nil {
		t.Fatal(err)
	}

	s2, err := sched.Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	// The reopened store sees "now" 1 minute past the last missed slot.
	restarted := now.Add(time.Minute)
	_ = s2.SetNow(func() time.Time { return restarted })

	events := s2.CatchUp(restarted)
	if len(events) != 1 || events[0].Automation.ID != b.ID {
		t.Fatalf("CatchUp = %+v, want exactly one event for the daily automation", events)
	}

	if events[0].Note == "" {
		t.Error("CatchUp note is empty — the missed-window note is required")
	}

	// A second restart must NOT fire it again (lastFired persisted).
	s3, err := sched.Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	_ = s3.SetNow(func() time.Time { return restarted.Add(time.Minute) })

	if events := s3.CatchUp(restarted.Add(time.Minute)); len(events) != 0 {
		t.Errorf("second CatchUp = %+v — lastFired must make catch-up exactly-once", events)
	}
}

// TestStoreJSON_Shape pins the persisted JSON shape (round-trip stable).
func TestStoreJSON_Shape(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s, err := sched.Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.Create(sched.Automation{Title: "t", Prompt: "p", DelayOnce: 8}); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, ".ass-guard", "schedule", "schedules.json"))
	if err != nil {
		t.Fatal(err)
	}

	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("store json: %v", err)
	}

	list, ok := doc["automations"].([]any)
	if !ok || len(list) != 1 {
		t.Fatalf("store json shape = %s, want {automations:[1]}", string(raw))
	}
}
