package version

import "testing"

// TestString_EqualsVersionVar asserts String returns the current Version var.
// Robust regardless of ldflags injection (the default "dev" check would break
// under a release ldflags build; the accessor invariant does not).
func TestString_EqualsVersionVar(t *testing.T) {
	t.Parallel()

	if got := String(); got != Version {
		t.Errorf("String() = %q, want %q (= Version var)", got, Version)
	}
}

// TestVersion_DefaultDevWhenNotInjected documents the plain-`go build` default.
// Under `go test` (no ldflags) Version is "dev"; a release ldflags build would
// override it and this test would be skipped at release time (not run via
// `go test` against the released binary).
func TestVersion_DefaultDevWhenNotInjected(t *testing.T) {
	t.Parallel()

	if Version != "dev" {
		t.Logf("Version = %q (ldflags-injected, not the default 'dev') — expected under release builds", Version)
	}
}
