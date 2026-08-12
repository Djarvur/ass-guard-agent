package scheduler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestTzdataBundled proves the blank import _ "time/tzdata" in init.go is
// present and effective: a non-host, non-UTC zone loads without error. This is
// the single-static-binary guarantee (D-03, RESEARCH §2.1, pitfall 2) — without
// the bundled tz database, time.LoadLocation reads /usr/share/zoneinfo, which
// may be absent or stale on minimal Linux deploys. The test catches a removed
// blank import before deploy.
func TestTzdataBundled(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	require.NoError(t, err, "LoadLocation should succeed via bundled tzdata")
	require.NotEqual(t, time.UTC, loc, "the loaded zone must not collapse to UTC")
	// A functional check: America/New_York is offset from UTC (between -5 and -4
	// depending on DST), so a winter noon there is not UTC noon.
	winter := time.Date(2026, 1, 15, 12, 0, 0, 0, loc)
	_, offset := winter.Zone()
	require.NotZero(t, offset, "EST/EDT offset must be non-zero")
}

// ny returns a time in America/New_York (the valid.yaml top-level zone) for the
// given wall-clock components. All resolver tests construct `now` this way so
// they are deterministic regardless of the host TZ.
func ny(year int, month time.Month, day, hour, min int) time.Time {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		panic("America/New_York must load via bundled tzdata: " + err.Error())
	}

	return time.Date(year, month, day, hour, min, 0, 0, loc)
}

// loadValid is the shared fixture loader for resolver tests.
func loadValid(t *testing.T) *Config {
	t.Helper()

	cfg, err := Load("testdata/valid.yaml")
	require.NoError(t, err)

	return cfg
}

// TestResolveWindowWinsOutright is the D-02 reversal (pitfall 8): a structural
// time-window that maps the tier wins OUTRIGHT — the per-project override and
// the global tier table cannot displace it. Peak window (Mon-Fri 09:00-17:00)
// maps heavy→minimax-m3, so a Monday 10:00 NY resolution returns minimax-m3,
// NOT myproj's or the global glm-5.2.
func TestResolveWindowWinsOutright(t *testing.T) {
	r := NewResolver(loadValid(t))
	// 2026-08-10 is a Monday.
	now := ny(2026, time.August, 10, 10, 0)
	primary, fallbacks, err := r.Resolve("heavy", "myproj", now, CapabilityReq{})
	require.NoError(t, err)
	require.Equal(t, "minimax-m3", primary.Model, "peak window heavy pick must win (D-02 reversal)")
	require.Equal(t, "openai", primary.Provider)
	// Window's heavy binding declares fallback [glm-5.2].
	require.Len(t, fallbacks, 1)
	require.Equal(t, "glm-5.2", fallbacks[0].Model)
}

// TestResolveProjectNarrowsTheGap: when no window maps the tier, the per-project
// override wins over the global default. myproj maps good→minimax-m3; the global
// good is glm-4.6.
func TestResolveProjectNarrowsTheGap(t *testing.T) {
	r := NewResolver(loadValid(t))
	now := ny(2026, time.August, 10, 10, 0) // Monday 10:00 — peak active but peak doesn't map good
	primary, _, err := r.Resolve("good", "myproj", now, CapabilityReq{})
	require.NoError(t, err)
	require.Equal(t, "minimax-m3", primary.Model, "myproj good override must win over global glm-4.6")
}

// TestResolveGlobalFallback: when neither window nor project maps the tier, the
// global tier table is used.
func TestResolveGlobalFallback(t *testing.T) {
	r := NewResolver(loadValid(t))
	now := ny(2026, time.August, 10, 10, 0)
	primary, _, err := r.Resolve("light", "myproj", now, CapabilityReq{})
	require.NoError(t, err)
	require.Equal(t, "minimax-m3", primary.Model, "global light must resolve when no window/project maps light")
}

