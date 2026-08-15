package coreexec

import (
	"context"
	"encoding/json"
	"testing"
)

// todoInput builds a TodoWrite input over item triples.
func todoInput(items ...[3]string) json.RawMessage {
	type item struct {
		Content  string `json:"content"`
		Status   string `json:"status"`
		Priority string `json:"priority"`
	}

	in := struct {
		Todos []item `json:"todos"`
	}{}

	for _, it := range items {
		in.Todos = append(in.Todos, item{Content: it[0], Status: it[1], Priority: it[2]})
	}

	b, _ := json.Marshal(in)

	return b
}

// todoEcho is the decoded TodoWrite echo / TodoRead result shape.
type todoEcho struct {
	OldTodos []TodoItem `json:"oldTodos"`
	Todos    []TodoItem `json:"todos"`
	Summary  TodoSummary `json:"summary"`
}

// TestTodoWrite_Echo (T3 Test 6): an empty store + a 4-item list returns the
// camelCase echo; a SECOND call's oldTodos equals the first call's todos
// (the store advances). Key order pinned by struct decode, not string-compare.
func TestTodoWrite_Echo(t *testing.T) {
	t.Parallel()

	store := NewTodoStore()
	exec := TodoWriteExecute(store)
	ctx := context.Background()

	first := todoInput(
		[3]string{"t1", "pending", "high"},
		[3]string{"t2", "pending", "high"},
		[3]string{"t3", "in_progress", "medium"},
		[3]string{"t4", "completed", "low"},
	)

	out, err := exec(ctx, first)
	if err != nil {
		t.Fatalf("err = %v; want nil", err)
	}

	var echo todoEcho
	if uerr := json.Unmarshal(out, &echo); uerr != nil {
		t.Fatalf("echo is not JSON: %v (%s)", uerr, out)
	}

	if len(echo.OldTodos) != 0 {
		t.Errorf("first call oldTodos = %v; want empty", echo.OldTodos)
	}

	if len(echo.Todos) != 4 {
		t.Fatalf("todos = %v; want the 4 items verbatim", echo.Todos)
	}

	if echo.Todos[2].Status != "in_progress" || echo.Todos[0].Content != "t1" {
		t.Errorf("todos items not verbatim: %+v", echo.Todos)
	}

	wantSum := TodoSummary{Total: 4, Pending: 2, InProgress: 1, Completed: 1}
	if echo.Summary != wantSum {
		t.Errorf("summary = %+v; want %+v (camelCase inProgress key)", echo.Summary, wantSum)
	}

	// The raw echo carries the camelCase key (fixture-pinned).
	if !jsonKeyPresent(t, out, "inProgress") {
		t.Errorf("echo = %s; want the literal inProgress key", out)
	}

	second := todoInput(
		[3]string{"t1", "completed", "high"},
		[3]string{"t3", "in_progress", "medium"},
	)

	out2, err := exec(ctx, second)
	if err != nil {
		t.Fatalf("second err = %v; want nil", err)
	}

	var echo2 todoEcho
	if uerr := json.Unmarshal(out2, &echo2); uerr != nil {
		t.Fatalf("second echo is not JSON: %v (%s)", uerr, out2)
	}

	if len(echo2.OldTodos) != 4 || echo2.OldTodos[0].Content != "t1" || echo2.OldTodos[2].Status != "in_progress" {
		t.Errorf("second oldTodos = %+v; want the FIRST call's todos (store advances)", echo2.OldTodos)
	}

	if echo2.Summary != (TodoSummary{Total: 2, Pending: 0, InProgress: 1, Completed: 1}) {
		t.Errorf("second summary = %+v", echo2.Summary)
	}
}

// jsonKeyPresent reports whether the raw JSON object carries key literally.
func jsonKeyPresent(t *testing.T, raw json.RawMessage, key string) bool {
	t.Helper()

	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("not an object: %v (%s)", err, raw)
	}

	_, ok := m[key]

	return ok
}

// TestTodoRead (T3 Test 7): returns {todos, summary} over the same store —
// CORPUS-ABSENT form, consistent with the echo minus oldTodos, flagged.
func TestTodoRead(t *testing.T) {
	t.Parallel()

	store := NewTodoStore()

	_, err := TodoWriteExecute(store)(context.Background(), todoInput(
		[3]string{"a", "pending", "high"},
		[3]string{"b", "in_progress", "low"},
	))
	if err != nil {
		t.Fatalf("seed write err = %v", err)
	}

	out, err := TodoReadExecute(store)(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("err = %v; want nil", err)
	}

	var res struct {
		Todos   []TodoItem `json:"todos"`
		Summary TodoSummary `json:"summary"`
	}

	if uerr := json.Unmarshal(out, &res); uerr != nil {
		t.Fatalf("result is not JSON: %v (%s)", uerr, out)
	}

	if len(res.Todos) != 2 || res.Todos[1].Status != "in_progress" {
		t.Errorf("todos = %+v; want the current list", res.Todos)
	}

	if res.Summary != (TodoSummary{Total: 2, Pending: 1, InProgress: 1}) {
		t.Errorf("summary = %+v", res.Summary)
	}

	if jsonKeyPresent(t, out, "oldTodos") {
		t.Errorf("result = %s; TodoRead carries oldTodos (echo-minus-oldTodos shape)", out)
	}
}

// TestTodoStore_PerSessionIsolation (T3 Test 8): two TodoStores never observe
// each other's writes (the sessionFor construction gives each session its
// own).
func TestTodoStore_PerSessionIsolation(t *testing.T) {
	t.Parallel()

	s1, s2 := NewTodoStore(), NewTodoStore()
	ctx := context.Background()

	_, err := TodoWriteExecute(s1)(ctx, todoInput([3]string{"s1-only", "pending", "high"}))
	if err != nil {
		t.Fatalf("s1 write err = %v", err)
	}

	out, err := TodoReadExecute(s2)(ctx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("s2 read err = %v", err)
	}

	var res struct {
		Todos []TodoItem `json:"todos"`
	}

	if uerr := json.Unmarshal(out, &res); uerr != nil {
		t.Fatalf("s2 result is not JSON: %v", uerr)
	}

	if len(res.Todos) != 0 {
		t.Errorf("s2 saw s1's todos: %+v (cross-session bleed — D-16 violation)", res.Todos)
	}
}
