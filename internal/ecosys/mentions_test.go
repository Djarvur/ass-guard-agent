package ecosys_test

import (
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/ecosys"
)

// Repeated fixture literals (goconst).
const (
	mentionPathReadme   = "docs/readme.md"
	mentionTokenReadme  = "@docs/readme.md"
	mentionSpacedPath   = "path with spaces"
	mentionSpacedQuoted = `@"path with spaces"`
)

// mentionCases is the @-mention PARSE contract (21-04 Task 1, D-10) encoded
// as one table — written BEFORE the implementation (the REQ's parse table
// pins the contract deterministic before expansion builds on it). Parse is
// PURE: no I/O, no resolution, no IsDir knowledge (that is the runtime
// layer's job — the expand.go ParseInvocation split precedent).
//
//nolint:funlen // one row per contract edge — the table IS the spec
func mentionCases() []struct {
	name string
	text string
	want []ecosys.Mention
} {
	return []struct {
		name string
		text string
		want []ecosys.Mention
	}{
		{
			name: "single relative file mention",
			text: "review " + mentionTokenReadme + " please",
			want: []ecosys.Mention{{Token: mentionTokenReadme, Path: mentionPathReadme}},
		},
		{
			name: "dir-shaped path parses with IsDir unknown",
			text: "look at @internal/ecosys",
			want: []ecosys.Mention{{Token: "@internal/ecosys", Path: "internal/ecosys"}},
		},
		{
			name: "double-quoted path with spaces is one token",
			text: `open @"` + mentionSpacedPath + `" now`,
			want: []ecosys.Mention{{Token: mentionSpacedQuoted, Path: mentionSpacedPath}},
		},
		{
			name: "double-quoted segment plus path tail is one token",
			text: `open @"my docs"/file.md now`,
			want: []ecosys.Mention{{Token: `@"my docs"/file.md`, Path: "my docs/file.md"}},
		},
		{
			name: "single-quoted path with spaces is one token",
			text: "open @'" + mentionSpacedPath + "' now",
			want: []ecosys.Mention{{Token: "@'" + mentionSpacedPath + "'", Path: mentionSpacedPath}},
		},
		{
			name: "trailing period does not join the path",
			text: "see " + mentionTokenReadme + ".",
			want: []ecosys.Mention{{Token: mentionTokenReadme, Path: mentionPathReadme}},
		},
		{
			name: "trailing comma does not join the path",
			text: "see @a.md, then rest",
			want: []ecosys.Mention{{Token: "@a.md", Path: "a.md"}},
		},
		{
			name: "trailing colon does not join the path",
			text: "see @a.md:",
			want: []ecosys.Mention{{Token: "@a.md", Path: "a.md"}},
		},
		{
			name: "mid-word at-sign is never a mention (email-like)",
			text: "email foo@bar.com about it",
			want: nil,
		},
		{
			name: "mention-like suffix of an ordinary token is not a mention",
			text: "path/to@thing stays plain",
			want: nil,
		},
		{
			name: "multiple mentions parse in order",
			text: "compare @a.md with @b.md and @c.md",
			want: []ecosys.Mention{
				{Token: "@a.md", Path: "a.md"},
				{Token: "@b.md", Path: "b.md"},
				{Token: "@c.md", Path: "c.md"},
			},
		},
		{
			name: "quoted mention among plain ones keeps its position",
			text: "@a.md then @\"b c.md\" then @d.md",
			want: []ecosys.Mention{
				{Token: "@a.md", Path: "a.md"},
				{Token: `@"b c.md"`, Path: "b c.md"},
				{Token: "@d.md", Path: "d.md"},
			},
		},
		{
			name: "mention at text start",
			text: mentionTokenReadme + " is the one",
			want: []ecosys.Mention{{Token: mentionTokenReadme, Path: mentionPathReadme}},
		},
		{
			name: "mention at text end",
			text: "the one is " + mentionTokenReadme,
			want: []ecosys.Mention{{Token: mentionTokenReadme, Path: mentionPathReadme}},
		},
		{
			name: "newline and tab delimit tokens",
			text: "first @a.md\nsecond\t@b.md",
			want: []ecosys.Mention{
				{Token: "@a.md", Path: "a.md"},
				{Token: "@b.md", Path: "b.md"},
			},
		},
		{
			name: "bare at-sign alone is not a mention",
			text: "just @ here",
			want: nil,
		},
		{
			name: "at-sign with only punctuation after it is not a mention",
			text: "just @. here",
			want: nil,
		},
		{
			name: "zero mentions yields empty",
			text: "just a normal message",
			want: nil,
		},
		{
			name: "empty text yields empty",
			text: "",
			want: nil,
		},
		{
			name: "unterminated quote is not a mention",
			text: `open @"no closing quote`,
			want: nil,
		},
	}
}

// TestParseMentions runs the mention-parse table (21-04 Task 1): ordered
// extraction, quote-aware path capture, trailing-punctuation trim, mid-word
// exclusion, and the empty contract.
func TestParseMentions(t *testing.T) {
	t.Parallel()

	for _, tc := range mentionCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := ecosys.ParseMentions(tc.text)
			if len(got) != len(tc.want) {
				t.Fatalf("ParseMentions(%q) = %v; want %v", tc.text, got, tc.want)
			}

			for i := range tc.want {
				if got[i].Token != tc.want[i].Token {
					t.Errorf("mention[%d].Token = %q; want %q", i, got[i].Token, tc.want[i].Token)
				}

				if got[i].Path != tc.want[i].Path {
					t.Errorf("mention[%d].Path = %q; want %q", i, got[i].Path, tc.want[i].Path)
				}

				// IsDir is UNKNOWN at parse time (the runtime layer resolves it)
				// — the parser must never guess.
				if got[i].IsDir {
					t.Errorf("mention[%d].IsDir = true; parse must leave it unknown (false)", i)
				}
			}
		})
	}
}
