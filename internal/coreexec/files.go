package coreexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// Captured file-tool result texts (fixture-pinned; em dash verbatim).
const (
	fileWriteCreatedPrefix = "File created successfully at: "
	fileUpdatedPrefix      = "The file "
	fileUpdatedMiddle      = " has been updated successfully." +
		" (file state is current in the context — no need to Read it back)"
	fileWriteCreatedSuffix = " (file state is current in the context — no need to Read it back)"
	readMissingPrefix      = "File does not exist. Note: your current working directory is "
	editNotFoundPrefix     = "<tool_use_error>String to replace not found in file.\nString: "
)

// File-tool write modes (gosec-bounded; the capture pins no on-disk mode —
// owner-writable is the conservative choice).
const (
	filePermWrite = 0o600
	dirPermWrite  = 0o750
)

// fileArgs is the shared input shape (observed key sets: Read {file_path,
// offset?, limit?}; Write {file_path, content}; Edit {file_path, old_string,
// new_string, replace_all?}).
type fileArgs struct {
	FilePath   string  `json:"file_path"`
	Offset     float64 `json:"offset"`
	Limit      float64 `json:"limit"`
	Content    string  `json:"content"`
	OldString  string  `json:"old_string"`
	NewString  string  `json:"new_string"`
	ReplaceAll bool    `json:"replace_all"`
}

// structuredError renders the shipped corpus-absent failure convention:
// {"error":…} + a non-nil error (IsError). Used ONLY for forms the capture
// does not show (fixture corpus_absent) — observed forms are rendered
// verbatim instead.
func structuredError(format string, args ...any) (json.RawMessage, error) {
	msg := fmt.Sprintf(format, args...)

	out, err := json.Marshal(map[string]string{keyError: msg})
	if err != nil {
		return nil, fmt.Errorf("coreexec: marshal error for %q: %w", msg, err)
	}

	return out, errors.New(msg) //nolint:err113 // dynamic structured error
}

// absPath resolves the input path against the session WorkDir when relative
// (the captured inputs are absolute per the schema; relative paths still
// work, resolved like a shell in the workdir would).
func (cfg Config) absPath(p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}

	base := cfg.WorkDir
	if base == "" {
		base, _ = os.Getwd()
	}

	return filepath.Join(base, p)
}

// ReadExecute returns the Read catalog Stub: line-numbered rendering
// `<n><TAB><content>` 1-based, no padding (fixture: Read.results.success),
// offset = 1-based start line (0/absent → 1; schema minimum 0), limit = line
// count. Missing file → the CAPTURED plain-text error (late-harvest
// observation); a directory → the structured corpus-absent convention.
func ReadExecute(cfg Config) toolcat.Stub {
	return func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		var a fileArgs

		err := json.Unmarshal(args, &a)
		if err != nil {
			return structuredError("read: invalid input: %v", err)
		}

		body, err := os.ReadFile(a.FilePath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				// The CAPTURED missing-file form (2x in the corpus).
				out, mErr := json.Marshal(readMissingPrefix + cfg.workDirForError() + ".")
				if mErr != nil {
					return nil, fmt.Errorf("coreexec: marshal read-missing form: %w", mErr)
				}

				return out, fmt.Errorf("coreexec: read: %w", err)
			}

			// CORPUS-ABSENT (directory, permissions, …): structured convention.
			return structuredError("read: %v", err)
		}

		out, rErr := json.Marshal(renderLines(body, a.Offset, a.Limit))
		if rErr != nil {
			return nil, fmt.Errorf("coreexec: marshal read form: %w", rErr)
		}

		return out, nil
	}
}

// workDirForError returns the workdir the captured Read error names.
func (cfg Config) workDirForError() string {
	if cfg.WorkDir != "" {
		return cfg.WorkDir
	}

	wd, _ := os.Getwd()

	return wd
}

// renderLines renders file bytes in the captured cat -n form. A trailing
// newline does NOT produce an empty final line (no artifact).
func renderLines(body []byte, offset, limit float64) string {
	text := strings.TrimSuffix(string(body), "\n")
	if text == "" && len(body) > 0 {
		// A file of just newlines: every line is empty but real.
		text = string(body)
	}

	lines := strings.Split(text, "\n")

	start := 0
	if offset > 1 {
		start = int(offset) - 1 // offset is the 1-based first line number
	}

	if start > len(lines) {
		start = len(lines)
	}

	end := len(lines)
	if limit > 0 && start+int(limit) < end {
		end = start + int(limit)
	}

	var sb strings.Builder
	for i := start; i < end; i++ {
		if sb.Len() > 0 {
			sb.WriteByte('\n')
		}

		sb.WriteString(strconv.Itoa(i + 1))
		sb.WriteByte('\t')
		sb.WriteString(lines[i])
	}

	return sb.String()
}

