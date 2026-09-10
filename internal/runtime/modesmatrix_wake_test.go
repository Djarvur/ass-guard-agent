package runtime //nolint:testpackage // internal package test

import (
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/modesmatrix"
)

// The ECOS-04 (TAIL-03, 24-05) wake-mode cells: the background wake-turn is
// Phase 22's D-01 contract, and Phase 22 is PLANNED-BUT-UNEXECUTED on the
// roadmap this harness ships against — so the three wake cells carry NO
// functional evidence and say so LOUDLY (T-24-05-03: a silent pass or a
// fabricated green is the plan's namesake prohibition). Each cell cites the
// contract it waits on and states, in prose, exactly which functional
// assertion replaces the skip once Phase 22 executes — no exercise bodies
// are faked against APIs whose substrate phase has not shipped (Pitfall 9).
//
// The plan's verify command counts the PRECONDITION-UNMET(22 lines in this
// file's -v output and requires EXACTLY three — the count assertion is the
// loudness proof.
//
// Replacement assertions (per surface), for the executor of Phase 22:
//   - wake x commands: a completed background subagent wakes the model; the
//     wake turn's input (the <task-notification> block) rides the turn with
//     a leading /matrix-echo invocation expanding through the SAME adapter
//     seam the automation row proves — assert the transcript's wake-turn
//     user message carries the expanded MATRIX-ECHO-EXPANSION body.
//   - wake x skills: the same wake turn over /matrix-skill — assert the
//     SKILL.md body expansion in the wake turn's user message.
//   - wake x hooks: the wake turn's tool call consults the gate head and
//     the fixture's PreToolUse hook fires — assert the marker file inside
//     the temp project receives the stdin JSON, exactly as the automation
//     hooks cell does, plus the SubagentStop firing at the completing
//     background dispatch.
//
// The registry row prints alongside the automation row's cells (this test
// binary's combined report).

// TestModesMatrixWake registers the three wake cells as precondition-unmet
// against 22-CONTEXT D-01 and skips each subtest loudly.
func TestModesMatrixWake(t *testing.T) {
	t.Parallel() // resumes after the serial cron cells record — the shared registry prints both rows

	t.Run("commands", func(t *testing.T) {
		t.Parallel()

		// Phase 22 executes → replace with the wake x commands assertion
		// stated in the file comment above (expanded MATRIX-ECHO-EXPANSION
		// body in the wake turn's user message).
		modesmatrix.SkipPrecondition(t, cronRegistry,
			modesmatrix.ModeWake, modesmatrix.SurfaceCommands,
			"22", "background wake-turn D-01",
			"once the wake turn is live: the woken turn's /matrix-echo invocation must expand "+
				"(assert the wake-turn user message carries the MATRIX-ECHO-EXPANSION body)")
	})

	t.Run("skills", func(t *testing.T) {
		t.Parallel()

		// Phase 22 executes → replace with the wake x skills assertion
		// (MATRIX-SKILL-BODY expansion in the wake turn's user message).
		modesmatrix.SkipPrecondition(t, cronRegistry,
			modesmatrix.ModeWake, modesmatrix.SurfaceSkills,
			"22", "background wake-turn D-01",
			"once the wake turn is live: the woken turn's /matrix-skill invocation must expand "+
				"(assert the wake-turn user message carries the MATRIX-SKILL-BODY)")
	})

	t.Run("hooks", func(t *testing.T) {
		t.Parallel()

		// Phase 22 executes → replace with the wake x hooks assertion
		// (PreToolUse marker inside the wake turn + SubagentStop at the
		// completing background dispatch).
		modesmatrix.SkipPrecondition(t, cronRegistry,
			modesmatrix.ModeWake, modesmatrix.SurfaceHooks,
			"22", "background wake-turn D-01",
			"once the wake turn is live: the woken turn's tool call must fire the fixture "+
				"PreToolUse hook (marker file) and the completing dispatch must fire SubagentStop")
	})

	// Cleanup (not the body tail): the subtests are parallel — the report
	// prints once they have all recorded.
	t.Cleanup(func() { cronRegistry.ReportT(t) })
}
