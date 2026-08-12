package learning

import (
	"regexp"
	"strings"
	"time"
)

// EntryStatus classifies a learned entry's lifecycle (LRN-03).
type EntryStatus string

const (
	// StatusCandidate is a provisional entry (confidence < 3) — still used by the
	// engine, but flagged so the operator knows it is unconfirmed.
	StatusCandidate EntryStatus = "candidate"
	// StatusActive is a confirmed entry (confidence >= 3).
	StatusActive EntryStatus = "active"
	// StatusConflict marks an entry whose stored answer disagrees with a later
	// observation — Lookup skips it (the operator resolves via revert).
	StatusConflict EntryStatus = "conflict"
)

// DefaultExpiryDays is the default entry review/expiry window (D-19 / LRN-03).
const DefaultExpiryDays = 30

// Entry is one learned setting — the question/situation asked, the answer/action
// learned, the confidence count, the expiry/review date, source-turn refs, and
// the status. The ID is a stable slug of Situation (deterministic).
type Entry struct {
	ID          string      `yaml:"id"`
	Situation   string      `yaml:"situation"`
	Answer      string      `yaml:"answer"`
	Confidence  int         `yaml:"confidence"`
	Expiry      time.Time   `yaml:"expiry"`
	SourceTurns []string    `yaml:"source_turns"`
	Status      EntryStatus `yaml:"status"`
}

// WorklogEntry is one observed manual step (the engine feeds these from the
// transcript; ProposeHooks mines them for repeated sequences).
type WorklogEntry struct {
	TurnID string
	Tool   string
	Args   string
}

// Proposal is one detected repeated sequence proposed as a new hook (LRN-02).
type Proposal struct {
	HookName    string
	Trigger     string
	Steps       []string
	Occurrences int
}

// slugNonAlnum matches runs of non-alphanumeric characters in a lowercased
// situation string (compiled once at package init).
var slugNonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// Slug produces the stable ID for a Situation (lowercase + non-alphanumerics →
// "-" + trim; deterministic — same situation ⇒ same ID so Lookup/Confirm find
// the right entry across runs).
func Slug(situation string) string {
	s := strings.ToLower(situation)
	s = slugNonAlnum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")

	return s
}
