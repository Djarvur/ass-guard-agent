package modesmatrix_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/modesmatrix"
)

// The totality gate (24-05 Task 3): the 12-cell grid is TOTAL — a registry
// that presents the ECOS-04 state cannot silently omit a cell, and a cell
// registered but never resolved fails the report (T-24-05-03: a fabricated
// green is structurally impossible when the grid itself refuses holes).

// TestMatrixTotality pins Validate/AssertComplete's contract: an empty
// registry is incomplete; eleven recorded cells still fail (the missing one
// is NAMED); all twelve resolve to clean.
func TestMatrixTotality(t *testing.T) {
	t.Parallel()

	empty := modesmatrix.NewRegistry()
	if err := empty.Validate(); err == nil {
		t.Fatal("empty registry validated as complete — the grid is total, holes must fail")
	}

	if missing := empty.Missing(); len(missing) != len(modesmatrix.AllCells) {
		t.Errorf("Missing() = %d cells; want all %d", len(missing), len(modesmatrix.AllCells))
	}

	// Eleven of twelve: the hole is named.
	almost := modesmatrix.NewRegistry()
	for _, c := range modesmatrix.AllCells {
		if c == (modesmatrix.Cell{Mode: modesmatrix.ModeWake, Surface: modesmatrix.SurfaceHooks}) {
			continue // leave exactly wake x hooks unresolved
		}

		almost.Record(c.Mode, c.Surface, modesmatrix.StatusPass, "cell")
	}

	err := almost.Validate()
	if err == nil {
		t.Fatal("an 11/12 registry validated as complete — the unresolved cell must fail the claim")
	}

	missingCell := (modesmatrix.Cell{
		Mode: modesmatrix.ModeWake, Surface: modesmatrix.SurfaceHooks,
	}).String()

	if !strings.Contains(err.Error(), missingCell) {
		t.Errorf("Validate's error does not name the missing cell %q: %v", missingCell, err)
	}

	// Twelve of twelve: clean.
	complete := modesmatrix.NewRegistry()
	for _, c := range modesmatrix.AllCells {
		complete.Record(c.Mode, c.Surface, modesmatrix.StatusPass, "cell")
	}

	if err := complete.Validate(); err != nil {
		t.Errorf("complete registry failed Validate: %v", err)
	}
}

// TestMatrixReportRendersAllCells pins the report's totality: every one of
// the twelve coordinates prints with an explicit status (recorded or
// not-exercised), plus the totals line.
func TestMatrixReportRendersAllCells(t *testing.T) {
	t.Parallel()

	r := modesmatrix.NewRegistry()
	r.Record(modesmatrix.ModeInteractive, modesmatrix.SurfaceHooks, modesmatrix.StatusPass, "reason")

	var buf bytes.Buffer

	if err := r.Report(&buf); err == nil {
		t.Fatal("partial registry's Report returned no totality error")
	}

	out := buf.String()

	// The grid pads columns; collapse runs of spaces before matching.
	collapsed := strings.Join(strings.Fields(out), " ")

	for _, c := range modesmatrix.AllCells {
		if !strings.Contains(collapsed, string(c.Mode)+" x "+string(c.Surface)) {
			t.Errorf("report omits cell %s", c)
		}
	}

	if !strings.Contains(out, "not-exercised") {
		t.Error("unrecorded cells do not print an explicit not-exercised status")
	}

	if !strings.Contains(out, "totals: 1/12 cells recorded") {
		t.Error("totals line missing or wrong")
	}
}

// TestSkipPreconditionMessagePrefix pins the loud-skip contract: the skip
// message BEGINS with PRECONDITION-UNMET(<phase>, <contract-ref>) — the
// grep-able vocabulary the wake cells' count assertion depends on — and the
// helper both records the cell and skips the test.
//
//nolint:paralleltest // the skip-helper subtest must run synchronously (channel-synced skip observation)
func TestSkipPreconditionMessagePrefix(t *testing.T) {
	msg := modesmatrix.PreconditionMessage(
		"22", "background wake-turn D-01", modesmatrix.ModeWake, modesmatrix.SurfaceCommands, "detail here")

	if !strings.HasPrefix(msg, "PRECONDITION-UNMET(22, background wake-turn D-01) wake x commands:") {
		t.Errorf("PreconditionMessage prefix wrong: %q", msg)
	}

	r := modesmatrix.NewRegistry()

	skipped := make(chan struct{})

	t.Run("helper", func(t *testing.T) {
		defer close(skipped)

		modesmatrix.SkipPrecondition(t, r, modesmatrix.ModeWake, modesmatrix.SurfaceCommands,
			"22", "background wake-turn D-01",
			"once Phase 22 executes: dispatch a background subagent, let the wake drain run, "+
				"assert the fixture surfaces")

		t.Error("SkipPrecondition returned without skipping — the loud skip is the contract")
	})

	<-skipped

	res := r.Snapshot()[modesmatrix.Cell{Mode: modesmatrix.ModeWake, Surface: modesmatrix.SurfaceCommands}]
	if res.Status != modesmatrix.StatusPreconditionUnmet {
		t.Errorf("cell status = %q; want precondition-unmet", res.Status)
	}

	if !strings.HasPrefix(res.Reason, "22 background wake-turn D-01:") {
		t.Errorf("cell reason does not cite phase+contract: %q", res.Reason)
	}
}