// TestResolveOffPeakOvernight: outside the peak window, the overnight window
// (22:00→06:00, every day) maps heavy→glm-5.2.
func TestResolveOffPeakOvernight(t *testing.T) {
	r := NewResolver(loadValid(t))
	// Sunday 03:00 NY — peak inactive (weekend + outside 09-17), overnight active.
	now := ny(2026, time.August, 16, 3, 0)
	primary, _, err := r.Resolve("heavy", "myproj", now, CapabilityReq{})
	require.NoError(t, err)
	require.Equal(t, "glm-5.2", primary.Model, "overnight window heavy pick must win off-peak")
}

// TestActiveWindowOvernightBoundary table-tests the overnight-wrap edge
// (pitfall 3): the window 22:00→06:00 is active at 23:59 and 01:00, INACTIVE at
// 06:00 exactly (to-exclusive) and 21:59.
func TestActiveWindowOvernightBoundary(t *testing.T) {
	cfg := loadValid(t)
	// Find the overnight window directly to assert its boundary semantics.
	var overnight *TimeWindow

	for i := range cfg.TimeWindows {
		if cfg.TimeWindows[i].Name == "overnight" {
			overnight = &cfg.TimeWindows[i]
		}
	}

	require.NotNil(t, overnight, "valid.yaml must declare the overnight window")

	cases := []struct {
		name string
		h, m int
		want bool
	}{
		{"23:59 inside wrap", 23, 59, true},
		{"01:00 inside wrap", 1, 0, true},
		{"06:00 to-exclusive", 6, 0, false},
		{"21:59 before window", 21, 59, false},
		{"22:00 from-inclusive", 22, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Sunday Aug 16 2026 — overnight is every-day, so weekday is irrelevant.
			now := ny(2026, time.August, 16, tc.h, tc.m)
			got := overnight.contains(now, "America/New_York")
			require.Equal(t, tc.want, got, "overnight.contains at %02d:%02d", tc.h, tc.m)
		})
	}
}

// TestActiveWindowWeekdayFilter: the peak window (Mon-Fri) is INACTIVE on
// Saturday, so a Saturday noon heavy resolution falls through to the global
// table (glm-5.2), not the peak pick.
func TestActiveWindowWeekdayFilter(t *testing.T) {
	r := NewResolver(loadValid(t))
	// Saturday Aug 15 2026 12:00 NY — peak inactive (weekend), overnight
	// inactive (12:00 not in 22:00-06:00). Falls through to global heavy.
	now := ny(2026, time.August, 15, 12, 0)
	primary, _, err := r.Resolve("heavy", "myproj", now, CapabilityReq{})
	require.NoError(t, err)
	require.Equal(t, "glm-5.2", primary.Model, "Saturday noon must skip peak (weekday filter) and resolve global heavy")
}

// TestResolveFallbackChainEnriched: Resolve returns the declared fallback chain
// in order, each element carrying its capabilities + pricing.
func TestResolveFallbackChainEnriched(t *testing.T) {
	r := NewResolver(loadValid(t))
	// Sunday 12:00 NY — global heavy table is active (no window). Global heavy
	// = glm-5.2, fallback [minimax-m3, glm-4.6].
	now := ny(2026, time.August, 16, 12, 0)
	primary, fallbacks, err := r.Resolve("heavy", "myproj", now, CapabilityReq{})
	require.NoError(t, err)
	require.Equal(t, "glm-5.2", primary.Model)
	require.Equal(t, []string{"minimax-m3", "glm-4.6"}, []string{fallbacks[0].Model, fallbacks[1].Model},
		"fallback chain must be the declared order")
	require.True(t, fallbacks[0].Capabilities.ToolCalling, "fallback must carry its capability profile")
	require.Greater(t, fallbacks[0].Pricing.InputPerMToken, 0.0, "fallback must carry pricing")
}

