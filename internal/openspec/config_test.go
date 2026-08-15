package openspec_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/openspec"
)

// TestDefaultConfig verifies the embedded seeded.toml parses + validates with
// the documented shape (>=2 patterns, >=1 handoff tool, >=4 commands — OPEN-02).
func TestDefaultConfig(t *testing.T) {
	t.Parallel()

	cfg, err := openspec.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}

	if len(cfg.Patterns) < 2 {
		t.Errorf("patterns = %d; want >= 2", len(cfg.Patterns))
	}

	if len(cfg.HandoffTools) < 1 {
		t.Errorf("handoff_tools = %d; want >= 1", len(cfg.HandoffTools))
	}

	if len(cfg.Commands) < 4 {
		t.Errorf("commands = %d; want >= 4", len(cfg.Commands))
	}
	// The seeded impl-complete pattern is the canonical handoff.
	var sawImpl bool

	for _, p := range cfg.Patterns {
		if p.ID == statusImplComplete {
			sawImpl = true

			if p.Action != stopContinue {
				t.Errorf("impl-complete action = %q; want continue", p.Action)
			}
		}
	}

	if !sawImpl {
		t.Error("seeded impl-complete pattern missing")
	}

	assertProbePinnedCommands(t, cfg)
}

// cmdList is the canonical read-only command key (goconst).
const cmdList = "list"

// assertProbePinnedCommands asserts the Phase-8 probe-pinned [commands] surface
// (archive mutating+fixable, phantoms absent, load-time defaults applied).
func assertProbePinnedCommands(t *testing.T, cfg *openspec.OpenSpecConfig) {
	t.Helper()

	archive, ok := cfg.Commands["archive"]
	if !ok || archive.Mutability != classMutating {
		t.Errorf("commands.archive = %+v ok=%v; want mutating", archive, ok)
	}

	if archive.ExitClass != openspec.ExitClassFixable {
		t.Errorf("commands.archive exit_class = %q; want fixable", archive.ExitClass)
	}

	if _, exists := cfg.Commands["apply"]; exists {
		t.Error("phantom commands.apply present; deleted in Phase 8")
	}

	if _, exists := cfg.Commands["implement"]; exists {
		t.Error("phantom commands.implement present; deleted in Phase 8")
	}
	// Load-time defaults: argv defaults to the key; multi-word argv preserved;
	// timeout defaults applied.
	if cfg.Commands[cmdList].Argv != cmdList || cfg.Commands[cmdList].TimeoutSecs != openspec.DefaultTimeoutSecs {
		t.Errorf("commands.%s = %+v; want argv=list timeout=%d",
			cmdList, cfg.Commands[cmdList], openspec.DefaultTimeoutSecs)
	}

	if got := cfg.Commands["new-change"].Argv; got != "new change" {
		t.Errorf("commands.new-change argv = %q; want \"new change\"", got)
	}
}

// TestLoadConfig_File verifies LoadConfig reads + validates a real file path.
func TestLoadConfig_File(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "openspec.toml")

	body := `
[[patterns]]
id = "x"
regex = "hello.*world"
action = "continue"

[[handoff_tools]]
id = "h"
tool = "mytool"
action = "continue"

[commands.foo]
mutability = "read-only"
`

	err := os.WriteFile(path, []byte(body), 0o644)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := openspec.LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if len(cfg.Patterns) != 1 || cfg.Patterns[0].ID != "x" {
		t.Errorf("patterns = %+v; want one [x]", cfg.Patterns)
	}
}

// TestLoadConfig_InvalidRegex verifies a bogus regex yields a ConfigError
// naming the offender.
func TestLoadConfig_InvalidRegex(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.toml")

	body := `
[[patterns]]
id = "badpat"
regex = "[unterminated"
action = "continue"
`

	err := os.WriteFile(path, []byte(body), 0o644)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err = openspec.LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig returned nil; want ConfigError for invalid regex")
	}

	if !contains(err.Error(), "badpat") {
		t.Errorf("err = %v; want it to name the bad pattern id", err)
	}
}

// TestLoadConfig_InvalidAction verifies an unknown action yields a ConfigError.
func TestLoadConfig_InvalidAction(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "badaction.toml")

	body := `
[[patterns]]
id = "x"
regex = "ok"
action = "explode"
`

	err := os.WriteFile(path, []byte(body), 0o644)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err = openspec.LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig returned nil; want ConfigError for unknown action")
	}

	if !contains(err.Error(), "explode") {
		t.Errorf("err = %v; want it to name the bad action", err)
	}
}

// TestLoadConfig_InvalidMutability verifies an unknown mutability yields a
// ConfigError naming the command.
func TestLoadConfig_InvalidMutability(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "badmut.toml")

	body := `
[commands.weird]
mutability = "maybe"
`

	err := os.WriteFile(path, []byte(body), 0o644)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err = openspec.LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig returned nil; want ConfigError for unknown mutability")
	}

	if !contains(err.Error(), "weird") || !contains(err.Error(), "maybe") {
		t.Errorf("err = %v; want it to name the command + the bad value", err)
	}
}

