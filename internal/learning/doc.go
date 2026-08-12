// Package learning implements the learned-config store + hook-proposal logic
// (Phase-4 LRN-01..04).
//
// When the engine encounters a launch situation it does not know how to handle
// (no matching pattern, no known handoff, no applicable hook), it asks the user
// once, executes the answer, and records a candidate learned entry
// (ask-once-and-remember — LRN-01/D-17). After ≥3 confirmations the entry
// becomes active (LRN-03/D-19); a DIFFERENT answer for the same situation flips
// it to conflict + the operator resolves it via revert. Entries carry an expiry
// (default 30 days; ignored past — the operator can renew or purge).
//
// ProposeHooks (LRN-02/D-18) detects repeated manual sequences in the work log
// and emits hook Proposals the engine surfaces via ActionAsk.
//
// The store is single-writer / copy-on-read: mutations serialize under a mutex
// (mirrors session.Manager — T-04-09), reads return copies, and saves are
// atomic (temp + rename). The learned file is plain YAML (git-trackable —
// LRN-04/D-20); `ass-guard learning list` + `ass-guard learning revert <id>`
// are the operator surface, and git history provides full revertibility.
package learning
