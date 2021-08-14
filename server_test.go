package requiem

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type TestController struct {
}

type TestRequest struct {
	Message string `validate:"required"`
}

func (c TestController) Load(router *Router) {
	r := router.NewRestRouter("/test")
	r.Get("/get", func(ctx HTTPContext) {
		ctx.SendStatus(http.StatusNoContent)
	})

	r.Get("/param/{id:[0-9]+}", func(ctx HTTPContext) {
		p := ctx.GetParam("id")
		ctx.SendJSON(TestRequest{Message: p})
	})

	r.Post("/post", func(ctx HTTPContext) {
		req := ctx.Body.(*TestRequest)
		ctx.SendJSON(req)
	}, TestRequest{})

	r.Post("/post_no_body", func(ctx HTTPContext) {
		ctx.SendStatus(http.StatusAccepted)
	}, nil)

	r.Put("/put", func(ctx HTTPContext) {
		req := ctx.Body.(*TestRequest)
		ctx.SendJSON(req)
	}, TestRequest{})

	r.Put("/put", func(ctx HTTPContext) {
		req := ctx.Body.(*TestRequest)
		ctx.SendJSON(req)
	}, TestRequest{})

	r.Delete("/delete", func(ctx HTTPContext) {
		req := ctx.Body.(*TestRequest)
		ctx.SendJSON(req)
	}, TestRequest{})

	r.Delete("/delete_no_body", func(ctx HTTPContext) {
		ctx.SendStatus(http.StatusAccepted)
	}, nil)

	r.Get("/good_interceptor", func(ctx HTTPContext) {
		ctx.SendStatus(http.StatusOK)
	}, func(ctx HTTPContext) bool {
		return true
	})

	r.Get("/bad_interceptor", func(ctx HTTPContext) {
		ctx.SendStatus(http.StatusOK)
	}, func(ctx HTTPContext) bool {
		ctx.SendStatus(http.StatusUnauthorized)
		return false
	})

	r.Get("/attr_interceptor", func(ctx HTTPContext) {
		ctx.SendJSON(ctx.GetAttribute("SomeAttr"))
	}, func(ctx HTTPContext) bool {
		r := TestRequest{Message: "This is an attribute"}
		ctx.SetAttribute("SomeAttr", r)
		return true
	})
}

type InvalidController struct {
}

func (c InvalidController) Load(router *Router) {
	r := router.NewRestRouter("/invalid")
	r.Put("/put_invalid", func(ctx HTTPContext) {
		req := ctx.Body.(*TestRequest)
		ctx.SendJSON(req)
	}, nil)
}

func TestNewServer(t *testing.T) {
	s := NewServer(TestController{})
	go s.Start()

	timer := time.NewTimer(time.Millisecond * 100)
	<-timer.C

	assertGet(t)
	assertParam(t)
	assertPost(t)
	assertPostBadBody(t)
	assertPostNoBody(t)
	assertPut(t)
	assertDelete(t)
	assertDeleteNoBody(t)
	assertGoodInterceptor(t)
	assertBadInterceptor(t)
	assertAttributeInterceptor(t)
}

func TestNewServer_EnableDB(t *testing.T) {
	s := NewServer()
	s.Port = 8081
	s.EnableDB = true
	s.ExitOnFatal = false
	go s.Start()

	timer := time.NewTimer(time.Millisecond * 100)
	<-timer.C
}

func TestNewServer_InvalidController(t *testing.T) {
	s := NewServer(InvalidController{})
	s.Port = 8082
	s.ExitOnFatal = false
	go s.Start()

	timer := time.NewTimer(time.Millisecond * 100)
	<-timer.C
}

func assertGet(t *testing.T) {
	// Verify endpoint get
	res, _ := http.Get("http://localhost:8080/api/test/get")
	assert.Equal(t, http.StatusNoContent, res.StatusCode, "GET should return 204 status")
}

