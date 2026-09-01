package perm

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// filePermOwner is the hard owner-only file permission for permissions.yaml
// (the repo's 0600 artifact-family discipline; T-17-02).
const filePermOwner = 0o600

// dirPerm is the max permission for directories the store creates (0750).
const dirPerm = 0o750

// yamlIndent is the permissions.yaml block indent (2-space, the pinned
// file shape — RESEARCH Pattern 5).
const yamlIndent = 2

// renameFunc is the atomic-replace seam (same-package test injection, the
// providerfactory convention): tests swap it to prove the prior file
// survives the crash-between-marshal-and-rename window.
var renameFunc = os.Rename //nolint:gochecknoglobals // same-package test-injection seam

// errNotSimpleEntry is the static guard error: the dialog's write surface is
// restricted to simple tool entries — richer rules stay hand-edit-only (D-01).
var errNotSimpleEntry = errors.New("dialog writes are restricted to simple tool entries")

// ErrCorrupt marks a DOCUMENT-level parse failure of an existing permissions
// file (17-REVIEW WR-01). It is distinct from a malformed rule LINE (those are
// skipped with a Warning — the tolerant hand-edit surface is about lines): a
// corrupt document (a stray tab, a duplicate key, a truncated write) previously
// degraded the session to "deny/allow rules UNENFORCED" silently — a deny rule
// like Bash(rm *) simply stopped denying. The typed error is what OpenRepaired
// keys the quarantine on; Open still returns it verbatim.
var ErrCorrupt = errors.New("perm: corrupt permissions document")

// fileEnvelope is the on-disk permissions.yaml envelope: three ordered lists
// of CC-parity rule strings (D-02). The dialog writes only simple entries
// into the allow/deny lists (D-01/D-03); richer rules are hand-edit-only but
// preserved verbatim through dialog writes.
type fileEnvelope struct {
	Deny  []string `yaml:"deny"`
	Ask   []string `yaml:"ask"`
	Allow []string `yaml:"allow"`
}

// Store is the single-writer permissions.yaml store: the accumulating
// tool-x-project trust state the Phase-17 dialog clicks persist (D-01/D-03).
// Mutations serialize under mu; Rules() returns a copy-on-read snapshot;
// saves are atomic (0600 temp + rename). A zero Store is unusable; use Open.
type Store struct {
	path  string
	mu    sync.Mutex
	lists fileEnvelope
	warns []Warning
	log   *slog.Logger
}

// Open opens (or creates) the permissions.yaml at path. On an absent file it
// creates the parent dirs (0750) and an empty-valid file (0600) — the
// zero-config floor. An existing file loads verbatim; malformed rule lines
// are skipped with one structured Warning each (tolerant hand-edit surface).
func Open(path string) (*Store, error) {
	s := &Store{path: path, log: slog.Default()}

	dir := filepath.Dir(path)

	err := os.MkdirAll(dir, dirPerm)
	if err != nil {
		return nil, fmt.Errorf("perm: mkdir %q: %w", dir, err)
	}

	// WR-04: the 0600/0750 discipline is only automatic on files the store
	// CREATES or REWRITES — an existing loose file (hand-created 0644, or a
	// pre-phase 0755 directory) previously stayed loose forever, leaking the
	// trust store's contents (which tools an operator denied) to other local
	// users until the next dialog click rewrote the file. Tighten on load.
	tightenPerm(dir, dirPerm)

	_, err = os.Stat(path)
	switch {
	case os.IsNotExist(err):
		werr := s.save(fileEnvelope{})
		if werr != nil {
			return nil, fmt.Errorf("perm: create %q: %w", path, werr)
		}
	case err != nil:
		return nil, fmt.Errorf("perm: stat %q: %w", path, err)
	default:
		lerr := s.load()
		if lerr != nil {
			return nil, lerr
		}

		tightenPerm(path, filePermOwner)
	}

	return s, nil
}

