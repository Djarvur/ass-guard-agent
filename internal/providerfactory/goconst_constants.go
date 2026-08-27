package providerfactory

// Repeated string literals extracted to constants (goconst).
// D-10 carve split: this package carries the subset of the former
// cmd/ass-guard/goconst_constants.go constants its moved code references;
// verbatim duplication across packages is allowed during the carve.
const tierHeavy = "heavy"

// File-mode discipline for the D-07 config layer writer (T-16-11): the target
// layer file is hard 0600; directories the writer creates are 0750 max.
const (
	filePermOwnerWrite = 0o600
	dirPermOwnerGroup  = 0o750
)
