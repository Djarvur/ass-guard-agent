package coreexec //nolint:testpackage // internal package test (decodeJSONString/loadFixture helpers)

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFileForTest creates a temp file with the given content, returning its
// absolute path.
func writeFileForTest(t *testing.T, dir, name, content string) string {
	t.Helper()

	p := filepath.Join(dir, name)

	err := os.WriteFile(p, []byte(content), 0o600)
	if err != nil {
		t.Fatalf("write %s: %v", p, err)
	}

	return p
}

// TestRead_LineNumberedForm (T3 Test 1): a 3-line file renders
// `1\t<line1>\n2\t<line2>\n3\t<line3>` — 1-based number, TAB, content, no
// leading padding (the corpus shows bare `25\t## Current Position`).
func TestRead_LineNumberedForm(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	p := writeFileForTest(t, dir, "three.txt", "alpha\nbeta\ngamma\n")

	exec := ReadExecute(Config{WorkDir: dir})

	out, err := exec(context.Background(), json.RawMessage(`{"file_path":`+jsonStringT(p)+`}`))
	if err != nil {
		t.Fatalf("err = %v; want nil", err)
	}

	if got := decodeJSONString(t, out); got != "1\talpha\n2\tbeta\n3\tgamma" {
		t.Errorf("Read Output = %q; want %q", got, "1\talpha\n2\tbeta\n3\tgamma")
	}
}

// TestRead_OffsetLimit (T3 Test 2): offset=2 + limit=1 returns only
// `2\t<line2>` (the captured offset-read shape: input keys
// {file_path, offset, limit}).
func TestRead_OffsetLimit(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	p := writeFileForTest(t, dir, "three.txt", "alpha\nbeta\ngamma\n")

	exec := ReadExecute(Config{WorkDir: dir})

	in := `{"file_path":` + jsonStringT(p) + `,"offset":2,"limit":1}`

	out, err := exec(context.Background(), json.RawMessage(in))
	if err != nil {
		t.Fatalf("err = %v; want nil", err)
	}

	if got := decodeJSONString(t, out); got != "2\tbeta" {
		t.Errorf("Read offset/limit Output = %q; want %q", got, "2\tbeta")
	}
}

// TestRead_Failures (T3 Test 3): a missing file returns the CAPTURED error
// form (`File does not exist. Note: your current working directory is <dir>.`)
// with a non-nil error (IsError); a directory path likewise fails with a
// structured error (corpus-absent form — flagged in files.go).
func TestRead_Failures(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	exec := ReadExecute(Config{WorkDir: dir})

	out, err := exec(context.Background(),
		json.RawMessage(`{"file_path":`+jsonStringT(filepath.Join(dir, "nope.md"))+`}`))
	if err == nil {
		t.Fatal("missing-file err = nil; want non-nil (IsError)")
	}

	want := "File does not exist. Note: your current working directory is " + dir + "."
	if got := decodeJSONString(t, out); got != want {
		t.Errorf("missing-file Output = %q; want the captured form %q", got, want)
	}

	out, err = exec(context.Background(), json.RawMessage(`{"file_path":`+jsonStringT(dir)+`}`))
	if err == nil {
		t.Fatal("directory err = nil; want non-nil (IsError)")
	}

	var structured struct {
		Error string `json:"error"`
	}

	uerr := json.Unmarshal(out, &structured)
	if uerr != nil || structured.Error == "" {
		t.Errorf("directory Output = %s; want the structured corpus-absent error form", out)
	}
}

