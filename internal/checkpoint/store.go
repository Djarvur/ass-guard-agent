// Package checkpoint implements the EARLY-01 shadow-git checkpoint store:
// the no-confirmation-tier agent's reversibility backstop. Every PARENT turn
// snapshots the workspace's file state into ONE shadow git repository under
// <workDir>/.ass-guard/checkpoints/shadow.git, addressed by turn
// (refs/checkpoints/<sessionID>-turn-<NNN>), and `ass-guard checkpoint
// list|restore` works on it from the terminal.
//
// Core invariants (all test-pinned):
//
//   - The USER's repository git state (index, HEAD, remotes, config, hooks)
//     is NEVER touched: the shadow store is a separate git dir with its own
//     index, neutralized global/system config, and disabled hooks.
//   - A snapshot failure is loud but NEVER fatal to the turn (AUD-03
//     discipline — the turn completes without a checkpoint).
//   - A ref appears only AFTER its commit object exists (commit-then-ref
//     ordering); an interrupted snapshot never exposes a partial ref.
//
// RED-phase stub: the API surface is final; every operation fails with
// errNotImplemented until the GREEN implementation lands.
package checkpoint

import (
	"context"
	"errors"
	"time"
)

// DefaultKeep is the retention bound: after each snapshot the store prunes
// its oldest turn refs beyond this many (T-14-03 DoS mitigation).
const DefaultKeep = 50

// errNotImplemented is the RED-phase stub error (GREEN replaces every body).
var errNotImplemented = errors.New("checkpoint: store not implemented yet") //nolint:err113,gochecknoglobals // RED-phase stub

// Entry is one restorable turn checkpoint, as surfaced by List.
type Entry struct {
	SessionID   string
	TurnNum     int
	Ref         string
	CommittedAt time.Time
}

// Store is the per-workspace shadow-git checkpoint store.
type Store struct{}

// Open resolves (and lazily initializes) the shadow store under
// <workDir>/.ass-guard/checkpoints/shadow.git.
func Open(_ string) (*Store, error) {
	return nil, errNotImplemented
}

// Snapshot records the workspace's current file state under the turn's
// checkpoint ref. Re-snapshotting the same turn updates the SAME ref.
func (*Store) Snapshot(_ context.Context, _, _ string) error {
	return errNotImplemented
}

// List returns the turn checkpoints, ascending by (sessionID, turn number).
func (*Store) List() ([]Entry, error) {
	return nil, errNotImplemented
}

// Restore returns the workspace to the checkpoint's recorded pre-turn state.
// The id is validated against the strict grammar BEFORE any git invocation.
func (*Store) Restore(_ context.Context, _ string) error {
	return errNotImplemented
}
