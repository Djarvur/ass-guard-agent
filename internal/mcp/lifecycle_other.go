//go:build !(darwin || linux)

package mcp

import (
	"errors"
	"os/exec"
	"time"
)

// shutdownGrace mirrors the darwin/linux constant so host.go compiles unchanged
// across platforms (the value is unused on non-unix — prepareCommand errors).
const shutdownGrace = 3 * time.Second

// errUnsupportedPlatform is returned by every lifecycle op on Windows/other,
// where process-group isolation (Setpgid) and Wait4 are unavailable. Windows is
// deferred per PROJECT.md; this stub keeps the package compiling for CI tooling.
var errUnsupportedPlatform = errors.New("ass-guard MCP process-group isolation: darwin/linux only in v1 (Windows deferred per PROJECT.md)")

// prepareCommand returns the unsupported-platform error so Start fails fast on
// Windows (v1 does not host MCP servers there).
func prepareCommand(_ *exec.Cmd, _ *ServerConfig) error {
	return errUnsupportedPlatform
}

// groupID returns -1 on unsupported platforms (no process group).
func groupID(_ *exec.Cmd) int { return -1 }

// signalGroup is a no-op on unsupported platforms.
func signalGroup(_ int, _ time.Duration) error { return nil }

// reap is a no-op on unsupported platforms.
func reap(_ int) {}

// processAlive reports false on unsupported platforms (no process table query).
func processAlive(_ int) bool { return false }
