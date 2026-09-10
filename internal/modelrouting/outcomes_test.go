package modelrouting //nolint:testpackage // internal package test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/provider"
)

// Fixed clocks for the D-07 feedback tests. House rule: no time.Now inside
// logic under test — replay consumes record.At chronologically, and Allow/Check
// take now explicitly, so both are pinned here.
var (
	outcomeSeedBase = time.Date(2026, time.September, 10, 12, 0, 0, 0, time.UTC)
	outcomeFixedNow = outcomeSeedBase.Add(30 * time.Second)
)

// outcomeEqual pins the exact D-04 field set: every field DispatchOutcome
// carries is compared here, so a content-bearing field added to the schema must
// surface in this comparison (T-24-01-01 — mechanical signals only).
func outcomeEqual(a, b DispatchOutcome) bool {
	return a.At.Equal(b.At) &&
		a.Provider == b.Provider &&
		a.Model == b.Model &&
		a.Tier == b.Tier &&
		a.Outcome == b.Outcome &&
		a.FallbackUsed == b.FallbackUsed &&
		a.LatencyMS == b.LatencyMS &&
		a.InTokens == b.InTokens &&
		a.OutTokens == b.OutTokens &&
		a.CostUSD == b.CostUSD &&
		a.Origin == b.Origin
}

// TestOutcomeStoreAppendReadRoundtrip: N appends read back as exactly N records
// with every D-04 field intact (D-05's append-only store contract). The probe at
// the end pins the no-dedup law: an identical re-append yields a SECOND stored
// record — one record per provider attempt is guaranteed by the recording sites
// (plan 24-02), never by the store.
func TestOutcomeStoreAppendReadRoundtrip(t *testing.T) {
	t.Parallel()

	store, err := NewOutcomeStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewOutcomeStore: %v", err)
	}

	want := []DispatchOutcome{
		{
			At: outcomeSeedBase, Provider: "prov-a", Model: "model-a", Tier: tierHeavy,
			Outcome: OutcomeOK, LatencyMS: 120, InTokens: 1000, OutTokens: 2000,
			CostUSD: 0.012, Origin: OutcomeOriginTurn,
		},
		{
			At: outcomeSeedBase.Add(time.Second), Provider: "prov-a", Model: "model-b", Tier: tierLight,
			Outcome: OutcomeTransient, FallbackUsed: true, LatencyMS: 5000, InTokens: 3000,
			Origin: OutcomeOriginSubagent,
		},
		{
			At: outcomeSeedBase.Add(2 * time.Second), Provider: "prov-b", Model: "model-a", Tier: tierHeavy,
			Outcome: OutcomeStructural, LatencyMS: 40, Origin: OutcomeOriginTurn,
		},
	}

	for _, rec := range want {
		if err := store.Append(rec); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	got, skipped, err := ReadOutcomes(store.Path())
	if err != nil {
		t.Fatalf("ReadOutcomes: %v", err)
	}
	if skipped != 0 {
		t.Errorf("clean store: skipped = %d, want 0", skipped)
	}
	if len(got) != len(want) {
		t.Fatalf("read back %d records, want %d", len(got), len(want))
	}
	for i := range want {
		if !outcomeEqual(got[i], want[i]) {
			t.Errorf("record %d round-trip mismatch:\n got  %+v\n want %+v", i, got[i], want[i])
		}
	}

	// Append-only, NO dedup: the same record again is a second stored line.
	if err := store.Append(want[1]); err != nil {
		t.Fatalf("re-Append: %v", err)
	}
	got, _, err = ReadOutcomes(store.Path())
	if err != nil {
		t.Fatalf("ReadOutcomes after re-append: %v", err)
	}
	if len(got) != len(want)+1 {
		t.Errorf("after identical re-append: %d records, want %d (the store never dedups)", len(got), len(want)+1)
	}
}

// TestOutcomeStoreTolerantRead: malformed lines are skipped and counted, and a
// record carrying an unknown FUTURE field is kept (16-D-20 additive discipline)
// — a partially corrupt file never fails the whole read.
func TestOutcomeStoreTolerantRead(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".ass-guard", "routing", "outcomes.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	first := DispatchOutcome{
		At: outcomeSeedBase, Provider: "prov-t", Model: "model-t", Tier: tierHeavy,
		Outcome: OutcomeOK, LatencyMS: 10, Origin: OutcomeOriginTurn,
	}
	line, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// A valid record carrying an unknown extra JSON field (forward-compatible).
	const withUnknownField = `{"at":"2026-09-10T12:00:05Z","provider":"prov-u","model":"model-u",` +
		`"tier":"light","outcome":"ok","origin":"turn","future_field":{"x":1}}`
	const malformed = `{"at": not json`

	content := string(line) + "\n" + malformed + "\n" + withUnknownField + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, skipped, err := ReadOutcomes(path)
	if err != nil {
		t.Fatalf("tolerant read must not fail the whole file: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("read %d valid records, want 2 (malformed skipped, unknown field tolerated)", len(got))
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want exactly 1 (the malformed line)", skipped)
	}
}

