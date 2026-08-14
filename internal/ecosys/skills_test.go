package ecosys_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/ecosys"
)

// The listing shape pinned here is CAPTURED GROUND TRUTH (D-06), transcribed
// from the real zcode session rollout record on this machine:
//
//	~/.zcode/cli/rollout/model-io-sess_fb066d52-fab9-4726-a174-ac8f86874dad.jsonl
//	request messages[5] (system role, immediately after the first user
//	message), 2026-08-14 extraction. Entry shape for project/user skills
//	(no plugin alias): "- <name>: <description> (file: <abs path>)". The
//	truncation cutoff (desc[:249] + "...") was measured on the same record's
//	go-ultimate entry.
const goldenListingSource = "model-io-sess_fb066d52-fab9-4726-a174-ac8f86874dad.jsonl messages[5]"

// Fixture skill names (goconst).
const (
	skillAlpha = "alpha"
	skillBeta  = "beta"
)

// writeSkillFixture writes one SKILL.md under dir and returns its path.
func writeSkillFixture(t *testing.T, dir, name, frontmatter, body string) string {
	t.Helper()

	p := filepath.Join(dir, name, "SKILL.md")

	err := os.MkdirAll(filepath.Dir(p), 0o750)
	if err != nil {
		t.Fatalf("mkdir skill fixture: %v", err)
	}

	err = os.WriteFile(p, []byte("---\n"+frontmatter+"---\n"+body), 0o600)
	if err != nil {
		t.Fatalf("write skill fixture %s: %v", name, err)
	}

	return p
}

// twoSkillFixture builds a registry with two fixed skills under a temp dir.
func twoSkillFixture(t *testing.T) (ecosys.Registry, string, string) { //nolint:gocritic // fixture triple
	t.Helper()

	dir := t.TempDir()
	alpha := writeSkillFixture(t, dir, skillAlpha, "name: alpha\ndescription: Alpha does first things.\n",
		"Alpha body instructions.\n")
	beta := writeSkillFixture(t, dir, skillBeta, "name: beta\ndescription: Beta does second things.\n",
		"Beta body instructions.\n")

	reg := ecosys.Registry{Skills: map[string]ecosys.Skill{
		skillAlpha: {Name: skillAlpha, Description: "Alpha does first things.", Path: alpha},
		skillBeta:  {Name: skillBeta, Description: "Beta does second things.", Path: beta},
	}}

	return reg, alpha, beta
}

// TestSkillListing_CapturedShapeGolden verifies SkillListing reproduces the
// captured zcode listing shape EXACTLY for a fixed two-skill registry — the
// golden fixture derived from the captured artifact named above.
func TestSkillListing_CapturedShapeGolden(t *testing.T) {
	t.Parallel()

	reg, alpha, beta := twoSkillFixture(t)

	want := "The following skills are available for use with the Skill tool:\n" +
		"\n" +
		"- alpha: Alpha does first things. (file: " + alpha + ")\n" +
		"- beta: Beta does second things. (file: " + beta + ")"

	if got := ecosys.SkillListing(reg); got != want {
		t.Errorf("SkillListing golden mismatch (captured shape: %s)\n got: %q\nwant: %q",
			goldenListingSource, got, want)
	}
}

// TestSkillListing_TruncationCutoff pins the captured description cutoff:
// descriptions longer than 249 runes hard-cut mid-word + "...".
func TestSkillListing_TruncationCutoff(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("x", 300)

	reg := ecosys.Registry{Skills: map[string]ecosys.Skill{
		"long": {Name: "long", Description: long, Path: "/x/SKILL.md"},
	}}

	got := ecosys.SkillListing(reg)
	if !strings.Contains(got, strings.Repeat("x", 249)+"...") {
		t.Errorf("listing = %q; want desc hard-cut at 249 + ellipsis", got[:min(320, len(got))])
	}

	if strings.Contains(got, strings.Repeat("x", 250)) {
		t.Errorf("listing = %q; description exceeded the captured 249 cutoff", got[:min(320, len(got))])
	}
}

// TestSkillListing_AllSkillsExposed (D-04): every discovered skill appears —
// three in, three entries out; an empty registry yields "" (no merge).
func TestSkillListing_AllSkillsExposed(t *testing.T) {
	t.Parallel()

	reg := ecosys.Registry{Skills: map[string]ecosys.Skill{
		"a": {Name: "a", Description: "A", Path: "/a"},
		"b": {Name: "b", Description: "B", Path: "/b"},
		"c": {Name: "c", Description: "C", Path: "/c"},
	}}

	got := ecosys.SkillListing(reg)
	for _, name := range []string{"a", "b", "c"} {
		if !strings.Contains(got, "- "+name+": ") {
			t.Errorf("listing missing skill %s:\n%s", name, got)
		}
	}

	if empty := ecosys.SkillListing(ecosys.Registry{}); empty != "" {
		t.Errorf("empty registry listing = %q; want \"\" (no merge)", empty)
	}
}

