//go:build linux

package sandbox //nolint:testpackage // internal package test

import (
	"errors"
	"os"
	"strings"
	"syscall"
	"testing"
)

// The landlock battery (22-05 Task 2, SAND-01): the strict probe taxonomy
// (never best-effort — Pitfall 1), the ruleset's derivation from the ONE
// policy's LandlockRules rows (D-06 symmetry on linux), and the re-exec
// child entry. Runs on Linux hosts; compile-gated on the darwin dev host
// (GOOS=linux go vet/build — RESEARCH Environment Availability).

// TestLandlock_ProbeTaxonomy: each failure class maps to a DISTINCT
// Availability reason (SAND-01's loud taxonomy; strict mode only).
func TestLandlock_ProbeTaxonomy(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		abi        int
		err        error
		wantAvail  bool
		wantReason string
	}{
		{
			name:       "ENOSYS — kernel lacks Landlock",
			err:        syscall.ENOSYS,
			wantReason: "kernel lacks Landlock",
		},
		{
			name:       "EOPNOTSUPP — Landlock disabled",
			err:        syscall.EOPNOTSUPP,
			wantReason: "Landlock disabled",
		},
		{
			name:       "ABI below 4 — no network rules",
			abi:        3,
			wantReason: "ABI >= 4",
		},
		{
			name:      "ABI 4 — available",
			abi:       4,
			wantAvail: true,
		},
		{
			name:      "ABI 6 — available",
			abi:       6,
			wantAvail: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			av := availabilityFromProbe(tc.abi, tc.err)

			if av.Available != tc.wantAvail {
				t.Errorf("Available = %v; want %v (reason=%q)", av.Available, tc.wantAvail, av.Reason)
			}

			if !tc.wantAvail && !strings.Contains(av.Reason, tc.wantReason) {
				t.Errorf("Reason = %q; want it to name %q (distinct per failure class)", av.Reason, tc.wantReason)
			}

			if av.Mode != "landlock" {
				t.Errorf("Mode = %q; want landlock", av.Mode)
			}
		})
	}
}

// TestLandlock_RulesetFromPolicyRows: ApplyChildRuleset's rules derive from
// the SAME LandlockRules() rows the darwin golden pinned — symmetry by
// shared code (D-06): every EXISTING rw row becomes a RWDirs entry, every
// EXISTING ro row a RODirs entry, and rows whose path is absent on THIS host
// are skipped (the 22-06 missing-path filter — a landlock ruleset row opens
// its path at restrict time, so one missing entry would fail the whole
// ruleset and kill every confined run; DefaultPolicy's darwin-only
// "/System"/"/Library" rows are the canonical absent-on-linux case).
func TestLandlock_RulesetFromPolicyRows(t *testing.T) {
	t.Parallel()

	// All-rw paths exist → every row survives the filter 1:1.
	w, g := t.TempDir(), t.TempDir()
	p := DefaultPolicy(w, t.TempDir(), g)

	rw, ro := rulesetPaths(p)

	for _, row := range p.LandlockRules() {
		if !pathExists(row.Path) {
			continue // absent on this host — asserted skipped below via counts
		}

		switch row.Access {
		case "rw":
			found := false

			for _, got := range rw {
				if got == row.Path {
					found = true
				}
			}

			if !found {
				t.Errorf("rw row %q missing from the ruleset's RWDirs — the ruleset must derive from LandlockRules()", row.Path)
			}
		case "ro":
			found := false

			for _, got := range ro {
				if got == row.Path {
					found = true
				}
			}

			if !found {
				t.Errorf("ro row %q missing from the ruleset's RODirs — the ruleset must derive from LandlockRules()", row.Path)
			}
		}
	}

	if len(rw) != len(p.RWPaths) {
		t.Errorf("rw count %d != policy %d (existing paths must map 1:1)", len(rw), len(p.RWPaths))
	}

	// The ro set: DefaultPolicy mixes majors — every row that exists on this
	// host maps, every row that doesn't is skipped (never a ruleset failure).
	liveRO := 0
	for _, path := range p.ROSysPaths {
		if pathExists(path) {
			liveRO++
		}
	}

	if len(ro) != liveRO {
		t.Errorf("ro count %d != existing-host ro rows %d (missing paths must be skipped, not fatal)", len(ro), liveRO)
	}
}

// pathExists reports whether the path resolves on THIS host (the filter's
// probe — mirrors rulesetPaths' os.Stat).
func pathExists(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}

// TestLandlock_ChildEntryContract: RunSandboxChild applies the ruleset then
// execs the target — the exec seam is injectable; with the seam recording,
// the applied policy matches the env's JSON and the target argv survives.
func TestLandlock_ChildEntryContract(t *testing.T) {
	// NOT t.Parallel: the battery t.Setenv's the sentinel pair, and Setenv
	// panics inside parallel tests (surfaced when the linux leg first ran
	// live — it had been compile-gated since 22-05).
	policyJSON, merr := marshalPolicy(DefaultPolicy("/w", "/t", "/g"))
	if merr != nil {
		t.Fatal(merr)
	}

	var (
		applied  Policy
		execArgv []string
	)

	restore := swapSandboxExec(func(policy Policy, argv []string) error {
		applied = policy
		execArgv = argv

		return errors.New("stop-here") //nolint:goerr113 // test sentinel
	})
	defer restore()

	t.Setenv(ChildEnvSentinel, "1")
	t.Setenv(ChildEnvPolicy, policyJSON)

	err := RunSandboxChild([]string{"ass-guard", ChildTargetArgv, "sh", "-c", "echo hi"}, os.Environ())

	if err == nil || !strings.Contains(err.Error(), "stop-here") {
		t.Fatalf("RunSandboxChild err = %v; want the exec seam's sentinel (apply-then-exec reached)", err)
	}

	if applied.RWPaths == nil || len(applied.RWPaths) != 3 {
		t.Errorf("applied policy RWPaths = %v; want the env's 3-row triple", applied.RWPaths)
	}

	if len(execArgv) != 3 || execArgv[0] != "sh" || execArgv[2] != "echo hi" {
		t.Errorf("exec argv = %v; want the original target tail [sh -c 'echo hi']", execArgv)
	}
}
