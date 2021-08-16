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
	Port        int
	BasePath    string
	ExitOnFatal bool
	db          *gorm.DB
	controllers []IHttpController
}

func (s *Server) UsePostgresDB() {
	s.db = newPostgresDBConnection()
}

func (s *Server) UseInMemoryDB() {
	s.db = newInMemoryDBConnection()
}

func (s *Server) AutoMigrate(models ...interface{}) {
	for idx := range models {
		s.db.AutoMigrate(models[idx])
	}
}

// Start initializes the API and starts running on the specified port
// Blocks on current thread
func (s *Server) Start() {
	// Create logger
	InitLogger(s.ExitOnFatal)

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
	return &Server{
		Port:        defaultPort,
		BasePath:    defaultBasePath,
		ExitOnFatal: true,
		controllers: controllers,
	}
}