// TestResolveSkill_BodyAfterFrontmatter verifies the body re-read: the
// frontmatter is stripped, the markdown body returned; unknown names miss.
func TestResolveSkill_BodyAfterFrontmatter(t *testing.T) {
	t.Parallel()

	reg, _, _ := twoSkillFixture(t)

	body, ok := ecosys.ResolveSkill(reg, skillAlpha)
	if !ok {
		t.Fatal("ResolveSkill(alpha) miss; want hit")
	}

	if !strings.Contains(body, "Alpha body instructions.") {
		t.Errorf("body = %q; want the markdown body after frontmatter", body)
	}

	if strings.Contains(body, "description:") {
		t.Errorf("body = %q; frontmatter leaked into the body", body)
	}

	if _, ok := ecosys.ResolveSkill(reg, "nope"); ok {
		t.Error("ResolveSkill(nope) hit; want miss")
	}
}

// TestSkillExecute_ReturnsBodyAndStructuredErrors verifies the closure: hit →
// {"content": body}; unknown → {"error":..., "available":[...]} with a NIL Go
// error (structure over error); read failure → error carrying the path.
func TestSkillExecute_ReturnsBodyAndStructuredErrors(t *testing.T) {
	t.Parallel()

	reg, alpha, _ := twoSkillFixture(t)
	exec := ecosys.SkillExecute(reg)

	// Hit: the SKILL.md body as the result.
	out, err := exec(context.Background(), json.RawMessage(`{"skill":"`+skillAlpha+`"}`))
	if err != nil {
		t.Fatalf("Execute err = %v; want nil", err)
	}

	var res struct {
		Content string `json:"content"`
	}

	err = json.Unmarshal(out, &res)
	if err != nil {
		t.Fatalf("result not JSON: %v (%s)", err, out)
	}

	if !strings.Contains(res.Content, "Alpha body instructions.") {
		t.Errorf("content = %q; want the %s body", res.Content, skillAlpha)
	}

	// Unknown: structured, nil Go error, available list names the hits.
	out, err = exec(context.Background(), json.RawMessage(`{"skill":"ghost"}`))
	if err != nil {
		t.Fatalf("Execute(unknown) err = %v; want nil (structured result)", err)
	}

	var unknown struct {
		Error     string   `json:"error"`
		Available []string `json:"available"`
	}

	err = json.Unmarshal(out, &unknown)
	if err != nil {
		t.Fatalf("unknown result not JSON: %v (%s)", err, out)
	}

	if !strings.Contains(unknown.Error, "unknown skill ghost") {
		t.Errorf("error = %q; want unknown skill ghost", unknown.Error)
	}

	if len(unknown.Available) != 2 ||
		unknown.Available[0] != skillAlpha || unknown.Available[1] != skillBeta {
		t.Errorf("available = %v; want [%s %s]", unknown.Available, skillAlpha, skillBeta)
	}

	assertSkillReadFailureStructured(t, exec, alpha)
}

// assertSkillReadFailureStructured verifies the read-failure outcome: the
// registry entry exists but the file is gone → structured error naming the path.
func assertSkillReadFailureStructured(t *testing.T, exec ecosysStub, alpha string) {
	t.Helper()

	err := os.Remove(alpha)
	if err != nil {
		t.Fatalf("remove alpha fixture: %v", err)
	}

	out, err := exec(context.Background(), json.RawMessage(`{"skill":"`+skillAlpha+`"}`))
	if err != nil {
		t.Fatalf("Execute(read-fail) err = %v; want nil (structured result)", err)
	}

	var readFail struct {
		Error string `json:"error"`
		File  string `json:"file"`
	}

	err = json.Unmarshal(out, &readFail)
	if err != nil {
		t.Fatalf("read-fail result not JSON: %v (%s)", err, out)
	}

	if readFail.Error != "skill read failed" || readFail.File != alpha {
		t.Errorf("read-fail = %+v; want the error + fixture path", readFail)
	}
}

// ecosysStub aliases the closure shape SkillExecute returns (keeps the helper
// signature short).
type ecosysStub = func(context.Context, json.RawMessage) (json.RawMessage, error)
