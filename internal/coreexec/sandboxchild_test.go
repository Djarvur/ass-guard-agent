//go:build linux

package coreexec

import (
	"fmt"
	"os"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/sandbox"
)

// TestMain makes the package's test binary double as the LANDLOCK SANDBOX
// LOADER for the live 22-06 batteries (22-05 Pattern 4's re-exec sentinel):
// on linux a wrapSandboxCmd-wrapped child is THIS binary re-invoked with the
// sentinel env — exactly the production shape (cmd/ass-guard/main.go's
// unconditional child hook before root dispatch, source-pinned by Task 1).
// The sentinel argv never reaches m.Run: apply the ruleset, exec the target
// (never returning), or exit 1 fail-closed (the 22-05 contract).
func TestMain(m *testing.M) {
	if sandbox.IsSandboxChild() {
		if err := sandbox.RunSandboxChild(os.Args, os.Environ()); err != nil {
			fmt.Fprintf(os.Stderr, "ass-guard: sandbox child failed (refusing to run unconfined): %v\n", err)

			os.Exit(1)
		}

		os.Exit(0) // unreachable on exec success — type completeness
	}

	os.Exit(m.Run())
}
