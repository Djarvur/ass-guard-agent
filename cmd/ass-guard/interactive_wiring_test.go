package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/coreexec"
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// TestInteractiveWiring_FamilyExecutes (12-04 Task 2, Test 5): through the
// REAL per-session registration path, every interactive-family catalog entry
// carries Execute (the plan-mode pair + the messaging pair) — the growing
// dead-end-removal proof at the wiring level.
func TestInteractiveWiring_FamilyExecutes(t *testing.T) {
	t.Parallel()

	catalog := toolcat.NewCatalog()
	coreexec.RegisterInteractive(catalog, coreexec.InteractiveConfig{
		PlanMode: nil, // nil-tolerant: registration still lands Execute
		Mailbox:  coreexec.NewAgentMailbox(),
		Sessions: coreexec.NewSessionReader(t.TempDir()),
	})

	for _, name := range []string{"EnterPlanMode", "ExitPlanMode", "SendMessage", "ReadSessionContext"} {
		tool, ok := catalog.Get(name)
		if !ok {
			t.Fatalf("catalog missing %s", name)
		}

		if tool.Execute == nil {
			t.Errorf("%s Execute nil after RegisterInteractive (a dead end the plan removes)", name)
		}
	}
}

// TestInteractiveWiring_SendMessageEndToEnd (Test 5's e2e half): a registered
// live agent receives the model's message through the REAL executor; the ack
// form names the agent (the wiring-level proof the catalog path delivers).
func TestInteractiveWiring_SendMessageEndToEnd(t *testing.T) {
	t.Parallel()

	catalog := toolcat.NewCatalog()
	mailbox := coreexec.NewAgentMailbox()

	var got coreexec.AgentMessage

	mailbox.Register("agent_wiring-0000-4000-8000-000000000001", func(m coreexec.AgentMessage) error {
		got = m

		return nil
	})

	coreexec.RegisterInteractive(catalog, coreexec.InteractiveConfig{Mailbox: mailbox})

	tool, ok := catalog.Get("SendMessage")
	if !ok || tool.Execute == nil {
		t.Fatal("SendMessage not registered")
	}

	input := `{"to":"agent_wiring-0000-4000-8000-000000000001",` +
		`"summary":"status check","message":"how far along are you?"}`

	out, err := tool.Execute(context.Background(), json.RawMessage(input))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if got.Message != "how far along are you?" {
		t.Errorf("delivered = %+v; want byte-faithful", got)
	}

	var ack string

	_ = json.Unmarshal(out, &ack)

	if !strings.Contains(ack, "agent_wiring-0000-4000-8000-000000000001") {
		t.Errorf("ack = %q; want the agent named", ack)
	}
}
