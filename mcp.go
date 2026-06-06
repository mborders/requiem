package requiem

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// defaultMCPPath is appended to the API's common route prefix when MCPConfig.Path
// is empty, so the MCP endpoint sits in the same URL namespace as the API itself.
const defaultMCPPath = "/mcp"

// mcpProtocolVersion is the latest MCP protocol version this addon implements. It
// is advertised when the client requests a version we don't support (per the MCP
// spec, the server responds with a version it does support).
const mcpProtocolVersion = "2025-06-18"

// supportedProtocolVersions are the MCP protocol revisions this addon can speak.
// On initialize, the client's requested version is echoed back only if listed
// here; otherwise we fall back to mcpProtocolVersion.
var supportedProtocolVersions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

func isSupportedProtocol(v string) bool {
	for _, s := range supportedProtocolVersions {
		if s == v {
			return true
		}
	}
	return false
}

// MCPConfig configures the optional MCP (Model Context Protocol) addon. It mirrors
// the OpenAPI addon: enable it via Server.UseMCP and every registered route is
// exposed as an MCP tool unless opted out with Route.ExcludeFromMCP.
type MCPConfig struct {
	// Name is the server name reported to MCP clients (serverInfo.name).
	Name string
	// Version is the server version reported to MCP clients (serverInfo.version).
	Version string
	// Instructions is optional free-form guidance returned during initialize.
	Instructions string
	// Path overrides the endpoint path. Defaults to the API's common route prefix
	// plus "/mcp" (e.g. "/widgets/mcp").
	Path string
	// OptIn flips the addon to allowlist mode: when true, no route is exposed
	// unless it is explicitly opted in with Route.IncludeInMCP. The default (false)
	// keeps the expose-everything behavior. ExcludeFromMCP always wins over both.
	OptIn bool
	// Authenticate optionally gates tools/list and tools/call: the caller must pass
	// it, and the context it populates (e.g. an authenticated user) is what each
	// route's Authorizer is evaluated against to filter the tool list. When nil,
	// the endpoint stays unauthenticated and the list is unfiltered (legacy).
	Authenticate HTTPInterceptor
}

// ExcludeFromMCP hides this route from the MCP tool list. It takes precedence over
// IncludeInMCP and over MCPConfig.OptIn.
func (rt *Route) ExcludeFromMCP() *Route {
	rt.mcpExcluded = true
	return rt
}

// IncludeInMCP exposes this route when the addon runs in allowlist mode
// (MCPConfig.OptIn). It is a no-op in the default expose-everything mode.
func (rt *Route) IncludeInMCP() *Route {
	rt.mcpIncluded = true
	return rt
}

// MCPTool overrides the auto-generated MCP tool name for this route.
func (rt *Route) MCPTool(name string) *Route {
	rt.mcpToolName = name
	return rt
}

// Authorizer reports whether an already-authenticated caller may use a route. It
// is a pure predicate: it reads identity from the context (set by MCPConfig.Authenticate)
// and must not write to the response.
type Authorizer func(ctx HTTPContext) bool

// Authorize attaches a per-caller authorization predicate to this route. When
// MCPConfig.Authenticate is set, tools/list returns a route's tool only if its
// Authorizer passes for the caller; a route without one is always visible.
func (rt *Route) Authorize(a Authorizer) *Route {
	rt.authorizer = a
	return rt
}

type mcpTool struct {
	name        string
	description string
	inputSchema map[string]interface{}
	authorizer  Authorizer
	route       *Route
}

type mcpController struct {
	cfg    MCPConfig
	router *Router

	once  sync.Once
	tools []mcpTool
	index map[string]*mcpTool
}

func (c *mcpController) Load(router *Router) {
	c.router = router

	path := c.cfg.Path
	if path == "" {
		path = commonRoutePrefix(router.routes) + defaultMCPPath
	}

	router.MuxRouter.HandleFunc(path, c.handleRPC).Methods(http.MethodPost)
	// Clients probing for an SSE stream (GET) get a clean 405 rather than a 404.
	router.MuxRouter.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
	}).Methods(http.MethodGet)
}

// --- JSON-RPC 2.0 envelope ---

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

