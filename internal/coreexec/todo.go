package coreexec

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// TodoItem is one todo-list entry — the captured camelCase shape
// {content, status, priority} (fixture: TodoWrite input/output item keys;
// status enum pending|in_progress|completed, priority high|medium|low).
type TodoItem struct {
	Content  string `json:"content"`
	Status   string `json:"status"`
	Priority string `json:"priority"`
}

// TodoSummary is the captured echo's summary block (camelCase inProgress).
type TodoSummary struct {
	Total      int `json:"total"`
	Pending    int `json:"pending"`
	InProgress int `json:"inProgress"` //nolint:tagliatelle // captured echo key
	Completed  int `json:"completed"`
}

// TodoStore is the per-session todo list state TodoWrite/TodoRead close over
// (D-16 isolation: constructed once at the sessionFor registration site, one
// per session — two stores never observe each other's writes). Mutex-guarded
// because DispatchBatch may run read-only tools concurrently with... nothing
// else touches the store mid-batch, but TodoRead is classified read-only and
// can run beside another session-owning goroutine's projection.
type TodoStore struct {
	mu    sync.Mutex
	todos []TodoItem
}

// NewTodoStore returns an empty per-session todo store.
func NewTodoStore() *TodoStore {
	return &TodoStore{}
}

// Swap replaces the stored list with todos and returns the PREVIOUS list
// (the captured echo's oldTodos — the store advances on every write).
func (s *TodoStore) Swap(todos []TodoItem) []TodoItem {
	s.mu.Lock()
	defer s.mu.Unlock()

	old := s.todos
	s.todos = append([]TodoItem(nil), todos...)

	return old
}

// Snapshot returns a copy of the current list.
func (s *TodoStore) Snapshot() []TodoItem {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]TodoItem(nil), s.todos...)
}

// Captured status enum values (fixture: TodoWrite input/output).
const (
	statusPending    = "pending"
	statusInProgress = "in_progress"
	statusCompleted  = "completed"
)

// Summarize computes the captured summary block over a list.
func Summarize(todos []TodoItem) TodoSummary {
	sum := TodoSummary{Total: len(todos)}
	for i := range todos {
		switch todos[i].Status {
		case statusPending:
			sum.Pending++
		case statusInProgress:
			sum.InProgress++
		case statusCompleted:
			sum.Completed++
		}
	}

	return sum
}

// marshalTodos is the shared canonical encoding for TodoWrite's echo and
// TodoRead's result (encoding/json struct-field order pins key order —
// oldTodos, todos, summary for the echo; todos, summary for the read).
func marshalTodos(v any) json.RawMessage {
	out, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{"error":"todo marshal failed"}`)
	}

	return out
}

// todoWriteArgs is the observed TodoWrite input shape: {todos:[…]} (the
// full list replaces the previous one — schema semantics).
type todoWriteArgs struct {
	Todos []TodoItem `json:"todos"`
}

// todoWriteEcho is the CAPTURED TodoWrite result: the camelCase JSON echo
// (fixture: TodoWrite.results.echo). Field order pins the wire key order.
type todoWriteEcho struct {
	OldTodos []TodoItem  `json:"oldTodos"` //nolint:tagliatelle // captured echo key
	Todos    []TodoItem  `json:"todos"`
	Summary  TodoSummary `json:"summary"`
}

// TodoWriteExecute returns the TodoWrite catalog Stub over the per-session
// store: swaps old→new and echoes {oldTodos, todos, summary} in the captured
// camelCase shape (inProgress key spelling pinned). Empty lists marshal as
// [], never null (shape fidelity).
func TodoWriteExecute(store *TodoStore) toolcat.Stub {
	return func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
		if store == nil {
			return structuredError("todoWrite: no todo store configured")
		}

		var a todoWriteArgs

		err := json.Unmarshal(args, &a)
		if err != nil {
			return structuredError("todoWrite: invalid input: %v", err)
		}

		if a.Todos == nil {
			a.Todos = []TodoItem{} // echo shape: [] not null
		}

		old := store.Swap(a.Todos)

		if old == nil {
			old = []TodoItem{}
		}

		echo := todoWriteEcho{OldTodos: old, Todos: a.Todos, Summary: Summarize(a.Todos)}

		return marshalTodos(echo), nil
	}
}

// todoReadResult is the TodoRead result: CORPUS-ABSENT form (no TodoRead
// result observed in either harvest — fixture flags it), consistent with the
// echo minus oldTodos.
type todoReadResult struct {
	Todos   []TodoItem  `json:"todos"`
	Summary TodoSummary `json:"summary"`
}

// TodoReadExecute returns the TodoRead catalog Stub: the current list +
// summary over the same per-session store (corpus-absent, flagged).
func TodoReadExecute(store *TodoStore) toolcat.Stub {
	return func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
		if store == nil {
			return structuredError("todoRead: no todo store configured")
		}

		todos := store.Snapshot()
		if todos == nil {
			todos = []TodoItem{}
		}

		return marshalTodos(todoReadResult{Todos: todos, Summary: Summarize(todos)}), nil
	}
}
