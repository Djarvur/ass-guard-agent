package coreexec

import (
	"context"
	"errors"
)

// errPTYScaffold is the RED-phase stand-in error: every scaffold Run fails,
// so the persistent-shell battery fails on behavior assertions (not build
// errors) before the feat commit lands the real manager.
var errPTYScaffold = errors.New("coreexec: ptty: RED-phase scaffold: persistent shell not implemented")

// PTYOpts configures the per-session persistent-shell manager (22-04,
// PAR-09): WorkDir is the shell's start directory; NoteFn emits the
// dead-shell restart note (D-08's visible state-loss acknowledgment).
type PTYOpts struct {
	WorkDir string
	NoteFn  func(format string, args ...any)
}

// PTYManager owns the session's ONE persistent shell (D-07).
//
// RED-PHASE SCAFFOLDING: the inert struct exists only so the battery (and
// coreexec.Config.PTY) compiles; the feat commit of this plan replaces it
// with the real lazily-started PTY shell over creack/pty v1.1.24.
type PTYManager struct{}

// NewPTYManager returns a lazy manager (scaffold: inert).
func NewPTYManager(opts PTYOpts) *PTYManager { return &PTYManager{} }

// Run executes one command in the session's persistent shell (scaffold:
// always errors — the battery's persistence assertions fail for the right
// reason).
func (m *PTYManager) Run(ctx context.Context, command string) (string, int, error) {
	return "", 0, errPTYScaffold
}

// Drain ends the shell and closes the master fd (scaffold: no-op).
func (m *PTYManager) Drain() {}
