package requiem

import (
	"fmt"
	"net/http"

	"github.com/jinzhu/gorm"
)

// Server represents a REST API server container
type Server struct {
	port        int
	controllers []IHttpController
	enableDB    bool
}

// Start initializes the API and starts running on the specified port
func (s *Server) Start() {
	// Create logger
	InitLogger()

	var db *gorm.DB
	if s.enableDB {
		db = newDBConnection()
		defer db.Close()
	}

	// Create API router and load controllers
	r := newRouter("/api", db, s.controllers)
	r.printRoutes()

	// Create HTTP server using API router
	srv := &http.Server{
		Handler: r.MuxRouter,
		Addr:    fmt.Sprintf(":%d", s.port),
	}

	Logger.Info("Starting server on port %d", s.port)
	Logger.Fatal(srv.ListenAndServe().Error())
}

// NewServer creates a route-based REST API server instance
func NewServer(port int, enableDB bool, controllers ...IHttpController) *Server {
	return &Server{
		port:        port,
		enableDB:    enableDB,
		controllers: controllers,
	}
}
