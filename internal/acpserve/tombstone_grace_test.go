package acpserve

// 18-04 (D-09) TestTombstoneGraceConfig: the tombstoneGraceDays option on
// the Phase-16 config surface — default 30d, integer >= 1 validation with
// the typed 16-D-09 reject, persist through the layer surface, and the next
// sweep reading the configured grace. Lives HERE (not in internal/session's
// tombstone_test.go): the option surface is the acpserve ConfigSurface, and
// a session-package test cannot import acpserve (acpserve imports session).

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// graceFixture writes one conforming transcript under work/.ass-guard/,
// returning its path.
func graceFixture(t *testing.T, work, sid string) string {
	t.Helper()

	store := filepath.Join(work, ".ass-guard")

	err := os.MkdirAll(store, 0o750)
	if err != nil {
		t.Fatalf("mkdir store: %v", err)
	}

	path := filepath.Join(store, "transcript_"+sid+".jsonl")

	body := `{"type":"session_start","timestamp":"2026-08-01T10:00:00Z","text":"` + sid + `"}` + "\n"

	err = os.WriteFile(path, []byte(body), 0o600)
	if err != nil {
		t.Fatalf("write fixture transcript: %v", err)
	}

	return path
}

// TestTombstoneGraceConfig drives set-then-apply through the Phase-16
// set_config_option surface: the value persists on the addressed layer,
// the refreshed menu advertises it, EffectiveTombstoneGrace (where
// acpserve's startup sweep reads the grace) follows it, and invalid input
// is the typed ConfigViolationError with violation detail (16-D-09).
//
//nolint:funlen,gocyclo,cyclop // one ordered config round-trip end-to-end
func TestTombstoneGraceConfig(t *testing.T) {
	t.Parallel()

	globalPath := filepath.Join(t.TempDir(), "config.yaml")
	projectPath := filepath.Join(t.TempDir(), "config.yaml")

	s := NewConfigSurface(globalPath, projectPath, "zai", &strings.Builder{})

	if got := s.EffectiveTombstoneGrace(); got != session.DefaultTombstoneGrace {
		t.Errorf("default grace = %v; want %v (D-09)", got, session.DefaultTombstoneGrace)
	}

	for _, bad := range []string{"abc", "1.5", "0", "-3"} {
		_, err := s.Set("sess", optTombstoneGrace, bad)

		var violation *acp.ConfigViolationError
		if !errors.As(err, &violation) {
			t.Errorf("Set(%q) error = %v; want *acp.ConfigViolationError (16-D-09)", bad, err)

			continue
		}

		if violation.Violation == "" {
			t.Errorf("Set(%q) rejection carries no violation detail", bad)
		}
	}

	frames, err := s.Set("sess", optTombstoneGrace, "7")
	if err != nil {
		t.Fatalf("Set(tombstoneGraceDays, 7): %v", err)
	}

	found := false

	for _, f := range frames {
		if f.ID == optTombstoneGrace {
			found = true

			if f.CurrentValue != "7" {
				t.Errorf("advertised current = %q; want 7 after the applied write", f.CurrentValue)
			}
		}
	}

	if !found {
		t.Errorf("refreshed menu missing the %q entry", optTombstoneGrace)
	}

	if got := s.EffectiveTombstoneGrace(); got != 7*24*time.Hour {
		t.Errorf("effective grace after Set(7) = %v; want 7d (the sweep reads this)", got)
	}

	// Apply on the NEXT sweep: a 10d-old tombstoned pair survives the 30d
	// default but purges under the configured 7d.
	work := t.TempDir()
	sid := "12345678-1234-4123-8123-123456789abc"

	transcript := graceFixture(t, work, sid)

	marker := filepath.Join(work, ".ass-guard", sid+".deleted")

	err = os.WriteFile(marker, nil, 0o600)
	if err != nil {
		t.Fatalf("write marker: %v", err)
	}

	tenDaysAgo := time.Now().UTC().Add(-10 * 24 * time.Hour)

	if cerr := os.Chtimes(marker, tenDaysAgo, tenDaysAgo); cerr != nil {
		t.Fatalf("chtimes marker: %v", cerr)
	}

	if _, serr := session.SweepTombstones(work, session.DefaultTombstoneGrace, time.Now().UTC()); serr != nil {
		t.Fatalf("default-grace sweep: %v", serr)
	}

	if _, lerr := os.Stat(transcript); lerr != nil {
		t.Fatal("10d-old tombstoned pair purged under the 30d default (grace window broken)")
	}

	removed, serr := session.SweepTombstones(work, s.EffectiveTombstoneGrace(), time.Now().UTC())
	if serr != nil {
		t.Fatalf("configured-grace sweep: %v", serr)
	}

	if len(removed) != 1 || removed[0] != sid {
		t.Errorf("configured-grace sweep removed = %v; want [%s]", removed, sid)
	}
}
