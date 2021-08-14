package requiem

import (
	"net/http"

	"github.com/gorilla/mux"
	validator "gopkg.in/go-playground/validator.v9"
	"gorm.io/gorm"
)

// Router maintains routing paths for a single REST API
type Router struct {
	MuxRouter *mux.Router
	DB        *gorm.DB
}

// IHttpController represents a REST API that can be loaded into a router
type IHttpController interface {
	Load(router *Router)
}

// RestRouter represents a router for a specific REST API
type RestRouter struct {
	router *mux.Router
	DB     *gorm.DB
}

// Load adds all of the given REST controller routes into the router
func (r *Router) load(controllers []IHttpController) {
	for _, c := range controllers {
		c.Load(r)
	}
}

// PrintRoutes logs all of the router's registered paths
func (r *Router) printRoutes() {
	Logger.Info("====== Routes =============================================")

	r.MuxRouter.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
		s, _ := route.GetMethods()
		if len(s) == 0 {
			return nil
		}

		t, _ := route.GetPathTemplate()
		Logger.Info("Mapped %6s => %s", s[0], t)

		return nil
	})

	Logger.Info("===============================================================")
}

// newRouter initializes a new router starting at the given path
func newRouter(path string, db *gorm.DB, controllers []IHttpController) *Router {
	mr := mux.NewRouter().PathPrefix(path).Subrouter()
	r := &Router{MuxRouter: mr, DB: db}
	r.load(controllers)
	return r
}

// HandleFunc wraps the router HandleFunc to inject an HTTPContext for use
// by subsequent handlers.
func (r *RestRouter) handleFunc(method string, path string, handle func(HTTPContext), interceptors ...HTTPInterceptor) {
	r.router.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		ctx := HTTPContext{Request: r, Response: w, attributes: make(map[string]interface{})}

		if processInterceptors(interceptors, ctx) {
			handle(ctx)
		}
	}).Methods(method)
}

func (r *RestRouter) handleFuncBody(method string, path string, handle func(HTTPContext), v interface{}, interceptors ...HTTPInterceptor) {
	if v == nil {
		Logger.Fatal("[%s] %s => Body interface cannot be nil", method, path)
	}

	r.router.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		b := ReadJSON(r.Body, v)

		if validator.New().Struct(b) != nil {
			SendStatus(w, http.StatusBadRequest)
			return
		}

		ctx := HTTPContext{Request: r, Response: w, Body: b, attributes: make(map[string]interface{})}

		if processInterceptors(interceptors, ctx) {
			handle(ctx)
		}
	}).Methods(method)
}

// NewRestRouter initializes a new REST router on at the given path
func (r *Router) NewRestRouter(path string) *RestRouter {
	return &RestRouter{router: r.MuxRouter.PathPrefix(path).Subrouter(), DB: r.DB}
}

// Get handles GET HTTP requests for the given path
func (r *RestRouter) Get(path string, handle func(HTTPContext), interceptors ...HTTPInterceptor) {
	r.handleFunc(http.MethodGet, path, handle, interceptors...)
}

// Post handles POST HTTP requests for the given path
func (r *RestRouter) Post(path string, handle func(HTTPContext), v interface{}, interceptors ...HTTPInterceptor) {
	if v == nil {
		r.handleFunc(http.MethodPost, path, handle, interceptors...)
	} else {
		r.handleFuncBody(http.MethodPost, path, handle, v, interceptors...)
	}
}

// Put handles PUT HTTP requests for the given path
func (r *RestRouter) Put(path string, handle func(HTTPContext), v interface{}, interceptors ...HTTPInterceptor) {
	r.handleFuncBody(http.MethodPut, path, handle, v, interceptors...)
}

// Delete handles DELETE HTTP requests for the given path
func (r *RestRouter) Delete(path string, handle func(HTTPContext), v interface{}, interceptors ...HTTPInterceptor) {
	if v == nil {
		r.handleFunc(http.MethodDelete, path, handle, interceptors...)
	} else {
		r.handleFuncBody(http.MethodDelete, path, handle, v, interceptors...)
	}
}

func processInterceptors(interceptors []HTTPInterceptor, ctx HTTPContext) bool {
	for idx := range interceptors {
		i := interceptors[idx]

		if !i(ctx) {
			return false
		}
	}

	return true
}
