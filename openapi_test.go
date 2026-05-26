package requiem

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type Widget struct {
	ID        string    `json:"id" validate:"required"`
	Name      string    `json:"name" validate:"required,min=1,max=100"`
	Quantity  int       `json:"quantity" validate:"min=0,max=999"`
	CreatedAt time.Time `json:"created_at"`
	internal  string
	Secret    string `json:"-"`
}

type CreateWidget struct {
	Name     string `json:"name" validate:"required"`
	Quantity int    `json:"quantity,omitempty"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type DocController struct{}

func (c DocController) Load(router *Router) {
	r := router.NewRestRouter("/widgets")

	r.Get("/", func(ctx HTTPContext) {
		ctx.SendJSON([]Widget{})
	}).
		Summary("List widgets").
		Tags("widgets").
		Query("limit", "integer", false, "Max items").
		Returns(200, []Widget{}, "Array of widgets")

	r.Get("/{id}", func(ctx HTTPContext) {
		ctx.SendJSON(Widget{})
	}).
		Summary("Get widget").
		Tags("widgets").
		Param("id", "string", "Widget identifier").
		Returns(200, Widget{}, "The widget").
		Returns(404, ErrorBody{}, "Not found")

	r.Post("/", func(ctx HTTPContext) {
		ctx.SendStatus(http.StatusCreated)
	}, CreateWidget{}).
		Summary("Create widget").
		Tags("widgets").
		Returns(201, Widget{}, "Created").
		Returns(400, ErrorBody{}, "Bad request")

	r.Delete("/{id}", func(ctx HTTPContext) {
		ctx.SendStatus(http.StatusNoContent)
	}, nil).
		Summary("Delete widget").
		Tags("widgets").
		Returns(204, nil, "Deleted").
		Deprecated()
}

func TestOpenAPI_Spec(t *testing.T) {
	s := NewServer(DocController{})
	s.Port = 8090
	s.ExitOnFatal = false
	s.UseOpenAPI(OpenAPIConfig{
		Title:       "Widget API",
		Version:     "1.2.3",
		Description: "Test API",
		Servers:     []string{"/api"},
	})
	go s.Start()

	<-time.NewTimer(time.Millisecond * 200).C

	res, err := http.Get("http://localhost:8090/api/openapi.json")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, "application/json", res.Header.Get("Content-Type"))

	body, _ := io.ReadAll(res.Body)
	res.Body.Close()

	var spec map[string]interface{}
	assert.NoError(t, json.Unmarshal(body, &spec))

	assert.Equal(t, "3.0.3", spec["openapi"])

	info := spec["info"].(map[string]interface{})
	assert.Equal(t, "Widget API", info["title"])
	assert.Equal(t, "1.2.3", info["version"])
	assert.Equal(t, "Test API", info["description"])

	servers := spec["servers"].([]interface{})
	assert.Len(t, servers, 1)
	assert.Equal(t, "/api", servers[0].(map[string]interface{})["url"])

	paths := spec["paths"].(map[string]interface{})
	assert.Contains(t, paths, "/widgets/")
	assert.Contains(t, paths, "/widgets/{id}")

	list := paths["/widgets/"].(map[string]interface{})
	getOp := list["get"].(map[string]interface{})
	assert.Equal(t, "List widgets", getOp["summary"])
	assert.Contains(t, getOp["tags"], "widgets")

	getParams := getOp["parameters"].([]interface{})
	assert.Len(t, getParams, 1)
	limit := getParams[0].(map[string]interface{})
	assert.Equal(t, "limit", limit["name"])
	assert.Equal(t, "query", limit["in"])

	postOp := list["post"].(map[string]interface{})
	body201 := postOp["responses"].(map[string]interface{})["201"].(map[string]interface{})
	assert.Equal(t, "Created", body201["description"])
	requestBody := postOp["requestBody"].(map[string]interface{})
	assert.Equal(t, true, requestBody["required"])

	one := paths["/widgets/{id}"].(map[string]interface{})
	delOp := one["delete"].(map[string]interface{})
	assert.Equal(t, true, delOp["deprecated"])
	delParams := delOp["parameters"].([]interface{})
	assert.Len(t, delParams, 1)
	idParam := delParams[0].(map[string]interface{})
	assert.Equal(t, "id", idParam["name"])
	assert.Equal(t, "path", idParam["in"])
	assert.Equal(t, true, idParam["required"])

	components := spec["components"].(map[string]interface{})
	schemas := components["schemas"].(map[string]interface{})
	assert.Contains(t, schemas, "Widget")
	assert.Contains(t, schemas, "CreateWidget")
	assert.Contains(t, schemas, "ErrorBody")

	widget := schemas["Widget"].(map[string]interface{})
	assert.Equal(t, "object", widget["type"])
	props := widget["properties"].(map[string]interface{})
	assert.Contains(t, props, "id")
	assert.Contains(t, props, "name")
	assert.Contains(t, props, "quantity")
	assert.Contains(t, props, "created_at")
	assert.NotContains(t, props, "internal")
	assert.NotContains(t, props, "Secret")

	createdAt := props["created_at"].(map[string]interface{})
	assert.Equal(t, "string", createdAt["type"])
	assert.Equal(t, "date-time", createdAt["format"])

	name := props["name"].(map[string]interface{})
	assert.Equal(t, "string", name["type"])
	assert.EqualValues(t, 1, name["minLength"])
	assert.EqualValues(t, 100, name["maxLength"])

	quantity := props["quantity"].(map[string]interface{})
	assert.Equal(t, "integer", quantity["type"])
	assert.EqualValues(t, 0, quantity["minimum"])
	assert.EqualValues(t, 999, quantity["maximum"])

	required := widget["required"].([]interface{})
	assert.Contains(t, required, "id")
	assert.Contains(t, required, "name")
	assert.NotContains(t, required, "quantity")

	create := schemas["CreateWidget"].(map[string]interface{})
	createReq := create["required"].([]interface{})
	assert.Contains(t, createReq, "name")
	assert.NotContains(t, createReq, "quantity")
}

func TestOpenAPI_DocsPage(t *testing.T) {
	s := NewServer(DocController{})
	s.Port = 8091
	s.ExitOnFatal = false
	s.UseOpenAPI(OpenAPIConfig{
		Title:   "Widget API",
		Version: "1.0.0",
	})
	go s.Start()

	<-time.NewTimer(time.Millisecond * 200).C

	res, err := http.Get("http://localhost:8091/api/docs")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Contains(t, res.Header.Get("Content-Type"), "text/html")

	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	html := string(body)
	assert.Contains(t, html, "swagger-ui")
	assert.Contains(t, html, "/api/openapi.json")
	assert.Contains(t, html, "Widget API")
}

func TestOpenAPI_PathRegexStripped(t *testing.T) {
	router := newRouter("/api", nil, []IHttpController{&regexController{}})
	spec := string(buildDoc(OpenAPIConfig{Title: "T", Version: "1"}, router.routes))

	assert.Contains(t, spec, "/thing/{id}")
	assert.NotContains(t, spec, "[0-9]+")
}

type regexController struct{}

func (c *regexController) Load(router *Router) {
	r := router.NewRestRouter("/thing")
	r.Get("/{id:[0-9]+}", func(ctx HTTPContext) {
		ctx.SendStatus(http.StatusOK)
	}).Summary("Get thing")
}

func TestOpenAPI_ExcludesItsOwnRoutes(t *testing.T) {
	router := newRouter("/api", nil, []IHttpController{
		DocController{},
		&openapiController{cfg: OpenAPIConfig{Title: "T", Version: "1"}},
	})

	var spec map[string]interface{}
	json.Unmarshal(buildDoc(OpenAPIConfig{Title: "T", Version: "1"}, router.routes), &spec)

	paths := spec["paths"].(map[string]interface{})
	assert.NotContains(t, paths, "/openapi.json")
	assert.NotContains(t, paths, "/docs")
}

type embedded struct {
	Inner string `json:"inner"`
}

type withNestedStruct struct {
	Name  string   `json:"name" validate:"required,min=1,max=50"`
	Child embedded `json:"child" validate:"required,min=1"`
}

type nestedController struct{}

func (c nestedController) Load(router *Router) {
	r := router.NewRestRouter("/nested")
	r.Post("/", func(ctx HTTPContext) {
		ctx.SendStatus(http.StatusCreated)
	}, withNestedStruct{}).Returns(201, withNestedStruct{}, "OK")
}

func TestOpenAPI_ValidateTagNotAppliedToRef(t *testing.T) {
	router := newRouter("/api", nil, []IHttpController{nestedController{}})

	var spec map[string]interface{}
	json.Unmarshal(buildDoc(OpenAPIConfig{Title: "T", Version: "1"}, router.routes), &spec)

	schemas := spec["components"].(map[string]interface{})["schemas"].(map[string]interface{})
	props := schemas["withNestedStruct"].(map[string]interface{})["properties"].(map[string]interface{})

	child := props["child"].(map[string]interface{})
	assert.Contains(t, child, "$ref")
	assert.NotContains(t, child, "minLength")
	assert.NotContains(t, child, "minimum")
	assert.NotContains(t, child, "minItems")
	assert.Len(t, child, 1)

	name := props["name"].(map[string]interface{})
	assert.EqualValues(t, 1, name["minLength"])
	assert.EqualValues(t, 50, name["maxLength"])
}

func TestOpenAPI_RouteRegistry(t *testing.T) {
	router := newRouter("/api", nil, []IHttpController{DocController{}})

	assert.Greater(t, len(router.routes), 0)

	var got []string
	for _, rt := range router.routes {
		got = append(got, rt.method+" "+rt.path)
	}
	assert.Contains(t, got, "GET /widgets/")
	assert.Contains(t, got, "GET /widgets/{id}")
	assert.Contains(t, got, "POST /widgets/")
	assert.Contains(t, got, "DELETE /widgets/{id}")
}

type nonConflictController struct{}

func (c nonConflictController) Load(router *Router) {
	r := router.NewRestRouter("/things")
	r.Get("/list", func(ctx HTTPContext) { ctx.SendStatus(http.StatusOK) })
	r.Get("/detail/{id}", func(ctx HTTPContext) { ctx.SendStatus(http.StatusOK) })
}

type multiPrefixController struct{}

func (c multiPrefixController) Load(router *Router) {
	a := router.NewRestRouter("/widgets")
	a.Get("/", func(ctx HTTPContext) { ctx.SendStatus(http.StatusOK) })
	b := router.NewRestRouter("/users")
	b.Get("/", func(ctx HTTPContext) { ctx.SendStatus(http.StatusOK) })
}

func TestCommonRoutePrefix_SharedPrefixNoConflict(t *testing.T) {
	router := newRouter("/api", nil, []IHttpController{nonConflictController{}})
	assert.Equal(t, "/things", commonRoutePrefix(router.routes))
}

type singleRouteController struct{}

func (c singleRouteController) Load(router *Router) {
	r := router.NewRestRouter("/analytics")
	r.Post("/events", func(ctx HTTPContext) { ctx.SendStatus(http.StatusOK) }, nil)
}

func TestCommonRoutePrefix_SingleRouteUsesRouterPrefix(t *testing.T) {
	router := newRouter("/api", nil, []IHttpController{singleRouteController{}})
	assert.Equal(t, "/analytics", commonRoutePrefix(router.routes))
}

func TestCommonRoutePrefix_FallsBackOnWildcardConflict(t *testing.T) {
	router := newRouter("/api", nil, []IHttpController{DocController{}})
	assert.Equal(t, "", commonRoutePrefix(router.routes))
}

func TestCommonRoutePrefix_NoSharedPrefix(t *testing.T) {
	router := newRouter("/api", nil, []IHttpController{multiPrefixController{}})
	assert.Equal(t, "", commonRoutePrefix(router.routes))
}

func TestCommonRoutePrefix_IgnoresExcludedRoutes(t *testing.T) {
	router := newRouter("/api", nil, []IHttpController{
		HealthcheckController{},
		nonConflictController{},
	})
	assert.Equal(t, "/things", commonRoutePrefix(router.routes))
}

func TestOpenAPI_AutoPrefixesSpecAndDocs(t *testing.T) {
	s := NewServer(nonConflictController{})
	s.Port = 8094
	s.ExitOnFatal = false
	s.UseHealthcheck()
	s.UseOpenAPI(OpenAPIConfig{Title: "T", Version: "1"})
	go s.Start()

	<-time.NewTimer(time.Millisecond * 200).C

	res, err := http.Get("http://localhost:8094/api/things/openapi.json")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	res.Body.Close()

	res, err = http.Get("http://localhost:8094/api/things/docs")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	res.Body.Close()
}

func TestOpenAPI_ExplicitSpecPathOverridesAutoPrefix(t *testing.T) {
	s := NewServer(nonConflictController{})
	s.Port = 8095
	s.ExitOnFatal = false
	s.UseOpenAPI(OpenAPIConfig{
		Title:    "T",
		Version:  "1",
		SpecPath: "/openapi.json",
		DocsPath: "/docs",
	})
	go s.Start()

	<-time.NewTimer(time.Millisecond * 200).C

	res, err := http.Get("http://localhost:8095/api/openapi.json")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	res.Body.Close()
}

func TestOpenAPI_DiscardedReturnValueStillRegisters(t *testing.T) {
	router := newRouter("/api", nil, []IHttpController{simpleController{}})

	count := 0
	for _, rt := range router.routes {
		if strings.HasPrefix(rt.path, "/simple") {
			count++
		}
	}
	assert.Equal(t, 1, count)
}

type simpleController struct{}

func (c simpleController) Load(router *Router) {
	r := router.NewRestRouter("/simple")
	r.Get("/", func(ctx HTTPContext) {
		ctx.SendStatus(http.StatusOK)
	})
}

type customSchemaScalar string

func (c customSchemaScalar) OpenAPISchema() map[string]interface{} {
	return map[string]interface{}{"type": "string", "format": "date"}
}

type customSchemaContainer struct {
	When customSchemaScalar `json:"when" validate:"required"`
}

type customSchemaController struct{}

func (c customSchemaController) Load(router *Router) {
	r := router.NewRestRouter("/custom")
	r.Get("/", func(ctx HTTPContext) {
		ctx.SendJSON(customSchemaContainer{})
	}).Returns(200, customSchemaContainer{}, "OK")
}

func TestOpenAPI_OpenAPISchemaProvider(t *testing.T) {
	router := newRouter("/api", nil, []IHttpController{customSchemaController{}})

	var spec map[string]interface{}
	json.Unmarshal(buildDoc(OpenAPIConfig{Title: "T", Version: "1"}, router.routes), &spec)

	schemas := spec["components"].(map[string]interface{})["schemas"].(map[string]interface{})
	props := schemas["customSchemaContainer"].(map[string]interface{})["properties"].(map[string]interface{})
	when := props["when"].(map[string]interface{})

	assert.Equal(t, "string", when["type"])
	assert.Equal(t, "date", when["format"])
	// The named scalar type must not leak into components.schemas as a $ref —
	// OpenAPISchema is the source of truth for its representation.
	_, hasScalarSchema := schemas["customSchemaScalar"]
	assert.False(t, hasScalarSchema)
}

type EmbeddedSummary struct {
	ID    string `json:"id" validate:"required"`
	Title string `json:"title" validate:"required"`
}

type WithEmbedded struct {
	EmbeddedSummary
	Children []string `json:"children" validate:"required"`
}

type embeddedController struct{}

func (c embeddedController) Load(router *Router) {
	r := router.NewRestRouter("/embedded")
	r.Get("/", func(ctx HTTPContext) {
		ctx.SendJSON(WithEmbedded{})
	}).Returns(200, WithEmbedded{}, "OK")
}

func TestOpenAPI_FlattensEmbeddedStructs(t *testing.T) {
	router := newRouter("/api", nil, []IHttpController{embeddedController{}})

	var spec map[string]interface{}
	json.Unmarshal(buildDoc(OpenAPIConfig{Title: "T", Version: "1"}, router.routes), &spec)

	schemas := spec["components"].(map[string]interface{})["schemas"].(map[string]interface{})
	schema := schemas["WithEmbedded"].(map[string]interface{})
	props := schema["properties"].(map[string]interface{})

	// Embedded struct's fields surface on the parent, not as a nested object.
	assert.Contains(t, props, "id")
	assert.Contains(t, props, "title")
	assert.Contains(t, props, "children")
	assert.NotContains(t, props, "EmbeddedSummary")

	required := schema["required"].([]interface{})
	assert.Contains(t, required, "id")
	assert.Contains(t, required, "title")
	assert.Contains(t, required, "children")
}

type TaggedEmbedWrapper struct {
	EmbeddedSummary `json:"summary"`
	Children        []string `json:"children"`
}

type taggedEmbedController struct{}

func (c taggedEmbedController) Load(router *Router) {
	r := router.NewRestRouter("/tagged")
	r.Get("/", func(ctx HTTPContext) {
		ctx.SendJSON(TaggedEmbedWrapper{})
	}).Returns(200, TaggedEmbedWrapper{}, "OK")
}

func TestOpenAPI_EmbeddedWithExplicitJSONTagStaysNested(t *testing.T) {
	router := newRouter("/api", nil, []IHttpController{taggedEmbedController{}})

	var spec map[string]interface{}
	json.Unmarshal(buildDoc(OpenAPIConfig{Title: "T", Version: "1"}, router.routes), &spec)

	schemas := spec["components"].(map[string]interface{})["schemas"].(map[string]interface{})
	props := schemas["TaggedEmbedWrapper"].(map[string]interface{})["properties"].(map[string]interface{})

	// Explicit json tag on the embedded field keeps it as a nested property,
	// matching encoding/json's behavior.
	assert.Contains(t, props, "summary")
	assert.NotContains(t, props, "id")
	assert.NotContains(t, props, "title")
}