// WriteExecute returns the Write catalog Stub: MkdirAll for the parent
// (documented corpus-silent choice — propose/apply write into
// openspec-created trees), 0o644 write, then the CAPTURED text with the
// ABSOLUTE path — created-form for a new file, updated-form for an overwrite
// (late-harvest observation, 2x). Failures use the structured corpus-absent
// convention. NOTE: the captured `File has not been read yet` /
// `File has been modified since read` enforcement (read-tracking) is
// deliberately NOT implemented — flagged in the fixture's corpus_absent for
// the operator's disposition.
func WriteExecute(cfg Config) toolcat.Stub {
	return func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		var a fileArgs

		err := json.Unmarshal(args, &a)
		if err != nil {
			return structuredError("write: invalid input: %v", err)
		}

		p := cfg.absPath(a.FilePath)
		if p == "" {
			return structuredError("write: empty file_path")
		}

		err = os.MkdirAll(filepath.Dir(p), dirPermWrite)
		if err != nil {
			return structuredError("write: %v", err)
		}

		_, statErr := os.Stat(p)

		err = os.WriteFile(p, []byte(a.Content), filePermWrite)
		if err != nil {
			return structuredError("write: %v", err)
		}

		var text string
		if errors.Is(statErr, os.ErrNotExist) {
			text = fileWriteCreatedPrefix + p + fileWriteCreatedSuffix
		} else {
			text = fileUpdatedPrefix + p + fileUpdatedMiddle
		}

		out, mErr := json.Marshal(text)
		if mErr != nil {
			return nil, fmt.Errorf("coreexec: marshal write form: %w", mErr)
		}

		return out, nil
	}
}

// EditExecute returns the Edit catalog Stub: exact string replacement with
// captured semantics — zero occurrences → the CAPTURED
// <tool_use_error>String to replace not found…</tool_use_error> form;
// multiple occurrences without replace_all → the structured corpus-absent
// not-unique error naming replace_all; replace_all replaces every
// occurrence; success writes back + returns the CAPTURED updated text with
// the ABSOLUTE path. (Read-tracking enforcement is deliberately not
// implemented — see WriteExecute's note.)
func EditExecute(cfg Config) toolcat.Stub {
	return func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		var a fileArgs

		err := json.Unmarshal(args, &a)
		if err != nil {
			return structuredError("edit: invalid input: %v", err)
		}

		p := cfg.absPath(a.FilePath)
		if p == "" {
			return structuredError("edit: empty file_path")
		}

		body, err := os.ReadFile(p)
		if err != nil {
			return structuredError("edit: %v", err)
		}

		text := string(body)
		n := strings.Count(text, a.OldString)

		switch {
		case n == 0:
			// The CAPTURED not-found form (late harvest).
			out, mErr := json.Marshal(editNotFoundPrefix + a.OldString + "</tool_use_error>")
			if mErr != nil {
				return nil, fmt.Errorf("coreexec: marshal edit not-found form: %w", mErr)
			}

			//nolint:err113 // the not-found detail IS the message
			return out, fmt.Errorf("coreexec: edit: %q not found in %s", a.OldString, p)
		case n > 1 && !a.ReplaceAll:
			// CORPUS-ABSENT (not-unique): structured convention, named for
			// the schema's escape hatch.
			return structuredError(
				"edit: old_string appears %d times in %s; make it unique or pass replace_all:true", n, p)
		case a.ReplaceAll:
			text = strings.ReplaceAll(text, a.OldString, a.NewString)
		default:
			text = strings.Replace(text, a.OldString, a.NewString, 1)
		}

		//nolint:gosec // T-8-33: the model's path IS the input (locked safety model)
		err = os.WriteFile(p, []byte(text), filePermWrite)
		if err != nil {
			return structuredError("edit: %v", err)
		}

		out, mErr := json.Marshal(fileUpdatedPrefix + p + fileUpdatedMiddle)
		if mErr != nil {
			return nil, fmt.Errorf("coreexec: marshal edit form: %w", mErr)
		}

		return out, nil
	}
}
