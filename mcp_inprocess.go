package requiem

import (
	"errors"
	"fmt"
	"net/http"
)

var (
	ErrMCPNotReady     = errors.New("requiem: MCP endpoint is not loaded yet")
	ErrMCPUnauthorized = errors.New("requiem: MCP caller is not authenticated")
)

// MCP gives in-process access to the tools the MCP endpoint serves, as seen by one caller.
type MCP struct {
	c *mcpController
}

// MCPTool is one tool as tools/list reports it.
type MCPTool struct {
	Name        string
	Description string
	InputSchema map[string]interface{}
}

// MCPResult is one tool call's outcome as tools/call reports it.
type MCPResult struct {
	Text    string
	IsError bool
}

// Tools lists the tools visible to the caller whose credentials are on r.
func (m *MCP) Tools(r *http.Request) ([]MCPTool, error) {
	ctx, err := m.c.authenticate(r)
	if err != nil {
		return nil, err
	}
	visible := m.c.visibleTools(ctx)
	out := make([]MCPTool, 0, len(visible))
	for _, t := range visible {
		out = append(out, MCPTool{Name: t.name, Description: t.description, InputSchema: t.inputSchema})
	}
	return out, nil
}

// Call runs a tool visible to the caller whose credentials are on r, under r's context.
func (m *MCP) Call(r *http.Request, name string, args map[string]interface{}) (MCPResult, error) {
	ctx, err := m.c.authenticate(r)
	if err != nil {
		return MCPResult{}, err
	}
	m.c.build()
	tool, ok := m.c.index[name]
	if !ok || !m.c.visible(ctx, tool) {
		return MCPResult{}, fmt.Errorf("requiem: unknown MCP tool %q", name)
	}
	res, rerr := m.c.call(r, tool, args)
	if rerr != nil {
		return MCPResult{}, errors.New(rerr.Message)
	}
	return res, nil
}

func (c *mcpController) authenticate(r *http.Request) (HTTPContext, error) {
	if c.router == nil {
		return HTTPContext{}, ErrMCPNotReady
	}
	ctx := c.newContext(r)
	if c.cfg.Authenticate != nil && !c.cfg.Authenticate(ctx) {
		return HTTPContext{}, ErrMCPUnauthorized
	}
	return ctx, nil
}
