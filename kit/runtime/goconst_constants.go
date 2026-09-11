package runtime

// Repeated string literals extracted to constants (goconst) — the set the
// PRODUCTION files of this package use (25-09 trim: the 25-08 first cut
// duplicated other packages' test tables here; test-only vocabulary moved
// to goconst_constants_test.go, dead entries deleted).
const (
	blockText   = "text"
	stopEndTurn = "end_turn"
	tierLight   = "light"
	tierHeavy   = "heavy"
	blockImage  = "image"
)
