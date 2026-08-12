package toolexec_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
	"github.com/Djarvur/ass-guard-agent/internal/toolexec"
)

// fakeBackend is a Backend test double that records calls + returns canned JSON.
type fakeBackend struct {
	name       string
	queries    []string
	urls       []string
	searchOut  json.RawMessage
	fetchOut   json.RawMessage
	searchErr  error
}

func (f *fakeBackend) Name() string { return f.name }
func (f *fakeBackend) Search(_ context.Context, q string) (json.RawMessage, error) {
	f.queries = append(f.queries, q)
	return f.searchOut, f.searchErr
}
func (f *fakeBackend) Fetch(_ context.Context, u string) (json.RawMessage, error) {
	f.urls = append(f.urls, u)
	return f.fetchOut, f.searchErr
}

// TestRealExecutor_CatalogLookup verifies a catalog tool's Execute is invoked +
// its output returned; an unknown tool errors.
func TestRealExecutor_CatalogLookup(t *testing.T) {
	cat := toolcat.NewCatalog()
	cat.Register(toolcat.Tool{
		Name:       "Read",
		Mutability: toolcat.MutabilityReadOnly,
		Execute: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"read":"ok"}`), nil
		},
	})
	re := &toolexec.RealExecutor{Catalog: cat}

	out, err := re.Execute(context.Background(), "Read", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(out) != `{"read":"ok"}` {
		t.Errorf("out = %s; want {\"read\":\"ok\"}", out)
	}

	if _, err := re.Execute(context.Background(), "Mystery", json.RawMessage(`{}`)); err == nil {
		t.Error("unknown tool = nil err; want structured error")
	}
}

// TestRealExecutor_WebSearchDelegates verifies WebSearch routes to the
// configured Backend + forwards the parsed query.
func TestRealExecutor_WebSearchDelegates(t *testing.T) {
	be := &fakeBackend{name: "fake", searchOut: json.RawMessage(`{"hits":["x"]}`)}
	re := &toolexec.RealExecutor{Backends: map[string]toolexec.Backend{"WebSearch": be}}
	out, err := re.Execute(context.Background(), "WebSearch", json.RawMessage(`{"query":"glms"}`))
	if err != nil {
		t.Fatalf("WebSearch: %v", err)
	}
	if string(out) != `{"hits":["x"]}` {
		t.Errorf("out = %s; want hits", out)
	}
	if len(be.queries) != 1 || be.queries[0] != "glms" {
		t.Errorf("backend queries = %+v; want [glms]", be.queries)
	}
}

// TestRealExecutor_WebFetchDelegates verifies WebFetch routes to the Backend +
// forwards the parsed URL.
func TestRealExecutor_WebFetchDelegates(t *testing.T) {
	be := &fakeBackend{name: "fake", fetchOut: json.RawMessage(`{"page":true}`)}
	re := &toolexec.RealExecutor{Backends: map[string]toolexec.Backend{"WebFetch": be}}
	out, err := re.Execute(context.Background(), "WebFetch", json.RawMessage(`{"url":"https://example.com"}`))
	if err != nil {
		t.Fatalf("WebFetch: %v", err)
	}
	if string(out) != `{"page":true}` {
		t.Errorf("out = %s; want page", out)
	}
	if len(be.urls) != 1 || be.urls[0] != "https://example.com" {
		t.Errorf("backend urls = %+v; want [https://example.com]", be.urls)
	}
}

// TestRealExecutor_BackendErrorPropagates verifies a backend error surfaces.
func TestRealExecutor_BackendErrorPropagates(t *testing.T) {
	be := &fakeBackend{name: "fake", searchErr: errors.New("backend down")}
	re := &toolexec.RealExecutor{Backends: map[string]toolexec.Backend{"WebSearch": be}}
	if _, err := re.Execute(context.Background(), "WebSearch", json.RawMessage(`{"query":"x"}`)); err == nil {
		t.Error("err = nil; want backend down")
	}
}

// TestRealExecutor_NilCatalogUnknownErrors verifies a RealExecutor with no
// catalog + a non-backend tool returns a structured error (never a panic).
func TestRealExecutor_NilCatalogUnknownErrors(t *testing.T) {
	re := &toolexec.RealExecutor{}
	if _, err := re.Execute(context.Background(), "Anything", json.RawMessage(`{}`)); err == nil {
		t.Error("err = nil; want structured no-catalog error")
	}
}

// TestRealExecutor_NilExecutor verifies a nil RealExecutor surfaces ErrNoExecutor.
func TestRealExecutor_NilExecutor(t *testing.T) {
	var re *toolexec.RealExecutor
	_, err := re.Execute(context.Background(), "Read", json.RawMessage(`{}`))
	if !errors.Is(err, toolexec.ErrNoExecutor) {
		t.Errorf("err = %v; want ErrNoExecutor", err)
	}
}