// TestWrite_Form (T3 Test 4): writing a NEW file returns exactly the captured
// text with the ABSOLUTE path, the file exists with the content, and parent
// directories are created (documented corpus-silent choice — propose/apply
// write into openspec-created trees). Overwriting an existing file returns
// the CAPTURED updated-form (late-harvest observation).
func TestWrite_Form(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	exec := WriteExecute(Config{WorkDir: dir})

	p := filepath.Join(dir, "nested", "tree", "new.md")

	in := `{"file_path":` + jsonStringT(p) + `,"content":"body\n"}`

	out, err := exec(context.Background(), json.RawMessage(in))
	if err != nil {
		t.Fatalf("err = %v; want nil", err)
	}

	want := "File created successfully at: " + p +
		" (file state is current in the context — no need to Read it back)"
	if got := decodeJSONString(t, out); got != want {
		t.Errorf("Write created Output = %q; want the captured literal %q", got, want)
	}

	body, rerr := os.ReadFile(p)
	if rerr != nil || string(body) != "body\n" {
		t.Errorf("file content = %q (%v); want the written body", body, rerr)
	}

	// Overwrite → the captured updated-form (late harvest).
	in = `{"file_path":` + jsonStringT(p) + `,"content":"replaced\n"}`

	out, err = exec(context.Background(), json.RawMessage(in))
	if err != nil {
		t.Fatalf("overwrite err = %v; want nil", err)
	}

	wantUpdated := "The file " + p + " has been updated successfully." +
		" (file state is current in the context — no need to Read it back)"
	if got := decodeJSONString(t, out); got != wantUpdated {
		t.Errorf("Write updated Output = %q; want the captured literal %q", got, wantUpdated)
	}

	if body, _ := os.ReadFile(p); string(body) != "replaced\n" {
		t.Errorf("overwritten content = %q; want the replacement", body)
	}
}

// TestEdit_Form (T3 Test 5): a single-occurrence replace returns the captured
// text with the file updated; zero occurrences → the CAPTURED not-found
// error; multiple occurrences without replace_all → structured not-unique
// error (corpus-absent); replace_all replaces every occurrence.
func TestEdit_Form(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	exec := EditExecute(Config{WorkDir: dir})

	p := writeFileForTest(t, dir, "edit.txt", "one two three\n")

	in := `{"file_path":` + jsonStringT(p) + `,"old_string":"two","new_string":"TWO"}`

	out, err := exec(context.Background(), json.RawMessage(in))
	if err != nil {
		t.Fatalf("err = %v; want nil", err)
	}

	want := "The file " + p + " has been updated successfully." +
		" (file state is current in the context — no need to Read it back)"
	if got := decodeJSONString(t, out); got != want {
		t.Errorf("Edit Output = %q; want the captured literal %q", got, want)
	}

	if body, _ := os.ReadFile(p); string(body) != "one TWO three\n" {
		t.Errorf("edited content = %q; want the replacement", body)
	}

	// Not-found → the captured <tool_use_error> form + non-nil error.
	in = `{"file_path":` + jsonStringT(p) + `,"old_string":"absent","new_string":"x"}`

	out, err = exec(context.Background(), json.RawMessage(in))
	if err == nil {
		t.Fatal("not-found err = nil; want non-nil (IsError)")
	}

	wantNotFound := "<tool_use_error>String to replace not found in file.\nString: absent</tool_use_error>"
	if got := decodeJSONString(t, out); got != wantNotFound {
		t.Errorf("not-found Output = %q; want the captured form %q", got, wantNotFound)
	}
}

// TestEdit_NotUniqueAndReplaceAll (T3 Test 5, second half): multiple
// occurrences without replace_all → structured not-unique error
// (CORPUS-ABSENT form); replace_all:true replaces every occurrence (schema
// semantics).
func TestEdit_NotUniqueAndReplaceAll(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	exec := EditExecute(Config{WorkDir: dir})

	p := writeFileForTest(t, dir, "dup.txt", "x a x a x\n")

	in := `{"file_path":` + jsonStringT(p) + `,"old_string":"a","new_string":"b"}`

	out, err := exec(context.Background(), json.RawMessage(in))
	if err == nil {
		t.Fatal("not-unique err = nil; want non-nil (IsError)")
	}

	var structured struct {
		Error string `json:"error"`
	}

	uerr := json.Unmarshal(out, &structured)
	if uerr != nil || !strings.Contains(structured.Error, "replace_all") {
		t.Errorf("not-unique Output = %s; want the structured corpus-absent error naming replace_all", out)
	}

	in = `{"file_path":` + jsonStringT(p) + `,"old_string":"a","new_string":"b","replace_all":true}`

	out, err = exec(context.Background(), json.RawMessage(in))
	if err != nil {
		t.Fatalf("replace_all err = %v; want nil", err)
	}

	if body, _ := os.ReadFile(p); string(body) != "x b x b x\n" {
		t.Errorf("replace_all content = %q; want every occurrence replaced", body)
	}

	_ = out
}

