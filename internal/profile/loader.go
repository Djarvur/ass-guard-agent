package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Loader reads named profiles from a root directory. Each profile lives under
// <root>/<name>/ and is loaded by name (PROF-01). The loader is profile-agnostic
// (PROF-02): the same code path loads the zcode profile and any synthetic
// fixture.
type Loader struct {
	Root string
}

// NewLoader returns a Loader rooted at root.
func NewLoader(root string) *Loader { return &Loader{Root: root} }

// Load reads the profile named name from the loader's root. The directory must
// contain: profile.yaml, system/block-*.txt (one per system block, loaded in
// lexical order), tools.json, identity.yaml, thinking.json, tool_choice.json.
func (l *Loader) Load(name string) (Profile, error) {
	dir := filepath.Join(l.Root, name)

	info, err := os.Stat(dir)
	if err != nil {
		return Profile{}, fmt.Errorf("profile %q: %w", name, err)
	}

	if !info.IsDir() {
		return Profile{}, fmt.Errorf("profile %q: not a directory", name)
	}

	var p Profile

	err = loadYAML(filepath.Join(dir, "profile.yaml"), &p)
	if err != nil {
		return Profile{}, fmt.Errorf("profile %q profile.yaml: %w", name, err)
	}

	blocks, err := readSystemBlocks(dir)
	if err != nil {
		return Profile{}, fmt.Errorf("profile %q system blocks: %w", name, err)
	}

	p.System = blocks

	err = loadJSON(filepath.Join(dir, "tools.json"), &p.Tools)
	if err != nil {
		return Profile{}, fmt.Errorf("profile %q tools.json: %w", name, err)
	}

	var id struct {
		Headers []Header `yaml:"headers"`
	}

	err = loadYAML(filepath.Join(dir, "identity.yaml"), &id)
	if err != nil {
		return Profile{}, fmt.Errorf("profile %q identity.yaml: %w", name, err)
	}

	p.Headers = id.Headers

	p.Thinking, err = os.ReadFile(filepath.Join(dir, "thinking.json"))
	if err != nil {
		return Profile{}, fmt.Errorf("profile %q thinking.json: %w", name, err)
	}

	p.ToolChoice, err = os.ReadFile(filepath.Join(dir, "tool_choice.json"))
	if err != nil {
		return Profile{}, fmt.Errorf("profile %q tool_choice.json: %w", name, err)
	}

	return p, nil
}

// readSystemBlocks loads system/block-*.txt in lexical order as TextBlocks of
// type "text". The files are read verbatim (byte-faithful — TIER-1), with no
// trailing-newline normalization beyond what the file stores.
func readSystemBlocks(dir string) ([]TextBlock, error) {
	sysDir := filepath.Join(dir, "system")

	entries, err := os.ReadDir(sysDir)
	if err != nil {
		return nil, fmt.Errorf("read system dir: %w", err)
	}

	var names []string

	for _, e := range entries {
		if e.IsDir() {
			continue
		}

		if !strings.HasPrefix(e.Name(), "block-") || !strings.HasSuffix(e.Name(), ".txt") {
			continue
		}

		names = append(names, e.Name())
	}

	sort.Strings(names)

	if len(names) == 0 {
		return nil, fmt.Errorf("no system/block-*.txt files in %s", sysDir)
	}

	blocks := make([]TextBlock, 0, len(names))

	for _, n := range names {
		raw, rerr := os.ReadFile(filepath.Join(sysDir, n))
		if rerr != nil {
			return nil, fmt.Errorf("read %s: %w", n, rerr)
		}

		blocks = append(blocks, TextBlock{Type: "text", Text: string(raw)})
	}

	return blocks, nil
}

func loadYAML(path string, out any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("call: %w", err)
	}

	return yaml.Unmarshal(raw, out)
}

func loadJSON(path string, out any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("call: %w", err)
	}

	return json.Unmarshal(raw, out)
}
