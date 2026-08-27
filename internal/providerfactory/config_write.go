package providerfactory

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/Djarvur/ass-guard-agent/internal/modelrouting"
)

// tempFilePattern names the sibling temp file the atomic replace stages into —
// a hidden dot-prefixed pattern in the layer's own directory, so a rename is
// same-filesystem and the leftover glob in the test family is precise.
const tempFilePattern = ".ass-guard-config-write-*.tmp"

// errEmptyKeyPath is the static guard error for a zero-length key path.
var errEmptyKeyPath = errors.New("empty key path")

// renameFunc is the atomic-replace seam (same-package test injection, the
// Phase-15 parity-seam convention): tests swap it to prove the original layer
// file survives the crash-between-marshal-and-rename window.
var renameFunc = os.Rename //nolint:gochecknoglobals // same-package test-injection seam

// LayerReadError reports that an EXISTING layer file could not be read or
// parsed. The layer file is untouched when this returns — a failed read never
// clobbers the operator's file, not even a corrupt one. Callers (16-05) map
// this type to a distinct JSON-RPC error class from LayerWriteError.
type LayerReadError struct {
	Path string
	Err  error
}

// Error names the layer path — investigate-and-fix-ready.
func (e *LayerReadError) Error() string {
	return fmt.Sprintf("read config layer %q: %v", e.Path, e.Err)
}

// Unwrap exposes the cause for errors.As/Is chains.
func (e *LayerReadError) Unwrap() error { return e.Err }

// LayerWriteError reports that persisting the merged layer failed (marshal,
// directory creation, temp-file write, or the atomic rename). The existing
// layer file is byte-identical when this returns — the write goes to a sibling
// temp file and only the rename touches the target, so no failure can
// half-apply (D-07, T-16-10). Callers (16-05) map this type to a distinct
// JSON-RPC error class from LayerReadError.
type LayerWriteError struct {
	Path string
	Err  error
}

// Error names the layer path — investigate-and-fix-ready.
func (e *LayerWriteError) Error() string {
	return fmt.Sprintf("write config layer %q: %v", e.Path, e.Err)
}

// Unwrap exposes the cause for errors.As/Is chains.
func (e *LayerWriteError) Unwrap() error { return e.Err }

// WriteLayerOption mutates one operator config layer file (the global
// GlobalConfigPath() or project ProjectConfigPath(workDir) config.yaml, as
// addressed by the caller): it reads the existing file as a generic map (a
// missing file starts an empty map — the write creates the layer), deep-merges
// the key-path write with EXACTLY the overlay semantics modelrouting.Load
// applies between layers (modelrouting.DeepMerge — the reuse is load-bearing
// for round-trip fidelity), marshals YAML, and atomically replaces the file:
// sibling temp file written at hard 0600 (T-16-11) then renamed over the
// target. Created directories use 0750 max. No fsync: the repo's
// artifact-write convention (session transcript append path) carries none,
// and this writer keeps parity with it.
//
// Round-trip contract (D-07 write half): modelrouting.Load over a layer this
// function wrote resolves the written value at the written key path, with
// every unrelated key preserved in structure. Note YAML re-marshal reflows the
// file (comments drop, formatting reflows) — the contract is on the LOADED
// config, not raw bytes.
//
// Concurrency contract: WriteLayerOption takes no lock. The caller serializes
// writes per layer path (16-05 applies each set_config_option under one path);
// concurrent writes to the same layer file are a caller bug.
//
// The value whitelist is the caller's contract too (T-16-12): this primitive
// writes any key path it is handed; 16-05 restricts the writable key set at
// the wire.
func WriteLayerOption(layerPath string, keyPath []string, value any) error {
	if len(keyPath) == 0 {
		return &LayerWriteError{Path: layerPath, Err: errEmptyKeyPath}
	}

	layer := make(map[string]any)

	raw, err := os.ReadFile(layerPath)
	switch {
	case err == nil:
		uerr := yaml.Unmarshal(raw, &layer)
		if uerr != nil {
			return &LayerReadError{Path: layerPath, Err: uerr}
		}
	case os.IsNotExist(err):
		// Absent layer — the write creates it.
	default:
		return &LayerReadError{Path: layerPath, Err: err}
	}

	modelrouting.DeepMerge(layer, nestedMap(keyPath, value))

	out, merr := yaml.Marshal(layer)
	if merr != nil {
		return &LayerWriteError{Path: layerPath, Err: merr}
	}

	werr := atomicReplace(layerPath, out)
	if werr != nil {
		return werr
	}

	return nil
}

// atomicReplace stages out into a sibling temp file (hard 0600, T-16-11) in
// the target's directory and renames it over layerPath — the rename is the
// ONLY step that touches the target, so any failure leaves the existing file
// byte-identical with no half-applied state (D-07, T-16-10). Created
// directories use 0750 max. The staged temp file never survives a failure.
func atomicReplace(layerPath string, out []byte) error {
	dir := filepath.Dir(layerPath)

	derr := os.MkdirAll(dir, dirPermOwnerGroup)
	if derr != nil {
		return &LayerWriteError{Path: layerPath, Err: derr}
	}

	tmp, cerr := os.CreateTemp(dir, tempFilePattern)
	if cerr != nil {
		return &LayerWriteError{Path: layerPath, Err: cerr}
	}

	tmpName := tmp.Name()

	// Whatever happens below — including a failed rename — no temp leftovers
	// survive (after a successful rename the name is gone; Remove is a no-op).
	defer func() { _ = os.Remove(tmpName) }()

	_, werr := tmp.Write(out)
	if werr != nil {
		_ = tmp.Close()

		return &LayerWriteError{Path: layerPath, Err: werr}
	}

	clerr := tmp.Close()
	if clerr != nil {
		return &LayerWriteError{Path: layerPath, Err: clerr}
	}

	// CreateTemp already opens 0600; the explicit chmod makes the hard 0600
	// deterministic under any umask.
	perr := os.Chmod(tmpName, filePermOwnerWrite)
	if perr != nil {
		return &LayerWriteError{Path: layerPath, Err: perr}
	}

	rerr := renameFunc(tmpName, layerPath)
	if rerr != nil {
		return &LayerWriteError{Path: layerPath, Err: rerr}
	}

	return nil
}

// nestedMap builds {"k1": {"k2": …: value}} from the key path — the overlay
// the DeepMerge applies so the leaf value lands exactly at the written path.
func nestedMap(keyPath []string, value any) map[string]any {
	root := make(map[string]any)

	cur := root

	for _, k := range keyPath[:len(keyPath)-1] {
		next := make(map[string]any)
		cur[k] = next
		cur = next
	}

	cur[keyPath[len(keyPath)-1]] = value

	return root
}
