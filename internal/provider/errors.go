package provider

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/Djarvur/ass-guard-agent/internal/redact"
)

// ErrorKind classifies a provider failure so the scheduler can pattern-match on
// it instead of re-inspecting HTTP status (D-04: the adapter classifies, the
// scheduler pattern-matches). The three kinds cover every failure mode the
// scheduler treats differently.
type ErrorKind string

const (
	// KindTransient marks a retryable failure (429/5xx/network/timeout). The
	// scheduler walks the fallback chain on Transient (D-05).
	KindTransient ErrorKind = "Transient"
	// KindStructural marks a non-retryable failure (400/401/403/404/...). The
	// scheduler reports it to the turn loop immediately (D-04 — N5's silent
	// retry-storm failure mode engineered out by the typed Kind).
	KindStructural ErrorKind = "Structural"
	// The cost-ceiling hard-stop kind (D-08). NEVER produced by ClassifyHTTP —
	// only the scheduler's cost tracker constructs it (the adapter cannot see
	// the budget). ClassifyHTTP references only the two kinds above; this
	// invariant is unit-tested (pitfall 7, TestClassifyExhaustedInvariant).
	KindExhausted ErrorKind = "Exhausted"
)

// ProviderError is the typed error every Provider implementation returns from
// Send/Stream. The adapter classifies the HTTP failure via ClassifyHTTP; the
// scheduler pattern-matches on Kind. The Cause is routed through
// redact.ScrubError in Error() so a wrapped SDK error containing a request body
// never leaks a credential (C2).
type ProviderError struct {
	Kind       ErrorKind
	Provider   string
	Model      string
	StatusCode int
	Reason     string
	Cause      error
}

// Error formats the failure investigate-and-fix-ready (C5): provider, model,
// kind, HTTP status, reason, and a redacted cause. provider/model slugs are
// operator-visible (not secret); the Cause may wrap an SDK error carrying a
// request body, so it is scrubbed (C2).
func (e *ProviderError) Error() string {
	cause := ""
	if e.Cause != nil {
		cause = redact.ScrubError(e.Cause)
	}

	if e.StatusCode != 0 {
		return fmt.Sprintf("provider %s model %s: %s (HTTP %d): %s: %s",
			e.Provider, e.Model, e.Kind, e.StatusCode, e.Reason, cause)
	}

	return fmt.Sprintf("provider %s model %s: %s: %s: %s",
		e.Provider, e.Model, e.Kind, e.Reason, cause)
}

// Unwrap exposes the wrapped underlying error for errors.Is / errors.As.
func (e *ProviderError) Unwrap() error { return e.Cause }

// ClassifyHTTP maps an HTTP status + wrapped error to a *ProviderError (D-04,
// RESEARCH §3.2). The adapter calls this when it gets a non-2xx response or a
// transport error; the scheduler pattern-matches on the resulting Kind.
//
// Transient: 408/425/429/5xx, plus net errors (context deadline/cancel,
// *net.OpError, *url.Error) regardless of status.
// Structural: 400/401/403/404/405/411/413/422.
// Any other non-2xx status defaults to Transient (safe-side: a weird 5xx-ish
// thing is more likely transient than structural).
//
// ClassifyHTTP NEVER returns the cost-tracker-only kind — that is the cost
// ceiling's responsibility (D-04 invariant, pitfall 7, asserted by
// TestClassifyExhaustedInvariant).
func ClassifyHTTP(providerSlug, model string, status int, err error) *ProviderError {
	perr := &ProviderError{
		Kind:       KindTransient,
		Provider:   providerSlug,
		Model:      model,
		StatusCode: status,
		Cause:      err,
		Reason:     reasonFor(status, err),
	}

	// Net / context errors are transient regardless of status (often status==0).
	if isNetOrContextError(err) {
		perr.Kind = KindTransient

		return perr
	}

	switch {
	case isTransientStatus(status):
		perr.Kind = KindTransient
	case isStructuralStatus(status):
		perr.Kind = KindStructural
	default:
		// Unknown non-2xx (e.g. 418, 499, 5xx not in the explicit list, 0 with
		// no recognized net error): default Transient — safe-side.
		perr.Kind = KindTransient
	}

	return perr
}

// isNetOrContextError reports whether err is a transport/context failure that is
// transient by definition (RESEARCH §3.2 "net errors" row).
func isNetOrContextError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}

	var urlErr *url.Error

	return errors.As(err, &urlErr)
}

var transientStatuses = map[int]struct{}{
	408: {}, 425: {}, 429: {},
	500: {}, 502: {}, 503: {}, 504: {},
}

func isTransientStatus(status int) bool {
	_, ok := transientStatuses[status]

	return ok
}

var structuralStatuses = map[int]struct{}{
	400: {}, 401: {}, 403: {}, 404: {}, 405: {},
	411: {}, 413: {}, 422: {},
}

func isStructuralStatus(status int) bool {
	_, ok := structuralStatuses[status]

	return ok
}

// reasonFor returns a short investigate-and-fix-ready reason string for the
// status/error, falling back to http.StatusText.
func reasonFor(status int, err error) string {
	if status != 0 {
		if r, ok := statusReasons[status]; ok {
			return r
		}

		if txt := http.StatusText(status); txt != "" {
			return strings.ToLower(txt)
		}

		return fmt.Sprintf("http %d", status)
	}

	if err != nil {
		// Transport / context error with no HTTP status.
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return "context deadline exceeded"
		case errors.Is(err, context.Canceled):
			return "context canceled"
		}

		return "transport error"
	}

	return "unknown"
}

var statusReasons = map[int]string{
	400: "bad request",
	401: "unauthenticated",
	403: "forbidden",
	404: "not found",
	405: "method not allowed",
	408: "request timeout",
	411: "length required",
	413: "payload too large",
	422: "unprocessable request",
	425: "too early",
	429: rateLimited,
	500: "server error",
	502: "bad gateway",
	503: "service unavailable",
	504: "gateway timeout",
}
