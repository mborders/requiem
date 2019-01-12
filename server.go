package requiem

import (
	"fmt"
	"net/http"

	"github.com/jinzhu/gorm"
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
	controllers []IHttpController
	EnableDB    bool
}

// Start initializes the API and starts running on the specified port
// Blocks on current thread
func (s *Server) Start() {
	// Create logger
	InitLogger()

	var db *gorm.DB
	if s.EnableDB {
		db = newDBConnection()
		defer db.Close()
	}

	// Create API router and load controllers
	r := newRouter(s.BasePath, db, s.controllers)
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
		EnableDB:    false,
		controllers: controllers,
	}
}
