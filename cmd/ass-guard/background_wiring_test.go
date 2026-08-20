package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/coreexec"
	"github.com/Djarvur/ass-guard-agent/internal/sched"
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// The 12-06 Task 2 wiring-level battery: TaskStop semantics, close-reaps-all,
// per-session isolation, and the landed-so-far catalog-completeness proof.

// TestBackgroundWiring_StopKillsGroup (T2 Test 1): a background sleep 30,
// TaskStop'd mid-flight — the ack names the task, the registry state is
// stopped, and a subsequent Output observes TERMINATION (no pipe-holding
// orphans — the 08-03 discipline).
func TestBackgroundWiring_StopKillsGroup(t *testing.T) {
	t.Parallel()

	reg := coreexec.NewTaskRegistry()
	bashExec := coreexec.BashExecute(coreexec.Config{WorkDir: t.TempDir(), Tasks: reg})

	out, err := bashExec(context.Background(), json.RawMessage(
		`{"command":"sleep 30","run_in_background":true}`))
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	var text string

	_ = json.Unmarshal(out, &text)

	id := text[len("Command running in background with ID: "):]
	if i := strings.IndexAny(id, " ."); i >= 0 {
		id = id[:i]
	}

	stop := coreexec.TaskStopExecute(reg)

	out2, err2 := stop(context.Background(), json.RawMessage(`{"task_id":"`+id+`"}`))
	if err2 != nil {
		t.Fatalf("TaskStop: %v", err2)
	}

	var ack string

	_ = json.Unmarshal(out2, &ack)

	if !strings.Contains(ack, id) || !strings.Contains(ack, "stopped") {
		t.Errorf("ack = %q; want the task named + stopped", ack)
	}

	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		out3, _ := coreexec.TaskOutputExecute(reg)(context.Background(),
			json.RawMessage(`{"task_id":"`+id+`","block":false,"timeout":100}`))

		var state string

		_ = json.Unmarshal(out3, &state)

		if strings.Contains(state, "<status>stopped</status>") {
			return // terminated + observed — pass
		}

		time.Sleep(50 * time.Millisecond)
	}

	t.Fatal("the stopped task never reached the observed terminated state")
}

// TestBackgroundWiring_DeprecatedShellID (T2 Test 2): shell_id resolves
// against the same registry (task_id preferred — the schema's deprecation).
func TestBackgroundWiring_DeprecatedShellID(t *testing.T) {
	t.Parallel()

	reg := coreexec.NewTaskRegistry()

	id, err := reg.Start(t.TempDir(), "sleep 5")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	_, serr := coreexec.TaskStopExecute(reg)(context.Background(), json.RawMessage(`{"shell_id":"`+id+`"}`))
	if serr != nil {
		t.Errorf("shell_id stop: %v; want the deprecated alias accepted", serr)
	}
}

// TestBackgroundWiring_ReapAll (T2 Test 3): two live tasks, ReapAll — both
// groups reaped (state stopped; the OnClose chain in sessionFor calls this).
func TestBackgroundWiring_ReapAll(t *testing.T) {
	t.Parallel()

	reg := coreexec.NewTaskRegistry()

	id1, err1 := reg.Start(t.TempDir(), "sleep 30")
	if err1 != nil {
		t.Fatal(err1)
	}

	id2, err2 := reg.Start(t.TempDir(), "sleep 30")
	if err2 != nil {
		t.Fatal(err2)
	}

	reg.ReapAll()

	for _, id := range []string{id1, id2} {
		if state, ok := reg.Lookup(id); !ok || string(state) != "stopped" {
			t.Errorf("task %s state = %q ok=%v; want stopped after ReapAll", id, state, ok)
		}
	}
}

// TestBackgroundWiring_CrossSessionIsolation (T2 Test 4): session B's TaskStop
// with session A's task id returns the unknown-id error (per-session scoping).
func TestBackgroundWiring_CrossSessionIsolation(t *testing.T) {
	t.Parallel()

	regA := coreexec.NewTaskRegistry()
	regB := coreexec.NewTaskRegistry()

	id, err := regA.Start(t.TempDir(), "sleep 5")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	_, serr := coreexec.TaskStopExecute(regB)(context.Background(), json.RawMessage(`{"task_id":"`+id+`"}`))
	if serr == nil {
		t.Error("cross-session stop succeeded; want the unknown-id error (per-session scoping)")
	}
}

// TestBackgroundWiring_CoreCompleteness (12-07 T2 Test 5 — the FULL phase
// gate, cron exception DROPPED): every NON-mcp catalog tool carries Execute
// after the FULL per-session registration path — the complete dead-end
// removal: the 08-08 six, AskUserQuestion, the plan pair, the messaging
// pair, the background trio, and the cron quartet. This is the phase-gate
// dead-end check expressed as a permanent regression test.
func TestBackgroundWiring_CoreCompleteness(t *testing.T) {
	t.Parallel()

	catalog := toolcat.NewCatalog()
	coreexec.RegisterCore(catalog, coreexec.Config{
		WorkDir: t.TempDir(), Todos: coreexec.NewTodoStore(), Tasks: coreexec.NewTaskRegistry(),
	})
	coreexec.RegisterAsk(catalog, nil)

	schedStore, err := sched.Open(t.TempDir())
	if err != nil {
		t.Fatalf("sched.Open: %v", err)
	}

	coreexec.RegisterInteractive(catalog, coreexec.InteractiveConfig{
		PlanMode: nil, Mailbox: coreexec.NewAgentMailbox(),
		Sessions: coreexec.NewSessionReader(t.TempDir()), Tasks: coreexec.NewTaskRegistry(),
		Schedule: schedStore,
	})

	// Exceptions are BY-DESIGN non-catalog execution routes, not dead ends:
	// WebSearch/WebFetch execute via RealExecutor's swappable-backend routing
	// (intercepted BEFORE the catalog — the "no implementation" fallback
	// cannot fire); Agent is the subagent dispatch (routed before the batch,
	// PARA-01); Skill's Execute is set at the wiring site (the 08-05 registry
	// closure, proven by its own battery — not reproducible without the
	// runner's registry).
	exceptions := map[string]bool{
		"WebSearch": true, "WebFetch": true, "Agent": true, skillToolName: true,
	}

	var missing []string

	for _, name := range catalog.Names() {
		if strings.HasPrefix(name, "mcp__") {
			continue // dynamically-registered MCP tools are host-scoped, not core
		}

		if exceptions[name] {
			continue // by-design non-catalog route
		}

		tool, ok := catalog.Get(name)
		if !ok || tool.Execute == nil {
			missing = append(missing, name)
		}
	}

	if len(missing) > 0 {
		t.Errorf("schema-only core tools remain (dead ends): %v", missing)
	}
}
