package coreexec

import "strings"

// StripANSI removes terminal escape sequences from PTY-captured output
// (22-04, PAR-09). Scope (the pinned discretion, RESEARCH A3): the CSI/SGR
// class — ESC [ <params> <final in @-~> — which covers colors (m), cursor
// moves (H, J, K), and erase ops. OSC sequences (ESC ] ... BEL/ST) are
// stripped whole (title-set noise from interactive shells); a LONE ESC not
// followed by [ or ] is dropped as a byte. Hand-rolled by the
// deliberately-NOT-a-dependency decision; never applied to foreground Bash
// output (persistent-shell capture only).
func StripANSI(s string) string {
	if !strings.ContainsRune(s, '\x1b') {
		return s // fast path — plain text is byte-identical
	}

	var b strings.Builder

	b.Grow(len(s))

	for i := 0; i < len(s); {
		if s[i] != '\x1b' {
			b.WriteByte(s[i])
			i++

			continue
		}

		// Escape sequence: ESC [ ... final(@-~) | ESC ] ... (BEL|ST) | lone ESC.
		if i+1 >= len(s) {
			i++ // trailing ESC at end-of-string

			continue
		}

		if s[i+1] == '[' {
			j := i + 2

			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++ // parameter/intermediate bytes
			}

			if j < len(s) {
				j++ // the final byte
			}

			i = j

			continue
		}

		if s[i+1] == ']' {
			j := i + 2

			for j < len(s) {
				if s[j] == '\a' { // BEL terminator
					j++

					break
				}

				if s[j] == '\\' && j > i+2 { // ST terminator
					j++

					break
				}

				if s[j] == '\x1b' { // unterminated OSC ends at the next escape —
					// unless that escape IS the ST terminator (ESC \).
					if j+1 < len(s) && s[j+1] == '\\' {
						j += 2
					}

					break
				}

				j++
			}

			i = j

			continue
		}

		i++ // lone ESC (not CSI/OSC) — drop the ESC byte only, never text
	}

	return b.String()
}
