package toolcat //nolint:testpackage // internal package test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testMCPHello is the repeated test-fixture tool name (golangci goconst).
const testMCPHello = "mcp__echo__hello"

// fakeInnerExec is a stub ToolExecutor returning a canned result for every call.
type fakeInnerExec struct{ out json.RawMessage }

func (f fakeInnerExec) Execute(_ context.Context, _ string, _ json.RawMessage) (json.RawMessage, error) {
	return f.out, nil
}

// fakeMCPExec is a stub MCPExecuter returning a canned result for mcp__* calls
// and recording whether it was invoked.
type fakeMCPExec struct {
	out      json.RawMessage
	called   bool
	lastName string
}

func (f *fakeMCPExec) CallTool(_ context.Context, name string, _ json.RawMessage) (json.RawMessage, error) {
	f.called = true
	f.lastName = name

	return f.out, nil
}

// failingMCPExec is a stub MCPExecuter that always errors.
type failingMCPExec struct{}

func (failingMCPExec) CallTool(_ context.Context, _ string, _ json.RawMessage) (json.RawMessage, error) {
	return nil, errors.New("no such server") //nolint:err113 // test stub
}

func TestMCPExecutorRoutesMCPToHost(t *testing.T) {
	t.Parallel()

	inner := fakeInnerExec{out: json.RawMessage(`{"inner":"ok"}`)}
	host := &fakeMCPExec{out: json.RawMessage(`{"mcp":"ok"}`)}
	exec := NewMCPExecutor(inner, host)

	out, err := exec.Execute(context.Background(), "mcp__echo__hello", json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{"mcp":"ok"}`, string(out))
	assert.True(t, host.called, "mcp host must be called for mcp__* names")
	assert.Equal(t, "mcp__echo__hello", host.lastName)
}

func TestMCPExecutorFallthroughNonMCP(t *testing.T) {
	t.Parallel()

	inner := fakeInnerExec{out: json.RawMessage(`{"inner":"ok"}`)}
	host := &fakeMCPExec{out: json.RawMessage(`{"mcp":"ok"}`)}
	exec := NewMCPExecutor(inner, host)

	out, err := exec.Execute(context.Background(), "Read", json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{"inner":"ok"}`, string(out))
	assert.False(t, host.called, "mcp host must NOT be called for non-mcp names")
}

func TestMCPExecutorNilHostErrors(t *testing.T) {
	t.Parallel()

	inner := fakeInnerExec{out: json.RawMessage(`{"inner":"ok"}`)}
	exec := NewMCPExecutor(inner, nil)

	_, err := exec.Execute(context.Background(), "mcp__echo__hello", json.RawMessage(`{}`))
	require.Error(t, err)
}

func TestMCPExecutorHostErrorPropagates(t *testing.T) {
	t.Parallel()

	inner := fakeInnerExec{out: json.RawMessage(`{"inner":"ok"}`)}
	exec := NewMCPExecutor(inner, failingMCPExec{})

	_, err := exec.Execute(context.Background(), "mcp__echo__hello", json.RawMessage(`{}`))
	require.Error(t, err)
}

// TestRegisterAddsTool verifies the existing Register method (added in Phase 4)
// works for MCP tools: registered tool is retrievable via Get, appears in Names
// and Decls, and a second Register overwrites (not duplicates).
func TestRegisterAddsTool(t *testing.T) {
	t.Parallel()

	c := NewCatalog()
	c.Register(Tool{Name: testMCPHello, Description: "v1", InputSchema: json.RawMessage(`{}`)})

	got, ok := c.Get("mcp__echo__hello")
	require.True(t, ok)
	assert.Equal(t, "v1", got.Description)

	assert.Contains(t, c.Names(), "mcp__echo__hello")

	declFound := false

	for _, d := range c.Decls() {
		if d.Name == testMCPHello {
			declFound = true
		}
	}

	assert.True(t, declFound)

	// Overwrite (not duplicate).
	c.Register(Tool{Name: "mcp__echo__hello", Description: "v2", InputSchema: json.RawMessage(`{}`)})
	got2, ok := c.Get("mcp__echo__hello")
	require.True(t, ok)
	assert.Equal(t, "v2", got2.Description)

	count := 0

	for _, n := range c.Names() {
		if n == "mcp__echo__hello" {
			count++
		}
	}

	assert.Equal(t, 1, count, "Register must overwrite, not duplicate")
}

// TestRegisterNilMapSafe verifies Register on a zero-value Catalog (nil map) works.
func TestRegisterNilMapSafe(t *testing.T) {
	t.Parallel()

	var c Catalog // zero value: nil tools map
	c.Register(Tool{Name: "mcp__x__y"})

	got, ok := c.Get("mcp__x__y")
	require.True(t, ok)
	assert.Equal(t, "mcp__x__y", got.Name)
}
