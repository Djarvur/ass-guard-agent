package learning

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// ErrConflict signals a Confirm observed a different answer than the stored one
// (LRN-03 / T-04-07): the entry flips to conflict + Lookup skips it. The
// operator resolves via Revert + re-ask.
var ErrConflict = errors.New("learned entry conflict: same situation, different answer")

// ErrNotFound signals a Confirm/Revert referenced an entry that is not present.
var ErrNotFound = errors.New("learned entry not found")

// fileEnvelope is the on-disk YAML envelope.
type fileEnvelope struct {
	Entries []Entry `yaml:"entries"`
}

// Store is the single-writer learned-config store (D-16). Mutations serialize
// under mu (mirrors session.Manager — T-04-09); reads return copies
// (copy-on-read); saves are atomic (temp + rename). A zero Store is unusable;
// use Open.
type Store struct {
	path string
	mu   sync.Mutex
	log  *slog.Logger
}

// Open opens (or creates) the learned.yaml at path. If the file is absent, an
// empty-valid envelope is written so the zero-config floor is honored (the
// engine reads it at startup; a missing file degrades to "no learned settings").
func Open(path string) (*Store, error) {
	s := &Store{path: path, log: slog.Default()}
	// Ensure the parent dir exists (the operator may point at .ass-guard/...).
	if dir := filepath.Dir(path); dir != "" {
		err := os.MkdirAll(dir, 0o755)
		if err != nil {
			return nil, fmt.Errorf("learning: mkdir %q: %w", dir, err)
		}
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		werr := s.save(nil)
		if werr != nil {
			return nil, fmt.Errorf("learning: create %q: %w", path, werr)
		}
	}

	return s, nil
}

// Lookup returns the entry matching situation's slug, skipping conflicted +
// expired entries. Returns (Entry{}, false) when no usable entry exists (the
// engine re-asks in that case).
func (s *Store) Lookup(situation string) (Entry, bool) {
	entries := s.load()
	slug := Slug(situation)
	now := time.Now()

	for _, e := range entries {
		if e.ID != slug {
			continue
		}

		if e.Status == StatusConflict {
			continue
		}

		if !e.Expiry.IsZero() && now.After(e.Expiry) {
			// Expired — skip (NOT deleted; List still returns it so the operator
			// can renew/purge).
			continue
		}

		return copyEntry(e), true
	}

	return Entry{}, false
}

// RecordCandidate creates a confidence-0 candidate entry for the situation (the
// engine uses the answer immediately; subsequent observations call Confirm).
// Idempotent: an existing entry with the slug is NOT overwritten (the operator
// must Revert to change a stored answer — LRN-03).
func (s *Store) RecordCandidate(situation, answer, sourceTurnID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries := s.load()

	slug := Slug(situation)
	for _, e := range entries {
		if e.ID == slug {
			return nil // idempotent — do not overwrite
		}
	}

	entry := Entry{
		ID:          slug,
		Situation:   situation,
		Answer:      answer,
		Confidence:  0,
		Expiry:      time.Now().Add(DefaultExpiryDays * 24 * time.Hour),
		SourceTurns: []string{sourceTurnID},
		Status:      StatusCandidate,
	}

	return s.save(append(entries, entry))
}

// Confirm increments the confidence of the situation's entry when the answer
// matches; at >=3 it flips to active (LRN-03). A DIFFERENT answer flips it to
// conflict + Confidence is NOT incremented + ErrConflict is returned. A missing
// entry yields ErrNotFound (the caller should RecordCandidate first).
func (s *Store) Confirm(situation, answer, sourceTurnID string) (Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries := s.load()

	slug := Slug(situation)
	for i := range entries {
		if entries[i].ID != slug {
			continue
		}

		if entries[i].Answer != answer {
			entries[i].Status = StatusConflict
			_ = s.save(entries)

			return copyEntry(entries[i]), ErrConflict
		}

		entries[i].Confidence++

		entries[i].SourceTurns = appendUnique(entries[i].SourceTurns, sourceTurnID)
		if entries[i].Confidence >= 3 {
			entries[i].Status = StatusActive
		}

		_ = s.save(entries)

		return copyEntry(entries[i]), nil
	}

	return Entry{}, ErrNotFound
}

// List returns a copy of every entry (in file order) — including conflicted +
// expired ones, so the operator sees the full picture. Deterministic order.
func (s *Store) List() []Entry {
	entries := s.load()

	out := make([]Entry, len(entries))
	for i := range entries {
		out[i] = copyEntry(entries[i])
	}

	return out
}

// Revert removes the entry with the matching ID + atomically rewrites the file
// (LRN-04 / D-20). A missing id is a no-op (not an error) — the operator may
// re-run revert after a manual edit. Returns ErrNotFound... no: per T3 Test 4,
// a re-Revert of the same id is a no-op (the CLI warns but Revert itself does
// not error).
func (s *Store) Revert(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries := s.load()

	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if e.ID == id {
			continue // drop
		}

		out = append(out, e)
	}

	return s.save(out)
}

// load reads the file + unmarshals it. A missing/empty file yields nil (no
// entries). Read-only — callers that mutate MUST hold s.mu.
func (s *Store) load() []Entry {
	raw, err := os.ReadFile(s.path)
	if err != nil || len(raw) == 0 {
		return nil
	}

	var env fileEnvelope
	if err := yaml.Unmarshal(raw, &env); err != nil {
		return nil
	}

	return env.Entries
}

// save atomically writes the entries (temp + rename). MUST be called under s.mu
// by mutating methods.
func (s *Store) save(entries []Entry) error {
	env := fileEnvelope{Entries: entries}

	raw, err := yaml.Marshal(env)
	if err != nil {
		return fmt.Errorf("learning: marshal: %w", err)
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("learning: write tmp %q: %w", tmp, err)
	}

	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("learning: rename %q → %q: %w", tmp, s.path, err)
	}

	return nil
}

// copyEntry returns a deep-enough copy so callers cannot mutate the store's
// in-memory slice via the returned Entry (the SourceTurns slice is copied).
func copyEntry(e Entry) Entry {
	if e.SourceTurns != nil {
		e.SourceTurns = append([]string(nil), e.SourceTurns...)
	}

	return e
}

// appendUnique appends s to list only if it is not already present (provenance
// dedup — Test 8).
func appendUnique(list []string, s string) []string {
	if s == "" {
		return list
	}

	if slices.Contains(list, s) {
		return list
	}

	return append(list, s)
}
