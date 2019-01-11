package restimator

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
)

// Router maintains routing paths for a single API
type Router struct {
	MuxRouter *mux.Router
	DB        *gorm.DB
}

// IHttpController represents an API that can be loaded into a router
type IHttpController interface {
	Load(router *Router)
}

// APIRouter represents a router for a specific API
type APIRouter struct {
	router *mux.Router
}

// Load adds all of the given API controller routes into the router
func (r *Router) load(controllers []IHttpController) {
	for _, c := range controllers {
		c.Load(r)
	}
}

// PrintRoutes logs all of the router's registered paths
func (r *Router) PrintRoutes() {
	Logger.Info("====== API Routes =============================================")

	r.MuxRouter.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
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
func NewRouter(path string, db *gorm.DB, controllers []IHttpController) *Router {
	mr := mux.NewRouter().PathPrefix(path).Subrouter()
	r := &Router{MuxRouter: mr, DB: db}
	r.load(controllers)
	return r
}

// NewAPIRouter initializes a new API router on at the given path
func (r *Router) NewAPIRouter(path string) *APIRouter {
	return &APIRouter{router: r.MuxRouter.PathPrefix(path).Subrouter()}
}

// Get creates a GET router with the given handler
func (ar *APIRouter) Get(path string, handler func(w http.ResponseWriter, r *http.Request)) {
	ar.router.HandleFunc(path, handler).Methods(http.MethodGet)
}

// Post creates a POST router with the given handler
func (ar *APIRouter) Post(path string, handler func(w http.ResponseWriter, r *http.Request)) {
	ar.router.HandleFunc(path, handler).Methods(http.MethodPost)
}

// Put creates a PUT router with the given handler
func (ar *APIRouter) Put(path string, handler func(w http.ResponseWriter, r *http.Request)) {
	ar.router.HandleFunc(path, handler).Methods(http.MethodPut)
}

// Delete creates a DELETE router with the given handler
func (ar *APIRouter) Delete(path string, handler func(w http.ResponseWriter, r *http.Request)) {
	ar.router.HandleFunc(path, handler).Methods(http.MethodDelete)
}
