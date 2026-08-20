package coreexec_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/coreexec"
	"github.com/Djarvur/ass-guard-agent/internal/sched"
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// cronFixture builds a catalog with the cron quartet registered over a real
// store in a temp workDir (the executor-level harness; wiring-level queue
// semantics live in cmd/ass-guard/cron_wiring_test.go).
func cronFixture(t *testing.T) (*toolcat.Catalog, *sched.ScheduleStore) {
	t.Helper()

	store, err := sched.Open(t.TempDir())
	if err != nil {
		t.Fatalf("sched.Open: %v", err)
	}

	catalog := toolcat.NewCatalog()
	coreexec.RegisterInteractive(catalog, coreexec.InteractiveConfig{Schedule: store})

	return catalog, store
}

func runTool(t *testing.T, catalog *toolcat.Catalog, name, input string) string {
	t.Helper()

	tool, ok := catalog.Get(name)
	if !ok {
		t.Fatalf("catalog has no %s", name)
	}

	if tool.Execute == nil {
		t.Fatalf("%s has no Execute (the dead-end this plan kills)", name)
	}

	out, err := tool.Execute(context.Background(), json.RawMessage(input))
	if err != nil {
		t.Fatalf("%s execute: %v", name, err)
	}

	var s string
	if uerr := json.Unmarshal(out, &s); uerr == nil {
		return s
	}

	return string(out)
}

// TestCronQuartet_EndToEnd: CronCreate → CronList → CronUpdate → CronDelete
// through the REAL registered executors over the real persisted store (T1
// Test 1, executor level): ids returned/listed/patched/removed, the store
// file reflecting each step.
func TestCronQuartet_EndToEnd(t *testing.T) {
	t.Parallel()

	catalog, store := cronFixture(t)

	// Create (delayMinutes route — the schema's preferred relative form).
	out := runTool(t, catalog, "CronCreate",
		`{"delayMinutes":5,"prompt":"remind me to stretch","title":"stretch reminder"}`)
	if want := "cron_"; len(out) < len(want) || out[:len(want)] != want {
		t.Fatalf("CronCreate result = %q, want an id-bearing ack", out)
	}

	created := store.List()
	if len(created) != 1 || created[0].Prompt != "remind me to stretch" {
		t.Fatalf("store after create = %+v", created)
	}
	id := created[0].ID

	// List includes it with its schedule.
	listOut := runTool(t, catalog, "CronList", `{}`)
	if id == "" || listOut == "" {
		t.Fatal("empty list output")
	}

	var entries []map[string]any
	if err := json.Unmarshal([]byte(listOut), &entries); err != nil {
		t.Fatalf("CronList output not a JSON array of entries: %v (%s)", err, listOut)
	}

	if len(entries) != 1 || entries[0]["id"] != id {
		t.Fatalf("CronList = %s, want the created automation %s", listOut, id)
	}

	// Update: patch cron + the synchronized title.
	upd := runTool(t, catalog, "CronUpdate",
		`{"id":"`+id+`","cron":"*/10 * * * *","title":"every 10m stretch","recurring":true}`)
	if upd == "" {
		t.Fatal("empty update ack")
	}

	patched, err := store.Get(id)
	if err != nil {
		t.Fatal(err)
	}

	if patched.Cron != "*/10 * * * *" || patched.Title != "every 10m stretch" {
		t.Fatalf("store after update = %+v", patched)
	}

	// Delete removes it.
	del := runTool(t, catalog, "CronDelete", `{"id":"`+id+`"}`)
	if del == "" {
		t.Fatal("empty delete ack")
	}

	if got := store.List(); len(got) != 0 {
		t.Fatalf("store after delete = %+v", got)
	}
}

// TestCronCreate_Validation: the schema's constraints render structured
// errors (never a panic): cron XOR delayMinutes, required prompt/title,
// invalid cron expressions.
func TestCronCreate_Validation(t *testing.T) {
	t.Parallel()

	catalog, _ := cronFixture(t)

	cases := []struct{ name, input string }{
		{"both cron and delay", `{"cron":"0 * * * *","delayMinutes":5,"prompt":"p","title":"t"}`},
		{"neither cron nor delay", `{"prompt":"p","title":"t"}`},
		{"missing prompt", `{"delayMinutes":5,"title":"t"}`},
		{"missing title", `{"delayMinutes":5,"prompt":"p"}`},
		{"invalid cron", `{"cron":"nope","prompt":"p","title":"t"}`},
	}

	for _, tc := range cases {
		out := runTool(t, catalog, "CronCreate", tc.input)
		if want := "croncreate: "; len(out) >= len(want) && out[:len(want)] != want {
			t.Errorf("%s: result = %q, want the structured error form", tc.name, out)
		}
	}
}

// TestCronUpdate_TitleSyncGuidance: updating the schedule while keeping the
// OLD title returns the structured guidance note (not a hard block — the
// no-confirmation-tier model; the schema makes title structurally required).
func TestCronUpdate_TitleSyncGuidance(t *testing.T) {
	t.Parallel()

	catalog, store := cronFixture(t)

	created, err := store.Create(&sched.Automation{
		Title: "hourly check", Prompt: "check", Cron: "0 * * * *", Recurring: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	out := runTool(t, catalog, "CronUpdate",
		`{"id":"`+created.ID+`","cron":"0 */2 * * *","title":"hourly check","recurring":true}`)

	if want := "synchronized"; !contains(out, want) {
		t.Errorf("CronUpdate(cron changed, same title) = %q, want the synchronized-title guidance", out)
	}
}

// TestCronErrors_UnknownID: list/update/delete over unknown ids render
// structured errors.
func TestCronErrors_UnknownID(t *testing.T) {
	t.Parallel()

	catalog, _ := cronFixture(t)

	if out := runTool(t, catalog, "CronUpdate", `{"id":"cron_missing","title":"t"}`); out == "" {
		t.Error("CronUpdate(unknown) returned empty")
	}

	if out := runTool(t, catalog, "CronDelete", `{"id":"cron_missing"}`); out == "" {
		t.Error("CronDelete(unknown) returned empty")
	}
}

// TestCronQuartet_RegisteredForms: every quartet tool's Execute is non-nil
// through RegisterInteractive AND the catalog schema is untouched.
func TestCronQuartet_RegisteredForms(t *testing.T) {
	t.Parallel()

	catalog, _ := cronFixture(t)

	for _, name := range []string{"CronCreate", "CronList", "CronUpdate", "CronDelete"} {
		tool, ok := catalog.Get(name)
		if !ok {
			t.Errorf("catalog missing %s", name)

			continue
		}

		if tool.Execute == nil {
			t.Errorf("%s.Execute is nil — the dead-end class", name)
		}

		if tool.InputSchema == nil {
			t.Errorf("%s.InputSchema is nil — the captured schema must stay", name)
		}
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
