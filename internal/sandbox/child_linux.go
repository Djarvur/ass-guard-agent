//go:build linux

package sandbox

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// The re-exec sentinel entrypoint (22-05, Pattern 4): os/exec has no
// pre-exec child hook, so the confined child is ass-guard's OWN binary
// re-invoked with __ASS_GUARD_SANDBOX_CHILD=1 and the JSON policy in the
// environment; the child main path (ApplySandboxChildHook, wired in 22-06)
// intercepts BEFORE normal root-command dispatch, applies the landlock
// ruleset, then syscall.Execs the real target — ass-guard's binary is the
// sandbox loader, no extra artifact. If review ever rejects the self
// re-exec shape, the fallback is a tiny internal helper binary with the
// same semantics (RESEARCH A2).

// errNotChild marks an argv that is not a sandbox-child invocation.
var errNotChild = errors.New("sandbox: argv is not a sandbox child")

// sandboxChildExec is the apply-then-exec unit (a seam so tests on Linux
// hosts can verify the contract WITHOUT confining the test process — the
// default applies a real ruleset and never returns on success).
//
// TARGET RESOLUTION (22-06 live-linux discovery): syscall.Exec does NO PATH
// lookup — the wrapped child's tail argv carries the ORIGINAL cmd.Args, whose
// [0] is the bare invocation name ("sh"), so exec'ing it verbatim dies ENOENT
// on every confined run (invisible while the linux leg was compile-gated).
// A non-absolute target resolves through exec.LookPath BEFORE the exec; the
// ro ruleset (already applied at this point) still grants the read+execute
// the lookup and exec need.
var sandboxChildExec = func(policy Policy, argv []string) error {
	if aerr := ApplyChildRuleset(policy); aerr != nil {
		return fmt.Errorf("sandbox: child ruleset: %w", aerr)
	}

	target := argv[0]
	if !filepath.IsAbs(target) {
		resolved, lerr := exec.LookPath(target)
		if lerr != nil {
			return fmt.Errorf("sandbox: resolve target %q: %w", target, lerr)
		}

		target = resolved
	}

	return syscall.Exec(target, argv, targetEnv(os.Environ())) //nolint:wrapcheck // exec never returns on success
}

// RunSandboxChild is the loader entrypoint: argv is this process's own
// [--sandbox-child <original target argv...>] form, environ carries the
// JSON policy. Applies the ruleset, then execs the target with the
// sentinel vars stripped (the target never sees sandbox internals). The
// policy env is operator/session-controlled — the model never supplies it
// (T-22-23: the env is set by ass-guard at spawn, and landlock is
// irreversible once applied).
func RunSandboxChild(argv []string, environ []string) error {
	if len(argv) < 3 || argv[1] != ChildTargetArgv {
		return errNotChild
	}

	raw := envLookup(environ, ChildEnvPolicy)
	if raw == "" {
		return fmt.Errorf("sandbox: %s env missing: %w", ChildEnvPolicy, errNotChild)
	}

	policy, perr := unmarshalPolicy(raw)
	if perr != nil {
		return perr
	}

	return sandboxChildExec(policy, argv[2:]) //nolint:wrapcheck // the seam's error is the caller's signal
}

// applyChildHookPlatform is the linux leg of the portable main hook: with
// the sentinel env set it drives RunSandboxChild (apply + exec — never
// returning to ass-guard's normal main path on success). A confinement
// failure EXITS 1 — a failed confinement must never fall through to run
// the target unconfined (the fail-closed counterpart of the probe's loud
// degrade: there the sandbox was known-absent BEFORE promising it; here it
// was promised and broke).
func applyChildHookPlatform() bool {
	if !IsSandboxChild() {
		return false
	}

	argv := os.Args

	if err := RunSandboxChild(argv, os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "ass-guard: sandbox child failed (refusing to run unconfined): %v\n", err)
		os.Exit(1)
	}

	return true // unreachable on exec success — type completeness
}

// targetEnv strips the sandbox sentinel vars from the target's environment.
func targetEnv(environ []string) []string {
	out := make([]string, 0, len(environ))

	for _, e := range environ {
		if strings.HasPrefix(e, ChildEnvSentinel+"=") || strings.HasPrefix(e, ChildEnvPolicy+"=") {
			continue
		}

		out = append(out, e)
	}

	return out
}

// envLookup reads one key from an environ slice.
func envLookup(environ []string, key string) string {
	prefix := key + "="

	for _, e := range environ {
		if strings.HasPrefix(e, prefix) {
			return e[len(prefix):]
		}
	}

	return ""
}

// swapSandboxExec replaces the apply-then-exec seam (test hook) and returns
// the restore func.
func swapSandboxExec(fn func(policy Policy, argv []string) error) func() {
	old := sandboxChildExec
	sandboxChildExec = fn

	return func() { sandboxChildExec = old }
}
