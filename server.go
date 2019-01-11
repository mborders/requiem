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
	r := NewRouter("/api")

	for _, c := range s.Controllers {
		r.Load(c, db)
	}

	r.PrintRoutes()

	// Create HTTP server using API router
	srv := &http.Server{
		Handler: r.Router,
		Addr:    fmt.Sprintf(":%s", s.Port),
	}

	Logger.Info("Starting server on port %s", s.Port)
	Logger.Fatal(srv.ListenAndServe().Error())
}