// TestOutcomeStoreInterruptedTail: a crash mid-append leaves at most one
// truncated trailing line (open-per-append keeps the window to one line); the
// read returns the earlier records and SKIPS the tail — never fails, never
// drops earlier lines.
func TestOutcomeStoreInterruptedTail(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".ass-guard", "routing", "outcomes.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	rec := DispatchOutcome{
		At: outcomeSeedBase, Provider: "prov-i", Model: "model-i", Tier: tierHeavy,
		Outcome: OutcomeOK, Origin: OutcomeOriginTurn,
	}
	line, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	content := string(line) + "\n" + `{"at":"2026-09-10T12` // interrupted write: valid prefix, no terminator
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, skipped, err := ReadOutcomes(path)
	if err != nil {
		t.Fatalf("interrupted tail must not fail the read: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("read %d records, want 1 (earlier lines survive the torn tail)", len(got))
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1 (the truncated trailing line)", skipped)
	}
	if !outcomeEqual(got[0], rec) {
		t.Errorf("surviving record mismatch:\n got  %+v\n want %+v", got[0], rec)
	}
}

// TestOutcomeStorePermsAndGitignore: the store honors the .ass-guard/ house
// conventions — dirs at most 0750, store file at most 0600, and the
// self-exclusion .gitignore body written by the store itself (modelrouting does
// NOT import internal/ecosys or internal/session for this).
func TestOutcomeStorePermsAndGitignore(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := NewOutcomeStore(root)
	if err != nil {
		t.Fatalf("NewOutcomeStore: %v", err)
	}
	if err := store.Append(DispatchOutcome{
		At: outcomeSeedBase, Provider: "prov-p", Model: "model-p", Tier: tierHeavy,
		Outcome: OutcomeOK, Origin: OutcomeOriginTurn,
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	for _, dir := range []string{
		filepath.Join(root, ".ass-guard"),
		filepath.Join(root, ".ass-guard", "routing"),
	} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("Stat %s: %v", dir, err)
		}
		if perm := info.Mode().Perm(); perm&^0o750 != 0 {
			t.Errorf("dir %s mode %o exceeds 0750", dir, perm)
		}
	}

	info, err := os.Stat(store.Path())
	if err != nil {
		t.Fatalf("Stat store file: %v", err)
	}
	if perm := info.Mode().Perm(); perm&^0o600 != 0 {
		t.Errorf("store file mode %o exceeds 0600", perm)
	}

	body, err := os.ReadFile(filepath.Join(root, ".ass-guard", ".gitignore"))
	if err != nil {
		t.Fatalf("self-gitignore must exist after the first append: %v", err)
	}
	if string(body) != "*\n!.gitignore\n" {
		t.Errorf("self-gitignore body = %q, want %q", string(body), "*\n!.gitignore\n")
	}
}

// TestOutcomeStoreConcurrentAppend: 10 goroutines x 10 records produce exactly
// 100 intact JSONL lines (mutex + O_APPEND keep every append one whole line —
// parent turns and subagents share one store).
func TestOutcomeStoreConcurrentAppend(t *testing.T) {
	t.Parallel()

	store, err := NewOutcomeStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewOutcomeStore: %v", err)
	}

	const (
		goroutines = 10
		perG       = 10
	)
	var wg sync.WaitGroup
	for g := range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range perG {
				rec := DispatchOutcome{
					At: outcomeSeedBase.Add(time.Duration(g*perG+i) * time.Second),
					Provider: "prov-c", Model: fmt.Sprintf("model-%d-%d", g, i), Tier: tierLight,
					Outcome: OutcomeOK, LatencyMS: int64(i), Origin: OutcomeOriginSubagent,
				}
				if err := store.Append(rec); err != nil {
					t.Errorf("Append %d-%d: %v", g, i, err)
				}
			}
		}()
	}
	wg.Wait()

	got, skipped, err := ReadOutcomes(store.Path())
	if err != nil {
		t.Fatalf("ReadOutcomes: %v", err)
	}
	if len(got) != goroutines*perG {
		t.Errorf("read %d records, want %d (one intact line per append)", len(got), goroutines*perG)
	}
	if skipped != 0 {
		t.Errorf("skipped = %d, want 0 (no torn lines under the append mutex)", skipped)
	}
}

