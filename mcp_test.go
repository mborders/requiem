package requiem

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// MCPController registers a small API plus MCP-specific cases: an excluded route,
// a tool-name override, and an auth-guarded route to verify header forwarding.
type MCPController struct{}

func (c MCPController) Load(router *Router) {
	r := router.NewRestRouter("/widgets")

	r.Get("/", func(ctx HTTPContext) {
		ctx.SendJSON([]Widget{})
	}).
		Summary("List widgets").
		Query("limit", "integer", false, "Max items")

	// Registered before "/{id}" so mux's first-match-wins routing doesn't swallow it.
	r.Get("/secret", func(ctx HTTPContext) {
		ctx.SendJSON(map[string]string{"ok": "true"})
	}, func(ctx HTTPContext) bool {
		if ctx.Request.Header.Get("X-Token") != "letmein" {
			ctx.SendStatus(http.StatusUnauthorized)
			return false
		}
		return true
	}).Summary("Secret")

	r.Get("/{id}", func(ctx HTTPContext) {
		ctx.SendJSON(Widget{ID: ctx.GetParam("id"), Name: "found"})
	}).
		Summary("Get widget").
		Param("id", "string", "Widget identifier").
		MCPTool("fetch_widget")

	r.Post("/", func(ctx HTTPContext) {
		ctx.SendJSONWithStatus(ctx.Body, http.StatusCreated)
	}, CreateWidget{}).
		Summary("Create widget")

	r.Delete("/{id}", func(ctx HTTPContext) {
		ctx.SendStatus(http.StatusNoContent)
	}, nil).
		Summary("Delete widget").
		ExcludeFromMCP()
}

// mcpRouter builds a loaded router with the MCP addon, ready for in-process dispatch.
func mcpRouter() *Router {
	return newRouter(defaultBasePath, nil, []IHttpController{
		MCPController{},
		&mcpController{cfg: MCPConfig{Name: "Widget API", Version: "1.0.0"}},
	})
}

// rpc posts a JSON-RPC request to /api/mcp and returns the decoded response.
func rpc(t *testing.T, r *Router, method string, params interface{}, headers map[string]string) rpcResponse {
	t.Helper()
	payload := map[string]interface{}{"jsonrpc": "2.0", "id": 1, "method": method}
	if params != nil {
		payload["params"] = params
	}
	b, _ := json.Marshal(payload)
	rec := rawRPC(t, r, b, headers)

	var resp rpcResponse
	assert.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp
}

// rawRPC posts an arbitrary body to /api/mcp and returns the raw recorder, for
// exercising malformed requests and notifications (no decode/assert imposed).
func rawRPC(t *testing.T, r *Router, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	r.MuxRouter.ServeHTTP(rec, req)
	return rec
}

func TestMCP_Initialize(t *testing.T) {
	r := mcpRouter()
	resp := rpc(t, r, "initialize", map[string]interface{}{"protocolVersion": "2025-03-26"}, nil)

	assert.Nil(t, resp.Error)
	result := resp.Result.(map[string]interface{})
	// Echoes the client's requested protocol version.
	assert.Equal(t, "2025-03-26", result["protocolVersion"])
	caps := result["capabilities"].(map[string]interface{})
	assert.Contains(t, caps, "tools")
	info := result["serverInfo"].(map[string]interface{})
	assert.Equal(t, "Widget API", info["name"])
	assert.Equal(t, "1.0.0", info["version"])
}

func TestMCP_ToolsList(t *testing.T) {
	r := mcpRouter()
	resp := rpc(t, r, "tools/list", nil, nil)
	assert.Nil(t, resp.Error)

	tools := resp.Result.(map[string]interface{})["tools"].([]interface{})
	names := map[string]map[string]interface{}{}
	for _, tv := range tools {
		tm := tv.(map[string]interface{})
		names[tm["name"].(string)] = tm
	}

	// Auto-named, overridden, and absent (excluded) tools.
	assert.Contains(t, names, "get_widgets")
	assert.Contains(t, names, "fetch_widget") // MCPTool override
	assert.Contains(t, names, "post_widgets")
	assert.NotContains(t, names, "delete_widgets_id") // ExcludeFromMCP

	// Path param is a required top-level property.
	get := names["fetch_widget"]
	schema := get["inputSchema"].(map[string]interface{})
	props := schema["properties"].(map[string]interface{})
	assert.Contains(t, props, "id")
	assert.Contains(t, schema["required"], "id")

	// Request body is nested under "body".
	post := names["post_widgets"]
	postSchema := post["inputSchema"].(map[string]interface{})
	postProps := postSchema["properties"].(map[string]interface{})
	assert.Contains(t, postProps, "body")
	assert.Contains(t, postSchema["required"], "body")
}