func assertParam(t *testing.T) {
	// Verify endpoint param
	id := "123"
	var result TestRequest
	res, _ := http.Get(fmt.Sprintf("http://localhost:8080/api/test/param/%s", id))
	json.NewDecoder(res.Body).Decode(&result)
	assert.Equal(t, id, result.Message, "Path param should have expected value")
}

func assertPost(t *testing.T) {
	// Verify endpoint post
	m := "HelloPost"
	b, _ := json.Marshal(TestRequest{Message: m})
	res, _ := http.Post("http://localhost:8080/api/test/post", "application/json", bytes.NewReader(b))

	var result TestRequest
	json.NewDecoder(res.Body).Decode(&result)
	assert.Equal(t, m, result.Message, "POST should have expected body")
}

func assertPostBadBody(t *testing.T) {
	// Verify endpoint post
	b, _ := json.Marshal(TestRequest{Message: ""})
	res, _ := http.Post("http://localhost:8080/api/test/post", "application/json", bytes.NewReader(b))

	var result TestRequest
	json.NewDecoder(res.Body).Decode(&result)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode, "POST with invalid body should have bad request status")
}

func assertPostNoBody(t *testing.T) {
	// Verify endpoint post_no_body
	req, _ := http.NewRequest("POST", "http://localhost:8080/api/test/post_no_body", nil)

	client := &http.Client{}
	res, _ := client.Do(req)
	assert.Equal(t, http.StatusAccepted, res.StatusCode, "POST no body should have expected status")
}

func assertPut(t *testing.T) {
	// Verify endpoint put
	m := "HelloPut"
	b, _ := json.Marshal(TestRequest{Message: m})
	req, _ := http.NewRequest("PUT", "http://localhost:8080/api/test/put", bytes.NewReader(b))
	req.Header.Add("Content-Type", "application/json")

	client := &http.Client{}
	res, _ := client.Do(req)

	var result TestRequest
	json.NewDecoder(res.Body).Decode(&result)
	assert.Equal(t, m, result.Message, "PUT should have expected body")
}

func assertDelete(t *testing.T) {
	// Verify endpoint delete
	m := "HelloDelete"
	b, _ := json.Marshal(TestRequest{Message: m})
	req, _ := http.NewRequest("DELETE", "http://localhost:8080/api/test/delete", bytes.NewReader(b))
	req.Header.Add("Content-Type", "application/json")

	client := &http.Client{}
	res, _ := client.Do(req)

	var result TestRequest
	json.NewDecoder(res.Body).Decode(&result)
	assert.Equal(t, m, result.Message, "DELETE should have expected body")
}

func assertDeleteNoBody(t *testing.T) {
	// Verify endpoint delete_no_body
	req, _ := http.NewRequest("DELETE", "http://localhost:8080/api/test/delete_no_body", nil)

	client := &http.Client{}
	res, _ := client.Do(req)
	assert.Equal(t, http.StatusAccepted, res.StatusCode, "DELETE no body should have expected status")
}

func assertGoodInterceptor(t *testing.T) {
	// Verify endpoint good_interceptor
	res, _ := http.Get("http://localhost:8080/api/test/good_interceptor")
	assert.Equal(t, http.StatusOK, res.StatusCode, "Good interceptor should return 200 status")
}

func assertBadInterceptor(t *testing.T) {
	// Verify endpoint bad_interceptor
	res, _ := http.Get("http://localhost:8080/api/test/bad_interceptor")
	assert.Equal(t, http.StatusUnauthorized, res.StatusCode, "Bad interceptor should return 401 status")
}

func assertAttributeInterceptor(t *testing.T) {
	// Verify endpoint attr_interceptor
	res, _ := http.Get("http://localhost:8080/api/test/attr_interceptor")

	var result TestRequest
	json.NewDecoder(res.Body).Decode(&result)
	assert.Equal(t, "This is an attribute", result.Message, "Attribute interceptor should return expected attribute body")
}
