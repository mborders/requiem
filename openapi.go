package requiem

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const (
	defaultSpecPath = "/openapi.json"
	defaultDocsPath = "/docs"
)

type OpenAPIConfig struct {
	Title       string
	Version     string
	Description string
	Servers     []string
	SpecPath    string
	DocsPath    string
}

type Route struct {
	method      string
	path        string
	bodyType    reflect.Type
	summary     string
	description string
	tags        []string
	responses   map[int]responseSpec
	params      []paramSpec
	deprecated  bool
	excluded    bool
}

type responseSpec struct {
	typ         reflect.Type
	description string
}

type paramSpec struct {
	name        string
	in          string
	typ         string
	required    bool
	description string
}

func (rt *Route) Summary(s string) *Route {
	rt.summary = s
	return rt
}

func (rt *Route) Description(s string) *Route {
	rt.description = s
	return rt
}

func (rt *Route) Tags(t ...string) *Route {
	rt.tags = append(rt.tags, t...)
	return rt
}

func (rt *Route) Deprecated() *Route {
	rt.deprecated = true
	return rt
}

func (rt *Route) Returns(status int, v interface{}, description string) *Route {
	if rt.responses == nil {
		rt.responses = make(map[int]responseSpec)
	}
	var t reflect.Type
	if v != nil {
		t = reflect.TypeOf(v)
	}
	rt.responses[status] = responseSpec{typ: t, description: description}
	return rt
}

func (rt *Route) Query(name, typ string, required bool, description string) *Route {
	rt.params = append(rt.params, paramSpec{name: name, in: "query", typ: typ, required: required, description: description})
	return rt
}

func (rt *Route) Header(name, typ string, required bool, description string) *Route {
	rt.params = append(rt.params, paramSpec{name: name, in: "header", typ: typ, required: required, description: description})
	return rt
}

func (rt *Route) Param(name, typ, description string) *Route {
	rt.params = append(rt.params, paramSpec{name: name, in: "path", typ: typ, required: true, description: description})
	return rt
}

func (rt *Route) ExcludeFromSpec() *Route {
	rt.excluded = true
	return rt
}

type openapiController struct {
	cfg    OpenAPIConfig
	router *Router

	once sync.Once
	spec []byte
}

func (c *openapiController) Load(router *Router) {
	c.router = router

	specPath := c.cfg.SpecPath
	if specPath == "" {
		specPath = defaultSpecPath
	}
	docsPath := c.cfg.DocsPath
	if docsPath == "" {
		docsPath = defaultDocsPath
	}

	router.MuxRouter.HandleFunc(specPath, c.serveSpec).Methods(http.MethodGet)
	if docsPath != "-" {
		router.MuxRouter.HandleFunc(docsPath, c.serveDocs(specPath, docsPath)).Methods(http.MethodGet)
	}
}

func (c *openapiController) serveSpec(w http.ResponseWriter, r *http.Request) {
	c.once.Do(func() {
		c.spec = buildDoc(c.cfg, c.router.routes)
	})
	w.Header().Set("Content-Type", "application/json")
	w.Write(c.spec)
}

func (c *openapiController) serveDocs(specPath, docsPath string) http.HandlerFunc {
	title := c.cfg.Title
	if title == "" {
		title = "API"
	}
	return func(w http.ResponseWriter, r *http.Request) {
		base := strings.TrimSuffix(r.URL.Path, docsPath)
		specURL := base + specPath
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, swaggerUIHTML, title, specURL)
	}
}

type docBuilder struct {
	schemas map[string]map[string]interface{}
}

func newDocBuilder() *docBuilder {
	return &docBuilder{schemas: make(map[string]map[string]interface{})}
}

