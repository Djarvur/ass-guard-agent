package coreexec

import (
	"strings"
	"testing"
)

// The ANSI-strip table battery (22-04 Task 1, PAR-09): the pinned CSI/SGR
// scope — escapes never reach the tool result; plain text is
// byte-identical; a lone ESC does not swallow text.

func TestAnsiStrip_Table(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain text identical", "hello world", "hello world"},
		{"SGR color", "\x1b[31mred\x1b[0m", "red"},
		{"SGR bold+color", "\x1b[1;32mgreen bold\x1b[0m", "green bold"},
		{"CSI cursor move", "ab\x1b[2;3Hcd", "abcd"},
		{"CSI erase line", "x\x1b[2Ky", "xy"},
		{"multiple sequences", "\x1b[1mstart\x1b[0m-\x1b[31mmid\x1b[0m-end", "start-mid-end"},
		{"OSC title BEL", "\x1b]0;window title\atext", "text"},
		{"OSC title ST", "\x1b]2;title\x1b\\kept", "kept"},
		{"lone ESC keeps text", "a\x1bb", "ab"},
		{"trailing lone ESC", "abc\x1b", "abc"},
		{"empty", "", ""},
		{"only escapes", "\x1b[31m\x1b[0m\x1b[2J", ""},
		{"multibyte text untouched", "héllo 😀 wörld", "héllo 😀 wörld"},
		{"escape inside multibyte-safe stream", "é\x1b[31mö", "éö"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := StripANSI(tc.in); got != tc.want {
				t.Errorf("StripANSI(%q) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestAnsiStrip_NoEscapeBytesSurvive: output laden with the full CSI/SGR
// family leaves zero ESC bytes in the result.
func TestAnsiStrip_NoEscapeBytesSurvive(t *testing.T) {
	t.Parallel()

	in := "\x1b[31mred\x1b[0m \x1b[1;34;42mbold\x1b[0m \x1b[2;3H\x1b[2J\x1b[0Ktail \x1b]0;t\adone"

	got := StripANSI(in)

	if strings.ContainsRune(got, '\x1b') {
		t.Errorf("stripped output still carries ESC bytes: %q", got)
	}

	if got != "red bold tail done" && !strings.HasPrefix(got, "red bold") {
		t.Errorf("stripped output = %q; want the plain text", got)
	}
}
