package ecosys

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"strings"

	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// The skills-listing shape is CAPTURED GROUND TRUTH (D-06), pinned from a real
// zcode session on this machine — rollout record
// ~/.zcode/cli/rollout/model-io-sess_fb066d52-fab9-4726-a174-ac8f86874dad.jsonl,
// request messages[5]: a dedicated system-role message placed immediately
// after the first user message, carrying exactly:
//
//	The following skills are available for use with the Skill tool:
//
//	- <name>: <description> (file: <absolute SKILL.md path>)
//
// Observed details pinned by the golden fixture + truncation test:
//   - header line, blank line, then one entry per line ("- " prefix);
//   - project/user skills carry NO alias (only plugin-namespaced skills show
//     "(also loadable as <alias>)" — ass-guard discovers no plugin skills);
//   - long descriptions hard-cut mid-word at 249 chars + "..." (observed
//     cutoff on the go-ultimate entry: desc[:249] + "...");
//   - the entry ends with " (file: <absolute path>)".
const (
	skillListingHeader = "The following skills are available for use with the Skill tool:"

	// skillDescMax is the captured description cutoff: descriptions longer
	// than this are hard-cut (mid-word) and suffixed with "...".
	skillDescMax = 249

	// skillEllipsis terminates a truncated listing description.
	skillEllipsis = "..."
)

// SkillListing renders every discovered skill (AllSkills — D-04: ALL
// discovered skills exposed, no filtering) in the captured listing shape.
// An empty registry yields "" (the caller skips the merge — zero-skill
// degradation leaves the profile copy untouched).
func SkillListing(reg Registry) string {
	skills := reg.AllSkills()
	if len(skills) == 0 {
		return ""
	}

	entries := make([]string, 0, len(skills))
	for _, s := range skills {
		entries = append(entries, "- "+s.Name+": "+truncateSkillDesc(s.Description)+" (file: "+s.Path+")")
	}

	return skillListingHeader + "\n\n" + strings.Join(entries, "\n")
}

// truncateSkillDesc applies the captured cutoff: >skillDescMax runes are
// hard-cut mid-word and suffixed with "...".
func truncateSkillDesc(desc string) string {
	r := []rune(desc)
	if len(r) <= skillDescMax {
		return desc
	}

	return string(r[:skillDescMax]) + skillEllipsis
}

// agentListingHeader heads the agent-type listing. CORPUS-ABSENT header text:
// no captured zcode session on this machine carries ass-guard's discovered
// plugin/agents entries — the captured Agent tool description lists only
// zcode's own built-in types. The listing follows the SAME dedicated-system-
// block merge as the skills listing (the established dynamic-merge pattern)
// and is flagged for the re-capture to pin the native placement.
const agentListingHeader = "The following specialized agent types are available for use with the Agent tool:"

// AgentListing renders every discovered agent definition (12-02) in the
// captured Agent-tool type-listing entry shape:
//
//	- <name>: <description> (Tools: <comma-joined tools>)
//
// An empty registry yields "" (the caller skips the merge — zero-agent
// degradation leaves the profile copy untouched). Tool lists reuse the
// captured "(Tools: …)" suffix form; an empty Tools list renders "(Tools: *)"
// (the captured unrestricted-entry convention).
func AgentListing(reg Registry) string {
	agents := reg.AllAgents()
	if len(agents) == 0 {
		return ""
	}

	entries := make([]string, 0, len(agents))
	for _, a := range agents {
		tools := strings.Join(a.Tools, ", ")
		if tools == "" {
			tools = "*"
		}

		entries = append(entries, "- "+a.Name+": "+truncateSkillDesc(a.Description)+" (Tools: "+tools+")")
	}

	return agentListingHeader + "\n\n" + strings.Join(entries, "\n")
}

// ResolveSkill returns the SKILL.md BODY (after frontmatter — the same
// splitFrontmatter semantics the loader applies to commands) for a registry
// key. ok=false when the name is not in the registry or the body cannot be
// read (resolution is BY REGISTRY KEY — Path comes from discovery, never from
// call input; T-8-20).
func ResolveSkill(reg Registry, name string) (string, bool) {
	sk, ok := reg.Skills[name]
	if !ok {
		return "", false
	}

	data, err := os.ReadFile(sk.Path)
	if err != nil {
		return "", false
	}

	_, body := splitFrontmatter(string(data))

	return body, true
}

// SkillExecute builds the per-session Execute closure for the captured Skill
// tool (CMD-06 / D-05): input {"skill": "<name>", "args": "<optional>"} → the
// SKILL.md body loads into the turn as the tool result. Unknown names and
// read failures return STRUCTURED results (never Go errors — the model sees
// the shape and adapts, same discipline as D-10):
//
//   - unknown skill → {"error":"unknown skill <name>","available":[sorted names]}
//   - read failure  → {"error":"skill read failed","skill":<name>,"file":<path>}
//   - success       → {"content":"<SKILL.md body>"}
//
// The result shape note: no captured zcode session on this machine records a
// Skill tool_use result (the listing is present; an invocation is not), so
// the success shape follows ass-guard's established content-result convention
// (WebFetch's {"content": …}) rather than a capture — flagged for the Phase-9
// re-capture to pin.
func SkillExecute(reg Registry) toolcat.Stub {
	return func(_ context.Context, input json.RawMessage) (json.RawMessage, error) {
		var in struct {
			Skill string `json:"skill"`
			Args  string `json:"args"`
		}

		_ = json.Unmarshal(input, &in) // best-effort; empty skill → unknown below

		sk, inReg := reg.Skills[in.Skill]
		if !inReg {
			return json.Marshal(struct {
				Error     string   `json:"error"`
				Available []string `json:"available"`
			}{
				Error:     "unknown skill " + in.Skill,
				Available: sortedSkillNames(reg),
			})
		}

		body, ok := ResolveSkill(reg, in.Skill)
		if !ok {
			return json.Marshal(struct {
				Error string `json:"error"`
				Skill string `json:"skill"`
				File  string `json:"file"`
			}{
				Error: "skill read failed",
				Skill: in.Skill,
				File:  sk.Path,
			})
		}

		return json.Marshal(struct {
			Content string `json:"content"`
		}{Content: body})
	}
}

// sortedSkillNames returns the registry's skill names sorted (the unknown
// error's "available" list — deterministic).
func sortedSkillNames(reg Registry) []string {
	out := make([]string, 0, len(reg.Skills))
	for name := range reg.Skills {
		out = append(out, name)
	}

	sort.Strings(out)

	return out
}
