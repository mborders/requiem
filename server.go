package requiem

import (
	"fmt"
	"net/http"

	"gorm.io/gorm"
)

const (
	defaultPort     = 8080
	defaultBasePath = "/api"
)

// Server represents a REST API server container
// Default port is 8080
// Default path is /api
type Server struct {
	Port               int
	BasePath           string
	ExitOnFatal        bool
	healthcheckEnabled bool
	openapiEnabled     bool
	mcpEnabled         bool
	db                 *gorm.DB
	controllers        []IHttpController
}

func (s *Server) UsePostgresDB(debugMode bool) {
	s.db = newPostgresDBConnection(debugMode)
}

func (s *Server) UseInMemoryDB(debugMode bool) {
	s.db = newInMemoryDBConnection(debugMode)
}

func (s *Server) UseHealthcheck() {
	if !s.healthcheckEnabled {
		s.controllers = append(s.controllers, HealthcheckController{})
		s.healthcheckEnabled = true
	}
}

func (s *Server) UseOpenAPI(cfg OpenAPIConfig) {
	if !s.openapiEnabled {
		s.controllers = append(s.controllers, &openapiController{cfg: cfg})
		s.openapiEnabled = true
	}
}

// GetOpenAPISpec returns the OpenAPI 3.0 spec for the server's routes as JSON
// bytes without starting an HTTP listener. Controllers are loaded into a
// throwaway router solely to register their routes. Returns nil if UseOpenAPI
// was never called.
func (s *Server) GetOpenAPISpec() []byte {
	if !s.openapiEnabled {
		return nil
	}
	r := newRouter(s.BasePath, s.db, s.controllers)
	for _, c := range s.controllers {
		if oc, ok := c.(*openapiController); ok {
			return buildDoc(oc.cfg, r.routes)
		}
	}
	return nil
}

// UseMCP enables an optional Model Context Protocol (MCP) endpoint that exposes
// the server's registered REST routes as MCP tools. It is purely additive: when
// called, an mcpController is appended to the server's controllers and serves a
// JSON-RPC 2.0 endpoint (default: the API's namespace + "/mcp"). Once enabled,
// every route is exposed by default; opt individual routes out with
// Route.ExcludeFromMCP.
func (s *Server) UseMCP(cfg MCPConfig) {
	if !s.mcpEnabled {
		s.controllers = append(s.controllers, &mcpController{cfg: cfg})
		s.mcpEnabled = true
	}
}

func (s *Server) AutoMigrate(models ...interface{}) {
	for idx := range models {
		s.db.AutoMigrate(models[idx])
	}
}

// Start initializes the API and starts running on the specified port
// Blocks on current thread
func (s *Server) Start() {
	if s.db != nil {
		sqlDB, _ := s.db.DB()
		defer sqlDB.Close()
	}

	// Create API router and load controllers
	r := newRouter(s.BasePath, s.db, s.controllers)
	r.printRoutes()

	// Create HTTP server using API router
	srv := &http.Server{
		Handler: r.MuxRouter,
		Addr:    fmt.Sprintf(":%d", s.Port),
	}

	Logger.Info("Starting server on port %d", s.Port)
	Logger.Fatal(srv.ListenAndServe().Error())
}

// NewServer creates a route-based REST API server instance
func NewServer(controllers ...IHttpController) *Server {
	s := Server{
		Port:        defaultPort,
		BasePath:    defaultBasePath,
		ExitOnFatal: true,
		controllers: controllers,
	}

	// Create logger
	InitLogger(s.ExitOnFatal)

	return &s
}
