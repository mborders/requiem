package restimator

import (
	"fmt"
	"net/http"

	"github.com/jinzhu/gorm"
)

// Server represents a REST API server container
type Server struct {
	Port        string
	Controllers []IHttpController
	EnableDB    bool
}

// Start initializes the API and starts running on the specified port
func (s *Server) Start() {
	// Create logger
	InitLogger()

	var db *gorm.DB
	if s.EnableDB {
		db = NewDBConnection()
		defer db.Close()
	}

	// Create API router and load controllers
	r := NewRouter("/api", db, s.Controllers)
	r.PrintRoutes()

	// Create HTTP server using API router
	srv := &http.Server{
		Handler: r.MuxRouter,
		Addr:    fmt.Sprintf(":%s", s.Port),
	}

	Logger.Info("Starting server on port %s", s.Port)
	Logger.Fatal(srv.ListenAndServe().Error())
}
