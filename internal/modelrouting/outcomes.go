package modelrouting

import "time"

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

// OutcomeStore is the append-only JSONL outcome store under
// root/.ass-guard/routing/outcomes.jsonl (D-05).
//
// RED stub (plan 24-01 Task 1): signatures only, zero-value behavior — the
// failing suite in outcomes_test.go is the contract Task 2 implements.
type OutcomeStore struct{}

// NewOutcomeStore resolves the store under root/.ass-guard/routing and ensures
// the directory tree plus the .ass-guard self-gitignore exist.
//
// RED stub: does nothing yet.
func NewOutcomeStore(_ string) (*OutcomeStore, error) {
	return &OutcomeStore{}, nil
}

// Append writes one record as one JSONL line.
//
// RED stub: does nothing yet.
func (*OutcomeStore) Append(_ DispatchOutcome) error {
	return nil
}

// Path returns the store file's absolute path.
//
// RED stub: always empty.
func (*OutcomeStore) Path() string {
	return ""
}

// ReadOutcomes reads a store file tolerantly: malformed or unknown-shape lines
// are skipped and counted, never failing the whole read.
//
// RED stub: does nothing yet.
func ReadOutcomes(_ string) ([]DispatchOutcome, int, error) {
	return nil, 0, nil
}