// TestFileFixtureConformance (T3 Test 9, first half): Read/Write/Edit outputs
// equal T1's fixture templates (prefix + placeholder substitution).
func TestFileFixtureConformance(t *testing.T) {
	t.Parallel()

	f := loadFixture(t)
	dir := t.TempDir()
	ctx := context.Background()

	// Read template documents the N\t form; sample shows it.
	p := writeFileForTest(t, dir, "f.txt", "l1\nl2\n")

	readOut, err := ReadExecute(Config{WorkDir: dir})(ctx, json.RawMessage(`{"file_path":`+jsonStringT(p)+`}`))
	if err != nil {
		t.Fatalf("read err = %v", err)
	}

	if got := decodeJSONString(t, readOut); !strings.HasPrefix(got, "1\t") || !strings.Contains(got, "\n2\t") {
		t.Errorf("read form = %q; want the fixture's <line-number><TAB> form", got)
	}

	// Write: the created template with the absolute path substituted.
	wp := filepath.Join(dir, "w.txt")

	writeExec := WriteExecute(Config{WorkDir: dir})

	wOut, err := writeExec(ctx,
		json.RawMessage(`{"file_path":`+jsonStringT(wp)+`,"content":"c"}`))
	if err != nil {
		t.Fatalf("write err = %v", err)
	}

	tpl := f.Tools["Write"].Results["success_created"].Template
	want := strings.ReplaceAll(tpl, "<absolute path>", wp)

	if got := decodeJSONString(t, wOut); got != want {
		t.Errorf("write form = %q; want fixture template %q", got, want)
	}

	// Edit: the success template with the absolute path substituted.
	editExec := EditExecute(Config{WorkDir: dir})

	eOut, err := editExec(ctx,
		json.RawMessage(`{"file_path":`+jsonStringT(wp)+`,"old_string":"c","new_string":"d"}`))
	if err != nil {
		t.Fatalf("edit err = %v", err)
	}

	etpl := f.Tools["Edit"].Results["success"].Template
	ewant := strings.ReplaceAll(etpl, "<absolute path>", wp)

	if got := decodeJSONString(t, eOut); got != ewant {
		t.Errorf("edit form = %q; want fixture template %q", got, ewant)
	}
}

// jsonStringT marshals s as a JSON string literal (test-side helper;
// marshal of a string cannot fail — errchkjson satisfied by the checked path).
func jsonStringT(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic("marshal string: " + err.Error())
	}

	return string(b)
}

// TestIsErrorCorpusForm_ReadMissingFile (14-06 Task 8 pin): the live
// 2026-08-19 corpus scan (docs/tool-contract-inventory.md §b) shows Read's
// missing-file error is the plain-text `File does not exist. Note: your
// current working directory is <dir>.` form with IsError set — pinned so the
// emission cannot silently diverge from the corpus (T-14-17).
func TestIsErrorCorpusForm_ReadMissingFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	stub := ReadExecute(Config{WorkDir: dir})

	out, err := stub(context.Background(), json.RawMessage(`{"file_path":"no-such-file.txt"}`))
	if err == nil {
		t.Fatal("missing file must surface a non-nil error (IsError)")
	}

	var text string

	uErr := json.Unmarshal(out, &text)
	if uErr != nil {
		t.Fatalf("output not the captured plain-text form: %v (%s)", uErr, out)
	}

	want := "File does not exist. Note: your current working directory is " + dir + "."
	if text != want {
		t.Errorf("corpus form = %q; want %q", text, want)
	}
}
