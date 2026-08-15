package audit // (09-05, AUD-03/D-01: the capped body store — see audit.go's package doc)

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/Djarvur/ass-guard-agent/internal/redact"
)

// DefaultBodyStoreCap is the total-bytes cap (64 MiB — generous for a day of
// debugging; the dominant ~80 KB identical-catalog class dedups to ONE file).
const DefaultBodyStoreCap = 64 << 20

// Directory/file permission constants (owner-only, matching the audit sink).
const (
	bodyStoreDirPerm  = 0o700
	bodyStoreFilePerm = 0o600
)

// ErrBodyNotFound is returned by Get for a ref with no stored body (evicted or
// never written). Typed so callers can distinguish absence from IO failure.
var ErrBodyNotFound = errors.New("audit: body not found (evicted or never stored)")

// BodyStore is the size-capped, oldest-evicted, content-addressed store of
// REDACTED request bodies. Redaction happens INSIDE Put, before any byte hits
// disk (the ONE chokepoint discipline, Pitfall 9). All diagnostics travel as
// returned errors — the CALLER logs to stderr (loud, never turn-fatal); this
// file never writes to stdout (transport discipline).
type BodyStore struct {
	dir      string
	capBytes int64

	mu sync.Mutex
}

// NewBodyStore returns a store rooted at dir. capBytes <= 0 selects
// DefaultBodyStoreCap. Construction is lazy: the directory tree is created on
// first Put, so a store over a not-yet-existing (or unwritable) path still
// constructs and reports errors per-Put — audit degradation, never refusal.
func NewBodyStore(dir string, capBytes int64) *BodyStore {
	if capBytes <= 0 {
		capBytes = DefaultBodyStoreCap
	}

	return &BodyStore{dir: dir, capBytes: capBytes}
}

// Dir returns the store's root directory (tests/inspection).
func (s *BodyStore) Dir() string { return s.dir }

// bodyRef computes the content hash of the REDACTED bytes — the ONE helper
// behind both the store key and the line ref (they cannot diverge, T-9-20).
func bodyRef(redacted []byte) string {
	sum := sha256.Sum256(redacted)

	return hex.EncodeToString(sum[:])
}

// Put stores the REDACTED form of body content-addressed and returns the ref.
// Redaction errors fall back to ScrubError bytes (mirroring audit.go's
// handle()); write failures are RETURNED (the caller logs); cap enforcement
// evicts oldest-mtime files best-effort (eviction errors are returned with the
// ref — the body IS stored even when the sweep lags).
func (s *BodyStore) Put(body []byte) (string, error) {
	redacted, err := redact.Redact(body)
	if err != nil {
		//nolint:err113 // dynamic error message
		redacted = []byte(redact.ScrubError(fmt.Errorf("%s", string(body))))
	}

	ref := bodyRef(redacted)
	path := filepath.Join(s.dir, ref[:2], ref+".json")

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, statErr := os.Stat(path); statErr == nil { //nolint:noinlineerr // boolean guard, not error flow
		return ref, s.evictLocked() // dedup: content already stored
	}

	mkErr := os.MkdirAll(filepath.Dir(path), bodyStoreDirPerm)
	if mkErr != nil {
		return ref, fmt.Errorf("audit: body store mkdir: %w", mkErr)
	}

	wErr := os.WriteFile(path, redacted, bodyStoreFilePerm)
	if wErr != nil {
		return ref, fmt.Errorf("audit: body store write: %w", wErr)
	}

	evErr := s.evictLocked()
	if evErr != nil {
		return ref, fmt.Errorf("audit: body store evict: %w", evErr)
	}

	return ref, nil
}

// Get returns the stored (redacted) body for ref. ErrBodyNotFound when the ref
// was evicted or never written.
func (s *BodyStore) Get(ref string) ([]byte, error) {
	// minRefLen guards the ref[:2] shard slicing below (a "xx…" prefix).
	const minRefLen = 3

	if len(ref) < minRefLen {
		return nil, ErrBodyNotFound
	}

	raw, err := os.ReadFile(filepath.Join(s.dir, ref[:2], ref+".json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrBodyNotFound
		}

		return nil, fmt.Errorf("audit: body store read: %w", err)
	}

	return raw, nil
}

// evictLocked sweeps the store and removes oldest-mtime files until the total
// is under the cap. Best-effort: individual removal errors abort the sweep
// (returned) without corrupting anything — the next Put retries.
func (s *BodyStore) evictLocked() error {
	type entry struct {
		path string
		size int64
		mod  int64 // unix nano
	}

	var entries []entry

	var total int64

	err := filepath.WalkDir(s.dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}

		info, infoErr := d.Info()
		if infoErr != nil {
			return fmt.Errorf("stat %s: %w", path, infoErr)
		}

		entries = append(entries, entry{path: path, size: info.Size(), mod: info.ModTime().UnixNano()})
		total += info.Size()

		return nil
	})
	if err != nil {
		return fmt.Errorf("sweep: %w", err)
	}

	if total <= s.capBytes {
		return nil
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].mod < entries[j].mod }) // oldest first

	for _, e := range entries {
		if total <= s.capBytes {
			break
		}

		rmErr := os.Remove(e.path)
		if rmErr != nil && !os.IsNotExist(rmErr) {
			return fmt.Errorf("remove %s: %w", e.path, rmErr)
		}

		total -= e.size
	}

	return nil
}

// RequestMeta is the metadata-only fingerprint of a shaped request (D-01:
// correlation IDs, shapes, content hashes — NO inline bodies). The shape
// fields are exactly the within-session stability invariants (system-block
// count, tool count, model), so parity drift is visible by eyeballing
// consecutive lines.
type RequestMeta struct {
	Ref string `json:"ref"`
	//nolint:tagliatelle // on-disk format: camelCase matches the transcript's Line convention
	SystemBlocks int    `json:"systemBlocks"`
	Tools        int    `json:"tools"`
	Model        string `json:"model"`
	Bytes        int    `json:"bytes"`
}

// SummarizeRequest produces the metadata fingerprint of a shaped request body.
// Ref is the sha256 of the REDACTED bytes — the SAME hash Put stores under
// (one shared helper, bodyRef) — so the line's ref and the store key cannot
// diverge even when the store write itself fails (the hash exists regardless).
func SummarizeRequest(body []byte) (RequestMeta, error) {
	redacted, rerr := redact.Redact(body)
	if rerr != nil {
		//nolint:err113 // dynamic error message
		redacted = []byte(redact.ScrubError(fmt.Errorf("%s", string(body))))
	}

	var shape struct {
		Model  string          `json:"model"`
		System json.RawMessage `json:"system"`
		Tools  json.RawMessage `json:"tools"`
	}

	// A malformed body still yields a usable ref (the fingerprint fields stay
	// zero) — metadata lines are better than dropped lines.
	_ = json.Unmarshal(body, &shape)

	meta := RequestMeta{
		Ref:   bodyRef(redacted),
		Model: shape.Model,
		Bytes: len(body),
	}

	if len(shape.System) > 0 {
		var blocks []json.RawMessage
		if json.Unmarshal(shape.System, &blocks) == nil {
			meta.SystemBlocks = len(blocks)
		}
	}

	if len(shape.Tools) > 0 {
		var tools []json.RawMessage
		if json.Unmarshal(shape.Tools, &tools) == nil {
			meta.Tools = len(tools)
		}
	}

	return meta, nil
}
