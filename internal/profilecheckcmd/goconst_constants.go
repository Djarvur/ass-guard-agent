package profilecheckcmd

// Repeated string literals extracted to constants (goconst).
// D-10 carve split: this package carries the subset of the former
// cmd/ass-guard/goconst_constants.go constants its relocated test suite
// references; verbatim duplication across packages is allowed during the carve.
const (
	blockText    = "text"
	keySessionID = "sessionId"
	keyType      = "type"
	profileZcode = "zcode"
)