func buildDoc(cfg OpenAPIConfig, routes []*Route) []byte {
	db := newDocBuilder()

	paths := map[string]map[string]interface{}{}
	for _, rt := range routes {
		if rt.excluded {
			continue
		}
		openapiPath := stripPathRegex(rt.path)
		if _, ok := paths[openapiPath]; !ok {
			paths[openapiPath] = map[string]interface{}{}
		}
		paths[openapiPath][strings.ToLower(rt.method)] = db.buildOperation(rt)
	}

	info := map[string]interface{}{
		"title":   cfg.Title,
		"version": cfg.Version,
	}
	if cfg.Description != "" {
		info["description"] = cfg.Description
	}

	spec := map[string]interface{}{
		"openapi": "3.0.3",
		"info":    info,
		"paths":   paths,
	}

	if len(cfg.Servers) > 0 {
		servers := make([]map[string]interface{}, 0, len(cfg.Servers))
		for _, u := range cfg.Servers {
			servers = append(servers, map[string]interface{}{"url": u})
		}
		spec["servers"] = servers
	}

	if len(db.schemas) > 0 {
		spec["components"] = map[string]interface{}{
			"schemas": db.schemas,
		}
	}

	b, _ := json.MarshalIndent(spec, "", "  ")
	return b
}

func (db *docBuilder) buildOperation(rt *Route) map[string]interface{} {
	op := map[string]interface{}{}

	if rt.summary != "" {
		op["summary"] = rt.summary
	}
	if rt.description != "" {
		op["description"] = rt.description
	}
	if len(rt.tags) > 0 {
		op["tags"] = rt.tags
	}
	if rt.deprecated {
		op["deprecated"] = true
	}

	pathParams := extractPathParams(rt.path)
	parameters := []map[string]interface{}{}
	declaredPath := map[string]bool{}

	for _, p := range rt.params {
		if p.in == "path" {
			declaredPath[p.name] = true
		}
		parameters = append(parameters, paramObject(p))
	}
	for _, name := range pathParams {
		if declaredPath[name] {
			continue
		}
		parameters = append(parameters, paramObject(paramSpec{
			name:     name,
			in:       "path",
			typ:      "string",
			required: true,
		}))
	}

	if len(parameters) > 0 {
		op["parameters"] = parameters
	}

	if rt.bodyType != nil {
		op["requestBody"] = map[string]interface{}{
			"required": true,
			"content": map[string]interface{}{
				"application/json": map[string]interface{}{
					"schema": db.schemaFor(rt.bodyType),
				},
			},
		}
	}

	responses := map[string]interface{}{}
	if len(rt.responses) == 0 {
		responses["200"] = map[string]interface{}{
			"description": http.StatusText(http.StatusOK),
		}
	} else {
		codes := make([]int, 0, len(rt.responses))
		for c := range rt.responses {
			codes = append(codes, c)
		}
		sort.Ints(codes)
		for _, code := range codes {
			r := rt.responses[code]
			desc := r.description
			if desc == "" {
				desc = http.StatusText(code)
			}
			resp := map[string]interface{}{"description": desc}
			if r.typ != nil {
				resp["content"] = map[string]interface{}{
					"application/json": map[string]interface{}{
						"schema": db.schemaFor(r.typ),
					},
				}
			}
			responses[strconv.Itoa(code)] = resp
		}
	}
	op["responses"] = responses

	return op
}

func paramObject(p paramSpec) map[string]interface{} {
	typ := p.typ
	if typ == "" {
		typ = "string"
	}
	obj := map[string]interface{}{
		"name":     p.name,
		"in":       p.in,
		"required": p.required,
		"schema":   map[string]interface{}{"type": typ},
	}
	if p.description != "" {
		obj["description"] = p.description
	}
	return obj
}

var pathParamRegex = regexp.MustCompile(`\{([^:}]+)(?::[^}]+)?\}`)

func extractPathParams(path string) []string {
	matches := pathParamRegex.FindAllStringSubmatch(path, -1)
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		names = append(names, m[1])
	}
	return names
}

func stripPathRegex(path string) string {
	return pathParamRegex.ReplaceAllString(path, "{$1}")
}
