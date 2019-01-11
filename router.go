package restimator

import (
	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
)

// Router maintains routing paths for a single API
type Router struct {
	Router *mux.Router
}

// IHttpController represents an API that can be loaded into a router
type IHttpController interface {
	Load(router *mux.Router, db *gorm.DB)
}

// Load adds the given API's routes into the router
func (r *Router) Load(c IHttpController, db *gorm.DB) {
	c.Load(r.Router, db)
}

// PrintRoutes logs all of the router's registered paths
func (r *Router) PrintRoutes() {
	Logger.Info("====== API Routes =============================================")

	r.Router.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
		s, _ := route.GetMethods()
		if len(s) == 0 {
			return nil
		}

		t, err := route.GetPathTemplate()
		if err != nil {
			return err
		}

		Logger.Info("Mapped %6s => %s", s[0], t)

		return nil
	})

	Logger.Info("===============================================================")
}

// NewRouter initializes a new router starting at the given path
func NewRouter(path string) *Router {
	router := mux.NewRouter().PathPrefix(path).Subrouter()
	apiRouter := &Router{Router: router}
	return apiRouter
}
