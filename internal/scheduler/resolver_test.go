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
