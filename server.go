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