func (c *mcpController) handleRPC(w http.ResponseWriter, r *http.Request) {
	var req rpcRequest
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeRPC(w, errorResponse(nil, -32700, "Parse error"))
		return
	}

	// JSON-RPC notifications carry no id and must never receive a response body.
	notification := len(req.ID) == 0

	// Structural validation (JSON-RPC 2.0 §4): a request must declare jsonrpc
	// "2.0" and a non-empty method. A malformed one is an Invalid Request (-32600),
	// not a Method-not-found (-32601).
	if req.JSONRPC != "2.0" || req.Method == "" {
		if notification {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		writeRPC(w, errorResponse(req.ID, -32600, "Invalid Request"))
		return
	}

	// Client->server notifications are acknowledged with no body.
	switch req.Method {
	case "notifications/initialized", "notifications/cancelled":
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// tools/list and tools/call are gated when an Authenticate hook is configured;
	// initialize/ping stay open for protocol negotiation. The populated context is
	// what tools/list filters against and what tools/call dispatches with.
	ctx := c.newContext(r)
	if mcpRequiresAuth(req.Method) && c.cfg.Authenticate != nil {
		if !c.cfg.Authenticate(ctx) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	}

	result, rerr := c.dispatch(ctx, req)

	if notification {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if rerr != nil {
		writeRPC(w, rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: rerr})
		return
	}
	writeRPC(w, c.success(req.ID, result))
}

// dispatch routes a validated JSON-RPC request to its handler, returning either a
// result or a JSON-RPC error. It never writes to the response, so the caller can
// uniformly suppress output for notifications.
func (c *mcpController) dispatch(ctx HTTPContext, req rpcRequest) (interface{}, *rpcError) {
	switch req.Method {
	case "initialize":
		return c.initializeResult(req.Params), nil
	case "ping":
		return map[string]interface{}{}, nil
	case "tools/list":
		return map[string]interface{}{"tools": c.toolList(ctx)}, nil
	case "tools/call":
		return c.invoke(ctx.Request, req.Params)
	default:
		return nil, &rpcError{Code: -32601, Message: fmt.Sprintf("Method not found: %s", req.Method)}
	}
}

// mcpRequiresAuth reports the methods gated by MCPConfig.Authenticate. initialize
// and ping stay open so a client can negotiate the protocol before presenting
// credentials; the tool surface itself is never exposed without authentication.
func mcpRequiresAuth(method string) bool {
	return method == "tools/list" || method == "tools/call"
}

// newContext builds a request-scoped context for the JSON-RPC dispatch. The
// response is a throwaway recorder so an Authenticate hook that writes a status on
// failure (e.g. 401) doesn't corrupt the real JSON-RPC response.
func (c *mcpController) newContext(r *http.Request) HTTPContext {
	return HTTPContext{
		Request:    r,
		Response:   &responseRecorder{header: http.Header{}, status: http.StatusOK},
		attributes: map[string]interface{}{},
	}
}

func (c *mcpController) success(id json.RawMessage, result interface{}) rpcResponse {
	return rpcResponse{JSONRPC: "2.0", ID: id, Result: result}
}

func errorResponse(id json.RawMessage, code int, msg string) rpcResponse {
	return rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}}
}

func writeRPC(w http.ResponseWriter, resp rpcResponse) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (c *mcpController) initializeResult(params json.RawMessage) map[string]interface{} {
	version := mcpProtocolVersion
	if len(params) > 0 {
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if json.Unmarshal(params, &p) == nil && isSupportedProtocol(p.ProtocolVersion) {
			version = p.ProtocolVersion
		}
	}

	result := map[string]interface{}{
		"protocolVersion": version,
		"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
		"serverInfo": map[string]interface{}{
			"name":    c.cfg.Name,
			"version": c.cfg.Version,
		},
	}
	if c.cfg.Instructions != "" {
		result["instructions"] = c.cfg.Instructions
	}
	return result
}

// --- Tool generation ---

func (c *mcpController) build() {
	c.once.Do(func() {
		c.index = map[string]*mcpTool{}
		used := map[string]bool{}
		for _, rt := range c.router.routes {
			if rt.mcpExcluded {
				continue
			}
			// In allowlist mode, a route must opt in explicitly. Exclude still wins.
			if c.cfg.OptIn && !rt.mcpIncluded {
				continue
			}
			name := rt.mcpToolName
			if name == "" {
				name = defaultToolName(rt)
			}
			// Dedup both auto-generated and overridden names so a duplicate
			// MCPTool override can't silently shadow another tool.
			name = uniqueToolName(name, used)
			used[name] = true
			c.tools = append(c.tools, mcpTool{
				name:        name,
				description: toolDescription(rt),
				inputSchema: buildInputSchema(rt),
				authorizer:  rt.authorizer,
				route:       rt,
			})
		}
		for i := range c.tools {
			c.index[c.tools[i].name] = &c.tools[i]
		}
	})
}

// toolList returns the tool definitions visible to the caller. When Authenticate
// is configured, a tool with an Authorizer is included only when it passes for ctx
// (a tool without one is always visible). Without Authenticate there is no caller
// identity to filter against, so the full list is returned (legacy behavior).
func (c *mcpController) toolList(ctx HTTPContext) []map[string]interface{} {
	c.build()
	filter := c.cfg.Authenticate != nil
	out := make([]map[string]interface{}, 0, len(c.tools))
	for _, t := range c.tools {
		if filter && t.authorizer != nil && !t.authorizer(ctx) {
			continue
		}
		out = append(out, map[string]interface{}{
			"name":        t.name,
			"description": t.description,
			"inputSchema": t.inputSchema,
		})
	}
	return out
}

