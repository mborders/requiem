[![GoDoc](http://godoc.org/github.com/mborders/requiem?status.png)](http://godoc.org/github.com/mborders/requiem)
[![Build Status](https://travis-ci.com/mborders/requiem.svg?branch=master)](https://travis-ci.com/mborders/requiem)
[![Go Report Card](https://goreportcard.com/badge/github.com/mborders/requiem)](https://goreportcard.com/report/github.com/mborders/requiem)
[![codecov](https://codecov.io/gh/mborders/requiem/branch/master/graph/badge.svg)](https://codecov.io/gh/mborders/requiem)

# requiem

Controller-based REST API server container for Golang with Postgres support
- Uses [GORM v1.21.13](https://github.com/go-gorm/gorm) for DB interaction with Postgres
- Uses [Gorilla Mux v1.8.0](https://github.com/gorilla/mux) for routing
- Uses [logmatic v0.4.0](https://github.com/mborders/logmatic) for nice server logs
- Optional OpenAPI 3.0 spec + Swagger UI generated from registered routes
- Optional MCP (Model Context Protocol) endpoint exposing routes as tools
- Default port is 8080
- Default base path is /api

Documentation here: https://godoc.org/github.com/mborders/requiem

## Example Usage
### Without DB
```go
s := requiem.NewServer(... controllers)
s.Start()
```

### With DB
```go
s := requiem.NewServer(... controllers)
s.EnableDB = true
s.Start()
```

### Change port or base path
```go
s := requiem.NewServer(... controllers)
s.Port = 9090
s.BasePath = "/rest"
s.Start()
```

## HttpController example
```go
type MyController struct {
    DB *gorm.DB
}

type Response struct {
    Message string
}

type CreateRequest struct {
    SomeValue string
}

func (c MyController) getStuff(ctx requiem.HTTPContext) {
    m := Response{Message: "Hello, world!"}
    ctx.SendJSON(res)
}

func (c MyController) createStuff(ctx requiem.HTTPContext) {
    m := ctx.Body.(*CreateRequest)
    fmt.Println("Value: %s", m.SomeValue)
    ctx.SendStatus(Http.StatusNoContent)
}

func AuthInterceptor(ctx HTTPContext) bool {
    // Example:
    //   1) Check if user is authenticated
    //   2) Return false if not
    //   3) Use ctx.SetAttribute() to pass user claim
    
    return true
}

func (c MyController) Load(router *requiem.Router) {
    c.DB = router.DB
    r := router.NewRestRouter("/stuff")
    r.Get("/", c.getStuff)
    r.Post("/", c.createStuff)
    
    // Use AuthInterceptor
    r.Get("/interceptor", func(ctx HTTPContext) {
        ctx.SendStatus(http.StatusOK)
    }, AuthInterceptor)
}
```

## OpenAPI / Swagger UI

Enable an auto-generated OpenAPI 3.0 spec and Swagger UI page:

```go
s := requiem.NewServer(controllers...)
s.UseOpenAPI(requiem.OpenAPIConfig{
    Title:   "My API",
    Version: "1.0.0",
    Servers: []string{"/api"},
})
s.Start()
```

This mounts:
- `GET /api/openapi.json` — the OpenAPI 3.0 document (JSON)
- `GET /api/docs` — Swagger UI rendered against the spec

Paths, methods, and request body schemas are derived automatically from your route registrations. Attach richer metadata by chaining off the returned `*Route`:

```go
func (c MyController) Load(router *requiem.Router) {
    r := router.NewRestRouter("/stuff")

    r.Get("/", c.list).
        Summary("List stuff").
        Tags("stuff").
        Query("limit", "integer", false, "Max items").
        Returns(200, []Stuff{}, "List of stuff")

    r.Post("/", c.create, CreateStuff{}).
        Summary("Create stuff").
        Tags("stuff").
        Returns(201, Stuff{}, "Created").
        Returns(400, ErrorResponse{}, "Invalid input")

    r.Get("/{id}", c.getOne).
        Summary("Get one").
        Param("id", "string", "Stuff identifier").
        Returns(200, Stuff{}, "OK").
        Returns(404, ErrorResponse{}, "Not found")

    r.Delete("/internal", c.purge, nil).
        ExcludeFromSpec()
}
```

Request body schemas come from the struct passed to `Post`/`Put`/`Delete` (honors `json:"..."` and `validate:"required,min=,max=,oneof="` tags). Response schemas come from the value passed to `Returns(...)`. Struct types are emitted under `components.schemas` and referenced via `$ref`.

`OpenAPIConfig` defaults:
- `SpecPath` defaults to `/openapi.json`
- `DocsPath` defaults to `/docs` (set to `"-"` to disable the Swagger UI page)

## MCP (Model Context Protocol)

Expose your existing REST routes as MCP tools so LLM clients can discover and call them. Like the OpenAPI support, it is a purely optional addon — nothing changes unless you enable it:

```go
s := requiem.NewServer(controllers...)
s.UseMCP(requiem.MCPConfig{
    Name:    "My API",
    Version: "1.0.0",
})
s.Start()
```

This mounts a single JSON-RPC 2.0 endpoint (`POST /api/mcp` by default) implementing the MCP `initialize`, `tools/list`, and `tools/call` methods. Every registered route becomes a tool automatically:

- Tool names are derived from method + path (e.g. `GET /widgets/{id}` → `get_widgets_id`).
- Input schemas are built from the route's path params, query params, and request body struct (reusing the same schema generation as OpenAPI). The request body is nested under a `body` property.
- Descriptions come from `Summary`/`Description` metadata.

Tool calls are re-dispatched in-process through the same router, so request body validation, interceptors (auth, etc.), and handlers all run exactly as they do for a real HTTP request. Headers from the MCP request are forwarded onto the call so auth interceptors keep working.

Opt individual routes out, or override a tool name:

```go
r.Delete("/internal", c.purge, nil).ExcludeFromMCP()

r.Get("/{id}", c.getOne).
    Summary("Get one").
    MCPTool("fetch_widget")
```

`MCPConfig` defaults:
- `Path` defaults to the API's common route prefix plus `/mcp`
- `Instructions` is optional guidance returned during `initialize`

## DB Connection Environment Variables (if DB is enabled)
```
DB_HOST
DB_PORT
DB_NAME
DB_USERNAME
DB_PASSWORD
```
