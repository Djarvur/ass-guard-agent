package profile

import (
	"os"

	"gopkg.in/yaml.v3"
)

// FieldDiff is one TIER-1/2 field whose fresh-capture count differs from the
// manifest's declared count. Returned by CheckCoverage; never populated for
// TIER-3 fields (D-06 — audit-only).
type FieldDiff struct {
	Path     string `json:"path"     yaml:"path"`
	Declared int    `json:"declared" yaml:"declared"`
	Observed int    `json:"observed" yaml:"observed"`
	Tier     Tier   `json:"tier"     yaml:"tier"`
	Missing  bool   `json:"missing"  yaml:"missing"`
}

// LoadCoverage reads a coverage.yaml manifest from path.
func LoadCoverage(path string) (CoverageManifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return CoverageManifest{}, err
	}
	// Tier is marshaled as an int in the manifest; yaml.v3 unmarshals it into
	// the Tier (int) type directly.
	var m CoverageManifest
	err = yaml.Unmarshal(raw, &m)
	if err != nil {
		return CoverageManifest{}, err
	}

	return m, nil
}

// CheckCoverage compares a fresh capture's field counts against the manifest's
// declared counts and returns the drifted TIER-1/2 fields. TIER-3 fields are
// ignored. ok is true iff no TIER-1/2 field drifted (the PROF-05 incomplete-
// capture gate).
func CheckCoverage(manifest *CoverageManifest, freshCapture map[string]int) ([]FieldDiff, bool) {
	var diffs []FieldDiff

	for _, f := range manifest.Fields {
		if !f.Tier.DriftFlagged() {
			continue
		}

		got, ok := freshCapture[f.Path]
		if !ok {
			diffs = append(diffs, FieldDiff{
				Path: f.Path, Declared: f.ObservedCount,
				Observed: 0, Tier: f.Tier, Missing: true,
			})

			continue
		}

		if got != f.ObservedCount {
			diffs = append(diffs, FieldDiff{Path: f.Path, Declared: f.ObservedCount, Observed: got, Tier: f.Tier})
		}
	}

	return diffs, len(diffs) == 0
}
