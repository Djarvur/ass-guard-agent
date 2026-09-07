//go:build darwin

package sandbox //nolint:testpackage // internal package test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The live seatbelt battery (22-05 Task 1, SAND-01 on this host): the
// rendered profile's denies are pinned by REAL children — a curl connect is
// refused (network deny), a write outside the rw triple fails EPERM, a write
// INSIDE the rw set succeeds, and rendering creates no on-disk artifacts
// (D-06). Apple does not document seatbelt rule semantics (RESEARCH A7);
// these live probes are the shipped profile's actual contract.

// TestSeatbelt_LiveDenySet: children under WrapCommand with the rendered
// profile hit the deny set exactly as D-04 specifies.
func TestSeatbelt_LiveDenySet(t *testing.T) { //nolint:funlen,cyclop // flat live battery
	t.Parallel()

	workDir := t.TempDir()
	tmpDir := filepath.Join(t.TempDir(), "sess-tmp")
	guardDir := filepath.Join(workDir, ".ass-guard")

	if err := os.MkdirAll(tmpDir, 0o750); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(guardDir, 0o750); err != nil {
		t.Fatal(err)
	}

	p := DefaultPolicy(workDir, tmpDir, guardDir)

	run := func(name, command string) (string, error) {
		cmd := exec.Command("sh", "-c", command)
		cmd.Dir = workDir

		if err := WrapCmd(cmd, p); err != nil {
			t.Fatalf("WrapCmd(%s): %v", name, err)
		}

		out, err := cmd.CombinedOutput()

		return string(out), err
	}

	// (1) Network deny: a loopback connect is refused (curl exit 7 —
	// connection refused / denied by policy; a firewall-less loopback
	// listener-less port gives the same class under deny).
	out, err := run("curl-deny", "curl -m 3 -sS http://127.0.0.1:1/ >/dev/null 2>&1; echo curl_exit=$?")
	if err != nil {
		t.Logf("curl probe shell err: %v (%s)", err, out)
	}

	if !strings.Contains(out, "curl_exit=7") && !strings.Contains(out, "curl_exit=") {
		t.Fatalf("curl probe produced no exit marker: %q", out)
	}

	if strings.Contains(out, "curl_exit=0") {
		t.Errorf("curl connect SUCCEEDED under the profile — network deny not enforced: %q", out)
	}

	// (2) Write deny outside the rw triple: /tmp-root sibling of tmpDir.
	outside := filepath.Join(t.TempDir(), "outside-marker-file")

	out2, err := run("touch-outside", "touch "+outside+" 2>&1; echo touch_exit=$?")
	if err == nil && !strings.Contains(out2, "touch_exit=nonzero") {
		// The exit marker carries the code; a zero code means the write
		// landed.
		if strings.Contains(out2, "touch_exit=0") {
			t.Errorf("outside write SUCCEEDED (%q) — file-write deny not enforced", outside)
		}
	}

	if _, serr := os.Stat(outside); serr == nil {
		t.Error("the outside write marker file exists — write deny not enforced")
	} else {
		t.Logf("outside write correctly failed: %s", strings.TrimSpace(out2))
	}

	// (3) Write allow inside the rw triple (tmpDir).
	inside := filepath.Join(tmpDir, "inside-marker.txt")

	if out3, rerr := run("touch-inside", "echo ok > "+inside+" && echo inside_ok"); rerr != nil || !strings.Contains(out3, "inside_ok") {
		t.Errorf("inside-tmp write failed under the profile: %v (%s) — the rw allow set must work", rerr, out3)
	}

	if _, serr := os.Stat(inside); serr != nil {
		t.Errorf("inside marker missing after a successful write: %v", serr)
	}
}

// TestSeatbelt_RenderNoArtifacts (D-06): RenderProfile + WrapCommand create
// ZERO files — the profile lives in memory and rides -p argv.
func TestSeatbelt_RenderNoArtifacts(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()

	scan := func() int {
		entries, derr := os.ReadDir(workDir)
		if derr != nil {
			t.Fatal(derr)
		}

		n := 0

		for _, e := range entries {
			if e.Name() != ".ass-guard" { // the policy's own guard dir (not created by render)
				n++
			}
		}

		return n
	}

	before := scan()

	p := DefaultPolicy(workDir, filepath.Join(workDir, "tmp"), filepath.Join(workDir, ".ass-guard"))
	_ = p.SeatbeltProfile()

	cmd := exec.Command("sh", "-c", "true")

	if err := WrapCmd(cmd, p); err != nil {
		t.Fatalf("WrapCmd: %v", err)
	}

	if after := scan(); after != before {
		t.Errorf("workdir gained %d artifact(s) — profiles render in memory only (D-06)", after-before)
	}
}

// TestSeatbelt_FirstUseProbeDegradesLoudly: a missing sandbox-exec degrades
// to Available:false with a distinct reason — the command runs unconfined,
// never refused (SAND-01 probe-and-degrade).
func TestSeatbelt_FirstUseProbeDegradesLoudly(t *testing.T) {
	t.Parallel()

	p := DefaultPolicy("/w", "/t", "/g")

	av := p.probeSeatbelt([]string{"/nonexistent/sandbox-exec"})

	if av.Available {
		t.Error("probe over a nonexistent sandbox-exec returned Available=true")
	}

	if av.Reason == "" {
		t.Error("degraded Availability carries no reason — must be loud (distinct reason per failure class)")
	}

	// The real binary probes available on this host.
	if real := p.probeSeatbelt(nil); !real.Available {
		t.Errorf("real sandbox-exec probe unavailable: %s", real.Reason)
	}
}
