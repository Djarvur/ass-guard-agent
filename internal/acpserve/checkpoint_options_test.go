package acpserve //nolint:testpackage // internal package test (readLayerMap/mapHasPath assertions)

import (
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
)

// TestCheckpointOptions pins the D-10 checkpoint GC knobs (23-04): both ids
// advertised as enumerated selects with correct defaults, Set
// membership-validated + persisted under the checkpoint: layer key, and the
// EffectiveCheckpointGCBounds read-back (persisted > embedded defaults, loud
// fallback on junk).
func TestCheckpointOptions(t *testing.T) { //nolint:funlen // comprehensive option family battery
	t.Parallel()

	t.Run("advertisement", func(t *testing.T) {
		t.Parallel()

		f := newSurfaceFixture(t)
		opts := f.surface.Options()

		exp := optionByID(t, opts, optCheckpointExpiryDays)
		if exp.Type != acp.ConfigOptionTypeSelect || exp.CurrentValue != ckExpiryDefault {
			t.Errorf("expiry_days type/current = %q/%q; want select/%q", exp.Type, exp.CurrentValue, ckExpiryDefault)
		}

		if got := valuesOf(exp); strings.Join(got, ",") != "1,3,7,14,30" {
			t.Errorf("expiry_days offered = %v; want [1 3 7 14 30]", got)
		}

		max := optionByID(t, opts, optCheckpointMaxPerSession)
		if max.Type != acp.ConfigOptionTypeSelect || max.CurrentValue != ckMaxDefault {
			t.Errorf("max_per_session type/current = %q/%q; want select/%q", max.Type, max.CurrentValue, ckMaxDefault)
		}

		if got := valuesOf(max); strings.Join(got, ",") != "10,25,50,100" {
			t.Errorf("max_per_session offered = %v; want [10 25 50 100]", got)
		}

		// The global twins advertise too.
		optionByID(t, opts, optGlobalPrefix+optCheckpointExpiryDays)
		optionByID(t, opts, optGlobalPrefix+optCheckpointMaxPerSession)
	})

	t.Run("set persists the checkpoint layer key", func(t *testing.T) {
		t.Parallel()

		f := newSurfaceFixture(t)

		if _, err := f.surface.Set("sess-ck", optCheckpointExpiryDays, "14"); err != nil {
			t.Fatalf("Set expiry=14: %v", err)
		}

		if _, err := f.surface.Set("sess-ck", optCheckpointMaxPerSession, "25"); err != nil {
			t.Fatalf("Set max=25: %v", err)
		}

		m, merr := readLayerMap(f.projectPath)
		if merr != nil {
			t.Fatalf("readLayerMap: %v", merr)
		}

		if !mapHasPath(m, keyCheckpoint, keyCkExpiryDays) || !mapHasPath(m, keyCheckpoint, keyCkMaxPerSession) {
			t.Errorf("layer %s lacks the checkpoint keys: %v", f.projectPath, m)
		}

		if v, _ := m[keyCheckpoint].(map[string]any)[keyCkExpiryDays].(int); v != 14 {
			t.Errorf("persisted expiry_days = %v; want 14", m[keyCheckpoint])
		}
	})

	t.Run("unlisted value rejected with the membership error", func(t *testing.T) {
		t.Parallel()

		f := newSurfaceFixture(t)

		_, err := f.surface.Set("sess-ck", optCheckpointExpiryDays, "9")
		if err == nil || !strings.Contains(err.Error(), "not one of the offered options") {
			t.Fatalf("Set expiry=9 err = %v; want the membership violation", err)
		}

		var vio *acp.ConfigViolationError

		if !asViolation(err, &vio) {
			t.Errorf("err = %T; want *acp.ConfigViolationError", err)
		}
	})

	t.Run("read-back persisted, defaults, junk fallback", func(t *testing.T) {
		t.Parallel()

		// Defaults with no keys.
		f := newSurfaceFixture(t)

		days, per := f.surface.EffectiveCheckpointGCBounds()
		if days != 7 || per != 50 { //nolint:mnd // the pinned D-08 defaults
			t.Fatalf("default bounds = %d/%d; want 7/50", days, per)
		}

		// A planted layer is the reopen-equivalent truth (a fresh surface
		// over the same paths reads the same files — the layer IS the
		// persistence contract).
		f3 := newSurfaceFixture(t)
		writeLayer(t, f3.projectPath, "checkpoint:\n  expiry_days: 3\n  max_per_session: 10\n")

		days, per = f3.surface.EffectiveCheckpointGCBounds()
		if days != 3 || per != 10 { //nolint:mnd // the planted values
			t.Fatalf("planted bounds = %d/%d; want 3/10", days, per)
		}

		// Junk falls back loud to the defaults.
		f4 := newSurfaceFixture(t)
		writeLayer(t, f4.projectPath, "checkpoint:\n  expiry_days: banana\n")

		days, per = f4.surface.EffectiveCheckpointGCBounds()
		if days != 7 || per != 50 { //nolint:mnd // the fallback
			t.Fatalf("junk bounds = %d/%d; want 7/50 (loud fallback)", days, per)
		}

		if !strings.Contains(f4.stderr.String(), "checkpoint") {
			t.Errorf("junk fallback did not log loudly: %q", f4.stderr.String())
		}
	})
}

// valuesOf extracts a frame's offered values.
func valuesOf(f acp.ConfigOptionFrame) []string {
	out := make([]string, 0, len(f.Options))

	for _, v := range f.Options {
		out = append(out, v.Value)
	}

	return out
}

// asViolation is the errors.As helper for the violation assertion.
func asViolation(err error, target **acp.ConfigViolationError) bool {
	v, ok := err.(*acp.ConfigViolationError) //nolint:errorlint // concrete assert for the test
	if ok {
		*target = v
	}

	return ok
}
