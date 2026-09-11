package runtime //nolint:testpackage // internal package test

import (
	"context"
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/kit/session"
	"github.com/Djarvur/ass-guard-agent/kit/event"
)

// countingEmitter counts AgentMessageChunk emissions (the kit Emitter twin
// of the former acp.ChunkEmitter fake — same recording, kit vocabulary).
type countingEmitter struct {
	n      int
	chunks []string
}

func (c *countingEmitter) Emit(_ context.Context, ev event.Event) error {
	if chunk, ok := ev.(event.AgentMessageChunk); ok {
		c.n++

		c.chunks = append(c.chunks, chunk.Content)
	}

	return nil
}

// TestAdvisoryWiring_DedupeSemantics (13-03 T2 Test 8, D-05): two consecutive
// same-class advisory turns in one session → ONE client note (the first) +
// TWO audit lines; a DIFFERENT class in the same session → its own first
// note; a NEW session → the note fires again for the same class.
func TestAdvisoryWiring_DedupeSemantics(t *testing.T) { //nolint:gocognit,cyclop,funlen // D-05 battery
	t.Parallel()

	newRunner := func() (*Runner, *countingEmitter) {
		r, _ := newExpansionRunner(t, true,
			scriptedResp{text: "Done. Which would you like? (1/2)", finish: stopEndTurn},
			scriptedResp{text: "Done again. Which would you like? (1/2)", finish: stopEndTurn},
			scriptedResp{text: "Pick one — just say the word.", finish: stopEndTurn},
		)

		em := &countingEmitter{}

		return r, em
	}

	t.Run("same class twice one session", func(t *testing.T) {
		t.Parallel()

		r, em := newRunner()

		const sid = "sess-adv-dedupe"

		_, err := r.Run(context.Background(), sid, em, []session.ContentBlock{{Type: blockText, Text: "go"}})
		if err != nil {
			t.Fatalf("Run 1: %v", err)
		}

		_, err = r.Run(context.Background(), sid, em, []session.ContentBlock{{Type: blockText, Text: "again"}})
		if err != nil {
			t.Fatalf("Run 2: %v", err)
		}

		advisoryNotes := 0

		for _, c := range em.chunks {
			if strings.Contains(c, "AskUserQuestion") {
				advisoryNotes++
			}
		}

		if advisoryNotes != 1 {
			t.Errorf("advisory notes = %d; want exactly 1 (first-per-class-per-session visible)", advisoryNotes)
		}

		// BOTH audit lines exist (repeats are audit-trail-only).
		if got := countAdvisoryDecisions(t, r, sid); got != 2 {
			t.Errorf("advisory engine_decision lines = %d; want 2 (audit-always)", got)
		}
	})

	t.Run("different class same session", func(t *testing.T) {
		t.Parallel()

		r, _ := newExpansionRunner(t, true,
			scriptedResp{text: "Done. Which would you like? (1/2)", finish: stopEndTurn},
			scriptedResp{text: "Pick one — just say the word.", finish: stopEndTurn},
		)

		em := &countingEmitter{}

		const sid = "sess-adv-classes"

		_, err := r.Run(context.Background(), sid, em, []session.ContentBlock{{Type: blockText, Text: "go"}})
		if err != nil {
			t.Fatalf("Run 1: %v", err)
		}

		// Turn 2 closes with the OPEN-QUESTION class phrase.
		_, err = r.Run(context.Background(), sid, em, []session.ContentBlock{{Type: blockText, Text: "next"}})
		if err != nil {
			t.Fatalf("Run 2: %v", err)
		}

		advisoryNotes := 0

		for _, c := range em.chunks {
			if strings.Contains(c, "AskUserQuestion") {
				advisoryNotes++
			}
		}

		if advisoryNotes != 2 {
			t.Errorf("advisory notes = %d; want 2 (one per class)", advisoryNotes)
		}
	})

	t.Run("new session notes again", func(t *testing.T) {
		t.Parallel()

		r, em := newRunner()

		for _, sid := range []string{"sess-adv-a", "sess-adv-b"} {
			_, err := r.Run(context.Background(), sid, em, []session.ContentBlock{{Type: blockText, Text: "go"}})
			if err != nil {
				t.Fatalf("Run (%s): %v", sid, err)
			}
		}

		advisoryNotes := 0

		for _, c := range em.chunks {
			if strings.Contains(c, "AskUserQuestion") {
				advisoryNotes++
			}
		}

		if advisoryNotes != 2 {
			t.Errorf("advisory notes = %d; want 2 (per-session isolation)", advisoryNotes)
		}
	})
}

// countAdvisoryDecisions counts advisory-signal engine_decision lines.
func countAdvisoryDecisions(t *testing.T, r *Runner, sessionID string) int {
	t.Helper()

	lines, err := r.sessions[sessionID].Manager.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	n := 0

	for i := range lines {
		if lines[i].Type == session.TypeEngineDecision &&
			strings.Contains(string(lines[i].Input), "advisory:") {
			n++
		}
	}

	return n
}

// TestAdvisoryWiring_NoteWordingElements (13-03 T2 Test 10): the note names
// the question ending, names AskUserQuestion as the route, and carries NO
// wording that asserts a pending hold.
func TestAdvisoryWiring_NoteWordingElements(t *testing.T) {
	t.Parallel()

	note := advisoryNoteText

	if !strings.Contains(note, "question") {
		t.Errorf("note does not name the question ending: %q", note)
	}

	if !strings.Contains(note, "AskUserQuestion") {
		t.Errorf("note does not name AskUserQuestion as the route: %q", note)
	}

	for _, hold := range []string{"holding", "held", "waiting for your", "pending", "paused"} {
		if strings.Contains(strings.ToLower(note), hold) {
			t.Errorf("note asserts a pending hold (%q): %q", hold, note)
		}
	}
}
