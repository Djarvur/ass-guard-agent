package modesmatrix

import (
	"bytes"
	"strings"
	"testing"
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

	empty := NewRegistry()
	if err := empty.Validate(); err == nil {
		t.Fatal("empty registry validated as complete — the grid is total, holes must fail")
	}

	if missing := empty.Missing(); len(missing) != len(AllCells) {
		t.Errorf("Missing() = %d cells; want all %d", len(missing), len(AllCells))
	}

	// Eleven of twelve: the hole is named.
	almost := NewRegistry()
	for _, c := range AllCells {
		if c == (Cell{Mode: ModeWake, Surface: SurfaceHooks}) {
			continue // leave exactly wake x hooks unresolved
		}

		almost.Record(c.Mode, c.Surface, StatusPass, "cell")
	}

	err := almost.Validate()
	if err == nil {
		t.Fatal("an 11/12 registry validated as complete — the unresolved cell must fail the claim")
	}

	if !strings.Contains(err.Error(), (Cell{Mode: ModeWake, Surface: SurfaceHooks}).String()) {
		t.Errorf("Validate's error does not name the missing cell: %v", err)
	}

	// Twelve of twelve: clean.
	complete := NewRegistry()
	for _, c := range AllCells {
		complete.Record(c.Mode, c.Surface, StatusPass, "cell")
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

	r := NewRegistry()
	r.Record(ModeInteractive, SurfaceHooks, StatusPass, "reason")

	var buf bytes.Buffer

	if err := r.Report(&buf); err == nil {
		t.Fatal("partial registry's Report returned no totality error")
	}

	out := buf.String()

	// The grid pads columns; collapse runs of spaces before matching.
	collapsed := strings.Join(strings.Fields(out), " ")

	for _, c := range AllCells {
		if !strings.Contains(collapsed, string(c.Mode)+" x "+string(c.Surface)) {
			t.Errorf("report omits cell %s", c)
		}
	}

	if !strings.Contains(out, "not-exercised") {
		t.Error("unrecorded cells do not print an explicit not-exercised status")
	}

	if !strings.Contains(out, "totals: 1/12 cells recorded") {
		t.Errorf("totals line missing or wrong: %q", strings.SplitN(out, "\n", 0))
	}
}

// TestSkipPreconditionMessagePrefix pins the loud-skip contract: the skip
// message BEGINS with PRECONDITION-UNMET(<phase>, <contract-ref>) — the
// grep-able vocabulary the wake cells' count assertion depends on — and the
// helper both records the cell and skips the test.
func TestSkipPreconditionMessagePrefix(t *testing.T) {
	t.Parallel()

	msg := PreconditionMessage("22", "background wake-turn D-01", ModeWake, SurfaceCommands, "detail here")
	if !strings.HasPrefix(msg, "PRECONDITION-UNMET(22, background wake-turn D-01) wake x commands:") {
		t.Errorf("PreconditionMessage prefix wrong: %q", msg)
	}

	r := NewRegistry()

	skipped := make(chan struct{})

	t.Run("helper", func(t *testing.T) {
		defer close(skipped)

		SkipPrecondition(t, r, ModeWake, SurfaceCommands, "22", "background wake-turn D-01",
			"once Phase 22 executes: dispatch a background subagent, let the wake drain run, assert the fixture surfaces")
		t.Error("SkipPrecondition returned without skipping — the loud skip is the contract")
	})

	<-skipped

	res := r.Snapshot()[Cell{Mode: ModeWake, Surface: SurfaceCommands}]
	if res.Status != StatusPreconditionUnmet {
		t.Errorf("cell status = %q; want precondition-unmet", res.Status)
	}

	if !strings.HasPrefix(res.Reason, "22 background wake-turn D-01:") {
		t.Errorf("cell reason does not cite phase+contract: %q", res.Reason)
	}
}
