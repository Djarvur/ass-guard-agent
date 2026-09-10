package modelrouting

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Outcome classes (D-04): the mechanical result of one dispatch attempt. ok is
// the RecordSuccess path; transient is the RecordTransient path; structural and
// exhausted are terminal outcomes that never feed breakers (the Dispatch
// outcome-learning points — structural does not open breakers).
const (
	OutcomeOK         = "ok"
	OutcomeTransient  = "transient"
	OutcomeStructural = "structural"
	OutcomeExhausted  = "exhausted"
)

// Record origins (D-04): which execution surface produced the dispatch.
const (
	OutcomeOriginTurn     = "turn"
	OutcomeOriginSubagent = "subagent"
)

// DispatchOutcome is one mechanical dispatch-outcome record (D-04/D-05). The
// schema carries MECHANICAL signals only — no request/response content, no
// prompts, no LLM-judged quality signal; that zero-LLM boundary is hard and is
// pinned by TestOutcomeStoreAppendReadRoundtrip comparing the exact field set.
//
// Zero InTokens/OutTokens/CostUSD means UNKNOWN on the recording path, never
// free (the Send path carries no token counts) — aggregation must never read a
// zero as spend.
type DispatchOutcome struct {
	At           time.Time `json:"at"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	Tier         string    `json:"tier"`
	Outcome      string    `json:"outcome"`
	FallbackUsed bool      `json:"fallback_used"`
	LatencyMS    int64     `json:"latency_ms"`
	InTokens     int64     `json:"in_tokens"`
	OutTokens    int64     `json:"out_tokens"`
	CostUSD      float64   `json:"cost_usd"`
	Origin       string    `json:"origin"`
}

// Store layout constants (the .ass-guard/ artifact family house conventions —
// internal/sched/sched.go and internal/session/transcript.go precedents; the
// self-gitignore `*` rule covers the routing/ subdir).
const (
	outcomeStoreDir   = ".ass-guard"
	outcomeRoutingDir = "routing"
	outcomeStoreFile  = "outcomes.jsonl"
	outcomeFilePerm   = 0o600
	outcomeDirPerm    = 0o750
)

// outcomeSelfGitignore is the .ass-guard/.gitignore body — ignore everything
// except .gitignore itself. Deliberately a local copy of the
// transcript.go/ecosys pattern: modelrouting must not import internal/session
// or internal/ecosys for a two-line constant.
const outcomeSelfGitignore = "*\n!.gitignore\n"

// OutcomeStore is the append-only JSONL outcome store at
// root/.ass-guard/routing/outcomes.jsonl (D-05: no SQLite, no rewritten
// aggregate file — aggregation happens in memory at read time). One record per
// provider attempt is guaranteed by the recording sites, never by the store:
// there is no dedup, so an identical re-append is a second stored line.
type OutcomeStore struct {
	mu   sync.Mutex
	path string
}

// NewOutcomeStore resolves the store path under root/.ass-guard/routing,
// ensures the directory tree exists (0750) and writes the .ass-guard
// self-exclusion .gitignore when absent. I/O failures are returned, never
// panicked.
func NewOutcomeStore(root string) (*OutcomeStore, error) {
	dir := filepath.Join(root, outcomeStoreDir, outcomeRoutingDir)
	if err := os.MkdirAll(dir, outcomeDirPerm); err != nil {
		return nil, fmt.Errorf("outcome store: mkdir: %w", err)
	}

	giPath := filepath.Join(root, outcomeStoreDir, ".gitignore")
	if _, err := os.Stat(giPath); os.IsNotExist(err) {
		if err := os.WriteFile(giPath, []byte(outcomeSelfGitignore), outcomeFilePerm); err != nil {
			return nil, fmt.Errorf("outcome store: write self-gitignore: %w", err)
		}
	}

	return &OutcomeStore{path: filepath.Join(dir, outcomeStoreFile)}, nil
}

// Append writes one record as exactly one JSONL line. Appends are serialized
// on the store mutex and the file is opened O_APPEND|O_CREATE|O_WRONLY 0600
// per call, so the interrupted-write window never spans more than one line
// and concurrent turns/subagents cannot interleave bytes within a line.
//
// Store I/O failures are returned, never panicked; callers treat them as
// loud-non-fatal (a broken store must never fail the turn — T-24-01-02).
func (s *OutcomeStore) Append(rec DispatchOutcome) error {
	line, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("outcome store: marshal record: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, outcomeFilePerm)
	if err != nil {
		return fmt.Errorf("outcome store: open: %w", err)
	}

	if _, err := f.Write(append(line, '\n')); err != nil {
		f.Close() //nolint:errcheck // best-effort close; the error of interest is Write's
		return fmt.Errorf("outcome store: append: %w", err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("outcome store: close: %w", err)
	}

	return nil
}

// Path returns the store file's path (readers pass it to ReadOutcomes).
func (s *OutcomeStore) Path() string {
	return s.path
}

// ReadOutcomes reads a store file tolerantly (the D-05 read-time aggregation
// input): every line is one DispatchOutcome; malformed lines, a truncated
// trailing line (an interrupted append), and lines with an unknown shape are
// SKIPPED and counted — the read never fails on a partially corrupt file and
// never drops earlier records. A missing file is an empty store (no records
// before the first append), not an error. Genuine I/O failures are returned,
// never panicked.
func ReadOutcomes(path string) ([]DispatchOutcome, int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, nil
		}

		return nil, 0, fmt.Errorf("outcome store: read: %w", err)
	}

	var (
		records []DispatchOutcome
		skipped int
	)

	for _, line := range strings.Split(string(raw), "\n") {
		if line == "" {
			continue // trailing newline / blank line — not a record attempt
		}

		var rec DispatchOutcome
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			skipped++

			continue
		}

		if !rec.hasKnownShape() {
			skipped++

			continue
		}

		records = append(records, rec)
	}

	return records, skipped, nil
}

// hasKnownShape reports whether a parsed line carries the fields every record
// is guaranteed to carry: a timestamp, the (provider, model) identity, and a
// known outcome class. Lines missing them are skipped as unknown shapes rather
// than failing the read (16-D-20 tolerance: skip, never fail; a future outcome
// class reads as skipped on old binaries, never as an error).
func (r *DispatchOutcome) hasKnownShape() bool {
	if r.At.IsZero() || r.Provider == "" || r.Model == "" {
		return false
	}

	switch r.Outcome {
	case OutcomeOK, OutcomeTransient, OutcomeStructural, OutcomeExhausted:
		return true
	default:
		return false
	}
}
