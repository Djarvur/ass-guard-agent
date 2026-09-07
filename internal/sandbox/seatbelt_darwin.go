//go:build darwin

package sandbox

import (
	"fmt"
	"os"
	"os/exec"
)

// The macOS seatbelt backend (22-05, SAND-01): confinement rides
// /usr/bin/sandbox-exec with the policy's profile rendered IN MEMORY and
// passed via -p (D-06 — no runtime profile artifacts on disk). Targeted
// denies over (allow default); never deny-default. sandbox-exec is
// deprecated-in-man-page but shipped and used (Claude Code, Codex, Apple
// containerization — RESEARCH State of the Art); a first-use probe
// degrades LOUDLY if it ever disappears (T-22-25).

// sandboxExecName is the backend's binary.
const sandboxExecName = "sandbox-exec"

// probeProfile is the minimal profile the first-use probe runs (a real
// child exec — presence AND -p acceptance in one shot).
const probeProfile = "(version 1)\n(allow default)\n"

// probePlatform is the darwin leg of Probe: the seatbelt first-use probe,
// cached per process (real children never run per-exec).
func probePlatform() Availability {
	probeOnce.Do(func() {
		probeOnce.av = (&Policy{}).probeSeatbelt(nil)
	})

	return probeOnce.av
}

// probeSeatbelt probes sandbox-exec presence and -p acceptance. tryPaths
// overrides the lookup (tests scrub/point the path); nil uses the normal
// PATH resolution. Distinct reasons per failure class (SAND-01 taxonomy):
// binary missing vs profile rejection.
func (p Policy) probeSeatbelt(tryPaths []string) Availability {
	candidates := tryPaths
	if len(candidates) == 0 {
		candidates = []string{sandboxExecName}
	}

	var bin string

	for _, cand := range candidates {
		if cand == sandboxExecName {
			resolved, lerr := exec.LookPath(cand)
			if lerr != nil {
				continue
			}

			bin = resolved

			break
		}

		if _, serr := os.Stat(cand); serr == nil {
			bin = cand

			break
		}
	}

	if bin == "" {
		return Availability{
			Mode:   "seatbelt",
			Reason: "sandbox-exec not found on PATH — sandbox unavailable, tool processes run UNCONFINED",
		}
	}

	// -p acceptance probe: a real child under the minimal profile.
	probe := exec.Command(bin, "-p", probeProfile, "true") //nolint:gosec // fixed argv, no model input

	out, perr := probe.CombinedOutput()
	if perr != nil {
		return Availability{
			Mode:   "seatbelt",
			Reason: fmt.Sprintf("sandbox-exec -p probe failed (%v): %s — sandbox unavailable, tool processes run UNCONFINED", perr, firstLine(out)),
		}
	}

	return Availability{Mode: "seatbelt", Available: true}
}

// firstLine trims probe output for the reason string (one line, bounded).
func firstLine(b []byte) string {
	s := string(b)

	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return s[:i]
		}
	}

	if len(s) > 120 {
		return s[:120]
	}

	return s
}

// wrapPlatform is the darwin leg of Wrap: the child's argv becomes
// sandbox-exec -p <rendered> <original argv...> — the ORIGINAL argv survives
// as the tail; Dir/SysProcAttr untouched (substitution-only); Env appended
// only when the linux sentinel protocol needs it (never on darwin).
func wrapPlatform(cmd *exec.Cmd, p Policy) error {
	av := probePlatform()

	if !av.Available {
		fmt.Fprintf(os.Stderr, "ass-guard: sandbox: %s\n", av.Reason)

		return nil // degrade loudly, never refuse — the child runs unconfined
	}

	bin, lerr := exec.LookPath(sandboxExecName)
	if lerr != nil {
		return fmt.Errorf("sandbox: sandbox-exec lookup: %w", lerr)
	}

	orig := make([]string, len(cmd.Args))
	copy(orig, cmd.Args)

	cmd.Path = bin
	cmd.Args = append([]string{sandboxExecName, "-p", p.SeatbeltProfile()}, orig...)

	return nil
}

// applyChildHookPlatform: the darwin leg of the portable main hook — no
// re-exec sentinel on darwin (sandbox-exec IS the loader), so the hook is a
// permanent false with zero side effects.
func applyChildHookPlatform() bool {
	return false
}
