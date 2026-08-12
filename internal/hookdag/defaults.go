package hookdag

// DefaultHooks returns the embedded zero-config hook floor (HOOK-02). Operators
// override via the layered loader (Load with one or more paths); this is the
// no-args convenience for callers that want only the seed.
func DefaultHooks() ([]Hook, error) {
	return Load()
}