// TestOutcomeFeedbackBreakerOpens (D-07's deterministic half, breaker seam):
// seeding ConsecutiveFailures consecutive transients for one (provider, model)
// and replaying them through the EXISTING breaker seam opens that key's
// breaker; a key never seen in the store is ABSENT from the replayed map
// (absent = allowed by convention, the SetBreakers no-op fallback).
func TestOutcomeFeedbackBreakerOpens(t *testing.T) {
	t.Parallel()

	cfg := CircuitBreakerConfig{
		ConsecutiveFailures: 3, ErrorRateWindow: 10, ErrorRateThreshold: 0.5,
		Cooldown: time.Minute, HalfOpenProbes: 1,
	}

	records := make([]DispatchOutcome, 0, cfg.ConsecutiveFailures)
	for i := range cfg.ConsecutiveFailures {
		records = append(records, DispatchOutcome{
			At: outcomeSeedBase.Add(time.Duration(i) * time.Second),
			Provider: "providerA", Model: "modelA", Tier: tierHeavy,
			Outcome: OutcomeTransient, LatencyMS: 900, Origin: OutcomeOriginTurn,
		})
	}

	m := ReplayBreakers(records, cfg, nil)

	key := ProviderModelKey{Provider: "providerA", Model: "modelA"}
	b, ok := m[key]
	if !ok {
		t.Fatalf("replayed map must carry a breaker for the seeded key %+v", key)
	}
	// fixedNow is 28s after the trip: still inside the 1m cooldown -> Open.
	if b.Allow(outcomeFixedNow) {
		t.Errorf("breaker for %+v must be Open after %d consecutive transients", key, cfg.ConsecutiveFailures)
	}

	if _, seen := m[ProviderModelKey{Provider: "never", Model: "seen"}]; seen {
		t.Error("a key never seen in the store must be ABSENT from the replayed map (absent = allowed)")
	}
}

// TestOutcomeFeedbackBreakerRecoversAndSkipsStructural: an ok after transients
// resets the consecutive counter (the breaker stays Closed, Allow true), and
// structural/exhausted records NEVER create or feed a breaker on replay —
// replay mirrors the Dispatch outcome-learning points exactly (structural does
// not open breakers).
func TestOutcomeFeedbackBreakerRecoversAndSkipsStructural(t *testing.T) {
	t.Parallel()

	cfg := CircuitBreakerConfig{
		ConsecutiveFailures: 3, ErrorRateWindow: 10, ErrorRateThreshold: 0.5,
		Cooldown: time.Minute, HalfOpenProbes: 1,
	}

	records := []DispatchOutcome{
		{
			At: outcomeSeedBase, Provider: "provR", Model: "modelR", Tier: tierHeavy,
			Outcome: OutcomeTransient, Origin: OutcomeOriginTurn,
		},
		{
			At: outcomeSeedBase.Add(time.Second), Provider: "provR", Model: "modelR", Tier: tierHeavy,
			Outcome: OutcomeTransient, Origin: OutcomeOriginTurn,
		},
		{
			At: outcomeSeedBase.Add(2 * time.Second), Provider: "provR", Model: "modelR", Tier: tierHeavy,
			Outcome: OutcomeOK, Origin: OutcomeOriginTurn,
		},
		{
			At: outcomeSeedBase.Add(3 * time.Second), Provider: "provS", Model: "modelS", Tier: tierHeavy,
			Outcome: OutcomeStructural, Origin: OutcomeOriginTurn,
		},
		{
			At: outcomeSeedBase.Add(4 * time.Second), Provider: "provS", Model: "modelS", Tier: tierHeavy,
			Outcome: OutcomeExhausted, Origin: OutcomeOriginTurn,
		},
	}

	m := ReplayBreakers(records, cfg, nil)

	keyR := ProviderModelKey{Provider: "provR", Model: "modelR"}
	b, ok := m[keyR]
	if !ok {
		t.Fatalf("replayed map must carry a breaker for %+v", keyR)
	}
	if !b.Allow(outcomeFixedNow) {
		t.Errorf("breaker for %+v must have recovered (an ok resets consecutive transients)", keyR)
	}

	if _, seen := m[ProviderModelKey{Provider: "provS", Model: "modelS"}]; seen {
		t.Error("structural/exhausted records must never create or feed a breaker on replay")
	}
}