func TestMCP_ToolCall_GET(t *testing.T) {
	r := mcpRouter()
	resp := rpc(t, r, "tools/call", map[string]interface{}{
		"name":      "fetch_widget",
		"arguments": map[string]interface{}{"id": "42"},
	}, nil)
	assert.Nil(t, resp.Error)

	result := resp.Result.(map[string]interface{})
	assert.Equal(t, false, result["isError"])
	content := result["content"].([]interface{})
	text := content[0].(map[string]interface{})["text"].(string)

	// Handler echoed the path param back into the response body.
	var w Widget
	assert.NoError(t, json.Unmarshal([]byte(text), &w))
	assert.Equal(t, "42", w.ID)
}

func TestMCP_ToolCall_InvalidBody(t *testing.T) {
	r := mcpRouter()
	// CreateWidget.Name is validate:"required"; empty body should fail validation.
	resp := rpc(t, r, "tools/call", map[string]interface{}{
		"name":      "post_widgets",
		"arguments": map[string]interface{}{"body": map[string]interface{}{}},
	}, nil)
	assert.Nil(t, resp.Error)

	result := resp.Result.(map[string]interface{})
	assert.Equal(t, true, result["isError"])
}

func TestMCP_ToolCall_HeaderForwarding(t *testing.T) {
	r := mcpRouter()

	// Without the token, the auth interceptor rejects the call.
	denied := rpc(t, r, "tools/call", map[string]interface{}{
		"name": "get_widgets_secret",
	}, nil)
	assert.Equal(t, true, denied.Result.(map[string]interface{})["isError"])

	// With the token forwarded on the MCP request, it succeeds.
	allowed := rpc(t, r, "tools/call", map[string]interface{}{
		"name": "get_widgets_secret",
	}, map[string]string{"X-Token": "letmein"})
	assert.Equal(t, false, allowed.Result.(map[string]interface{})["isError"])
}

func TestMCP_UnknownMethod(t *testing.T) {
	r := mcpRouter()
	resp := rpc(t, r, "does/not/exist", nil, nil)
	assert.NotNil(t, resp.Error)
	assert.Equal(t, -32601, resp.Error.Code)
}

// A non-scalar value for a scalar parameter must be rejected with -32602 rather
// than stringified into a malformed query value.
func TestMCP_TypeConfusion_RejectsNonScalar(t *testing.T) {
	r := mcpRouter()
	resp := rpc(t, r, "tools/call", map[string]interface{}{
		"name":      "get_widgets",
		"arguments": map[string]interface{}{"limit": map[string]int{"nested": 123}},
	}, nil)

	assert.NotNil(t, resp.Error)
	assert.Equal(t, -32602, resp.Error.Code)
}

// A null query parameter is skipped, not stringified to "<nil>".
func TestMCP_NilQueryParamSkipped(t *testing.T) {
	r := mcpRouter()
	resp := rpc(t, r, "tools/call", map[string]interface{}{
		"name":      "get_widgets",
		"arguments": map[string]interface{}{"limit": nil},
	}, nil)
	assert.Nil(t, resp.Error)
	assert.Equal(t, false, resp.Result.(map[string]interface{})["isError"])
}

// A missing required path parameter is a param error, not a downstream 404.
func TestMCP_MissingPathParam(t *testing.T) {
	r := mcpRouter()
	resp := rpc(t, r, "tools/call", map[string]interface{}{
		"name":      "fetch_widget",
		"arguments": map[string]interface{}{},
	}, nil)
	assert.NotNil(t, resp.Error)
	assert.Equal(t, -32602, resp.Error.Code)
}

// The body schema's $ref must point into $defs (JSON Schema), not OpenAPI's
// components/schemas, and the referenced definition must be present.
func TestMCP_BodySchemaRefsResolve(t *testing.T) {
	r := mcpRouter()
	resp := rpc(t, r, "tools/list", nil, nil)
	tools := resp.Result.(map[string]interface{})["tools"].([]interface{})

	var post map[string]interface{}
	for _, tv := range tools {
		tm := tv.(map[string]interface{})
		if tm["name"] == "post_widgets" {
			post = tm
		}
	}
	assert.NotNil(t, post)

	schema := post["inputSchema"].(map[string]interface{})
	body := schema["properties"].(map[string]interface{})["body"].(map[string]interface{})
	ref := body["$ref"].(string)
	assert.True(t, strings.HasPrefix(ref, "#/$defs/"), "ref should point into $defs, got %q", ref)
	assert.NotContains(t, ref, "components/schemas")

	defs := schema["$defs"].(map[string]interface{})
	name := strings.TrimPrefix(ref, "#/$defs/")
	assert.Contains(t, defs, name)

	// No stale OpenAPI-style refs anywhere in the schema tree.
	raw, _ := json.Marshal(schema)
	assert.NotContains(t, string(raw), "#/components/schemas/")
}

