package coreexec

import (
	"encoding/json"
	"sync"
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
	InProgress int `json:"inProgress"`
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

// Summarize computes the captured summary block over a list.
func Summarize(todos []TodoItem) TodoSummary {
	sum := TodoSummary{Total: len(todos)}
	for i := range todos {
		switch todos[i].Status {
		case "pending":
			sum.Pending++
		case "in_progress":
			sum.InProgress++
		case "completed":
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
