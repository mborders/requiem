package requiem

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"

	"github.com/gorilla/mux"
)

// HTTPContext provides utility functions for HTTP requests/responses
type HTTPContext struct {
	Response   http.ResponseWriter
	Request    *http.Request
	Body       interface{}
	attributes map[string]interface{}
}

// SendJSON converts the given interface into JSON and writes to the response.
func (ctx *HTTPContext) SendJSON(v interface{}) {
	SendJSON(ctx.Response, v)
}

// SendStatus writes the given HTTP status code into the response.
func (ctx *HTTPContext) SendStatus(s int) {
	SendStatus(ctx.Response, s)
}

// SendJSONWithStatus writes a JSON response body to the response, along with the specified status code.
func (ctx *HTTPContext) SendJSONWithStatus(v interface{}, s int) {
	SendJSONWithStatus(ctx.Response, v, s)
}

// GetParam obtains the given parameter key from the request parameters.
func (ctx *HTTPContext) GetParam(p string) string {
	return mux.Vars(ctx.Request)[p]
}

// GetQueryParam obtains the given query parameter from the request URL.
func (ctx *HTTPContext) GetQueryParam(p string) string {
	return ctx.Request.URL.Query().Get(p)
}

// GetAttribute returns the context-scoped value for the given key
func (ctx *HTTPContext) GetAttribute(key string) interface{} {
	return ctx.attributes[key]
}

// SetAttribute sets the context-scoped value for the given key
func (ctx *HTTPContext) SetAttribute(key string, attr interface{}) {
	ctx.attributes[key] = attr
}

// ReadJSON decodes the provided stream into the given interface.
func ReadJSON(r io.Reader, v interface{}) interface{} {
	t := reflect.TypeOf(v)
	o := reflect.New(t).Interface()
	json.NewDecoder(r).Decode(o)
	return o
}

// SendJSON converts the given interface into JSON and writes to the provided stream.
func SendJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Add("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// SendStatus writes the given HTTP status code into the provided response stream.
func SendStatus(w http.ResponseWriter, s int) {
	w.WriteHeader(s)
}

// SendJSONWithStatus writes a JSON response body to the provided stream, along with the specified status code.
func SendJSONWithStatus(w http.ResponseWriter, v interface{}, s int) {
	w.Header().Add("Content-Type", "application/json")
	json, _ := json.Marshal(v)
	w.WriteHeader(s)
	w.Write(json)
}

// HTTPInterceptor allows for pre-processing request handlers
// ex. an authentication interceptor could verify a user session
type HTTPInterceptor func(ctx HTTPContext) bool
