package ecosys

import "strings"

// mentionTrailingPunct is the trailing-punctuation set never part of a
// mention's path (21-04 Task 1 parse table): a mention closed by sentence
// punctuation keeps a clean path — @docs/readme.md. → path docs/readme.md.
const mentionTrailingPunct = ".,:"

// Mention is one parsed @-mention (21-04, PAR-06/D-10): Token is the raw
// mention as typed (@path, INCLUDING its quote characters, EXCLUDING the
// trailing punctuation trimmed from the path); Path is the unquoted path the
// token carries. IsDir is UNKNOWN at parse time — the parser is deliberately
// blind to the filesystem (parse-only, the expand.go ParseInvocation split
// precedent); the runtime layer stats the path and fills IsDir there.
type Mention struct {
	Token string
	Path  string
	IsDir bool
}

// ParseMentions extracts every @-mention token from text, in order. The
// contract (the Task 1 parse table):
//
//   - whitespace-delimited, token-INITIAL @ only — a mid-word @ (email-like
//     foo@bar) is never a mention;
//   - a quote directly after the @ (@"path with spaces" / @'path with
//     spaces') captures spaces into ONE token; a path tail may follow the
//     closing quote (@"my docs"/file.md);
//   - trailing punctuation (. , :) never joins the path — quotes protect
//     their interior, the unquoted tail is trimmed;
//   - an unterminated quote, a bare @, or @ followed only by punctuation is
//     not a mention;
//   - zero mentions → nil (no error — absence is ordinary prose).
//
// Pure text extraction: no I/O, no resolution, no errors. Resolution,
// Read-rule gating, and provenance belong to the runtime layer consuming
// this (the expand.go split: parse here, decide there).
func ParseMentions(text string) []Mention {
	var out []Mention

	i, n := 0, len(text)

	for i < n {
		if isMentionSpace(text[i]) {
			i++
			continue
		}

		if text[i] == '@' {
			if m, next, ok := scanMention(text, i); ok {
				out = append(out, m)
				i = next

				continue
			}
		}

		// Ordinary token (or a failed mention shape): skip to whitespace.
		for i < n && !isMentionSpace(text[i]) {
			i++
		}
	}

	return out
}

// scanMention attempts to parse one mention starting at text[start] == '@'.
// It returns the mention, the scan resume offset (the token's end, including
// any trailing punctuation skipped), and ok=false when the shape is not a
// mention (bare @, only-punctuation path, unterminated quote).
//
//nolint:nonamedreturns // the (value, offset, ok) triple reads best named
func scanMention(text string, start int) (m Mention, next int, ok bool) {
	n := len(text)
	i := start + 1

	var path strings.Builder

	// A quote directly after the @ opens a space-capturing segment; the
	// closing quote may be followed by an ordinary path tail.
	if i < n && (text[i] == '"' || text[i] == '\'') {
		quote := text[i]
		i++

		segStart := i
		for i < n && text[i] != quote {
			i++
		}

		if i >= n {
			return Mention{}, 0, false // unterminated quote — not a mention
		}

		path.WriteString(text[segStart:i])
		i++ // closing quote
	}

	// Ordinary tail: everything up to whitespace.
	tailStart := i
	for i < n && !isMentionSpace(text[i]) {
		i++
	}

	tail := strings.TrimRight(text[tailStart:i], mentionTrailingPunct)
	path.WriteString(tail)

	if path.Len() == 0 {
		return Mention{}, 0, false
	}

	// The token ends before the trimmed punctuation; the scan itself resumes
	// at the whitespace boundary (next = i).
	tokenEnd := i - (len(text[tailStart:i]) - len(tail))

	return Mention{Token: text[start:tokenEnd], Path: path.String()}, i, true
}

// isMentionSpace reports whether c is a mention-token delimiter (ASCII
// whitespace — the parse table's "whitespace-delimited" set).
func isMentionSpace(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	default:
		return false
	}
}
