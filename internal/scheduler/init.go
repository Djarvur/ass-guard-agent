package scheduler

import (
	"time"

	// Blank-imported tz database (D-03, RESEARCH §2.1). Bundling ~450KB of pure-
	// Go IANA zoneinfo INTO the binary makes time.LoadLocation("America/New_York")
	// work identically on macOS + Linux, amd64 + arm64, regardless of whether the
	// host has /usr/share/zoneinfo. This is the single-static-binary guarantee
	// (PROJECT.md Constraints). TestTzdataBundled catches a removed import before
	// deploy (pitfall 2).
	_ "time/tzdata"
)

// defaultBreaker holds the documented D-07 circuit-breaker defaults applied when
// a loaded config has a zero-valued circuit_breaker block (RESEARCH §1.3). These
// are the tunable parameters with documented defaults; operators override via
// scheduling.yaml.
var defaultBreaker = CircuitBreakerConfig{
	ConsecutiveFailures: 5,
	ErrorRateWindow:     20,
	ErrorRateThreshold:  0.50,
	Cooldown:            60 * time.Second,
	HalfOpenProbes:      1,
}

// defaultCost holds the documented D-08 cost-ceiling defaults (RESEARCH §1.3).
var defaultCost = CostCeilingConfig{
	Window: 24 * time.Hour,
}