// tightenPerm tightens path to want when its current permission bits are
// LOOSER (WR-04). A correction is LOUD (structured warning); a stat or chmod
// failure is logged and non-fatal — Open's own error paths own the fatal
// cases.
func tightenPerm(path string, want os.FileMode) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}

	if info.Mode().Perm()&^want == 0 {
		return // no bits looser than want
	}

	if cerr := os.Chmod(path, want); cerr != nil {
		slog.Warn("perm: could not tighten loose permissions on the trust store",
			"path", path, "want", want.String(), "error", cerr.Error())

		return
	}

	slog.Warn("perm: tightened loose permissions on the trust store", "path", path, "mode", want.String())
}

// OpenRepaired opens the store with the WR-01 fail-safe for a corrupt
// document: the unreadable file is QUARANTINED to "<path>.corrupt" (any prior
// quarantine is replaced — the newest evidence wins), the floor 0600 file is
// recreated so dialog writes still work, and the degradation is LOUD — the
// operator's deny rules are not enforced until the file is restored. Only a
// document-level parse failure takes the quarantine path (a recoverable,
// known-cause state); environmental failures (mkdir/stat/create) and every
// healthy open pass through unchanged.
func OpenRepaired(path string) (*Store, error) {
	s, err := Open(path)
	if err == nil || !errors.Is(err, ErrCorrupt) {
		return s, err
	}

	corrupt := path + ".corrupt"

	if rmErr := os.Remove(corrupt); rmErr != nil && !os.IsNotExist(rmErr) {
		return nil, fmt.Errorf("perm: clear stale quarantine %q: %w", corrupt, rmErr)
	}

	if rerr := os.Rename(path, corrupt); rerr != nil {
		return nil, fmt.Errorf("perm: quarantine %q: %w", path, rerr)
	}

	slog.Error("perm: permissions file is corrupt — quarantined; "+
		"DENY/ALLOW RULES ARE NOT ENFORCED for this session until the file is restored",
		"file", path, "quarantined", corrupt)

	return Open(path)
}

// Rules returns a parsed snapshot of the current rule set (copy-on-read —
// mutating the returned RuleSet cannot affect the store; NewRuleSet
// allocates fresh rules).
func (s *Store) Rules() RuleSet {
	s.mu.Lock()
	defer s.mu.Unlock()

	rs, _ := NewRuleSet(s.lists.Deny, s.lists.Ask, s.lists.Allow)

	return rs
}

// Warnings returns a copy of the structured warnings gathered at Open (one
// per skipped malformed line) — the same diagnostics sink as NewRuleSet.
func (s *Store) Warnings() []Warning {
	s.mu.Lock()
	defer s.mu.Unlock()

	return slices.Clone(s.warns)
}

// AllowTool persists a dialog "always allow" click as a simple tool entry in
// the allow list (D-01): the ONLY write shape the dialog has in this
// direction. Idempotent — an already-present identical entry is a no-op
// (file bytes unchanged). Non-simple entries (specifiers, globs, spaces) are
// rejected: richer rules remain hand-edit-only.
func (s *Store) AllowTool(name string) error {
	return s.persist(name, func(env *fileEnvelope) *[]string { return &env.Allow })
}

// ForbidTool persists a dialog "reject, and don't ask again" click as a
// simple tool entry in the deny list (D-03) — the other dialog write shape.
// Idempotent; non-simple entries rejected, as in AllowTool.
func (s *Store) ForbidTool(name string) error {
	return s.persist(name, func(env *fileEnvelope) *[]string { return &env.Deny })
}

