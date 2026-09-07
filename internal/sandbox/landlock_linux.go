//go:build linux

package sandbox

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/landlock-lsm/go-landlock/landlock"
	"golang.org/x/sys/unix"
)

// The Linux landlock backend (22-05, SAND-01): strict-mode rulesets built
// from the ONE Policy's LandlockRules rows (D-06), applied INSIDE the
// re-exec child (D-05 — landlock is process-wide and irreversible; applying
// it in ass-guard itself would sever provider/MCP networking). Never
// best-effort degrade: the probe is the raw kernel version query with the
// ENOSYS/EOPNOTSUPP taxonomy, and enforcement errors surface as errors.

// ProbeABI is the STRICT kernel version probe: the raw
// landlock_create_ruleset(NULL, 0, LANDLOCK_CREATE_RULESET_VERSION) query
// (docs.kernel.org) — a pure version read that restricts NOTHING (the
// return value IS the highest supported ABI version, not an fd). ENOSYS =
// kernel lacks Landlock; EOPNOTSUPP = Landlock disabled. The raw Syscall6
// form is used because golang.org/x/sys v0.47.0 ships the syscall number
// and flags but not the wrapper (go-landlock's own internal package owns
// the wrapped flavor — not importable from here).
func ProbeABI() (int, error) {
	r0, _, e1 := unix.Syscall6(unix.SYS_LANDLOCK_CREATE_RULESET,
		0, 0, // attr=NULL, size=0 — the version-query form
		unix.LANDLOCK_CREATE_RULESET_VERSION,
		0, 0, 0,
	)
	if e1 != 0 {
		return 0, e1 //nolint:wrapcheck // errno passthrough — classified by availabilityFromProbe
	}

	return int(r0), nil
}

// availabilityFromProbe classifies the probe outcome into the SAND-01
// taxonomy — each failure class a DISTINCT reason, never a silent
// fail-open.
func availabilityFromProbe(abi int, err error) Availability {
	if err != nil {
		switch {
		case errors.Is(err, unix.ENOSYS):
			return Availability{Mode: "landlock",
				Reason: "kernel lacks Landlock (ENOSYS) — sandbox unavailable, tool processes run UNCONFINED"}
		case errors.Is(err, unix.EOPNOTSUPP):
			return Availability{Mode: "landlock",
				Reason: "Landlock disabled (EOPNOTSUPP) — sandbox unavailable, tool processes run UNCONFINED"}
		default:
			return Availability{Mode: "landlock",
				Reason: fmt.Sprintf("landlock probe failed (%v) — sandbox unavailable, tool processes run UNCONFINED", err)}
		}
	}

	if abi < 4 {
		return Availability{Mode: "landlock",
			Reason: fmt.Sprintf("Landlock ABI %d: network restrictions need ABI >= 4 — sandbox unavailable, tool processes run UNCONFINED", abi)}
	}

	return Availability{Mode: "landlock", Available: true}
}

// probePlatform is the linux leg of Probe (cached — a raw syscall, but the
// classification is stable per boot).
func probePlatform() Availability {
	probeOnce.Do(func() {
		abi, perr := ProbeABI()
		probeOnce.av = availabilityFromProbe(abi, perr)
	})

	return probeOnce.av
}

// rulesetPaths derives the ruleset's RWDirs/RODirs path lists from the
// policy's LandlockRules rows — the SAME rows the darwin golden pins
// (D-06: symmetry by shared code).
func rulesetPaths(p Policy) (rw, ro []string) {
	for _, row := range p.LandlockRules() {
		switch row.Access {
		case "rw":
			rw = append(rw, row.Path)
		case "ro":
			ro = append(ro, row.Path)
		}
	}

	return rw, ro
}

// ApplyChildRuleset builds and applies the strict V4 ruleset from the
// policy: RestrictPaths (RWDirs the D-04 triple, RODirs the system set) +
// RestrictNet with NO granted rules (= deny all classic TCP — D-04). Called
// only inside the re-exec child. WithRefer stays OFF (cross-dir rename
// deny-by-default — the symlink/rename-escape mitigation, T-22-22).
// go-landlock sets PR_SET_NO_NEW_PRIVS with every Restrict call (T-22-21).
//
// The applied domain is inherited by the child's OWN children (kernel
// clone-inheritance — docs.kernel.org) — the group-kill discipline in
// bash.go/background.go stays the lifecycle owner.
func ApplyChildRuleset(p Policy) error {
	rw, ro := rulesetPaths(p)

	perr := landlock.V4.RestrictPaths(
		landlock.RWDirs(rw...),
		landlock.RODirs(ro...),
	)
	if perr != nil {
		return fmt.Errorf("sandbox: landlock restrict paths: %w", perr)
	}

	if p.DenyNetwork {
		// No granted rules = no classic-TCP binds or connects (V4+).
		if nerr := landlock.V4.RestrictNet(); nerr != nil {
			return fmt.Errorf("sandbox: landlock restrict net: %w", nerr)
		}
	}

	return nil
}

// wrapPlatform is the linux leg of Wrap: the child becomes ass-guard's own
// binary re-invoked with the sentinel + JSON policy env (Pattern 4 — os/exec
// has no pre-exec child hook, so the self re-exec IS the sandbox loader).
// The original argv survives behind the ChildTargetArgv marker; the child
// main path intercepts via ApplySandboxChildHook before normal dispatch.
func wrapPlatform(cmd *exec.Cmd, p Policy) error {
	av := probePlatform()

	if !av.Available {
		fmt.Fprintf(os.Stderr, "ass-guard: sandbox: %s\n", av.Reason)

		return nil // degrade loudly, never refuse — the child runs unconfined
	}

	self, serr := os.Executable()
	if serr != nil {
		return fmt.Errorf("sandbox: self executable: %w", serr)
	}

	policyJSON, merr := marshalPolicy(p)
	if merr != nil {
		return merr
	}

	orig := make([]string, len(cmd.Args))
	copy(orig, cmd.Args)

	cmd.Path = self
	cmd.Args = append([]string{filepath.Base(self), ChildTargetArgv}, orig...)

	if len(cmd.Env) == 0 {
		cmd.Env = os.Environ() // sentinel env must ride the child; nil-Env means inherit — materialize it
	}

	cmd.Env = append(cmd.Env, ChildEnvSentinel+"=1", ChildEnvPolicy+"="+policyJSON)

	return nil
}