func TestMCP_Ping(t *testing.T) {
	r := mcpRouter()
	resp := rpc(t, r, "ping", nil, nil)
	assert.Nil(t, resp.Error)
	assert.NotNil(t, resp.Result)
}

func TestMCP_UnknownTool(t *testing.T) {
	r := mcpRouter()
	resp := rpc(t, r, "tools/call", map[string]interface{}{"name": "nope"}, nil)
	assert.NotNil(t, resp.Error)
	assert.Equal(t, -32602, resp.Error.Code)
}

func TestMCP_ParseError(t *testing.T) {
	r := mcpRouter()
	rec := rawRPC(t, r, []byte("{not json"), nil)
	var resp rpcResponse
	assert.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotNil(t, resp.Error)
	assert.Equal(t, -32700, resp.Error.Code)
}

func TestMCP_InvalidRequest_BadVersion(t *testing.T) {
	r := mcpRouter()
	rec := rawRPC(t, r, []byte(`{"jsonrpc":"1.0","id":1,"method":"ping"}`), nil)
	var resp rpcResponse
	assert.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotNil(t, resp.Error)
	assert.Equal(t, -32600, resp.Error.Code)
}

// A notification (no id) must not receive a JSON-RPC response body.
func TestMCP_NotificationNoResponse(t *testing.T) {
	r := mcpRouter()
	rec := rawRPC(t, r, []byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`), nil)
	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Empty(t, strings.TrimSpace(rec.Body.String()))
}

// An unsupported requested protocol version falls back to the server's latest.
func TestMCP_ProtocolVersionFallback(t *testing.T) {
	r := mcpRouter()
	resp := rpc(t, r, "initialize", map[string]interface{}{"protocolVersion": "1999-01-01"}, nil)
	assert.Nil(t, resp.Error)
	assert.Equal(t, mcpProtocolVersion, resp.Result.(map[string]interface{})["protocolVersion"])
}

func TestMCP_GetMethodNotAllowed(t *testing.T) {
	r := mcpRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/mcp", nil)
	rec := httptest.NewRecorder()
	r.MuxRouter.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

// Duplicate MCPTool overrides must not silently shadow one another.
func TestMCP_DuplicateOverrideNamesDeduped(t *testing.T) {
	c := &mcpController{}
	r := newRouter(defaultBasePath, nil, []IHttpController{dupOverrideController{}, c})
	_ = r
	c.build()
	assert.Len(t, c.tools, 2)
	names := map[string]bool{}
	for _, tl := range c.tools {
		assert.False(t, names[tl.name], "tool name %q duplicated", tl.name)
		names[tl.name] = true
	}
	assert.Contains(t, names, "dup")
	assert.Contains(t, names, "dup_2")
}

type dupOverrideController struct{}

func (dupOverrideController) Load(router *Router) {
	r := router.NewRestRouter("/things")
	r.Get("/a", func(ctx HTTPContext) { ctx.SendStatus(200) }).MCPTool("dup")
	r.Get("/b", func(ctx HTTPContext) { ctx.SendStatus(200) }).MCPTool("dup")
}

// A header parameter declared via Route.Header and supplied in tool arguments must
// be set on the in-process request so the handler can read it.
func TestMCP_HeaderParamForwarded(t *testing.T) {
	// Explicit Path: headerParamController's only route ("/things/echo") has no
	// wildcard, so commonRoutePrefix would otherwise mount MCP at /api/things/mcp.
	c := &mcpController{cfg: MCPConfig{Path: "/mcp"}}
	r := newRouter(defaultBasePath, nil, []IHttpController{headerParamController{}, c})

	resp := rpc(t, r, "tools/call", map[string]interface{}{
		"name":      "get_things_echo",
		"arguments": map[string]interface{}{"X-Trace": "abc123"},
	}, nil)
	assert.Nil(t, resp.Error)

	result := resp.Result.(map[string]interface{})
	assert.Equal(t, false, result["isError"])
	text := result["content"].([]interface{})[0].(map[string]interface{})["text"].(string)
	assert.Contains(t, text, "abc123")
}

type headerParamController struct{}

func (headerParamController) Load(router *Router) {
	r := router.NewRestRouter("/things")
	r.Get("/echo", func(ctx HTTPContext) {
		ctx.SendJSON(map[string]string{"trace": ctx.Request.Header.Get("X-Trace")})
	}).Header("X-Trace", "string", true, "trace id")
}
