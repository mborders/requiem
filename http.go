package restimator

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"
)

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
