package restimator

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"

	"github.com/gorilla/mux"
)

// HTTPContext provides utility functions for HTTP requests/responses
type HTTPContext struct {
	Response http.ResponseWriter
	Request  *http.Request
	Body     interface{}
}

// ReadJSON decodes the request body into the given interface.
func (ctx *HTTPContext) ReadJSON(v interface{}) interface{} {
	return ReadJSON(ctx.Request.Body, v)
}

// SendJSON converts the given interface into JSON and writes to the response.
func (ctx *HTTPContext) SendJSON(v interface{}) {
	SendJSON(ctx.Response, v)
}

// SendStatus writes the given HTTP status code into the response.
func (ctx *HTTPContext) SendStatus(s int) {
	SendStatus(ctx.Response, s)
}

// GetParam obtains the given parameter key from the request parameters.
func (ctx *HTTPContext) GetParam(p string) string {
	return mux.Vars(ctx.Request)[p]
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
