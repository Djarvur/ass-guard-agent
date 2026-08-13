package scheduler

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Resolver applies the D-02 precedence (time-window → project → global) to
// resolve a tier to a concrete Target at a given moment. It is a PURE FUNCTION
// of (config, tier, project, now): no sync, no logging, no network (RESEARCH
// §4, §13.1). Lazy per-request time evaluation (D-03): each Resolve call checks
// the current wall-clock against the configured IANA zone. time/tzdata is
// bundled (init.go) so time.LoadLocation works without host zoneinfo.
type Resolver struct {
	cfg *Config
}

// NewResolver returns a Resolver over the given (already-validated) config.
func NewResolver(cfg *Config) *Resolver {
	return &Resolver{cfg: cfg}
}

// Resolve resolves a tier to a concrete primary Target plus an ordered fallback
// chain (D-05), applying the D-02 precedence:
//  1. the active time-window's binding wins OUTRIGHT when it maps the tier
//     (structural — a project cannot override a window's pick, the user's
//     explicit reversal, pitfall 8);
//  2. else the per-project override (narrows the gap the window left);
//  3. else the global tier table.
//
// capReq is accepted for the request-time capability gate (D-09); the gate
// itself is wired into Resolve in Plan 03-04. This tracer returns the primary
// regardless of capReq so the D-02 precedence is the single focus (RESEARCH
// §4.1, §7.2). Every returned Target carries its CapabilityProfile (D-09) and
// Pricing (D-08).
//
//nolint:gocritic // conflicts w/ nonamedreturns
func (r *Resolver) Resolve(tier, project string, now time.Time, capReq CapabilityReq) (Target, []Target, error) {
	binding, ok := r.resolveBinding(tier, project, now)
	if !ok {
		return Target{}, nil, fmt.Errorf("scheduler: tier %q is not configured (no window/project/global binding)", tier) //nolint:err113 // dynamic error message
	}

	primary, err := r.buildTarget(binding.Model)
	if err != nil {
		return Target{}, nil, fmt.Errorf("resolve tier %q primary: %w", tier, err)
	}

	fallbacks := make([]Target, 0, len(binding.Fallback))

	for _, slug := range binding.Fallback {
		ft, err := r.buildTarget(slug)
		if err != nil {
			return Target{}, nil, fmt.Errorf("resolve tier %q fallback %q: %w", tier, slug, err)
		}

		fallbacks = append(fallbacks, ft)
	}
	// Apply the request-time capability gate (D-09): when capReq is non-zero,
	// skip incapable candidates and return the first capable one + the remaining
	// chain. A zero capReq leaves the chain unchanged (Plan 03-01 behavior).
	chosen, remaining, err := applyCapabilityGate(&primary, fallbacks, capReq)
	if err != nil {
		return Target{}, nil, fmt.Errorf("resolve tier %q: %w", tier, err)
	}

	return chosen, remaining, nil
}

// resolveBinding is the 4-line D-02 algorithm (RESEARCH §4.1).
func (r *Resolver) resolveBinding(tier, project string, now time.Time) (TierBinding, bool) {
	if w := r.activeWindow(now); w != nil {
		if b, ok := w.Tiers[tier]; ok {
			return b, true // STRUCTURAL: window wins outright (D-02 reversal)
		}
	}

	if project != "" {
		if po, ok := r.cfg.Projects[project]; ok {
			if b, ok := po.Tiers[tier]; ok {
				return b, true // PROJECT: narrows the gap
			}
		}
	}

	if b, ok := r.cfg.Tiers[tier]; ok {
		return b, true // GLOBAL: the default table
	}

	return TierBinding{}, false
}