// persist validates name as a simple entry, appends it to the selected list
// unless already present (idempotent no-op — no disk touch), and atomically
// saves. Mutations serialize under mu.
func (s *Store) persist(name string, list func(*fileEnvelope) *[]string) error {
	if !validSimpleEntry(name) {
		return fmt.Errorf("%w: %q", errNotSimpleEntry, name)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if slices.Contains(*list(&s.lists), name) {
		return nil // idempotent — converges to one entry
	}

	env := fileEnvelope{
		Deny:  slices.Clone(s.lists.Deny),
		Ask:   slices.Clone(s.lists.Ask),
		Allow: slices.Clone(s.lists.Allow),
	}

	*list(&env) = append(*list(&env), name)

	// Commit to memory only after the atomic save succeeded — a failed write
	// leaves both the file AND the in-memory rule set unchanged.
	serr := s.save(env)
	if serr != nil {
		return serr
	}

	s.lists = env

	return nil
}

// load reads + validates the file into s.lists. Absent optional keys and
// empty files load as empty lists; malformed rule lines are skipped with one
// warning each (logged structured to stderr AND retained in s.warns).
func (s *Store) load() error {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return fmt.Errorf("perm: read %q: %w", s.path, err)
	}

	var env fileEnvelope

	if len(raw) > 0 {
		uerr := yaml.Unmarshal(raw, &env)
		if uerr != nil {
			return fmt.Errorf("perm: parse %q: %w: %w", s.path, ErrCorrupt, uerr)
		}
	}

	env.Deny = s.validated(env.Deny)
	env.Ask = s.validated(env.Ask)
	env.Allow = s.validated(env.Allow)
	s.lists = env

	return nil
}

// validated returns the order-preserved lines that parse as rules, skipping
// malformed ones with a warning.
func (s *Store) validated(lines []string) []string {
	out := make([]string, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)

		_, err := ParseRule(line)
		if err != nil {
			s.warns = append(s.warns, Warning{Rule: line, Reason: err.Error()})
			s.log.Warn("perm: skipping malformed rule line",
				"file", s.path, "rule", line, "reason", err.Error())

			continue
		}

		out = append(out, line)
	}

	return out
}

// validSimpleEntry reports whether name is a simple tool entry — the ONLY
// shape the dialog may write (D-01): a non-empty identifier of letters,
// digits, and underscores (covering the mcp__ namespace), no specifiers, no
// globs, no whitespace.
func validSimpleEntry(name string) bool {
	if name == "" {
		return false
	}

	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
		default:
			return false
		}
	}

	return true
}

// save atomically writes the envelope (marshal → 0600 temp → rename): the
// rename is the ONLY step that touches the target, so any failure leaves the
// prior file byte-intact with no temp leftover (T-17-02). MUST be called
// under s.mu (or during Open before the store is shared).
func (s *Store) save(env fileEnvelope) error {
	raw, err := marshalEnvelope(env)
	if err != nil {
		return err
	}

	tmp := s.path + ".tmp"

	werr := os.WriteFile(tmp, raw, filePermOwner)
	if werr != nil {
		return fmt.Errorf("perm: write tmp %q: %w", tmp, werr)
	}

	// The explicit chmod makes the hard 0600 deterministic under any umask.
	perr := os.Chmod(tmp, filePermOwner)
	if perr != nil {
		_ = os.Remove(tmp)

		return fmt.Errorf("perm: chmod tmp %q: %w", tmp, perr)
	}

	rerr := renameFunc(tmp, s.path)
	if rerr != nil {
		_ = os.Remove(tmp)

		return fmt.Errorf("perm: rename %q → %q: %w", tmp, s.path, rerr)
	}

	return nil
}

// marshalEnvelope renders the envelope at the pinned 2-space indent.
func marshalEnvelope(env fileEnvelope) ([]byte, error) {
	var buf bytes.Buffer

	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(yamlIndent)

	eerr := enc.Encode(env)
	if eerr != nil {
		_ = enc.Close()

		return nil, fmt.Errorf("perm: marshal: %w", eerr)
	}

	cerr := enc.Close()
	if cerr != nil {
		return nil, fmt.Errorf("perm: close encoder: %w", cerr)
	}

	return buf.Bytes(), nil
}
