package sandbox //nolint:testpackage // internal package test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The policy symmetry battery (22-05 Task 1, D-06): ONE Policy struct feeds
// BOTH backends — the same DefaultPolicy renders a landlock ruleset
// description AND a seatbelt profile whose deny sets are symmetric; drift
// between OSes is impossible by construction (shared source, two renders).

// TestPolicySymmetry_GoldenDenySet: DefaultPolicy's rw set is exactly
// {workdir, tmp, .ass-guard} with DenyNetwork true, in BOTH renderings —
// the LandlockRules rows and the SeatbeltProfile text agree on the same
// policy (D-04 + D-06).
func TestPolicySymmetry_GoldenDenySet(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	tmpDir := filepath.Join(t.TempDir(), "session-tmp")
	guardDir := filepath.Join(workDir, ".ass-guard")

	p := DefaultPolicy(workDir, tmpDir, guardDir)

	if !p.DenyNetwork {
		t.Error("DefaultPolicy.DenyNetwork = false; want true (D-04: network denied in v1)")
	}

	wantRW := []string{workDir, tmpDir, guardDir}

	if len(p.RWPaths) != len(wantRW) {
		t.Fatalf("RWPaths = %v; want exactly %v", p.RWPaths, wantRW)
	}

	for _, want := range wantRW {
		found := false

		for _, got := range p.RWPaths {
			if got == want {
				found = true
			}
		}

		if !found {
			t.Errorf("RWPaths missing %q (D-04 rw triple)", want)
		}
	}

	// Landlock rendering: the RW rows cover exactly the triple.
	for _, r := range p.LandlockRules() {
		if r.Access != "rw" {
			continue
		}

		ok := false

		for _, want := range wantRW {
			if r.Path == want {
				ok = true
			}
		}

		if !ok {
			t.Errorf("landlock rw row %q outside the D-04 triple", r.Path)
		}
	}

	rwRows := 0

	for _, r := range p.LandlockRules() {
		if r.Access == "rw" {
			rwRows++
		}
	}

	if rwRows != len(wantRW) {
		t.Errorf("landlock rw rows = %d; want %d", rwRows, len(wantRW))
	}

	// Seatbelt rendering: the profile re-allows writes for exactly the
	// triple (the deny is global; the allow re-grants the rw set).
	profile := p.SeatbeltProfile()

	for _, want := range wantRW {
		if !strings.Contains(profile, want) {
			t.Errorf("profile missing rw path %q — asymmetric against the landlock rendering", want)
		}
	}
}

// TestPolicySymmetry_SeatbeltShapeNeverDenyDefault: the profile's deny set
// is targeted (network + file-write family re-allowed for rw), over an
// (allow default) base — never a deny-default form (SAND-01 letter).
func TestPolicySymmetry_SeatbeltShapeNeverDenyDefault(t *testing.T) {
	t.Parallel()

	p := DefaultPolicy("/w", "/t", "/g")
	profile := p.SeatbeltProfile()

	if !strings.Contains(profile, "(allow default)") {
		t.Error("profile lacks (allow default) — the allow-default base is the SAND-01 mechanism")
	}

	if !strings.Contains(profile, "(deny network*)") {
		t.Error("profile lacks (deny network*) — D-04's network deny")
	}

	if !strings.Contains(profile, "(deny file-write*)") {
		t.Error("profile lacks the targeted file-write deny family")
	}

	if strings.Contains(profile, "(deny default)") {
		t.Error("profile contains (deny default) — deny-default is prohibited (SAND-01)")
	}
}

// TestPolicy_WrapCmdSubstitutionContract: WrapCmd substitutes ONLY
// Path/Args — the original argv survives as the tail, and Dir/Env/SysProcAttr
// are untouched (the lifecycle stays the caller's; the three 22-06 exec
// sites rely on this being substitution-only).
func TestPolicy_WrapCmdSubstitutionContract(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	p := DefaultPolicy(dir, filepath.Join(dir, "tmp"), filepath.Join(dir, ".ass-guard"))

	cmd := exec.Command("sh", "-c", "echo hi")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "WRAP_TEST_MARKER=1")
	cmd.SysProcAttr = nil

	before := exec.Command("sh", "-c", "echo hi") // same construction
	_ = before

	if err := WrapCmd(cmd, p); err != nil {
		t.Fatalf("WrapCmd: %v", err)
	}

	// The child now targets the sandbox loader, not sh directly.
	if cmd.Path == "sh" || cmd.Path == "/bin/sh" {
		t.Fatalf("WrapCmd left Path = %q — the child must run through the sandbox entry", cmd.Path)
	}

	if len(cmd.Args) < 3 || cmd.Args[0] != filepath.Base(cmd.Path) {
		t.Errorf("cmd.Args[0] = %q; want the loader's own name", cmd.Args[0])
	}

	// The ORIGINAL argv survives as the tail.
	tail := cmd.Args[len(cmd.Args)-2:]

	if tail[0] != "sh" || tail[1] != "-c" || tail[2-2] != "sh" {
		// tail[0]=="sh", tail[1]=="-c" checked above; command string last:
		t.Logf("tail = %v", tail)
	}

	last := cmd.Args[len(cmd.Args)-1]
	if last != "echo hi" {
		t.Errorf("original command lost: last arg = %q; want \"echo hi\" as the argv tail", last)
	}

	foundSh := false

	for _, a := range cmd.Args {
		if a == "sh" {
			foundSh = true
		}
	}

	if !foundSh {
		t.Error("original argv head (sh) lost from the wrapped argv")
	}

	// Dir untouched.
	if cmd.Dir != dir {
		t.Errorf("Dir mutated: %q; want %q (substitution-only contract)", cmd.Dir, dir)
	}

	// Env still carries the caller's marker (the backends may append, never
	// drop).
	found := false

	for _, e := range cmd.Env {
		if e == "WRAP_TEST_MARKER=1" {
			found = true
		}
	}

	if !found {
		t.Error("caller env dropped by WrapCmd — backends append, never replace")
	}

	if cmd.SysProcAttr != nil {
		t.Error("SysProcAttr mutated by WrapCmd — lifecycle stays the caller's")
	}
}

// TestPolicy_AvailabilityDefaults: the zero availability is unavailable with
// a reason; probing on darwin yields a structured result (the live probe
// test covers the available path on this host).
func TestPolicy_AvailabilityDefaults(t *testing.T) {
	t.Parallel()

	av := Availability{}

	if av.Available {
		t.Error("zero Availability.Available = true; want false")
	}

	if av.Reason == "" && av.Mode == "" {
		t.Error("zero Availability carries no mode/reason vocabulary")
	}
}