// buildTarget enriches a model slug into a concrete Target by joining the model
// config with its provider's base URL + shape, capability profile, and pricing.
func (r *Resolver) buildTarget(modelSlug string) (Target, error) {
	m, ok := r.cfg.Models[modelSlug]
	if !ok {
		return Target{}, fmt.Errorf("model %q is not declared in models", modelSlug) //nolint:err113 // dynamic error message
	}

	p, ok := r.cfg.Providers[m.Provider]
	if !ok {
		return Target{}, fmt.Errorf("provider %q (for model %q) is not declared in providers", m.Provider, modelSlug) //nolint:err113 // dynamic error message
	}

	return Target{
		Provider:     m.Provider,
		Model:        modelSlug,
		BaseURL:      p.BaseURL,
		Shape:        p.Shape,
		Capabilities: m.Capabilities,
		Pricing:      m.Pricing,
	}, nil
}

// activeWindow returns the first TimeWindow whose schedule contains now (in the
// window's zone), or nil if none. First-match-in-config-order wins (RESEARCH
// §2.4 — deterministic; operators should avoid overlaps). fallbackZone is the
// config-level timezone used when a window omits its own zone.
func (r *Resolver) activeWindow(now time.Time) *TimeWindow {
	for i := range r.cfg.TimeWindows {
		w := &r.cfg.TimeWindows[i]
		if w.contains(now, r.cfg.Timezone) {
			return w
		}
	}

	return nil
}

// contains reports whether t falls inside the window's schedule. fallbackZone is
// used when the window declares no zone of its own (RESEARCH §2.3). The window
// evaluates in its own zone (loaded via the bundled tzdata), regardless of the
// host's local time.
func (w *TimeWindow) contains(t time.Time, fallbackZone string) bool {
	zone := w.Zone
	if zone == "" {
		zone = fallbackZone
	}

	loc, err := time.LoadLocation(zone)
	if err != nil || loc == nil {
		loc = time.UTC // safe fallback; the bundled tzdata makes LoadLocation succeed for valid IANA names
	}

	lt := t.In(loc)
	if !dayMatches(w.Schedule.Days, lt.Weekday()) {
		return false
	}

	from, err1 := parseHHMM(w.Schedule.From, lt)
	if err1 != nil {
		return false
	}

	to, err2 := parseHHMM(w.Schedule.To, lt)
	if err2 != nil {
		return false
	}

	if !from.After(to) {
		// same-day window (from <= to): [from, to) — to-exclusive (pitfall 3)
		return !lt.Before(from) && lt.Before(to)
	}
	// overnight wrap (from > to, e.g. 22:00→06:00): active if lt >= from OR lt < to
	return !lt.Before(from) || lt.Before(to)
}

// dayMatches reports whether the weekday is in the days list. Empty days means
// every day (RESEARCH §2.3).
func dayMatches(days []string, wd time.Weekday) bool {
	if len(days) == 0 {
		return true
	}

	abbr := weekdayAbbr(wd)

	return slices.Contains(days, abbr)
}

func weekdayAbbr(wd time.Weekday) string {
	switch wd {
	case time.Sunday:
		return "Sun"
	case time.Monday:
		return "Mon"
	case time.Tuesday:
		return "Tue"
	case time.Wednesday:
		return "Wed"
	case time.Thursday:
		return "Thu"
	case time.Friday:
		return "Fri"
	case time.Saturday:
		return "Sat"
	}

	return ""
}

// parseHHMM parses an "HH:MM" string as a time on the same calendar day (and in
// the same location) as day. Used to build today's from/to instants for window
// matching.
func parseHHMM(s string, day time.Time) (time.Time, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return time.Time{}, fmt.Errorf("parse HH:MM %q: want exactly one ':'", s) //nolint:err113 // dynamic error message
	}

	h, err := strconv.Atoi(parts[0])
	if err != nil {
		return time.Time{}, fmt.Errorf("parse HH:MM %q: hour: %w", s, err)
	}

	m, err := strconv.Atoi(parts[1])
	if err != nil {
		return time.Time{}, fmt.Errorf("parse HH:MM %q: minute: %w", s, err)
	}

	if h < 0 || h > 23 || m < 0 || m > 59 {
		return time.Time{}, fmt.Errorf("parse HH:MM %q: out of range", s) //nolint:err113 // dynamic error message
	}

	return time.Date(day.Year(), day.Month(), day.Day(), h, m, 0, 0, day.Location()), nil
}