var toolNameSanitize = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// defaultToolName derives a stable MCP tool name from a route's method and path,
// e.g. GET /widgets/{id} -> "get_widgets_id".
func defaultToolName(rt *Route) string {
	path := stripPathRegex(rt.path)
	path = strings.NewReplacer("{", "", "}", "").Replace(path)
	name := strings.ToLower(rt.method) + "_" + path
	name = toolNameSanitize.ReplaceAllString(name, "_")
	name = strings.Trim(name, "_")
	if name == "" {
		name = strings.ToLower(rt.method)
	}
	return name
}

func uniqueToolName(base string, used map[string]bool) string {
	if !used[base] {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s_%d", base, i)
		if !used[candidate] {
			return candidate
		}
	}
}

func toolDescription(rt *Route) string {
	parts := []string{}
	if rt.summary != "" {
		parts = append(parts, rt.summary)
	}
	if rt.description != "" {
		parts = append(parts, rt.description)
	}
	if len(parts) == 0 {
		return fmt.Sprintf("%s %s", rt.method, stripPathRegex(rt.path))
	}
	return strings.Join(parts, " — ")
}

// buildInputSchema builds an unambiguous JSON Schema: path params and query params
// are top-level properties; a request body (if any) is nested under "body".
func buildInputSchema(rt *Route) map[string]interface{} {
	db := newDocBuilder()
	properties := map[string]interface{}{}
	required := []string{}

	declaredPath := map[string]bool{}
	for _, p := range rt.params {
		schema := map[string]interface{}{"type": paramType(p.typ)}
		if p.description != "" {
			schema["description"] = p.description
		}
		properties[p.name] = schema
		if p.in == "path" {
			declaredPath[p.name] = true
			required = append(required, p.name)
		} else if p.required {
			required = append(required, p.name)
		}
	}
	// Auto-extracted path params not explicitly declared via Route.Param.
	for _, name := range extractPathParams(rt.path) {
		if declaredPath[name] {
			continue
		}
		properties[name] = map[string]interface{}{"type": "string"}
		required = append(required, name)
	}

	if rt.bodyType != nil {
		properties["body"] = db.schemaFor(rt.bodyType)
		required = append(required, "body")
	}

	schema := map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		sort.Strings(required)
		schema["required"] = required
	}
	// Carry any named struct schemas the body referenced under $defs (JSON Schema),
	// not components/schemas (OpenAPI). schemaFor emits OpenAPI-style $ref pointers,
	// so rewrite every $ref in the tree to match before returning.
	if len(db.schemas) > 0 {
		defs := make(map[string]interface{}, len(db.schemas))
		for name, s := range db.schemas {
			defs[name] = s
		}
		schema["$defs"] = defs
	}
	rewriteRefsToDefs(schema)
	return schema
}

const (
	openapiRefPrefix    = "#/components/schemas/"
	jsonSchemaRefPrefix = "#/$defs/"
)

// rewriteRefsToDefs recursively rewrites OpenAPI-style $ref pointers
// ("#/components/schemas/X") to JSON-Schema ones ("#/$defs/X"). MCP tool input
// schemas are plain JSON Schema with definitions under $defs, but the shared
// schema generator (schema.go) emits OpenAPI component refs, so they must be
// remapped or they won't resolve for an MCP client.
func rewriteRefsToDefs(v interface{}) {
	switch t := v.(type) {
	case map[string]interface{}:
		for k, val := range t {
			if k == "$ref" {
				if s, ok := val.(string); ok && strings.HasPrefix(s, openapiRefPrefix) {
					t[k] = jsonSchemaRefPrefix + strings.TrimPrefix(s, openapiRefPrefix)
				}
				continue
			}
			rewriteRefsToDefs(val)
		}
	case []interface{}:
		for _, e := range t {
			rewriteRefsToDefs(e)
		}
	}
}

func paramType(t string) string {
	if t == "" {
		return "string"
	}
	return t
}

// --- Tool invocation: in-process re-dispatch through the existing router ---

type callParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