// TestLoadConfig_NotViper verifies the loader does NOT use viper (D-14 — TOML
// directly via BurntSushi/toml). A grep-level guarantee is in acceptance; this
// test pins the config's structural correctness end-to-end.
func TestLoadConfig_CollectAll(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "multi.toml")
	// Two violations: a bad action AND a bad mutability.
	body := `
[[patterns]]
id = "x"
regex = "ok"
action = "nope"

[commands.c]
mutability = "also-nope"
`

	err := os.WriteFile(path, []byte(body), 0o644)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err = openspec.LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig returned nil; want ConfigError")
	}
	// Collect-all: BOTH violations appear.
	if !contains(err.Error(), "nope") || !contains(err.Error(), "also-nope") {
		t.Errorf("err = %v; want BOTH violations (collect-all)", err)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}

	return false
}

// TestMutability_CommandTable (08-04 Test 18) verifies the [command_mutability]
// seeded table classifies the opsx stages by write semantics (explore
// read-only; the artifact-writing stages mutating) and that an operator file
// flips a classification (file wins — the loader's normal precedence).
func TestMutability_CommandTable(t *testing.T) {
	t.Parallel()

	cfg, err := openspec.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}

	if got := cfg.CommandMutability["opsx:explore"]; got != openspec.MutabilityReadOnly {
		t.Errorf("opsx:explore mutability = %q; want read-only (writes no artifacts)", got)
	}

	for _, key := range []string{"opsx:propose", "opsx:apply", "opsx:sync", "opsx:update", "opsx:archive"} {
		if got := cfg.CommandMutability[key]; got != openspec.MutabilityMutating {
			t.Errorf("%s mutability = %q; want mutating (writes openspec/ artifacts)", key, got)
		}
	}

	// Operator overlay flips a classification (D-11 operator-overridable).
	dir := t.TempDir()
	overlay := filepath.Join(dir, "openspec.toml")

	err = os.WriteFile(overlay, []byte(
		"[command_mutability]\n\"opsx:explore\" = \"mutating\"\n"), 0o600)
	if err != nil {
		t.Fatalf("write overlay: %v", err)
	}

	ovr, err := openspec.LoadConfig(overlay)
	if err != nil {
		t.Fatalf("LoadConfig(overlay): %v", err)
	}

	if got := ovr.CommandMutability["opsx:explore"]; got != openspec.MutabilityMutating {
		t.Errorf("overlay opsx:explore mutability = %q; want flipped to mutating", got)
	}
}

// TestCommandPatterns_ConfigParsesRows (hybrid chaining, findings-6
// disposition): a [[command_patterns]] row — id/command/action/next — parses
// with every field preserved.
func TestCommandPatterns_ConfigParsesRows(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "openspec.toml")

	body := "[[command_patterns]]\nid = \"post-explore-handoff\"\n" +
		"command = \"opsx:explore\"\naction = \"continue\"\nnext = \"/opsx:propose\"\n"

	err := os.WriteFile(path, []byte(body), 0o600)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := openspec.LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if len(cfg.CommandPatterns) != 1 {
		t.Fatalf("command_patterns = %d; want 1", len(cfg.CommandPatterns))
	}

	p := cfg.CommandPatterns[0]
	if p.ID != idPostExploreHandoff {
		t.Errorf("id = %q; want post-explore-handoff", p.ID)
	}

	if p.Command != keyOpsxExplore {
		t.Errorf("command = %q; want opsx:explore", p.Command)
	}

	if p.Action != stopContinue {
		t.Errorf("action = %q; want continue", p.Action)
	}

	if p.Next != nextProposeCmd {
		t.Errorf("next = %q; want /opsx:propose", p.Next)
	}
}

// TestCommandPatterns_ValidationCollectAll: an unknown action or an empty
// command key in a [[command_patterns]] row is a ConfigError naming the row.
func TestCommandPatterns_ValidationCollectAll(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "bad.toml")

	body := "[[command_patterns]]\nid = \"bad-action\"\ncommand = \"opsx:x\"\naction = \"explode\"\n" +
		"[[command_patterns]]\nid = \"empty-command\"\ncommand = \"\"\naction = \"continue\"\n"

	err := os.WriteFile(path, []byte(body), 0o600)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err = openspec.LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig returned nil; want ConfigError for the bad command_patterns rows")
	}

	if !contains(err.Error(), "bad-action") || !contains(err.Error(), "explode") {
		t.Errorf("err = %v; want it to name the bad action row", err)
	}

	if !contains(err.Error(), "empty-command") {
		t.Errorf("err = %v; want it to name the empty-command row", err)
	}
}