// TestOutcomeFeedbackCostDegrade (D-07's deterministic half, cost seam): seeded
// token-bearing records replayed through the EXISTING CostTracker seam return
// CostDegrade from Check once the windowed spend crosses the configured
// ceiling.
func TestOutcomeFeedbackCostDegrade(t *testing.T) {
	t.Parallel()

	pricing := map[string]Pricing{
		"model-c": {InputPerMToken: 1.0, OutputPerMToken: 2.0},
	}
	cfg := CostCeilingConfig{AmountUSD: 0.01, Window: 24 * time.Hour, DegradeTo: tierLight}

	// 10k in + 10k out at $1/$2 per MToken = $0.03 — crosses the $0.01 ceiling.
	records := []DispatchOutcome{
		{
			At: outcomeSeedBase, Provider: "prov-c", Model: "model-c", Tier: tierHeavy,
			Outcome: OutcomeOK, InTokens: 10000, OutTokens: 10000,
			CostUSD: 0.03, Origin: OutcomeOriginTurn,
		},
	}

	tr := ReplayCostTracker(records, cfg, pricing, nil, nil)
	if tr == nil {
		t.Fatal("ReplayCostTracker returned a nil CostTracker")
	}
	if act := tr.Check(outcomeFixedNow); act != CostDegrade {
		t.Errorf("Check = %d, want CostDegrade (%d) — replayed spend must cross the ceiling", act, CostDegrade)
	}
}

// TestFirstAllowedDemotes: the fallback walk consults the breaker map — the
// first candidate whose breaker (absent = allowed) admits now wins, and the
// second return reports whether the walk demoted past the primary.
func TestFirstAllowedDemotes(t *testing.T) {
	t.Parallel()

	const (
		provPrimary  = "prov-1"
		modelPrimary = "model-1"
	)
	primary := Target{Provider: provPrimary, Model: modelPrimary}
	fallback := Target{Provider: "prov-2", Model: "model-2"}

	// newOpenBreaker builds a breaker tripped 1 minute before outcomeFixedNow
	// (inside its 10m cooldown), so Allow(outcomeFixedNow) is false.
	newOpenBreaker := func() *CircuitBreaker {
		cfg := CircuitBreakerConfig{
			ConsecutiveFailures: 2, ErrorRateWindow: 10, ErrorRateThreshold: 0.9,
			Cooldown: 10 * time.Minute, HalfOpenProbes: 1,
		}
		b := NewCircuitBreaker(ProviderModelKey{Provider: provPrimary, Model: modelPrimary}, cfg, nil)
		tripAt := outcomeFixedNow.Add(-time.Minute)
		perr := &provider.ProviderError{
			Kind: provider.KindTransient, Provider: provPrimary, Model: modelPrimary,
		}
		b.RecordTransient(tripAt, perr)
		b.RecordTransient(tripAt, perr)

		return b
	}

	cases := []struct {
		name        string
		candidates  []Target
		breakers    map[ProviderModelKey]Breaker
		wantTarget  Target
		wantDemoted bool
	}{
		{
			name:       "primary open — demotes to the fallback",
			candidates: []Target{primary, fallback},
			breakers: map[ProviderModelKey]Breaker{
				{Provider: provPrimary, Model: modelPrimary}: newOpenBreaker(),
			},
			wantTarget:  fallback,
			wantDemoted: true,
		},
		{
			name:       "all allowing — primary wins with no demotion",
			candidates: []Target{primary, fallback},
			breakers:   map[ProviderModelKey]Breaker{}, // absent key = allowed
			wantTarget:  primary,
			wantDemoted: false,
		},
	}

	for _, tc := range cases {
		got, demoted := FirstAllowed(tc.candidates, tc.breakers, outcomeFixedNow)
		if got.Provider != tc.wantTarget.Provider || got.Model != tc.wantTarget.Model {
			t.Errorf("%s: FirstAllowed = (%s, %s), want (%s, %s)",
				tc.name, got.Provider, got.Model, tc.wantTarget.Provider, tc.wantTarget.Model)
		}
		if demoted != tc.wantDemoted {
			t.Errorf("%s: demoted = %v, want %v", tc.name, demoted, tc.wantDemoted)
		}
	}
}
