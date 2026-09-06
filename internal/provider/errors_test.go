package provider //nolint:testpackage // internal package test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

var errConnectionRefused = errors.New("connection refused")
var errResetByPeer = errors.New("reset by peer")
var errUpstreamSdkFailure = errors.New("upstream sdk failure")
var errUpstreamBearer = errors.New("upstream returned: Authorization: Bearer sk-leaked-token-123456")

// TestClassifyTransientStatuses asserts every retryable HTTP status classifies
// as KindTransient (RESEARCH §3.2).
func TestClassifyTransientStatuses(t *testing.T) {
	t.Parallel()

	for _, status := range []int{408, 425, 429, 500, 502, 503, 504} {
		perr := ClassifyHTTP(providerAnthropic, modelGLM52, status, nil)
		require.Equal(t, KindTransient, perr.Kind, "status %d", status)
		require.Equal(t, providerAnthropic, perr.Provider)
		require.Equal(t, modelGLM52, perr.Model)
		require.Equal(t, status, perr.StatusCode)
	}
}

// TestClassifyStructuralStatuses asserts every non-retryable HTTP status
// classifies as KindStructural (RESEARCH §3.2).
func TestClassifyStructuralStatuses(t *testing.T) {
	t.Parallel()

	for _, status := range []int{400, 401, 403, 404, 405, 411, 413, 422} {
		perr := ClassifyHTTP("openai", "minimax-m3", status, nil)
		require.Equal(t, KindStructural, perr.Kind, "status %d", status)
	}
}

// TestClassifyNetErrorsTransient asserts transport/context errors classify as
// Transient regardless of status (status is typically 0).
func TestClassifyNetErrorsTransient(t *testing.T) {
	t.Parallel()

	cases := []error{
		context.DeadlineExceeded,
		context.Canceled,
		&net.OpError{Op: "dial", Net: "tcp", Err: errConnectionRefused},
		&net.OpError{Op: "read", Net: "tcp", Err: errResetByPeer},
	}
	for _, err := range cases {
		perr := ClassifyHTTP("openai", "minimax-m3", 0, err)
		require.Equal(t, KindTransient, perr.Kind, "err=%v", err)
	}
}

// TestClassifyExhaustedInvariant is the D-04 / pitfall-7 load-bearing invariant:
// ClassifyHTTP NEVER returns KindExhausted for any HTTP status — only the cost
// tracker constructs Exhausted. A spurious Exhausted from the adapter would
// trigger a false hard-stop.
func TestClassifyExhaustedInvariant(t *testing.T) {
	t.Parallel()

	for status := 100; status <= 599; status++ {
		perr := ClassifyHTTP(providerAnthropic, modelGLM52, status, nil)
		require.NotEqual(t, KindExhausted, perr.Kind, "status %d must never classify as Exhausted", status)
	}
	// Status 0 with no recognized error must also not be Exhausted.
	require.NotEqual(t, KindExhausted, ClassifyHTTP("p", "m", 0, nil).Kind)
}

// TestProviderErrorMessage asserts the Error() string is investigate-and-fix-
// ready (C5): it names the provider, model, kind, status, and reason.
func TestProviderErrorMessage(t *testing.T) {
	t.Parallel()

	perr := &ProviderError{
		Kind: KindTransient, Provider: providerAnthropic, Model: modelGLM52,
		StatusCode: 429, Reason: rateLimited,
	}

	msg := perr.Error()
	for _, want := range []string{providerAnthropic, modelGLM52, "Transient", "429", rateLimited} {
		require.Contains(t, msg, want, "Error() %q must contain %q", msg, want)
	}
}

// TestProviderErrorUnwrap asserts errors.Is/As traverse the Cause.
func TestProviderErrorUnwrap(t *testing.T) {
	t.Parallel()

	wrapped := errUpstreamSdkFailure
	perr := &ProviderError{Kind: KindTransient, Cause: wrapped}
	require.ErrorIs(t, perr, wrapped, "errors.Is must traverse Cause")

	var target *ProviderError

	require.ErrorAs(t, perr, &target, "errors.As must match *ProviderError")
}

// TestProviderErrorRedactsCause asserts a Cause wrapping a secret-bearing
// string is routed through redact.ScrubError so the credential never appears in
// Error() output (C2).
func TestProviderErrorRedactsCause(t *testing.T) {
	t.Parallel()

	perr := &ProviderError{
		Kind: KindTransient, Provider: providerAnthropic, Model: modelGLM52,
		StatusCode: 500,
		Cause:      errUpstreamBearer,
	}
	msg := perr.Error()
	require.NotContains(t, msg, "sk-leaked-token-123456", "Error() must scrub the leaked key (C2)")
	require.NotContains(t, msg, "Bearer sk-", "Error() must scrub the Bearer prefix")
	require.Contains(t, msg, "[REDACTED]", "Error() must replace the secret with the placeholder")
}

