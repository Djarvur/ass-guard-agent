package perm //nolint:testpackage // hasSubstitution is the unexported scanner subject — the store_test.go in-package precedent

import "testing"

// TestHasSubstitutionQuotes pins the WR-02 quote-nesting semantics of the
// hasSubstitution scanner directly, one row per shell-quoting fact in both
// directions: substitution is LIVE unquoted and inside double quotes (single
// quotes there are ordinary bytes — they do NOT open a literal span), while a
// single-quoted span is fully literal and a backslash escapes the next byte.
// The a710b75 rewrite dropped the double-quote state, so the first four live
// rows went dark — `git "log 'x $(cmd) y'"` under-detected and an allow rule
// could match a substitution-bearing command.
func TestHasSubstitutionQuotes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cmd  string
		want bool
	}{
		{
			// THE live-verified WR-02 case: the inner single quotes sit inside
			// double quotes, so they are ordinary bytes and must not open a
			// literal span.
			name: "nested-single-quotes-inside-double-quotes-live",
			cmd:  `git "log 'x $(cmd) y'"`,
			want: true,
		},
		{
			name: "apostrophe-inside-double-quotes-live",
			cmd:  `echo "it's fine $(reboot)"`,
			want: true,
		},
		{
			name: "single-quotes-inside-double-quotes-are-literal-bytes",
			cmd:  `echo "'$(reboot)'"`,
			want: true,
		},
		{
			name: "backtick-live-inside-nested-quotes",
			cmd:  "git \"log 'x `id` y'\"",
			want: true,
		},
		{
			// Already detected at master — pinned so the fix cannot regress it.
			name: "substitution-containing-single-quotes-live",
			cmd:  `echo "$(echo 'inner')"`,
			want: true,
		},
		{
			name: "unterminated-double-quote-fails-safe-toward-detection",
			cmd:  `echo "$(x`,
			want: true,
		},
		{
			name: "single-quoted-span-fully-literal",
			cmd:  `echo '$(safe literal)'`,
			want: false,
		},
		{
			// The symmetric direction: double quotes inside single quotes are
			// literal bytes.
			name: "double-quotes-inside-single-quotes-are-literal-bytes",
			cmd:  `echo '"$(x)"'`,
			want: false,
		},
		{
			name: "unterminated-single-quote-reports-no-substitution",
			cmd:  `echo '$(x`,
			want: false,
		},
		{
			name: "backslash-escapes-next-byte-inside-double-quotes",
			cmd:  `echo "\$(not live)"`,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := hasSubstitution(tt.cmd); got != tt.want {
				t.Errorf("hasSubstitution(%q) = %v; want %v", tt.cmd, got, tt.want)
			}
		})
	}
}