func (c *mcpController) invoke(httpReq *http.Request, params json.RawMessage) (map[string]interface{}, *rpcError) {
	c.build()

	var p callParams
	if json.Unmarshal(params, &p) != nil {
		return nil, &rpcError{Code: -32602, Message: "Invalid params"}
	}
	tool, ok := c.index[p.Name]
	if !ok {
		return nil, &rpcError{Code: -32602, Message: fmt.Sprintf("Unknown tool: %s", p.Name)}
	}
	rt := tool.route
	args := p.Arguments
	if args == nil {
		args = map[string]interface{}{}
	}

	// Resolve and validate path params (always required). Reject non-scalar or
	// missing values with -32602 rather than dispatching a broken path.
	pathValues := map[string]string{}
	for _, name := range extractPathParams(rt.path) {
		s, ok, rerr := paramValue(args[name])
		if rerr != nil {
			return nil, rerr
		}
		if !ok {
			return nil, &rpcError{Code: -32602, Message: fmt.Sprintf("Missing required path parameter: %s", name)}
		}
		pathValues[name] = s
	}
	path := substitutePathParams(rt.path, pathValues)

	// Build query string and collect declared header params from the arguments.
	query := url.Values{}
	headerArgs := map[string]string{}
	for _, pp := range rt.params {
		if pp.in != "query" && pp.in != "header" {
			continue
		}
		s, ok, rerr := paramValue(args[pp.name])
		if rerr != nil {
			return nil, rerr
		}
		if !ok {
			continue
		}
		if pp.in == "query" {
			query.Set(pp.name, s)
		} else {
			headerArgs[pp.name] = s
		}
	}

	target := c.router.basePath + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	var body []byte
	if rt.bodyType != nil {
		body, _ = json.Marshal(args["body"])
	}

	synthReq, err := http.NewRequest(rt.method, target, bytes.NewReader(body))
	if err != nil {
		return nil, &rpcError{Code: -32603, Message: "Failed to build request"}
	}
	synthReq.Header.Set("Content-Type", "application/json")
	// Forward headers from the incoming MCP request so auth interceptors keep
	// working, but strip hop-by-hop and client-controlled routing headers that
	// must not cross into an in-process dispatch.
	for k, vals := range httpReq.Header {
		if mcpSkipHeader(k) {
			continue
		}
		for _, v := range vals {
			synthReq.Header.Add(k, v)
		}
	}
	// Explicit header parameters from the tool arguments take precedence.
	for name, v := range headerArgs {
		synthReq.Header.Set(name, v)
	}

	rec := &responseRecorder{header: http.Header{}, status: http.StatusOK}
	c.router.MuxRouter.ServeHTTP(rec, synthReq)

	text := rec.body.String()
	if text == "" {
		text = http.StatusText(rec.status)
	}
	return map[string]interface{}{
		"content": []map[string]interface{}{
			{"type": "text", "text": text},
		},
		"isError": rec.status >= 400,
	}, nil
}

// substitutePathParams replaces {name} / {name:regex} placeholders with their
// URL-escaped values. It reuses pathParamRegex (openapi.go) rather than redefining
// the pattern.
func substitutePathParams(path string, values map[string]string) string {
	return pathParamRegex.ReplaceAllStringFunc(path, func(m string) string {
		name := pathParamRegex.FindStringSubmatch(m)[1]
		if v, ok := values[name]; ok {
			return url.PathEscape(v)
		}
		return m
	})
}

// paramValue coerces a JSON argument into the string form used for a path, query,
// or header parameter. Scalars (string/bool/number) convert; nil/absent yields
// ok=false (skip); objects and arrays are rejected with -32602 so a caller can't
// smuggle a structured value into a scalar slot.
func paramValue(v interface{}) (string, bool, *rpcError) {
	switch t := v.(type) {
	case nil:
		return "", false, nil
	case string:
		return t, true, nil
	case bool:
		return strconv.FormatBool(t), true, nil
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64), true, nil
	case json.Number:
		return t.String(), true, nil
	default:
		return "", false, &rpcError{Code: -32602, Message: "Invalid params: expected a scalar value"}
	}
}

// mcpSkipHeader reports headers that must not be copied from the MCP request onto
// the in-process dispatch: body-framing headers (set explicitly here), hop-by-hop
// headers (RFC 7230 §6.1), and client-controlled routing headers that downstream
// handlers or interceptors may trust for authorization.
func mcpSkipHeader(k string) bool {
	switch http.CanonicalHeaderKey(k) {
	case "Content-Type", "Content-Length",
		"Host", "Connection", "Keep-Alive", "Proxy-Authenticate",
		"Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade",
		"Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto":
		return true
	}
	return false
}

// responseRecorder is a minimal http.ResponseWriter that captures the response of
// an in-process dispatch, avoiding a dependency on net/http/httptest in non-test code.
type responseRecorder struct {
	header      http.Header
	body        bytes.Buffer
	status      int
	wroteHeader bool
}

func (r *responseRecorder) Header() http.Header { return r.header }

func (r *responseRecorder) WriteHeader(status int) {
	if !r.wroteHeader {
		r.status = status
		r.wroteHeader = true
	}
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.wroteHeader = true
	return r.body.Write(b)
}