// TestClassifyReason checks the reason text for a few representative statuses.
func TestClassifyReason(t *testing.T) {
	t.Parallel()
	require.Contains(t, ClassifyHTTP("p", "m", 429, nil).Reason, "rate")
	require.Contains(t, ClassifyHTTP("p", "m", 401, nil).Reason, "unauth")
	require.Contains(t, ClassifyHTTP("p", "m", 0, context.DeadlineExceeded).Reason, "deadline")
}

// TestIsOverflow pins the overflow message-class predicate (19-02, PAR-01's
// retry-once substrate): TRUE only for a *ProviderError whose message contains
// the case-insensitive "prompt is too long" class (research assumption A1 —
// community-documented form "prompt is too long: N tokens > M maximum" riding
// 400 invalid_request_error), FALSE for everything else. Constructed through
// ClassifyHTTP exactly the way the streaming status-check site builds the real
// error — the matcher must never fire on classification alone (a generic 400 is
// NOT overflow) nor on any non-matching Kind.
func TestIsOverflow(t *testing.T) {
	t.Parallel()

	overflowErr := ClassifyHTTP(providerAnthropic, modelGLM52, http.StatusBadRequest,
		errors.New("invalid_request_error: prompt is too long: 200936 tokens > 199999 maximum"))

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "400 prompt-is-too-long (exact A1 wording)",
			err:  overflowErr,
			want: true,
		},
		{
			name: "case-varied message",
			err: ClassifyHTTP(providerAnthropic, modelGLM52, http.StatusBadRequest,
				errors.New("Prompt Is Too Long: 12 tokens > 10 maximum")),
			want: true,
		},
		{
			name: "wrapped by the turn loop",
			err:  fmt.Errorf("session turn stream: %w", overflowErr),
			want: true,
		},
		{name: "nil", err: nil, want: false},
		{name: "plain error (not a ProviderError)", err: errConnectionRefused, want: false},
		{
			name: "network-classified ProviderError",
			err: ClassifyHTTP(providerAnthropic, modelGLM52, 0,
				&net.OpError{Op: "read", Net: "tcp", Err: errResetByPeer}),
			want: false,
		},
		{
			name: "generic 400 (different message)",
			err:  ClassifyHTTP(providerAnthropic, modelGLM52, http.StatusBadRequest, errors.New("invalid request: bad tool id")),
			want: false,
		},
		{
			name: "429 rate limited",
			err:  ClassifyHTTP(providerAnthropic, modelGLM52, http.StatusTooManyRequests, nil),
			want: false,
		},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, IsOverflow(tc.err), "%s: IsOverflow(%v)", tc.name, tc.err)
	}
}

// TestRetryOnlyTransient_ClassificationTable (14-06 pin): the FULL
// retry-only-transient classification table at the errors.go seam, statuses
// literal — Transient kinds are the only ones any retry path may act on;
// Structural kinds are never retried (the scheduler's D-04 gate consumes this
// table; docs/tool-contract-inventory.md §c quotes it).
func TestRetryOnlyTransient_ClassificationTable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		status int
		err    error
		want   ErrorKind
	}{
		{name: "408 request timeout", status: 408, want: KindTransient},
		{name: "425 too early", status: 425, want: KindTransient},
		{name: "429 rate limited", status: 429, want: KindTransient},
		{name: "500 server error", status: 500, want: KindTransient},
		{name: "502 bad gateway", status: 502, want: KindTransient},
		{name: "503 service unavailable", status: 503, want: KindTransient},
		{name: "504 gateway timeout", status: 504, want: KindTransient},
		{name: "net timeout", status: 0, err: context.DeadlineExceeded, want: KindTransient},
		{name: "400 bad request", status: 400, want: KindStructural},
		{name: "401 unauthenticated", status: 401, want: KindStructural},
		{name: "403 forbidden", status: 403, want: KindStructural},
		{name: "422 unprocessable", status: 422, want: KindStructural},
	}
	for _, tc := range cases {
		perr := ClassifyHTTP(providerAnthropic, modelGLM52, tc.status, tc.err)
		require.Equal(t, tc.want, perr.Kind, "%s (status %d): retry-only-transient table", tc.name, tc.status)
	}
}
