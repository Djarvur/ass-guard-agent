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
	// apply is mutating (the canonical boundary command — OPEN-03).
	if apply, ok := cfg.Commands["apply"]; !ok || apply.Mutability != classMutating {
		t.Errorf("commands.apply = %+v ok=%v; want mutating", apply, ok)
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
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
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
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := openspec.LoadConfig(path)
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
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := openspec.LoadConfig(path)
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
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := openspec.LoadConfig(path)
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
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := openspec.LoadConfig(path)
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
