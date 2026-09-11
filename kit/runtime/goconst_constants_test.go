package runtime

// permOptionAllowOnceKit mirrors the wire's allow_once permission-option
// value (internal/acp's PermOptionAllowOnce) for the kit-side batteries —
// the wire-parity pin lives app-side (internal/acp's own suite); the kit
// battery only needs the VALUE its AskOutcome carries.
const permOptionAllowOnceKit = "allow_once"

// The test battery's own vocabulary (goconst) — test-only values.
const (
	chunkDone                 = "done"
	chunkToolUse              = "tool_use"
	actionContinue            = "continue"
	implementationCompleteMsg = "## Implementation Complete — ready for review"
)
