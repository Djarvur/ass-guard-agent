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

const mnd5 = 5
const mnd20 = 20
const mnd050 = 0.50
const mnd60 = 60
const mnd24 = 24

// defaultBreaker holds the documented D-07 circuit-breaker defaults applied when
// a loaded config has a zero-valued circuit_breaker block (RESEARCH §1.3). These
// are the tunable parameters with documented defaults; operators override via
// config.yaml.
var defaultBreaker = CircuitBreakerConfig{ //nolint:gochecknoglobals // process-wide default singleton
	ConsecutiveFailures: mnd5,
	ErrorRateWindow:     mnd20,
	ErrorRateThreshold:  mnd050,
	Cooldown:            mnd60 * time.Second,
	HalfOpenProbes:      1,
}

// defaultCost holds the documented D-08 cost-ceiling defaults (RESEARCH §1.3).
var defaultCost = CostCeilingConfig{ //nolint:gochecknoglobals // process-wide default singleton
	Window: mnd24 * time.Hour,
}