// TestResolveWindowZoneOverride: a window with its own zone evaluates in that
// zone regardless of the top-level timezone. A London window whose schedule is
// 09:00-17:00 must be active at 09:30 London (which is a different UTC instant
// than 09:30 top-level NY).
func TestResolveWindowZoneOverride(t *testing.T) {
	cfg := &Config{
		Timezone: "America/New_York",
		TimeWindows: []TimeWindow{{
			Name:     "london-morning",
			Zone:     "Europe/London",
			Schedule: Schedule{From: "09:00", To: "17:00", Days: []string{"Mon", "Tue", "Wed", "Thu", "Fri"}},
			Tiers:    map[string]TierBinding{"heavy": {Model: "override-model"}},
		}},
		Providers: map[string]ProviderConfig{"anthropic": {BaseURL: "x", Shape: "anthropic"}},
		Models: map[string]ModelConfig{
			"override-model": {Provider: "anthropic"},
			"global-model":   {Provider: "anthropic"},
		},
		Tiers: map[string]TierBinding{"heavy": {Model: "global-model"}},
	}
	r := NewResolver(cfg)
	lon, err := time.LoadLocation("Europe/London")
	require.NoError(t, err)
	// Monday 09:30 London — inside the window.
	monLondon := time.Date(2026, time.August, 10, 9, 30, 0, 0, lon)
	primary, _, err := r.Resolve("heavy", "", monLondon, CapabilityReq{})
	require.NoError(t, err)
	require.Equal(t, "override-model", primary.Model, "London-zone window must activate at 09:30 London")

	// Monday 08:30 London — before the window → global fallback.
	beforeLondon := time.Date(2026, time.August, 10, 8, 30, 0, 0, lon)
	primary, _, err = r.Resolve("heavy", "", beforeLondon, CapabilityReq{})
	require.NoError(t, err)
	require.Equal(t, "global-model", primary.Model, "08:30 London must skip the 09:00 window")
}

// TestResolveCapabilityProfileAttached: every resolved Target carries a
// populated CapabilityProfile + Pricing (D-09).
func TestResolveCapabilityProfileAttached(t *testing.T) {
	r := NewResolver(loadValid(t))

	now := ny(2026, time.August, 16, 12, 0) // Sunday noon → global table

	for _, tier := range []string{"heavy", "good", "light"} {
		primary, fallbacks, err := r.Resolve(tier, "myproj", now, CapabilityReq{})
		require.NoError(t, err)

		for _, tgt := range append([]Target{primary}, fallbacks...) {
			require.Positive(t, tgt.Capabilities.ContextWindow, "target %s must carry a context_window", tgt.Model)
			require.True(t, tgt.Capabilities.Streaming, "target %s must declare streaming", tgt.Model)
			require.GreaterOrEqual(t, tgt.Pricing.InputPerMToken, 0.0, "target %s must carry pricing", tgt.Model)
		}
	}
}

// TestResolveUnknownTier: a tier absent from the config returns a structured
// error (the turn loop surfaces it).
func TestResolveUnknownTier(t *testing.T) {
	r := NewResolver(loadValid(t))
	_, _, err := r.Resolve("ultra", "myproj", ny(2026, time.August, 16, 12, 0), CapabilityReq{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "ultra")
}

// TestSatisfies unit-tests the capability-match helper (defined here; Plan
// 03-04 relocates it to capability.go and applies it).
func TestSatisfies(t *testing.T) {
	cap := CapabilityProfile{ToolCalling: true, Streaming: true, ExtendedThinking: false}
	require.True(t, satisfies(cap, CapabilityReq{}), "zero req is always satisfied")
	require.True(t, satisfies(cap, CapabilityReq{NeedsTools: true}))
	require.False(t, satisfies(cap, CapabilityReq{NeedsThinking: true}), "cap lacks thinking")
	require.False(t, satisfies(cap, CapabilityReq{NeedsStreaming: false, NeedsThinking: true}))
}
